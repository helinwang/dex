package dex

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"math"
	"strings"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type Transition struct {
	tokenCreations []Token
	txns           [][]byte
	state          *State
}

func newTransition(s *State) *Transition {
	return &Transition{state: s}
}

func (t *Transition) Account() consensus.Hash {
	return consensus.Hash(t.state.accounts.Hash())
}

// Record records a transition to the state transition.
func (t *Transition) Record(b []byte) (valid, success bool) {
	txn, acc, ready, valid := validateSigAndNonce(t.state, b)
	if !valid {
		return
	}

	if !ready {
		return true, false
	}

	if len(acc.NonceVec) <= int(txn.NonceIdx) {
		acc.NonceVec = append(acc.NonceVec, make([]uint64, int(txn.NonceIdx)-len(acc.NonceVec)+1)...)
	}
	acc.NonceVec[txn.NonceIdx]++

	dec := gob.NewDecoder(bytes.NewBuffer(txn.Data))
	switch txn.T {
	case PlaceOrder:
		var txn PlaceOrderTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("PlaceOrderTxn decode failed", "err", err)
			return
		}
		if !t.placeOrder(acc, txn) {
			log.Warn("PlaceOrderTxn failed")
			return
		}
	case CancelOrder:
		panic("not implemented")
	case CreateToken:
		var txn CreateTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("CreateTokenTxn decode failed", "err", err)
			return
		}
		if !t.createToken(acc, txn) {
			log.Warn("CreateTokenTxn failed")
			return
		}
	case SendToken:
		var txn SendTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("SendTokenTxn decode failed", "err", err)
			return
		}
		if !t.sendToken(acc, txn) {
			log.Warn("SendTokenTxn failed")
			return
		}
	default:
		panic("unknown txn type")
	}

	t.txns = append(t.txns, b)
	return true, true
}

func (t *Transition) placeOrder(owner *Account, txn PlaceOrderTxn) bool {
	// TODO: check if fee is sufficient
	baseInfo := t.state.tokenCache.Info(txn.Market.Base)
	if baseInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Base)
		return false
	}

	quoteInfo := t.state.tokenCache.Info(txn.Market.Quote)
	if quoteInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Quote)
		return false
	}

	var sellQuant uint64
	var sell TokenID
	if txn.SellSide {
		sellQuant = txn.Quant
		sell = txn.Market.Base
	} else {
		sellQuant = uint64(float64(txn.Quant) * txn.Price)
		sell = txn.Market.Quote
	}

	sb, ok := owner.Balances[sell]
	if !ok {
		log.Warn("does not have balance for the given token", "token", sell)
		return false
	}

	if sb.Available <= sellQuant {
		log.Warn("insufficient quant to sell", "token", sell, "quant", sellQuant)
		return false
	}

	owner.Balances[sell].Available -= sellQuant
	owner.Balances[sell].Pending += sellQuant
	t.state.UpdateAccount(owner)
	add := PendingOrder{
		Owner: owner.PK.Addr(),
		Order: Order{},
	}
	t.state.UpdatePendingOrder(txn.Market, &add, nil)
	return true
}

func (t *Transition) createToken(owner *Account, txn CreateTokenTxn) bool {
	if t.state.tokenCache.Exists(txn.Info.Symbol) {
		log.Warn("token symbol already exists", "symbol", txn.Info.Symbol)
		return false
	}

	for _, v := range t.tokenCreations {
		if strings.ToUpper(string(txn.Info.Symbol)) == strings.ToUpper(string(v.Symbol)) {
			log.Warn("token symbol already exists in the current transition", "symbol", txn.Info.Symbol)
			return false
		}
	}

	// TODO: fiture out how to pay fee.

	totalQuant := txn.Info.TotalSupply * uint64(math.Pow10(int(txn.Info.Decimals)))
	if totalQuant < txn.Info.TotalSupply {
		log.Warn("token total quant overflow", "total quant", totalQuant, "total supply", txn.Info.TotalSupply, "decimals", txn.Info.Decimals)
		return false
	}

	id := TokenID(t.state.tokenCache.Size() + len(t.tokenCreations))
	token := Token{ID: id, TokenInfo: txn.Info}
	t.tokenCreations = append(t.tokenCreations, token)

	b, err := rlp.EncodeToBytes(token)
	if err != nil {
		// should never happen
		panic(err)
	}

	path := make([]byte, 64)
	binary.LittleEndian.PutUint64(path, uint64(id))
	t.state.tokens.Update(path, b)

	if owner.Balances == nil {
		owner.Balances = make(map[TokenID]*Balance)
	}
	owner.Balances[id] = &Balance{Available: totalQuant}
	t.state.UpdateAccount(owner)
	return true
}

// TODO: all elements in trie should be serialized using rlp, not gob,
// since gob is not deterministic.
func (t *Transition) sendToken(owner *Account, txn SendTokenTxn) bool {
	if txn.Quant == 0 {
		return false
	}

	b, ok := owner.Balances[txn.TokenID]
	if !ok {
		log.Warn("trying to send token that the owner does not have", "tokenID", txn.TokenID)
		return false
	}

	if b.Available < txn.Quant {
		log.Warn("in sufficient available token balance", "tokenID", txn.TokenID, "quant", txn.Quant, "available", b.Available)
		return false
	}

	toAddr := txn.To.Addr()
	to, err := t.state.accounts.TryGet(toAddr[:])
	var toAcc *Account
	if err != nil || to == nil {
		toAcc = &Account{PK: txn.To, Balances: make(map[TokenID]*Balance)}
	} else {
		var de Account
		err = rlp.DecodeBytes(to, &de)
		if err != nil {
			log.Error("error decode recv account", "account", toAddr)
			return false
		}

		toAcc = &de
	}

	owner.Balances[txn.TokenID].Available -= txn.Quant
	if toAcc.Balances[txn.TokenID] == nil {
		toAcc.Balances[txn.TokenID] = &Balance{}
	}
	toAcc.Balances[txn.TokenID].Available += txn.Quant
	t.state.UpdateAccount(toAcc)
	t.state.UpdateAccount(owner)
	return true
}

func (t *Transition) Txns() [][]byte {
	return t.txns
}

// Commit commits the transition to the state root.
func (t *Transition) Commit() consensus.State {
	t.state.Commit()
	for _, v := range t.tokenCreations {
		t.state.tokenCache.Update(v.ID, &v.TokenInfo)
	}

	return t.state
}
