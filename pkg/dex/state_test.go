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
	m := MarketSymbol{Quote: 0, Base: 1}
	assert.True(t, m.Valid())

	m = MarketSymbol{Quote: 0, Base: 0}
	assert.False(t, m.Valid())

	m = MarketSymbol{Quote: 1, Base: 0}
	assert.False(t, m.Valid())
}

func TestMarketSymbolPathPrefix(t *testing.T) {
	m0 := MarketSymbol{Quote: (1 << 31) - 1, Base: 1 << 31}
	m1 := MarketSymbol{Quote: (1 << 1) - 2, Base: (1 << 31) - 1}

	assert.Equal(t, 32, int(unsafe.Sizeof(m0.Quote))*8, "PathPrefix assumes PathPrefix.Quote being 32 bits.")
	assert.Equal(t, 32, int(unsafe.Sizeof(m0.Base))*8, "PathPrefix assumes PathPrefix.Base being 32 bits.")

	p0 := m0.Bytes()
	p1 := m1.Bytes()
	assert.NotEqual(t, p0, p1)

	assert.True(t, len(p0) <= 12*8)
	assert.True(t, len(p1) <= 12*8)
}

func TestPendingOrders(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))

	// Add orders to some irrelevant markets to make sure the
	// pending order trie works correctly when other markets
	// exists too.
	m1 := MarketSymbol{Quote: 0, Base: 5}
	m2 := MarketSymbol{Quote: 0, Base: 2}
	m3 := MarketSymbol{Quote: 1, Base: 1}
	m4 := MarketSymbol{Quote: 1, Base: 0}
	add := PendingOrder{
		Owner: consensus.Addr{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		Order: matching.Order{SellSide: true, Quant: 100, Price: 1},
	}
	s.UpdatePendingOrder(m1, &add, nil)
	s.UpdatePendingOrder(m2, &add, nil)
	s.UpdatePendingOrder(m3, &add, nil)
	s.UpdatePendingOrder(m4, &add, nil)
	assert.Equal(t, 1, len(s.MarketPendingOrders(m1)))
	assert.Equal(t, 1, len(s.MarketPendingOrders(m2)))
	assert.Equal(t, 1, len(s.MarketPendingOrders(m3)))
	assert.Equal(t, 1, len(s.MarketPendingOrders(m4)))

	m := MarketSymbol{Quote: 0, Base: 1}
	assert.Equal(t, 0, len(s.MarketPendingOrders(m)))
	add = PendingOrder{
		Owner: consensus.Addr{1},
		Order: matching.Order{SellSide: true, Quant: 100, Price: 1},
	}
	s.UpdatePendingOrder(m, &add, nil)
	add1 := PendingOrder{
		Owner: consensus.Addr{2},
		Order: matching.Order{SellSide: false, Quant: 50, Price: 1},
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
