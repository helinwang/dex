package dex

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/matching"
	log "github.com/helinwang/log15"
)

// MarketSymbol is the symbol of a trading pair.
//
type MarketSymbol struct {
	Quote TokenID // the unit of the order's price
	Base  TokenID // the unit of the order's quantity
}

// Valid returns true if the market symbol is valid.
//
// Quote must be < Base for the symbol to be valid. This is to ensure
// there is only one market symbol per tranding pair. E.g.,
// MarketSymbol{Quote:1, Base: 2} is valid, MarketSymbol{Quote: 2,
// Base: 1} is invalid.
func (m *MarketSymbol) Valid() bool {
	return m.Quote < m.Base
}

// Bytes returns the bytes representation of the market symbol.
//
// The bytes is used as a prefix of a path of a patricia tree, It will
// be concatinated with the account addr path postfix to get the tree
// path. The path lead to the pending orders of an account in the
// market.
func (m *MarketSymbol) Bytes() []byte {
	bufA := make([]byte, 64)
	bufB := make([]byte, 64)
	binary.LittleEndian.PutUint64(bufA, uint64(m.Quote))
	binary.LittleEndian.PutUint64(bufB, uint64(m.Base))
	return append(bufA, bufB...)
}

type state struct {
	tokenCache    *TokenCache
	tokens        *trie.Trie
	accounts      *trie.Trie
	pendingOrders *trie.Trie
	reports       *trie.Trie
}

type PendingOrder struct {
	Owner consensus.Addr
	matching.Order
}

func (s *state) UpdateAccount(acc *Account) {
	addr := acc.PK.Addr()
	s.accounts.Update(addr[:], gobEncode(acc))
}

func (s *state) Account(addr consensus.Addr) *Account {
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

func pendingOrdersToOrders(p []PendingOrder) []matching.Order {
	o := make([]matching.Order, len(p))
	for i := range o {
		o[i] = p[i].Order
	}
	return o
}

func ordersToPendingOrders(owner consensus.Addr, o []matching.Order) []PendingOrder {
	p := make([]PendingOrder, len(o))
	for i := range p {
		p[i].Owner = owner
		p[i].Order = o[i]
	}

	return p
}

func encodePrefix(str []byte) []byte {
	l := len(str) * 2
	var nibbles = make([]byte, l)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	return nibbles
}

func decodeAddr(b []byte) []byte {
	n := len(b) / 2
	r := make([]byte, n)
	for i := 0; i < 2*n; i += 2 {
		r[i/2] = b[i]*16 + b[i+1]
	}
	return r
}

func (s *state) MarketPendingOrders(market MarketSymbol) []PendingOrder {
	prefix := market.Bytes()
	prefix = encodePrefix(prefix)
	emptyAddr := consensus.Addr{}
	start := append(prefix, encodePrefix(emptyAddr[:])...)
	iter := s.pendingOrders.NodeIterator(start)
	var p []PendingOrder

	hasNext := true
	foundPrefix := false
	for ; hasNext; hasNext = iter.Next(true) {
		if err := iter.Error(); err != nil {
			log.Error("error iterating pending orders trie", "err", err)
			break
		}

		if !iter.Leaf() {
			continue
		}

		path := iter.Path()
		if !bytes.HasPrefix(path, prefix) {
			if foundPrefix {
				break
			}

			continue
		}
		foundPrefix = true

		v := iter.LeafBlob()
		var o []matching.Order
		dec := gob.NewDecoder(bytes.NewReader(v))
		err := dec.Decode(&o)
		if err != nil {
			log.Error("error decode pending order", "market", market, "path", path, "err", err)
			continue
		}

		var owner consensus.Addr
		copy(owner[:], decodeAddr(path[len(prefix):]))
		po := ordersToPendingOrders(owner, o)
		p = append(p, po...)
	}

	return p
}

func (s *state) AccountPendingOrders(market MarketSymbol, addr consensus.Addr) []PendingOrder {
	path := append(market.Bytes(), addr[:]...)
	b, err := s.pendingOrders.TryGet(path)
	if err != nil {
		return nil
	}

	if b == nil {
		return nil
	}

	var o []matching.Order
	dec := gob.NewDecoder(bytes.NewReader(b))
	err = dec.Decode(&o)
	if err != nil {
		log.Error("error decode pending orders", "err", err, "market", market)
		return nil
	}

	p := ordersToPendingOrders(addr, o)
	return p
}

func (s *state) UpdatePendingOrder(market MarketSymbol, add, remove *PendingOrder) {
	if add == nil && remove == nil {
		return
	}

	var addr consensus.Addr
	if add != nil {
		addr = add.Owner
		if remove != nil && addr != remove.Owner {
			panic("the pending order must be updated for the same addr")
		}
	} else {
		addr = remove.Owner
	}

	p := s.AccountPendingOrders(market, addr)

	if remove != nil {
		removeIdx := -1
		for i := range p {
			if p[i] == *remove {
				removeIdx = i
				break
			}
		}

		if removeIdx < 0 {
			log.Error("can not find the pending order to be removed", "remove", *remove)
		} else {
			p = append(p[:removeIdx], p[removeIdx+1:]...)
		}
	}

	if add != nil {
		p = append(p, *add)
	}

	o := pendingOrdersToOrders(p)
	b := gobEncode(o)
	path := append(market.Bytes(), addr[:]...)
	s.pendingOrders.Update(path, b)
}

// State is the state of the DEX.
type State struct {
	state
	db *trie.Database
}

func NewState(db *trie.Database) *State {
	tokens, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	accounts, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	pendingOrders, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	reports, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	return &State{
		db: db,
		state: state{
			tokenCache:    newTokenCache(),
			tokens:        tokens,
			accounts:      accounts,
			pendingOrders: pendingOrders,
			reports:       reports,
		},
	}
}

func (s *State) Commit(t *Transition) {
	s.accounts = t.state.accounts
	s.tokens = t.state.tokens
	s.pendingOrders = t.state.pendingOrders
	s.reports = t.state.reports
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

func (s *State) MatchOrders() {
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

	state := state{
		tokenCache:    s.tokenCache,
		tokens:        tokens,
		accounts:      accounts,
		pendingOrders: pendingOrders,
		reports:       reports,
	}
	return newTransition(s, state)
}
