package dex

import (
	"math"
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func init() {
	bls.Init(int(bls.CurveFp254BNb))
}

func createAccount(s *State, quant uint64) (consensus.SK, consensus.Addr) {
	var acc Account
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	acc.PK = sk.GetPublicKey().Serialize()
	addr := consensus.SHA3(acc.PK).Addr()
	acc.Balances = make(map[TokenID]*Balance)
	acc.Balances[0] = &Balance{Available: quant}
	s.UpdateAccount(&acc)
	return consensus.SK(sk.GetLittleEndian()), addr
}

func TestSendToken(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	sk, addr := createAccount(s, 100)

	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))

	var skRecv bls.SecretKey
	skRecv.SetByCSPRNG()

	to := consensus.PK(skRecv.GetPublicKey().Serialize())
	txn := MakeSendTokenTxn(sk, to, 0, 20, 0, 0)
	trans := s.Transition()
	valid, success := trans.Record(txn, 1)
	assert.True(t, valid)
	assert.True(t, success)

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	s = trans.Commit().(*State)

	toAcc := s.Account(to.Addr())
	assert.Equal(t, 20, int(toAcc.Balances[0].Available))
	newAcc = s.Account(addr)
	assert.Equal(t, 80, int(newAcc.Balances[0].Available))
}

func TestTransitionNotCommitToDB(t *testing.T) {
	memDB := ethdb.NewMemDatabase()
	db := trie.NewDatabase(memDB)
	s := NewState(db)
	sk, addr := createAccount(s, 100)
	h, err := s.state.Commit(nil)
	if err != nil {
		panic(err)
	}

	err = db.Commit(h, false)
	if err != nil {
		panic(err)
	}

	dbLen := memDB.Len()
	assert.Equal(t, 1, dbLen)

	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	trans := s.Transition()

	for i := 0; i < 99; i++ {
		var skRecv bls.SecretKey
		skRecv.SetByCSPRNG()

		to := consensus.PK(skRecv.GetPublicKey().Serialize())
		txn := MakeSendTokenTxn(sk, to, 0, 1, uint8(i), 0)
		valid, success := trans.Record(txn, 1)
		assert.True(t, valid)
		assert.True(t, success)
	}

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	// test len does not change, transition not committed to DB
	assert.Equal(t, 1, memDB.Len())

	s = trans.Commit().(*State)
	newAcc = s.Account(addr)
	assert.Equal(t, 1, int(newAcc.Balances[0].Available))
}

var btcInfo = TokenInfo{
	Symbol:     "BTC",
	Decimals:   8,
	TotalUnits: 21000000 * 100000000,
}

func TestIssueNativeToken(t *testing.T) {
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	pk := consensus.PK(sk.GetPublicKey().Serialize())
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	s = s.IssueNativeToken(&pk).(*State)

	assert.True(t, s.tokenCache.Exists(BNBInfo.Symbol))
	assert.Equal(t, &BNBInfo, s.tokenCache.Info(0))

	acc := s.Account(pk.Addr())
	assert.Equal(t, BNBInfo.TotalUnits, acc.Balances[0].Available)
	assert.Equal(t, uint64(0), acc.Balances[0].Pending)
}

func TestIssueToken(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	s.tokenCache.Update(0, &BNBInfo)
	sk, addr := createAccount(s, 100)
	trans := s.Transition()
	txn := MakeIssueTokenTxn(sk, btcInfo, 0, 0)
	trans.Record(txn, 1)
	s = trans.Commit().(*State)

	assert.Equal(t, 2, s.tokenCache.Size())
	assert.True(t, s.tokenCache.Exists(btcInfo.Symbol))
	assert.Equal(t, &btcInfo, s.tokenCache.Info(1))

	acc := s.Account(addr)
	assert.Equal(t, btcInfo.TotalUnits, acc.Balances[1].Available)
	assert.Equal(t, uint64(0), acc.Balances[1].Pending)
}

func TestPlaceOrder(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	s.tokenCache.Update(0, &BNBInfo)
	s.tokenCache.Update(1, &btcInfo)
	sk, addr := createAccount(s, 100)
	order := PlaceOrderTxn{
		SellSide:     false,
		Quant:        40,
		Price:        100000000,
		ExpireHeight: 0,
		Market:       MarketSymbol{Quote: 0, Base: 1},
	}
	trans := s.Transition()
	trans.Record(MakePlaceOrderTxn(sk, order, 0, 0), 1)
	s = trans.Commit().(*State)

	acc := s.Account(addr)
	assert.Equal(t, 40, int(acc.Balances[0].Pending))
}

func TestPlaceOrderAlreadyExpire(t *testing.T) {
	// TODO
}

func TestPlaceOrderExpireLater(t *testing.T) {
	// TODO
	// TODO: also handle height reduced due to reorg.
}

func TestCalcBaseSellQuant(t *testing.T) {
	baseDecimals := uint8(8)
	baseDiv := uint64(math.Pow10(int(baseDecimals)))
	quoteDecimals := uint8(6)
	quoteDiv := uint64(math.Pow10(int(quoteDecimals)))
	quoteQuant := quoteDiv * 100
	priceDiv := uint64(math.Pow10(int(OrderPriceDecimals)))
	priceQuantUnit := uint64(0.1 * float64(priceDiv))
	baseQuant := calcBaseSellQuant(quoteQuant, quoteDecimals, priceQuantUnit, OrderPriceDecimals, baseDecimals)
	assert.Equal(t, uint64(10), baseQuant/baseDiv)
	assert.Equal(t, 40, int(calcBaseSellQuant(40, 8, 100000000, 8, 8)))
}
