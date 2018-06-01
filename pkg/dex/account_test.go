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
