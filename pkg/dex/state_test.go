package dex

import (
	"testing"
	"unsafe"

	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/matching"
	"github.com/stretchr/testify/assert"
)

func TestMarketSymbolValid(t *testing.T) {
	m := MarketSymbol{A: 0, B: 0}
	assert.True(t, m.Valid())

	m = MarketSymbol{A: 0, B: 1}
	assert.True(t, m.Valid())

	m = MarketSymbol{A: 1, B: 0}
	assert.False(t, m.Valid())
}

func TestMarketSymbolPathPrefix(t *testing.T) {
	m0 := MarketSymbol{A: (1 << 31) - 1, B: 1 << 31}
	m1 := MarketSymbol{A: (1 << 1) - 2, B: (1 << 31) - 1}

	assert.Equal(t, 32, int(unsafe.Sizeof(m0.A))*8, "PathPrefix assumes PathPrefix.A being 32 bits.")
	assert.Equal(t, 32, int(unsafe.Sizeof(m0.B))*8, "PathPrefix assumes PathPrefix.B being 32 bits.")

	p0 := m0.Bytes()
	p1 := m1.Bytes()
	assert.NotEqual(t, p0, p1)

	assert.True(t, len(p0) <= 12*8)
	assert.True(t, len(p1) <= 12*8)
}

func TestPendingOrders(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))

	// add orders to some irrelevant markets
	m1 := MarketSymbol{A: 0, B: 5}
	m2 := MarketSymbol{A: 0, B: 2}
	m3 := MarketSymbol{A: 1, B: 1}
	m4 := MarketSymbol{A: 1, B: 0}
	add := PendingOrder{
		Owner: consensus.Addr{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Order: matching.Order{SellSide: true, Quant: 100, SellBuyRatio: 1},
	}
	s.UpdatePendingOrder(m1, &add, nil)
	s.UpdatePendingOrder(m2, &add, nil)
	s.UpdatePendingOrder(m3, &add, nil)
	s.UpdatePendingOrder(m4, &add, nil)
	assert.Equal(t, 1, len(s.MarketPendingOrders(m1)))
	assert.Equal(t, 1, len(s.MarketPendingOrders(m2)))
	assert.Equal(t, 1, len(s.MarketPendingOrders(m3)))
	assert.Equal(t, 1, len(s.MarketPendingOrders(m4)))

	m := MarketSymbol{A: 0, B: 1}
	assert.Equal(t, 0, len(s.MarketPendingOrders(m)))
	add = PendingOrder{
		Owner: consensus.Addr{1},
		Order: matching.Order{SellSide: true, Quant: 100, SellBuyRatio: 1},
	}
	s.UpdatePendingOrder(m, &add, nil)
	add1 := PendingOrder{
		Owner: consensus.Addr{2},
		Order: matching.Order{SellSide: false, Quant: 50, SellBuyRatio: 1},
	}
	s.UpdatePendingOrder(m, &add1, nil)
	assert.Equal(t, 2, len(s.MarketPendingOrders(m)))

	p0 := s.AccountPendingOrders(m, add.Owner)
	assert.Equal(t, 1, len(p0))
	assert.Equal(t, add, p0[0])

	p1 := s.AccountPendingOrders(m, add1.Owner)
	assert.Equal(t, 1, len(p1))
	assert.Equal(t, add1, p1[0])
}
