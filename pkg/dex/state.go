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
// A must be <= B for the symbol to be valid. This is to ensure there
// is only one market symbol per tranding pair. E.g.,
// MarketSymbol{A:1, B: 2} is valid, MarketSymbol{A: 2, B: 1} is
// invalid.
type MarketSymbol struct {
	A TokenID
	B TokenID
}

// Valid returns true if the market symbol is valid.
func (m *MarketSymbol) Valid() bool {
	return m.A <= m.B
}

// Bytes returns the bytes representation of the market symbol.
//
// The bytes is used as a prefix of a path of a patricia tree, It will
// be concatinated with the account addr path postfix to get the tree
// path. The path lead to the pending orders of an account in the
// market.
func (m *MarketSymbol) Bytes() []byte {
	bufA := make([]byte, 32)
	bufB := make([]byte, 32)
	binary.LittleEndian.PutUint32(bufA, uint32(m.A))
	binary.LittleEndian.PutUint32(bufB, uint32(m.B))
	return append(bufA, bufB...)
}

type state struct {
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
