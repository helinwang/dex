package consensus

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestAddrID(t *testing.T) {
	addr := SHA3([]byte{1}).Addr()
	assert.Equal(t, addr.ID(), addr.ID())
	addr1 := SHA3([]byte{2}).Addr()
	assert.NotEqual(t, addr.ID(), addr1.ID())
}

func TestBlockProposalEncodeDecode(t *testing.T) {
	b := ShardBlockProposal{
		ShardIdx:  1,
		Round:     2,
		PrevBlock: Hash{3},
		Txns:      []byte{1, 2, 3},
		Owner:     Addr{4},
		OwnerSig:  []byte{4, 5, 6},
	}

	withSig := b.Encode(true)
	withoutSig := b.Encode(false)
	assert.NotEqual(t, withSig, withoutSig)

	var b0 ShardBlockProposal
	err := rlp.DecodeBytes(withSig, &b0)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 ShardBlockProposal
	err = rlp.DecodeBytes(withoutSig, &b1)
	if err != nil {
		panic(err)
	}

	b0.OwnerSig = []byte{}
	assert.Equal(t, b0, b1)

	b2 := b
	assert.Equal(t, b2.Encode(true), b.Encode(true))
	assert.Equal(t, b2.Encode(false), b.Encode(false))
}

func TestBlockEncodeDecode(t *testing.T) {
	b := Block{
		StateRoot:    Hash{1},
		Notarization: []byte{4, 5, 6},
		SysTxns:      []SysTxn{},
		ShardBlocks:  []Hash{{1}},
	}

	withSig := b.Encode(true)
	withoutSig := b.Encode(false)
	assert.NotEqual(t, withSig, withoutSig)

	var b0 Block
	err := rlp.DecodeBytes(withSig, &b0)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 Block
	err = rlp.DecodeBytes(withoutSig, &b1)
	if err != nil {
		panic(err)
	}

	b0.Notarization = []byte{}
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
	err := rlp.DecodeBytes(withSig, &b0)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, b, b0)

	var b1 RandBeaconSig
	err = rlp.DecodeBytes(withoutSig, &b1)
	if err != nil {
		panic(err)
	}

	b0.Sig = []byte{}
	assert.Equal(t, b0, b1)
}

func TestNtShareEncodeDecode(t *testing.T) {
	nt := ShardNtShare{
		Round:    1,
		BP:       Hash{2},
		SigShare: []byte{4},
		Owner:    Addr{5},
		Sig:      []byte{6},
	}

	var nt0 ShardNtShare
	err := rlp.DecodeBytes(nt.Encode(true), &nt0)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, nt, nt0)

	var nt1 ShardNtShare
	err = rlp.DecodeBytes(nt.Encode(false), &nt1)
	if err != nil {
		panic(err)
	}

	nt.Sig = []byte{}
	assert.Equal(t, nt, nt1)

}

func TestEncodeSlice(t *testing.T) {
	b := ShardBlockProposal{
		Txns: []byte{},
	}

	b0 := ShardBlockProposal{
		Txns: nil,
	}

	// make sure a nil slice and an empty slice are encoded
	// differently.
	assert.NotEqual(t, b, b0)
}
