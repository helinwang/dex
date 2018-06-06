package dex

import (
	"bytes"
	"encoding/gob"
	"math"
	"math/big"
	"strings"

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

// Record records a transition to the state transition.
func (t *Transition) Record(b []byte, round uint64) (valid, success bool) {
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
		if !t.placeOrder(acc, txn, round) {
			log.Warn("PlaceOrderTxn failed")
			return
		}
	case CancelOrder:
		panic("not implemented")
	case IssueToken:
		var txn IssueTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("IssueTokenTxn decode failed", "err", err)
			return
		}
		if !t.issueToken(acc, txn) {
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

func calcBaseSellQuant(quoteQuantUnit uint64, quoteDecimals uint8, priceQuantUnit uint64, priceDecimals, baseDecimals uint8) uint64 {
	var quantUnit big.Int
	var quantDenominator big.Int
	var priceU big.Int
	var priceDenominator big.Int
	var baseDenominator big.Int
	quantUnit.SetUint64(quoteQuantUnit)
	quantDenominator.SetUint64(uint64(math.Pow10(int(quoteDecimals))))
	priceU.SetUint64(priceQuantUnit)
	priceDenominator.SetUint64(uint64(math.Pow10(int(OrderPriceDecimals))))
	baseDenominator.SetUint64(uint64(math.Pow10(int(baseDecimals))))
	var result big.Int
	result.Mul(&quantUnit, &priceU)
	result.Mul(&result, &baseDenominator)
	result.Div(&result, &quantDenominator)
	result.Div(&result, &priceDenominator)
	return result.Uint64()
}

func (t *Transition) placeOrder(owner *Account, txn PlaceOrderTxn, round uint64) bool {
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

	var sellQuantUnit uint64
	var sell TokenID
	if txn.SellSide {
		sellQuantUnit = txn.QuantUnit
		sell = txn.Market.Base
	} else {
		sellQuantUnit = calcBaseSellQuant(txn.QuantUnit, quoteInfo.Decimals, txn.PriceUnit, OrderPriceDecimals, baseInfo.Decimals)
		sell = txn.Market.Quote
	}

	sb, ok := owner.Balances[sell]
	if !ok {
		log.Warn("does not have balance for the given token", "token", sell)
		return false
	}

	if sellQuantUnit == 0 {
		log.Warn("sell quant too small")
	}

	if sb.Available <= sellQuantUnit {
		log.Warn("insufficient quant to sell", "token", sell, "quant unit", sellQuantUnit)
		return false
	}

	owner.Balances[sell].Available -= sellQuantUnit
	owner.Balances[sell].Pending += sellQuantUnit
	order := Order{
		Owner:        owner.PK.Addr(),
		SellSide:     txn.SellSide,
		QuantUnit:    txn.QuantUnit,
		PriceUnit:    txn.PriceUnit,
		PlacedHeight: txn.PlacedHeight,
		ExpireHeight: txn.ExpireHeight,
	}
	t.state.AddOrder(owner, txn.Market, uint8(round%numOrderShardPerMarket), order)
	t.state.UpdateAccount(owner)
	return true
}

func (t *Transition) issueToken(owner *Account, txn IssueTokenTxn) bool {
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
	id := TokenID(t.state.tokenCache.Size() + len(t.tokenCreations))
	token := Token{ID: id, TokenInfo: txn.Info}
	t.tokenCreations = append(t.tokenCreations, token)
	t.state.UpdateToken(token)

	if owner.Balances == nil {
		owner.Balances = make(map[TokenID]*Balance)
	}
	owner.Balances[id] = &Balance{Available: txn.Info.TotalUnits}
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
	toAcc := t.state.Account(toAddr)
	if toAcc == nil {
		toAcc = &Account{PK: txn.To, Balances: make(map[TokenID]*Balance)}
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

func (t *Transition) StateHash() consensus.Hash {
	return t.state.Hash()
}

func (s *Transition) MatchOrders() *consensus.TrieBlob {
	return &consensus.TrieBlob{}
}

func (t *Transition) ApplyTrades(blob *consensus.TrieBlob) error {
	if blob.Root == consensus.ZeroHash {
		return nil
	}
	return nil
}

// Commit commits the transition to the state root.
func (t *Transition) Commit() consensus.State {
	t.state.Commit()
	for _, v := range t.tokenCreations {
		t.state.tokenCache.Update(v.ID, &v.TokenInfo)
	}

	return t.state
}
