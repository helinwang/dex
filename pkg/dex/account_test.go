package dex

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func TestAccountEncodeDecode(t *testing.T) {
	orderID := OrderID{ID: 1, Market: MarketSymbol{Base: 2, Quote: 3}}
	a := Account{
		pk:       consensus.PK{1, 2, 3},
		nonceVec: []uint64{4, 5},
		balances: map[TokenID]Balance{
			0: Balance{Available: 100, Pending: 20, Frozen: []Frozen{}},
			5: Balance{Available: 1<<64 - 1, Pending: 1, Frozen: []Frozen{}},
		},
		pendingOrders: map[OrderID]*PendingOrder{
			orderID: &PendingOrder{
				ID:       orderID,
				Executed: 4,
				Order:    Order{Price: 5}},
		},
		executionReports: []ExecutionReport{{Round: 1}},
	}

	b, err := rlp.EncodeToBytes(&a)
	if err != nil {
		panic(err)
	}

	var a1 Account
	err = rlp.DecodeBytes(b, &a1)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, a, a1)
}

func TestOrderIDEncodeDecode(t *testing.T) {
	const str = "1_2_3"
	var id OrderID
	err := id.Decode(str)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, str, id.Encode())
}

func TestAccountHashDeterministic(t *testing.T) {
	a := Account{
		pk:       consensus.PK{1, 2, 3},
		nonceVec: []uint64{4, 5},
		balances: map[TokenID]Balance{
			0: Balance{Available: 100, Pending: 20},
			5: Balance{Available: 1<<64 - 1, Pending: 1},
		},
	}

	var lastHash consensus.Hash
	for i := 0; i < 30; i++ {
		b, err := rlp.EncodeToBytes(&a)
		if err != nil {
			panic(err)
		}
		h := consensus.SHA3(b)
		if i > 0 {
			assert.Equal(t, lastHash, h)
		}
		lastHash = h
	}
}
