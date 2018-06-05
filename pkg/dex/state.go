package dex

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

const (
	numOrderShardPerMarket = 16
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

func encodePrefix(str []byte) []byte {
	l := len(str) * 2
	var nibbles = make([]byte, l)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	return nibbles
}

// State is the state of the DEX.
type State struct {
	tokenCache *TokenCache
	state      *trie.Trie
	db         *trie.Database
}

// TODO: add receipt for create, send, freeze, burn token.

var BNBInfo = TokenInfo{
	Symbol:     "BNB",
	Decimals:   8,
	TotalUnits: 200000000 * 100000000,
}

func NewState(db *trie.Database) *State {
	t, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	s := &State{
		db:         db,
		tokenCache: newTokenCache(),
		state:      t,
	}

	return s
}

func (s *State) Commit() {
	s.state.Commit(nil)
}

var (
	accountPrefix = []byte("a")
	orderPrefix   = []byte("p")
	tokenPrefix   = []byte("t")
)

func accountAddrToPath(addr consensus.Addr) []byte {
	return append(accountPrefix, addr[:]...)
}

func orderPath(path []byte) []byte {
	return append(orderPrefix, path...)
}

func tokenPath(tokenID TokenID) []byte {
	path := make([]byte, 64)
	binary.LittleEndian.PutUint64(path, uint64(tokenID))
	return append(tokenPrefix, path...)
}

func (s *State) UpdateToken(token Token) {
	path := tokenPath(token.ID)

	b, err := rlp.EncodeToBytes(token)
	if err != nil {
		// should never happen
		panic(err)
	}

	s.state.Update(path, b)
}

func (s *State) UpdateAccount(acc *Account) {
	addr := acc.PK.Addr()
	b, err := rlp.EncodeToBytes(acc)
	if err != nil {
		panic(err)
	}

	s.state.Update(accountAddrToPath(addr), b)
}

func (s *State) Account(addr consensus.Addr) *Account {
	acc := s.state.Get(accountAddrToPath(addr))
	if acc == nil {
		return nil
	}

	var account Account
	err := rlp.DecodeBytes(acc, &account)
	if err != nil {
		log.Error("decode account error", "err", err, "b", acc)
		return nil
	}

	return &account
}

func (s *State) AccountOrders(acc *Account) []Order {
	m := make(map[MarketSymbol]map[uint8]struct{})
	for i, market := range acc.OrderMarkets {
		if m[market] == nil {
			m[market] = make(map[uint8]struct{})
		}
		m[market][acc.OrderShards[i]] = struct{}{}
	}

	var r []Order
	for market, shards := range m {
		for shard := range shards {
			orders := s.GetOrders(market, shard)
			r = append(r, orders...)
		}
	}

	return r
}

func (s *State) AccountMarketOrders(acc *Account, market MarketSymbol) []Order {
	m := make(map[MarketSymbol]map[uint8]struct{})
	for i, market := range acc.OrderMarkets {
		if m[market] == nil {
			m[market] = make(map[uint8]struct{})
		}
		m[market][acc.OrderShards[i]] = struct{}{}
	}

	if len(m[market]) == 0 {
		return nil
	}

	var r []Order
	for shard := range m[market] {
		orders := s.GetOrders(market, shard)
		r = append(r, orders...)
	}

	return r
}

func (s *State) GetOrders(market MarketSymbol, shard uint8) []Order {
	prefix := orderPath(append(market.Bytes(), shard))
	prefix = encodePrefix(prefix)
	iter := s.state.NodeIterator(prefix)
	var p []Order

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
		var o []Order
		err := rlp.DecodeBytes(v, &o)
		if err != nil {
			log.Error("error decode pending order", "market", market, "path", path, "err", err)
			continue
		}

		p = append(p, o...)
	}

	return p
}

func sortOrders(orders []Order) {
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].SellSide != orders[j].SellSide {
			// buy always smaller than sell
			if !orders[i].SellSide {
				return true
			}

			return false
		}

		if orders[i].PriceUnit < orders[j].PriceUnit {
			return true
		} else if orders[i].PriceUnit > orders[j].PriceUnit {
			return false
		}

		if orders[i].SellSide {
			// i sell, j sell. Treat ealier order lower
			// in the sell side of the order book (higher
			// priority)
			return orders[i].PlacedHeight < orders[j].PlacedHeight
		}

		// i buy, j buy. Treat earlier order higher in the buy
		// side of the order book (higher priority).
		return orders[i].PlacedHeight > orders[j].PlacedHeight

	})
}

func (s *State) AddOrder(acc *Account, market MarketSymbol, shard uint8, order Order) {
	order.Owner = acc.PK.Addr()
	var orders []Order
	path := append(market.Bytes(), shard)
	b := s.state.Get(orderPath(path))
	if b != nil {
		err := rlp.DecodeBytes(b, &orders)
		if err != nil {
			panic(err)
		}
	}

	orders = append(orders, order)
	sortOrders(orders)
	b, err := rlp.EncodeToBytes(orders)
	if err != nil {
		panic(err)
	}

	acc.OrderMarkets = append(acc.OrderMarkets, market)
	acc.OrderShards = append(acc.OrderShards, shard)
	s.state.Update(orderPath(path), b)
}

func (s *State) IssueNativeToken(owner *consensus.PK) consensus.State {
	issueTokenTxn := IssueTokenTxn{Info: BNBInfo}
	trans := s.Transition().(*Transition)
	o := &Account{
		PK: *owner,
	}
	trans.issueToken(o, issueTokenTxn)
	return trans.Commit()
}

func (s *State) Hash() consensus.Hash {
	return consensus.Hash(s.state.Hash())
}

func (s *State) Transition() consensus.Transition {
	root, err := s.state.Commit(nil)
	if err != nil {
		panic(err)
	}

	trie, err := trie.New(root, s.db)
	if err != nil {
		panic(err)
	}

	state := &State{
		db:         s.db,
		tokenCache: s.tokenCache.Clone(),
		state:      trie,
	}
	return newTransition(state)
}
