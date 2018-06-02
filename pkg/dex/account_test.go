package dex

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
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
		PendingOrderMarkets: []MarketSymbol{
			{Quote: 0, Base: 1},
			{Quote: 1, Base: 2},
			{Quote: 2, Base: 3},
			{Quote: 4, Base: 5},
		},
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
