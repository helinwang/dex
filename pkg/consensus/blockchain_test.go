package consensus_test

import (
	"testing"

	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func TestBlockProposalEncodeDecode(t *testing.T) {
	b := consensus.BlockProposal{
		Data:     []byte{1, 2, 3},
		OwnerSig: []byte{4, 5, 6},
	}

	withSig := b.Encode(true)
	withoutSig := b.Encode(false)
	assert.NotEqual(t, withSig, withoutSig)

	var b0 consensus.BlockProposal
	err := b0.Decode(withSig)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 consensus.BlockProposal
	err = b1.Decode(withoutSig)
	if err != nil {
		panic(err)
	}

	b0.OwnerSig = nil
	assert.Equal(t, b0, b1)
}

func TestEncodeSlice(t *testing.T) {
	b := consensus.BlockProposal{
		Data: []byte{},
	}

	b0 := consensus.BlockProposal{
		Data: nil,
	}

	// make sure a nil slice and an empty slice are encoded
	// differently.
	assert.NotEqual(t, b, b0)
}
