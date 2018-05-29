package dex

import (
	"bytes"
	"encoding/gob"

	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type Transition struct {
	tokens        *trie.Trie
	accounts      *trie.Trie
	pendingOrders *trie.Trie
	reports       *trie.Trie
	state         *State
	txns          [][]byte
}

func newTransition(s *State, tokens, accounts, pendingOrders, reports *trie.Trie) *Transition {
	return &Transition{
		tokens:        tokens,
		accounts:      accounts,
		pendingOrders: pendingOrders,
		reports:       reports,
		state:         s,
	}
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

	dec := gob.NewDecoder(bytes.NewBuffer(txn.Data))
	switch txn.T {
	case Order:
		panic("not implemented")
	case CancelOrder:
		panic("not implemented")
	case CreateToken:
		panic("not implemented")
	case SendToken:
		var txn SendTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("SendTokenTxn decode failed", "err", err)
			return
		}
		if !t.sendToken(acc, txn) {
			return
		}
	default:
		panic("unknown txn type")
	}

	t.txns = append(t.txns, b)
	return true, true
}

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
	to, err := t.accounts.TryGet(toAddr[:])
	var toAcc *Account
	if err != nil || to == nil {
		toAcc = &Account{PK: txn.To, Balances: make(map[TokenID]*Balance)}
	} else {
		dec := gob.NewDecoder(bytes.NewBuffer(to))
		err := dec.Decode(toAcc)
		if err != nil {
			log.Error("error decode recv account", "account", toAddr)
			return false
		}
	}

	addr := consensus.SHA3(owner.PK).Addr()
	owner.Balances[txn.TokenID].Available -= txn.Quant
	if toAcc.Balances[txn.TokenID] == nil {
		toAcc.Balances[txn.TokenID] = &Balance{}
	}
	toAcc.Balances[txn.TokenID].Available += txn.Quant
	t.updateAccount(toAddr, toAcc)
	t.updateAccount(addr, owner)
	return true
}

func (t *Transition) updateAccount(addr consensus.Addr, acc *Account) {
	t.accounts.Update(addr[:], gobEncode(acc))
}

func (t *Transition) Txns() [][]byte {
	return t.txns
}

// Commit commits the transition to the state root.
func (t *Transition) Commit() {
	t.state.Commit(t)
}
