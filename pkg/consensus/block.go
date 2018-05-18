package consensus

import (
	"bytes"
	"encoding/gob"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

const (
	addrBytes = 20
)

// Addr is the address of an account.
type Addr [addrBytes]byte

// ID returns the ID associated with this address.
func (a Addr) ID() bls.ID {
	var fr bls.Fr
	fr.SetHashOf(a[:])
	var id bls.ID
	err := id.SetLittleEndian(fr.Serialize())
	if err != nil {
		// should not happen
		panic(err)
	}

	return id
}

// RandVal is a random value produced by the random beacon.
//
// It is the hash of the random beacon committee group signature.
type RandVal Hash

// RandBeaconSig is the random beacon signature produced by the random
// beacon committe.
type RandBeaconSig struct {
	Round       int
	LastRandVal Hash
	Sig         []byte
}

// Encode encodes the random beacon signature.
func (b *RandBeaconSig) Encode(withSig bool) []byte {
	use := b
	if !withSig {
		newB := *b
		newB.Sig = nil
		use = &newB
	}

	return gobEncode(use)
}

// Decode decodes the data into the random beacon signature.
func (b *RandBeaconSig) Decode(d []byte) error {
	var use RandBeaconSig
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*b = use
	return nil
}

// BlockProposal is a block proposal.
type BlockProposal struct {
	Round     int
	PrevBlock Hash
	Data      []byte
	SysTxns   []SysTxn
	Owner     Addr
	// The signature of the gob serialized BlockProposal with
	// OwnerSig set to nil.
	OwnerSig []byte
}

// Encode encodes the block proposal.
func (b *BlockProposal) Encode(withSig bool) []byte {
	use := b
	if !withSig {
		newB := *b
		newB.OwnerSig = nil
		use = &newB
	}

	return gobEncode(use)
}

// Decode decodes the data into the block proposal
func (b *BlockProposal) Decode(d []byte) error {
	var use BlockProposal
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*b = use
	return nil
}

// Block is generated from a block proposal collaboratively by the
// notarization committee, it is notarized by the notarization
// signature.
type Block struct {
	Round         int
	StateRoot     Hash
	BlockProposal Hash
	PrevBlock     Hash
	SysTxns       []SysTxn
	// The signature of the gob serialized block with
	// NotarizationSig set to nil.
	NotarizationSig []byte
}

// Encode encodes the block.
func (n *Block) Encode(withSig bool) []byte {
	use := n
	if !withSig {
		newB := *n
		newB.NotarizationSig = nil
		use = &newB
	}
	return gobEncode(use)
}

// Decode decodes the data into the block.
func (n *Block) Decode(d []byte) error {
	var use Block
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*n = use
	return nil
}
