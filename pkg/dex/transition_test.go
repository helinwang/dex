package dex

import (
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func createAccount(s *State, quant uint64) (consensus.SK, consensus.Addr) {
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	acc := NewAccount(sk.GetPublicKey().Serialize(), s)
	addr := acc.PK().Addr()
	acc.balances = make(map[TokenID]Balance)
	acc.balances[0] = Balance{Available: quant}
	s.CommitCache()
	return consensus.SK(sk.GetLittleEndian()), addr
}

func TestSendToken(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	sk, addr := createAccount(s, 100)

	// check balance not changed before commiting the txn
	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.balances[0].Available))

	var skRecv bls.SecretKey
	skRecv.SetByCSPRNG()

	to := consensus.PK(skRecv.GetPublicKey().Serialize())
	txn := MakeSendTokenTxn(sk, to, 0, 20, 0, 0)
	trans := s.Transition(1)
	valid, success := trans.Record(parseTxn(txn))
	assert.True(t, valid)
	assert.True(t, success)

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.balances[0].Available))
	s = trans.Commit().(*State)

	toAcc := s.Account(to.Addr())
	assert.Equal(t, 20, int(toAcc.balances[0].Available))
	newAcc = s.Account(addr)
	assert.Equal(t, 80, int(newAcc.balances[0].Available))
}

func TestFreezeToken(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	sk, addr := createAccount(s, 100)

	// check balance not changed before commiting the txn
	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.balances[0].Available))
	txn := MakeFreezeTokenTxn(sk, FreezeTokenTxn{TokenID: 0, AvailableRound: 3, Quant: 50}, 0, 0)

	trans := s.Transition(1)
	valid, success := trans.Record(parseTxn(txn))
	assert.True(t, valid)
	assert.True(t, success)
	s = trans.Commit().(*State)
	newAcc = s.Account(addr)
	assert.Equal(t, 50, int(newAcc.balances[0].Available))
	assert.Equal(t, []Frozen([]Frozen{Frozen{AvailableRound: 3, Quant: 50}}), newAcc.balances[0].Frozen)

	trans = s.Transition(2)
	s = trans.Commit().(*State)
	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.balances[0].Available))
	assert.Equal(t, 0, len(newAcc.balances[0].Frozen))
}

func TestTransitionNotCommitToDB(t *testing.T) {
	memDB := ethdb.NewMemDatabase()
	s := NewState(memDB)
	sk, addr := createAccount(s, 100)
	h, err := s.trie.Commit(nil)
	if err != nil {
		panic(err)
	}

	err = s.db.Commit(h, false)
	if err != nil {
		panic(err)
	}

	dbLen := memDB.Len()
	assert.Equal(t, 1, dbLen)

	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.balances[0].Available))
	trans := s.Transition(1)

	for i := 0; i < 99; i++ {
		var skRecv bls.SecretKey
		skRecv.SetByCSPRNG()

		to := consensus.PK(skRecv.GetPublicKey().Serialize())
		txn := MakeSendTokenTxn(sk, to, 0, 1, uint8(i), 0)
		valid, success := trans.Record(parseTxn(txn))
		assert.True(t, valid)
		assert.True(t, success)
	}

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.balances[0].Available))
	// test len does not change, transition not committed to DB
	assert.Equal(t, 1, memDB.Len())

	s = trans.Commit().(*State)
	newAcc = s.Account(addr)
	assert.Equal(t, 1, int(newAcc.balances[0].Available))
}

func TestIssueToken(t *testing.T) {
	var btcInfo = TokenInfo{
		Symbol:     "BTC",
		Decimals:   8,
		TotalUnits: 21000000 * 100000000,
	}

	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	sk, addr := createAccount(s, 100)
	trans := s.Transition(1)
	txn := MakeIssueTokenTxn(sk, btcInfo, 0, 0)
	trans.Record(parseTxn(txn))
	s = trans.Commit().(*State)

	assert.Equal(t, 2, len(s.Tokens()))
	cache := newTokenCache(s)
	assert.True(t, cache.Exists(btcInfo.Symbol))
	assert.Equal(t, &btcInfo, cache.Info(1))

	acc := s.Account(addr)
	assert.Equal(t, btcInfo.TotalUnits, acc.balances[1].Available)
	assert.Equal(t, uint64(0), acc.balances[1].Pending)
}

func TestOrderAlreadyExpired(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	sk, addr := createAccount(s, 100)
	order := PlaceOrderTxn{
		SellSide:    false,
		Quant:       40,
		Price:       100000000,
		ExpireRound: 1,
		Market:      MarketSymbol{Quote: 0, Base: 0},
	}

	trans := s.Transition(1)
	valid, success := trans.Record(parseTxn(MakePlaceOrderTxn(sk, order, 0, 0)))
	assert.False(t, valid)
	assert.False(t, success)
	s = trans.Commit().(*State)
	acc := s.Account(addr)
	assert.Equal(t, 0, len(acc.pendingOrders))
}

func TestOrderExpire(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	sk, addr := createAccount(s, 100)
	order := PlaceOrderTxn{
		SellSide:    false,
		Quant:       40,
		Price:       100000000,
		ExpireRound: 3,
		Market:      MarketSymbol{Quote: 0, Base: 0},
	}

	trans := s.Transition(1)
	valid, success := trans.Record(parseTxn(MakePlaceOrderTxn(sk, order, 0, 0)))
	assert.True(t, valid)
	assert.True(t, success)
	// transition for the current round will expire the order for
	// the next round.
	s = trans.Commit().(*State)
	acc := s.Account(addr)
	assert.Equal(t, 1, len(acc.pendingOrders))
	trans = s.Transition(2)
	s = trans.Commit().(*State)

	acc = s.Account(addr)
	assert.Equal(t, 0, len(acc.pendingOrders))
}

func TestPlaceOrder(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	sk, addr := createAccount(s, 100)
	order := PlaceOrderTxn{
		SellSide:    false,
		Quant:       40,
		Price:       100000000,
		ExpireRound: 3,
		Market:      MarketSymbol{Quote: 0, Base: 0},
	}
	trans := s.Transition(1)
	valid, success := trans.Record(parseTxn(MakePlaceOrderTxn(sk, order, 0, 0)))
	assert.True(t, valid)
	assert.True(t, success)
	s = trans.Commit().(*State)

	acc := s.Account(addr)
	assert.Equal(t, 40, int(acc.balances[0].Pending))

	trans = s.Transition(2)
	order = PlaceOrderTxn{
		SellSide:    true,
		Quant:       40,
		Price:       100000000,
		ExpireRound: 3,
		Market:      MarketSymbol{Quote: 0, Base: 0},
	}
	valid, success = trans.Record(parseTxn(MakePlaceOrderTxn(sk, order, 0, 1)))
	// TODO: rename trans.Commit to something that indicates a new
	// state is generated, rather than the prev state is modified.
	s = trans.Commit().(*State)
	assert.True(t, valid)
	assert.True(t, success)
	acc = s.Account(addr)
	assert.Equal(t, 0, int(acc.balances[0].Pending))
}

func TestCalcBaseSellQuant(t *testing.T) {
	assert.Equal(t, 40, int(calcBaseSellQuant(40, 8, 100000000, 8, 8)))
}
