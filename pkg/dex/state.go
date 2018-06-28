package dex

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

// MarketSymbol is the symbol of a trading pair.
//
type MarketSymbol struct {
	Base  TokenID // the unit of the order's quantity
	Quote TokenID // the unit of the order's price
}

// Valid checks if the market symbol is valid.
func (m *MarketSymbol) Valid() bool {
	return m.Base != m.Quote
}

// Encode returns the bytes representation of the market symbol.
//
// The bytes is used as a prefix of a path of a patricia tree, It will
// be concatinated with the account addr path postfix to get the tree
// path. The path lead to the pending orders of an account in the
// market.
func (m *MarketSymbol) Encode() []byte {
	buf := make([]byte, 128)
	n0 := binary.PutUvarint(buf, uint64(m.Quote))
	n1 := binary.PutUvarint(buf[n0:], uint64(m.Base))
	return buf[:n0+n1]
}

func (m *MarketSymbol) Decode(b []byte) (int, error) {
	v, n0 := binary.Uvarint(b)
	m.Quote = TokenID(v)
	v, n1 := binary.Uvarint(b[n0:])
	m.Base = TokenID(v)
	return n0 + n1, nil
}

// State is the state of the DEX.
type State struct {
	db     *trie.Database
	diskDB ethdb.Database

	mu           sync.Mutex
	trie         *trie.Trie
	accountCache map[consensus.Addr]*Account
}

// TODO: add receipt for create, send, freeze, burn token.

var BNBInfo = TokenInfo{
	Symbol:     "BNB",
	Decimals:   8,
	TotalUnits: 200000000 * 100000000,
}

func CreateGenesisState(recipients []PK, additionalTokens []TokenInfo) *State {
	memDB := ethdb.NewMemDatabase()
	s := NewState(memDB)
	tokens := make([]Token, len(additionalTokens)+1)

	var tokenID TokenID
	tokens[0] = Token{ID: tokenID, TokenInfo: BNBInfo}
	tokenID++

	for i, t := range additionalTokens {
		token := Token{ID: tokenID, TokenInfo: t}
		tokenID++
		tokens[i+1] = token
	}

	for _, t := range tokens {
		s.UpdateToken(t)
	}

	for _, pk := range recipients {
		account := s.NewAccount(pk)
		for _, t := range tokens {
			avg := t.TotalUnits / uint64(len(recipients))
			account.UpdateBalance(t.ID, Balance{Available: avg})
		}
	}

	s.CommitCache()
	return s
}

func newState(state *trie.Trie, db *trie.Database, diskDB ethdb.Database) *State {
	return &State{
		diskDB:       diskDB,
		db:           db,
		trie:         state,
		accountCache: make(map[consensus.Addr]*Account),
	}
}

func NewState(diskDB ethdb.Database) *State {
	db := trie.NewDatabase(diskDB)
	t, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	return newState(t, db, diskDB)
}

var (
	marketPrefix           = []byte{0}
	tokenPrefix            = []byte{1}
	orderExpirationPrefix  = []byte{2}
	freezeAtRoundPrefix    = []byte{3}
	pkPrefix               = []byte{4}
	noncePrefix            = []byte{5}
	balancePrefix          = []byte{6}
	pendingOrdersPrefix    = []byte{7}
	executionReportsPrefix = []byte{8}
	reportIdxPrefix        = []byte{9}
)

func addrReportIdxPath(addr consensus.Addr) []byte {
	return append(reportIdxPrefix, addr[:]...)
}

func freezeAtRoundToPath(round uint64) []byte {
	b := make([]byte, 64)
	binary.LittleEndian.PutUint64(b, round)
	return append(freezeAtRoundPrefix, b...)
}

func addrPKPath(addr consensus.Addr) []byte {
	return append(pkPrefix, addr[:]...)
}

func addrNoncePath(addr consensus.Addr) []byte {
	return append(noncePrefix, addr[:]...)
}

func addrBalancePath(addr consensus.Addr) []byte {
	return append(balancePrefix, addr[:]...)
}

func addrPendingOrderPath(addr consensus.Addr, orderID OrderID) []byte {
	orderBytes := orderID.Bytes()
	p := append(pendingOrdersPrefix, addr[:]...)
	p = append(p, orderBytes...)
	return p
}

func addrPendingOrdersPath(addr consensus.Addr) []byte {
	return append(pendingOrdersPrefix, addr[:]...)
}

func addrExecutionReportPath(addr consensus.Addr, idx uint32) []byte {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf, idx)
	p := append(executionReportsPrefix, addr[:]...)
	p = append(p, buf...)
	return p
}

func addrExecutionReportsPath(addr consensus.Addr) []byte {
	p := append(executionReportsPrefix, addr[:]...)
	return p
}

func expirationToPath(round uint64) []byte {
	b := make([]byte, 64)
	binary.LittleEndian.PutUint64(b, round)
	path := append(orderExpirationPrefix, b...)
	return path
}

func tokenPath(tokenID TokenID) []byte {
	path := make([]byte, 64)
	binary.LittleEndian.PutUint64(path, uint64(tokenID))
	return append(tokenPrefix, path...)
}

func marketPath(path []byte) []byte {
	return append(marketPrefix, path...)
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

func (s *State) cachedAccounts() []*Account {
	accounts := make([]*Account, len(s.accountCache))
	i := 0
	for _, acc := range s.accountCache {
		accounts[i] = acc
		i++
	}
	return accounts
}

func (s *State) commitCache() {
	for _, acc := range s.cachedAccounts() {
		// commit cache calls the methods of s, need to be
		// outside of s.mu
		acc.CommitCache(s)
	}
}

func (s *State) CommitCache() {
	s.mu.Lock()
	accounts := s.cachedAccounts()
	s.mu.Unlock()

	for _, acc := range accounts {
		// commit cache calls the methods of s, need to be
		// outside of s.mu
		acc.CommitCache(s)
	}
}

func (s *State) NewAccount(pk PK) *Account {
	account := &Account{
		addr:       pk.Addr(),
		pk:         pk,
		newAccount: true,
		state:      s,
	}

	s.mu.Lock()
	s.accountCache[account.addr] = account
	s.mu.Unlock()
	return account
}

func (s *State) pk(addr consensus.Addr) PK {
	b := s.trie.Get(addrPKPath(addr))
	return PK(b)
}

func (s *State) PK(addr consensus.Addr) PK {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pk(addr)
}

func (s *State) UpdatePK(pk PK) {
	addr := pk.Addr()

	s.mu.Lock()
	defer s.mu.Unlock()
	path := addrPKPath(addr)
	s.trie.Update(path, pk)
}

func (s *State) UpdateNonceVec(addr consensus.Addr, vec []uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := rlp.EncodeToBytes(vec)
	if err != nil {
		panic(err)
	}

	path := addrNoncePath(addr)
	s.trie.Update(path, b)
}

func (s *State) NonceVec(addr consensus.Addr) []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := s.trie.Get(addrNoncePath(addr))
	if len(b) == 0 {
		return nil
	}

	var vec []uint64
	err := rlp.DecodeBytes(b, &vec)
	if err != nil {
		panic(err)
	}

	return vec
}

type balanceIDs struct {
	B []Balance
	I []TokenID
}

func (s *State) UpdateBalances(addr consensus.Addr, balances []Balance, ids []TokenID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v := balanceIDs{B: balances, I: ids}
	b, err := rlp.EncodeToBytes(v)
	if err != nil {
		panic(err)
	}

	path := addrBalancePath(addr)
	s.trie.Update(path, b)
}

func (s *State) Balances(addr consensus.Addr) ([]Balance, []TokenID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := s.trie.Get(addrBalancePath(addr))
	if len(b) == 0 {
		return nil, nil
	}

	var v balanceIDs
	err := rlp.DecodeBytes(b, &v)
	if err != nil {
		panic(err)
	}

	return v.B, v.I
}

func (s *State) PendingOrder(addr consensus.Addr, id OrderID) (p PendingOrder, ok bool) {
	b := s.trie.Get(addrPendingOrderPath(addr, id))
	if len(b) == 0 {
		return
	}

	err := rlp.DecodeBytes(b, &p)
	if err != nil {
		panic(err)
	}

	return p, true
}

func (s *State) UpdatePendingOrder(addr consensus.Addr, p PendingOrder) {
	b, err := rlp.EncodeToBytes(p)
	if err != nil {
		panic(err)
	}

	s.trie.Update(addrPendingOrderPath(addr, p.ID), b)
}

func (s *State) RemovePendingOrder(addr consensus.Addr, id OrderID) {
	s.trie.Delete(addrPendingOrderPath(addr, id))
}

func (s *State) PendingOrders(addr consensus.Addr) []PendingOrder {
	prefix := encodePath(addrPendingOrdersPath(addr))
	iter := s.trie.NodeIterator(prefix)

	var r []PendingOrder
	hasNext := true
	foundPrefix := false

	for ; hasNext; hasNext = iter.Next(true) {
		if err := iter.Error(); err != nil {
			log.Error("error iterating state trie's tokens", "err", err)
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

		var order PendingOrder
		err := rlp.DecodeBytes(iter.LeafBlob(), &order)
		if err != nil {
			panic(err)
		}

		r = append(r, order)
	}
	return r
}

func (s *State) AddExecutionReport(addr consensus.Addr, e ExecutionReport, idx uint32) {
	b, err := rlp.EncodeToBytes(e)
	if err != nil {
		panic(err)
	}

	s.mu.Lock()
	path := addrExecutionReportPath(addr, idx)
	s.trie.Update(path, b)
	s.mu.Unlock()
}

func (s *State) ExecutionReports(addr consensus.Addr) []ExecutionReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := encodePath(addrExecutionReportsPath(addr))
	iter := s.trie.NodeIterator(prefix)

	var r []ExecutionReport
	hasNext := true
	foundPrefix := false

	for ; hasNext; hasNext = iter.Next(true) {
		if err := iter.Error(); err != nil {
			log.Error("error iterating state trie's tokens", "err", err)
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

		var e ExecutionReport
		err := rlp.DecodeBytes(iter.LeafBlob(), &e)
		if err != nil {
			panic(err)
		}

		r = append(r, e)
	}
	return r
}

func (s *State) UpdateToken(token Token) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := tokenPath(token.ID)

	b, err := rlp.EncodeToBytes(token)
	if err != nil {
		// should never happen
		panic(err)
	}

	s.trie.Update(path, b)
}

func (s *State) Account(addr consensus.Addr) *Account {
	s.mu.Lock()
	defer s.mu.Unlock()

	cache := s.accountCache[addr]
	if cache != nil {
		return cache
	}

	pk := s.pk(addr)
	account := &Account{
		addr:  pk.Addr(),
		pk:    pk,
		state: s,
	}

	s.accountCache[addr] = account
	return account
}

// loadOrderBook deserializes the order from the state trie.
func (s *State) loadOrderBook(m MarketSymbol) *orderBook {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := marketPath(m.Encode())
	b := s.trie.Get(path)
	if b == nil {
		return nil
	}

	var book orderBook
	err := rlp.DecodeBytes(b, &book)
	if err != nil {
		panic(err)
	}

	return &book
}

func (s *State) saveOrderBook(m MarketSymbol, book *orderBook) {
	b, err := rlp.EncodeToBytes(book)
	if err != nil {
		panic(err)
	}

	s.mu.Lock()
	path := marketPath(m.Encode())
	s.trie.Update(path, b)
	s.mu.Unlock()
}

// Tokens returns all issued tokens
func (s *State) Tokens() []Token {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := encodePath(tokenPrefix)
	iter := s.trie.NodeIterator(prefix)

	var r []Token
	hasNext := true
	foundPrefix := false

	for ; hasNext; hasNext = iter.Next(true) {
		if err := iter.Error(); err != nil {
			log.Error("error iterating state trie's tokens", "err", err)
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

		var token Token
		err := rlp.DecodeBytes(iter.LeafBlob(), &token)
		if err != nil {
			panic(err)
		}

		r = append(r, token)
	}
	return r
}

func (s *State) Serialize() (consensus.TrieBlob, error) {
	s.CommitCache()
	return serializeTrie(s.trie, s.db, s.db.DiskDB())
}

func (s *State) Deserialize(b consensus.TrieBlob) error {
	err := b.Fill(s.diskDB)
	if err != nil {
		return err
	}

	db := trie.NewDatabase(s.diskDB)
	t, err := trie.New(common.Hash(b.Root), db)
	if err != nil {
		return err
	}

	s.trie = t
	s.db = db
	return nil
}

// Hash returns the state root hash of the state trie.
func (s *State) Hash() consensus.Hash {
	s.mu.Lock()
	defer s.mu.Unlock()

	return consensus.Hash(s.trie.Hash())
}

// Transition returns the state change transition.
func (s *State) Transition(round uint64) consensus.Transition {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.commitCache()

	newTrie := *s.trie
	state := newState(&newTrie, s.db, s.diskDB)
	return newTransition(state, round)
}

type orderExpiration struct {
	ID    OrderID
	Owner consensus.Addr
}

func (s *State) GetOrderExpirations(round uint64) []orderExpiration {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getOrderExpirations(round)
}

func (s *State) getOrderExpirations(round uint64) []orderExpiration {
	var all []orderExpiration
	path := expirationToPath(round)
	exisiting := s.trie.Get(path)
	if len(exisiting) > 0 {
		err := rlp.DecodeBytes(exisiting, &all)
		if err != nil {
			panic(err)
		}
	}
	return all
}

func (s *State) AddOrderExpirations(round uint64, ids []orderExpiration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.getOrderExpirations(round)
	all = append(all, ids...)
	b, err := rlp.EncodeToBytes(all)
	if err != nil {
		panic(err)
	}

	path := expirationToPath(round)
	s.trie.Update(path, b)
}

func (s *State) RemoveOrderExpirations(round uint64, ids map[OrderID]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.getOrderExpirations(round)
	newExps := make([]orderExpiration, 0, len(all))
	for _, exp := range all {
		if !ids[exp.ID] {
			newExps = append(newExps, exp)
		}
	}

	b, err := rlp.EncodeToBytes(newExps)
	if err != nil {
		panic(err)
	}
	path := expirationToPath(round)
	s.trie.Update(path, b)
}

func (s *State) UpdateReportIdx(addr consensus.Addr, idx uint32) {
	b, err := rlp.EncodeToBytes(idx)
	if err != nil {
		panic(err)
	}

	s.mu.Lock()
	s.trie.Update(addrReportIdxPath(addr), b)
	s.mu.Unlock()
}

func (s *State) ReportIdx(addr consensus.Addr) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := s.trie.Get(addrReportIdxPath(addr))
	if len(b) == 0 {
		return 0
	}

	var r uint32
	err := rlp.DecodeBytes(b, &r)
	if err != nil {
		panic(err)
	}

	return r
}

type freezeToken struct {
	Addr    consensus.Addr
	TokenID TokenID
	Quant   uint64
}

func (s *State) GetFreezeTokens(round uint64) []freezeToken {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getFreezeTokens(round)
}

func (s *State) getFreezeTokens(round uint64) []freezeToken {
	path := freezeAtRoundToPath(round)
	b := s.trie.Get(path)
	if len(b) == 0 {
		return nil
	}

	var all []freezeToken
	err := rlp.DecodeBytes(b, &all)
	if err != nil {
		panic(err)
	}

	return all
}

func (s *State) FreezeToken(round uint64, f freezeToken) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all := s.getFreezeTokens(round)
	all = append(all, f)
	b, err := rlp.EncodeToBytes(all)
	if err != nil {
		panic(err)
	}

	path := freezeAtRoundToPath(round)
	s.trie.Update(path, b)
}
