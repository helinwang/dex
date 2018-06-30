package dex

import (
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func TestAccountUpdateBalance(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	pk, _ := RandKeyPair()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(0, Balance{Available: 100})
	assert.Equal(t, 100, int(acc.Balance(0).Available))

	addr := pk.Addr()
	acc0 := s.Account(addr)
	assert.Equal(t, 100, int(acc0.Balance(0).Available))

	acc0.UpdateBalance(0, Balance{Available: 200})
	assert.Equal(t, 200, int(acc.Balance(0).Available))
	assert.Equal(t, 200, int(acc0.Balance(0).Available))
}

func TestSendToken(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	pk, sk := RandKeyPair()
	addr := pk.Addr()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(0, Balance{Available: 100})

	to, _ := RandKeyPair()
	txn := MakeSendTokenTxn(sk, addr, to, 0, 20, 0)
	trans := s.Transition(1)
	pt, err := parseTxn(txn, &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}

	err = trans.Record(pt)
	assert.Nil(t, err)
	s = trans.Commit().(*State)

	send := s.Account(addr)
	assert.Equal(t, 80, int(send.Balance(0).Available))
	recv := s.Account(to.Addr())
	assert.Equal(t, 20, int(recv.Balance(0).Available))
}

func TestFreezeToken(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	pk, sk := RandKeyPair()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(0, Balance{Available: 100})

	addr := pk.Addr()
	txn := MakeFreezeTokenTxn(sk, addr, FreezeTokenTxn{TokenID: 0, AvailableRound: 3, Quant: 50}, 0)

	trans := s.Transition(1)
	pt, err := parseTxn(txn, &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}

	err = trans.Record(pt)
	assert.Nil(t, err)
	s = trans.Commit().(*State)

	acc = s.Account(addr)
	assert.Equal(t, 50, int(acc.Balance(0).Available))
	assert.Equal(t, []Frozen([]Frozen{Frozen{AvailableRound: 3, Quant: 50}}), acc.Balance(0).Frozen)

	trans = s.Transition(2)
	s = trans.Commit().(*State)
	acc = s.Account(addr)
	assert.Equal(t, 100, int(acc.Balance(0).Available))
	assert.Equal(t, 0, len(acc.Balance(0).Frozen))
}

func TestIssueToken(t *testing.T) {
	var btcInfo = TokenInfo{
		Symbol:     "BTC",
		Decimals:   8,
		TotalUnits: 21000000 * 100000000,
	}

	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	pk, sk := RandKeyPair()
	acc := s.NewAccount(pk)
	trans := s.Transition(1)
	addr := pk.Addr()
	txn := MakeIssueTokenTxn(sk, addr, btcInfo, 0)
	pt, err := parseTxn(txn, &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}

	trans.Record(pt)
	s = trans.Commit().(*State)

	assert.Equal(t, 2, len(s.Tokens()))
	cache := newTokenCache(s)
	assert.True(t, cache.Exists(btcInfo.Symbol))
	assert.Equal(t, &btcInfo, cache.Info(1))

	acc = s.Account(addr)
	assert.Equal(t, btcInfo.TotalUnits, acc.Balance(1).Available)
	assert.Equal(t, uint64(0), acc.Balance(1).Pending)
	assert.Equal(t, 0, len(acc.Balance(1).Frozen))
}

func TestOrderAlreadyExpired(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	pk, sk := RandKeyPair()
	acc := s.NewAccount(pk)
	addr := pk.Addr()
	order := PlaceOrderTxn{
		SellSide:    false,
		Quant:       40,
		Price:       100000000,
		ExpireRound: 1,
		Market:      MarketSymbol{Quote: 0, Base: 1},
	}

	trans := s.Transition(1)
	pt, err := parseTxn(MakePlaceOrderTxn(sk, addr, order, 0), &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}

	err = trans.Record(pt)
	assert.Contains(t, err.Error(), "expire")
	s = trans.Commit().(*State)
	acc = s.Account(addr)
	assert.Equal(t, 0, len(acc.PendingOrders()))
}

func TestBuyOrderExpire(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	s.UpdateToken(Token{ID: 1, TokenInfo: BNBInfo})
	pk, sk := RandKeyPair()
	addr := pk.Addr()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(1, Balance{Available: 300})

	order := PlaceOrderTxn{
		SellSide:    false,
		Quant:       100,
		Price:       2 * uint64(math.Pow10(OrderPriceDecimals)),
		ExpireRound: 3,
		Market:      MarketSymbol{Quote: 1, Base: 0},
	}
	trans := s.Transition(1)
	pt, err := parseTxn(MakePlaceOrderTxn(sk, addr, order, 0), &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}
	err = trans.Record(pt)
	assert.Nil(t, err)
	// transition for the current round will expire the order for
	// the next round.
	s = trans.Commit().(*State)
	acc = s.Account(addr)
	assert.Equal(t, 1, len(acc.PendingOrders()))
	assert.Equal(t, 200, int(acc.Balance(1).Pending))
	assert.Equal(t, 100, int(acc.Balance(1).Available))

	trans = s.Transition(2)
	s = trans.Commit().(*State)
	acc = s.Account(addr)
	assert.Equal(t, 0, len(acc.PendingOrders()))
	assert.Equal(t, 0, int(acc.Balance(1).Pending))
	assert.Equal(t, 300, int(acc.Balance(1).Available))
}

func TestSellOrderExpire(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	s.UpdateToken(Token{ID: 1, TokenInfo: BNBInfo})
	pk, sk := RandKeyPair()
	addr := pk.Addr()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(0, Balance{Available: 300})

	order := PlaceOrderTxn{
		SellSide:    true,
		Quant:       100,
		Price:       2 * uint64(math.Pow10(OrderPriceDecimals)),
		ExpireRound: 3,
		Market:      MarketSymbol{Quote: 1, Base: 0},
	}
	trans := s.Transition(1)
	pt, err := parseTxn(MakePlaceOrderTxn(sk, addr, order, 0), &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}
	err = trans.Record(pt)
	assert.Nil(t, err)
	// transition for the current round will expire the order for
	// the next round.
	s = trans.Commit().(*State)
	acc = s.Account(addr)
	assert.Equal(t, 1, len(acc.PendingOrders()))
	assert.Equal(t, 100, int(acc.Balance(0).Pending))
	assert.Equal(t, 200, int(acc.Balance(0).Available))

	trans = s.Transition(2)
	s = trans.Commit().(*State)
	acc = s.Account(addr)
	assert.Equal(t, 0, len(acc.PendingOrders()))
	assert.Equal(t, 0, int(acc.Balance(0).Pending))
	assert.Equal(t, 300, int(acc.Balance(0).Available))
}

func TestNonce(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	pk, sk := RandKeyPair()
	addr := pk.Addr()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(0, Balance{Available: 100})
	trans := s.Transition(1)

	to, _ := RandKeyPair()
	txn := MakeSendTokenTxn(sk, addr, to, 0, 20, 0)
	pt, err := parseTxn(txn, &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}

	err = trans.Record(pt)
	assert.Nil(t, err)
	s = trans.Commit().(*State)

	trans = s.Transition(2)
	err = trans.Record(pt)
	assert.Contains(t, err.Error(), "nonce not valid")

	txn = MakeSendTokenTxn(sk, addr, to, 0, 20, 1)
	pt, err = parseTxn(txn, &myPKer{m: map[consensus.Addr]PK{
		addr: pk,
	}})
	if err != nil {
		panic(err)
	}
	err = trans.Record(pt)
	assert.Nil(t, err)
}

func TestBurnToken(t *testing.T) {
	const burn = 1000
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	pk, sk := RandKeyPair()
	acc := s.NewAccount(pk)
	acc.UpdateBalance(0, Balance{Available: burn + 100})
	txn := MakeBurnTokenTxn(sk, pk.Addr(), BurnTokenTxn{ID: 0, Quant: burn}, 0)

	pker := &myPKer{m: map[consensus.Addr]PK{
		pk.Addr(): pk,
	}}
	pt, err := parseTxn(txn, pker)
	if err != nil {
		panic(err)
	}

	trans := s.Transition(1)
	err = trans.Record(pt)
	assert.Nil(t, err)
	s = trans.Commit().(*State)
	acc = s.Account(pk.Addr())
	assert.Equal(t, 100, int(acc.Balance(0).Available))
	cache := newTokenCache(s)
	assert.Equal(t, int(BNBInfo.TotalUnits-burn), int(cache.Info(0).TotalUnits))
}

func TestPlaceOrder(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	s.UpdateToken(Token{ID: 0, TokenInfo: BNBInfo})
	s.UpdateToken(Token{ID: 1, TokenInfo: BNBInfo})
	pkSell, skSell := RandKeyPair()
	pkBuy, skBuy := RandKeyPair()
	sellAcc := s.NewAccount(pkSell)
	buyAcc := s.NewAccount(pkBuy)
	buyAcc.UpdateBalance(1, Balance{Available: 200})
	sellAcc.UpdateBalance(0, Balance{Available: 100})

	pker := &myPKer{m: map[consensus.Addr]PK{
		pkBuy.Addr():  pkBuy,
		pkSell.Addr(): pkSell,
	}}
	trans := s.Transition(1)

	// buy 40
	order := PlaceOrderTxn{
		SellSide: false,
		// will be pending 40*2
		Quant:  40,
		Price:  2 * uint64(math.Pow10(OrderPriceDecimals)),
		Market: MarketSymbol{Quote: 1, Base: 0},
	}
	pt, err := parseTxn(MakePlaceOrderTxn(skBuy, pkBuy.Addr(), order, 0), pker)
	if err != nil {
		panic(err)
	}

	err = trans.Record(pt)
	assert.Nil(t, err)
	s = trans.Commit().(*State)

	buyAcc = s.Account(pkBuy.Addr())
	assert.Equal(t, 80, int(buyAcc.Balance(1).Pending))
	assert.Equal(t, 120, int(buyAcc.Balance(1).Available))
	assert.Equal(t, 1, len(buyAcc.PendingOrders()))

	// buy 20, sell 55
	trans = s.Transition(2)
	order = PlaceOrderTxn{
		SellSide: false,
		// will be pending 20*3
		Quant:  20,
		Price:  3 * uint64(math.Pow10(OrderPriceDecimals)),
		Market: MarketSymbol{Quote: 1, Base: 0},
	}
	pt, err = parseTxn(MakePlaceOrderTxn(skBuy, pkBuy.Addr(), order, 1), pker)
	if err != nil {
		panic(err)
	}
	err = trans.Record(pt)
	assert.Nil(t, err)

	order = PlaceOrderTxn{
		SellSide: true,
		// 20 fill at 3.0, 35 fill at 2.0
		Quant:  55,
		Price:  2 * uint64(math.Pow10(OrderPriceDecimals)),
		Market: MarketSymbol{Quote: 1, Base: 0},
	}
	pt, err = parseTxn(MakePlaceOrderTxn(skSell, pkSell.Addr(), order, 0), pker)
	if err != nil {
		panic(err)
	}
	err = trans.Record(pt)
	assert.Nil(t, err)

	s = trans.Commit().(*State)
	sellAcc = s.Account(pkSell.Addr())
	assert.Equal(t, 0, int(sellAcc.Balance(0).Pending))
	assert.Equal(t, 45, int(sellAcc.Balance(0).Available))
	assert.Equal(t, 20*3+35*2, int(sellAcc.Balance(1).Available))
	assert.Equal(t, 0, int(sellAcc.Balance(1).Pending))
	assert.Equal(t, 2, len(sellAcc.ExecutionReports()))
	assert.Equal(t, 0, len(sellAcc.PendingOrders()))

	buyAcc = s.Account(pkBuy.Addr())
	assert.Equal(t, 55, int(buyAcc.Balance(0).Available))
	assert.Equal(t, 0, int(buyAcc.Balance(0).Pending))
	assert.Equal(t, 60, int(buyAcc.Balance(1).Available))
	assert.Equal(t, 10, int(buyAcc.Balance(1).Pending))
	assert.Equal(t, 2, len(buyAcc.ExecutionReports()))
	assert.Equal(t, 1, len(buyAcc.PendingOrders()))
	po := buyAcc.PendingOrders()[0]
	assert.Equal(t, 35, int(po.Executed))
	assert.Equal(t, 40, int(po.Quant))
}

func TestCalcQuoteQuant(t *testing.T) {
	assert.Equal(t, 40, int(calcQuoteQuant(40, 8, uint64(math.Pow10(OrderPriceDecimals)), 8, 8)))
}
