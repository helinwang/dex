package dex

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

// State is the state of the DEX.
type State struct {
	db            *trie.Database
	tokens        *trie.Trie
	accounts      *trie.Trie
	pendingOrders *trie.Trie
	reports       *trie.Trie
}

func NewState(db *trie.Database) *State {
	tokens, err := trie.New(common.Hash{}, db)
	if err != nil {
		// should not happen
		panic(err)
	}

	accounts, err := trie.New(common.Hash{}, db)
	if err != nil {
		// should not happen
		panic(err)
	}

	pendingOrders, err := trie.New(common.Hash{}, db)
	if err != nil {
		// should not happen
		panic(err)
	}

	reports, err := trie.New(common.Hash{}, db)
	if err != nil {
		// should not happen
		panic(err)
	}

	return &State{
		db:            db,
		tokens:        tokens,
		accounts:      accounts,
		pendingOrders: pendingOrders,
		reports:       reports,
	}
}

func (s *State) Commit(t *Transition) {
	s.accounts = t.accounts
	s.tokens = t.tokens
	s.pendingOrders = t.pendingOrders
	s.reports = t.reports
}

func (s *State) Accounts() consensus.Hash {
	return consensus.Hash(s.accounts.Hash())
}

func (s *State) Tokens() consensus.Hash {
	return consensus.Hash(s.tokens.Hash())
}

func (s *State) PendingOrders() consensus.Hash {
	return consensus.Hash(s.pendingOrders.Hash())
}

func (s *State) Reports() consensus.Hash {
	return consensus.Hash(s.reports.Hash())
}

func (s *State) Transition() consensus.Transition {
	tokenRoot, err := s.tokens.Commit(nil)
	if err != nil {
		panic(err)
	}

	tokens, err := trie.New(tokenRoot, s.db)
	if err != nil {
		panic(err)
	}

	accRoot, err := s.accounts.Commit(nil)
	if err != nil {
		panic(err)
	}

	accounts, err := trie.New(accRoot, s.db)
	if err != nil {
		panic(err)
	}

	pendingOrderRoot, err := s.pendingOrders.Commit(nil)
	if err != nil {
		panic(err)
	}

	pendingOrders, err := trie.New(pendingOrderRoot, s.db)
	if err != nil {
		panic(err)
	}

	reportRoot, err := s.reports.Commit(nil)
	if err != nil {
		panic(err)
	}

	reports, err := trie.New(reportRoot, s.db)
	if err != nil {
		panic(err)
	}

	return newTransition(s, tokens, accounts, pendingOrders, reports)
}

func (s *State) Account(addr consensus.Addr) *Account {
	acc, err := s.accounts.TryGet(addr[:])
	if err != nil || acc == nil {
		if acc == nil {
			err = fmt.Errorf("account %x does not exist", addr)
		}
		log.Warn("get account error", "err", err)
		return nil
	}

	var account Account
	dec := gob.NewDecoder(bytes.NewBuffer(acc))
	err = dec.Decode(&account)
	if err != nil {
		log.Error("decode account error", "err", err)
		return nil
	}

	return &account
}
