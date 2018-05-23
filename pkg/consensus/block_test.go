package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddrID(t *testing.T) {
	addr := hash([]byte{1}).Addr()
	assert.Equal(t, addr.ID(), addr.ID())
	addr1 := hash([]byte{2}).Addr()
	assert.NotEqual(t, addr.ID(), addr1.ID())
}

func TestBlockProposalEncodeDecode(t *testing.T) {
	b := BlockProposal{
		Data:     []byte{1, 2, 3},
		OwnerSig: []byte{4, 5, 6},
	}

	withSig := b.Encode(true)
	withoutSig := b.Encode(false)
	assert.NotEqual(t, withSig, withoutSig)

	var b0 BlockProposal
	err := b0.Decode(withSig)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 BlockProposal
	err = b1.Decode(withoutSig)
	if err != nil {
		panic(err)
	}

	b0.OwnerSig = nil
	assert.Equal(t, b0, b1)
}

func TestBlockEncodeDecode(t *testing.T) {
	b := Block{
		StateRoot:       Hash{1},
		NotarizationSig: []byte{4, 5, 6},
	}

	withSig := b.Encode(true)
	withoutSig := b.Encode(false)
	assert.NotEqual(t, withSig, withoutSig)

	var b0 Block
	err := b0.Decode(withSig)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 Block
	err = b1.Decode(withoutSig)
	if err != nil {
		panic(err)
	}

	b0.NotarizationSig = nil
	assert.Equal(t, b0, b1)
}

func TestRandSigEncodeDecode(t *testing.T) {
	b := RandBeaconSig{
		LastSigHash: Hash{1},
		Sig:         []byte{4, 5, 6},
	}

	withSig := b.Encode(true)
	withoutSig := b.Encode(false)
	assert.NotEqual(t, withSig, withoutSig)

	var b0 RandBeaconSig
	err := b0.Decode(withSig)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 RandBeaconSig
	err = b1.Decode(withoutSig)
	if err != nil {
		panic(err)
	}

	b0.Sig = nil
	assert.Equal(t, b0, b1)
}

func TestEncodeSlice(t *testing.T) {
	b := BlockProposal{
		Data: []byte{},
	}

	b0 := BlockProposal{
		Data: nil,
	}

	// make sure a nil slice and an empty slice are encoded
	// differently.
	assert.NotEqual(t, b, b0)
}
