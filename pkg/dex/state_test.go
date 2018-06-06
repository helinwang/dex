package dex

import (
	"testing"
	"unsafe"

	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/stretchr/testify/assert"
)

func TestMarketSymbolBytes(t *testing.T) {
	m0 := MarketSymbol{Quote: (1 << 64) - 2, Base: (1 << 64) - 1}
	m1 := MarketSymbol{Quote: (1 << 64) - 2, Base: (1 << 64) - 3}

	assert.Equal(t, 64, int(unsafe.Sizeof(m0.Quote))*8, "PathPrefix assumes PathPrefix.Quote being 64 bits.")
	assert.Equal(t, 64, int(unsafe.Sizeof(m0.Base))*8, "PathPrefix assumes PathPrefix.Base being 64 bits.")

	p0 := m0.Encode()
	p1 := m1.Encode()
	assert.NotEqual(t, p0, p1)
}

func TestOrders(t *testing.T) {
	var owner Account
	addr := owner.PK.Addr()
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))

	// Add orders to some irrelevant markets to make sure the
	// pending order trie works correctly when other markets
	// exists too.
	m1 := MarketSymbol{Quote: 0, Base: 5}
	m2 := MarketSymbol{Quote: 0, Base: 2}
	m3 := MarketSymbol{Quote: 1, Base: 1}
	m4 := MarketSymbol{Quote: 1, Base: 0}
	add := Order{
		Owner:     addr,
		SellSide:  true,
		QuantUnit: 100,
		PriceUnit: 1,
	}
	s.AddOrder(&owner, m1, 1, add)
	s.AddOrder(&owner, m2, 1, add)
	s.AddOrder(&owner, m3, 1, add)
	s.AddOrder(&owner, m4, 1, add)
	assert.Equal(t, 1, len(s.Orders(m1, 1)))
	assert.Equal(t, 1, len(s.Orders(m2, 1)))
	assert.Equal(t, 1, len(s.Orders(m3, 1)))
	assert.Equal(t, 1, len(s.Orders(m4, 1)))

	m := MarketSymbol{Quote: 0, Base: 1}
	assert.Equal(t, 0, len(s.Orders(m, 1)))
	add = Order{
		Owner:     addr,
		SellSide:  true,
		QuantUnit: 100,
		PriceUnit: 1,
	}
	s.AddOrder(&owner, m, 1, add)
	add1 := Order{
		Owner:     addr,
		SellSide:  false,
		QuantUnit: 50,
		PriceUnit: 0,
	}
	s.AddOrder(&owner, m, 1, add1)
	assert.Equal(t, 2, len(s.Orders(m, 1)))

	p1 := s.AccountOrders(&owner, m)
	assert.Equal(t, 2, len(p1))
}

func TestSortOrders(t *testing.T) {
	orders := []Order{
		{PriceUnit: 300, PlacedHeight: 1},
		{PriceUnit: 200},
		{PriceUnit: 300},
		{PriceUnit: 100},
		{PriceUnit: 600},
		{PriceUnit: 200, SellSide: true},
		{PriceUnit: 400, SellSide: true},
		{PriceUnit: 300, SellSide: true},
		{PriceUnit: 500, SellSide: true},
		{PriceUnit: 300, PlacedHeight: 1, SellSide: true},
	}

	sortOrders(orders)

	assert.Equal(t, []Order{
		{PriceUnit: 100},
		{PriceUnit: 200},
		{PriceUnit: 300, PlacedHeight: 1},
		{PriceUnit: 300},
		{PriceUnit: 600},
		{PriceUnit: 200, SellSide: true},
		{PriceUnit: 300, SellSide: true},
		{PriceUnit: 300, PlacedHeight: 1, SellSide: true},
		{PriceUnit: 400, SellSide: true},
		{PriceUnit: 500, SellSide: true},
	}, orders)
}

func TestMarkets(t *testing.T) {
	var owner Account
	addr := owner.PK.Addr()
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	m1 := MarketSymbol{Quote: 0, Base: 5}
	m2 := MarketSymbol{Quote: 0, Base: 2}
	add := Order{
		Owner:     addr,
		SellSide:  true,
		QuantUnit: 100,
		PriceUnit: 1,
	}
	s.AddOrder(&owner, m1, 1, add)
	s.AddOrder(&owner, m2, 1, add)
	markets := s.Markets()
	assert.Equal(t, 2, len(markets))
	assert.Equal(t, m2, markets[0])
	assert.Equal(t, m1, markets[1])
}

func TestMarketEncodeDecode(t *testing.T) {
	m := MarketSymbol{Base: 1<<64 - 1, Quote: 1}
	var m1 MarketSymbol
	err := m1.Decode(m.Encode())
	if err != nil {
		panic(err)
	}

	assert.Equal(t, m, m1)
}
