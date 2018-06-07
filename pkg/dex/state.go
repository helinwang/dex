package dex

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

const (
	numShardPerMarket = 16
)

// MarketSymbol is the symbol of a trading pair.
//
type MarketSymbol struct {
	Quote TokenID // the unit of the order's price
	Base  TokenID // the unit of the order's quantity
}

// Encode returns the bytes representation of the market symbol.
//
// The bytes is used as a prefix of a path of a patricia tree, It will
// be concatinated with the account addr path postfix to get the tree
// path. The path lead to the pending orders of an account in the
// market.
func (m *MarketSymbol) Encode() []byte {
	bufA := make([]byte, 64)
	bufB := make([]byte, 64)
	binary.LittleEndian.PutUint64(bufA, uint64(m.Quote))
	binary.LittleEndian.PutUint64(bufB, uint64(m.Base))
	return append(bufA, bufB...)
}

func (m *MarketSymbol) Decode(b []byte) error {
	if len(b) != 128 {
		return fmt.Errorf("bytes len not correct, expected 128, received %d", len(b))
	}

	m.Quote = TokenID(binary.LittleEndian.Uint64(b[:64]))
	m.Base = TokenID(binary.LittleEndian.Uint64(b[64:]))
	return nil
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

func encodePath(str []byte) []byte {
	l := len(str) * 2
	var nibbles = make([]byte, l)
	for i, b := range str {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	return nibbles
}

func decodePath(b []byte) []byte {
	n := len(b) / 2
	r := make([]byte, n)
	for i := 0; i < 2*n; i += 2 {
		r[i/2] = b[i]*16 + b[i+1]
	}
	return r
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

func (s *State) AccountOrders(acc *Account, market MarketSymbol) []Order {
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
		orders := s.Orders(market, shard)
		r = append(r, orders...)
	}

	return r
}

func (s *State) Orders(market MarketSymbol, shard uint8) []Order {
	prefix := orderPath(append(market.Encode(), shard))
	prefix = encodePath(prefix)
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

// MarketOrders returns the sorted orders of the given market.
func (s *State) MarketOrders(market MarketSymbol) []Order {
	var shards [numShardPerMarket][]Order
	total := 0
	for i := range shards {
		shards[i] = s.Orders(market, uint8(i))
		total += len(shards[i])
	}

	// each shard is sorted, merge them with k-way merge
	r := merge(shards)
	matchOrders(r)
	return nil
}

// Markets returns the trading markets.
func (s *State) Markets() []MarketSymbol {
	prefix := orderPath(nil)
	prefix = encodePath(prefix)
	iter := s.state.NodeIterator(prefix)

	var r []MarketSymbol
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

		// extract the encodedMarket from the trie path
		marketBytes := decodePath(path[len(prefix):])
		// last byte is the shard index, remove
		marketBytes = marketBytes[:len(marketBytes)-1]
		var m MarketSymbol
		err := m.Decode(marketBytes)
		if err != nil {
			panic(err)
		}
		r = append(r, m)
	}
	return r
}

// AddOrder adds one order into its trading market.
func (s *State) AddOrder(acc *Account, market MarketSymbol, shard uint8, order Order) {
	// orders were saved into the trie with the path as:
	// orderPrefix - encodedMarket - shard - encodedOrders.
	order.Owner = acc.PK.Addr()
	var orders []Order
	path := append(market.Encode(), shard)
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

// IssueNativeToken issues the native token, it is only called in
// during the chain creation.
func (s *State) IssueNativeToken(owner *consensus.PK) consensus.State {
	issueTokenTxn := IssueTokenTxn{Info: BNBInfo}
	trans := s.Transition().(*Transition)
	o := &Account{
		PK: *owner,
	}
	trans.issueToken(o, issueTokenTxn)
	return trans.Commit()
}

// Hash returns the state root hash of the state trie.
func (s *State) Hash() consensus.Hash {
	return consensus.Hash(s.state.Hash())
}

// Transition returns the state change transition.
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
