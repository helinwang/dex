package dex

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func TestAccountEncodeDecode(t *testing.T) {
	a := Account{
		PK:       consensus.PK{1, 2, 3},
		NonceVec: []uint64{4, 5},
		Balances: map[TokenID]*Balance{
			0: &Balance{Available: 100, Pending: 20},
			5: &Balance{Available: 1<<64 - 1, Pending: 1},
		},
		PendingOrders: []PendingOrder{
			{
				ID:       OrderID{ID: 1, Market: MarketSymbol{Base: 2, Quote: 3}},
				Executed: 4,
				Order:    Order{Price: 5}},
		},
	}

	b := stableGobEncode(a)

	var a1 Account
	dec := gob.NewDecoder(bytes.NewReader(b))
	err := dec.Decode(&a1)
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
		PK:       consensus.PK{1, 2, 3},
		NonceVec: []uint64{4, 5},
		Balances: map[TokenID]*Balance{
			0: &Balance{Available: 100, Pending: 20},
			5: &Balance{Available: 1<<64 - 1, Pending: 1},
		},
	}

	var lastHash consensus.Hash
	for i := 0; i < 30; i++ {
		b := stableGobEncode(a)
		h := consensus.SHA3(b)
		if i > 0 {
			assert.Equal(t, lastHash, h)
		}
		lastHash = h
	}
}
