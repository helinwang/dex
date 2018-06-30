package dex

import (
	"testing"

	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func TestAccountCommitCache(t *testing.T) {
	s := NewState(ethdb.NewMemDatabase())
	pk, _ := RandKeyPair()
	acc := s.NewAccount(pk)
	acc.UpdateNonce(1)
	assert.Equal(t, 1, int(acc.Nonce()))
	s.CommitCache()
	acc0 := s.Account(pk.Addr())
	assert.Equal(t, acc, acc0)
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
		pk:    PK{1, 2, 3},
		nonce: 4,
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
