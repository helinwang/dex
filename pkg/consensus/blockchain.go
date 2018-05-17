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

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(use)
	if err != nil {
		// should never happen
		panic(err)
	}

	return buf.Bytes()
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

// BlockProposal is a block proposal, or a unnotarized block.
type BlockProposal struct {
	Round            int
	PrevNotarization Hash
	Data             []byte
	Owner            Addr
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

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(use)
	if err != nil {
		// should never happen
		panic(err)
	}

	return buf.Bytes()
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

// Notarization is a notarization for a block proposal.
type Notarization struct {
	Round         int
	StateRoot     Hash
	BlockProposal Hash
	PrevNt        Hash
	// The signature of the gob serialized Notarization with
	// GroupSig set to nil.
	GroupSig []byte
}

// Encode encodes the notarization.
func (n *Notarization) Encode(withSig bool) []byte {
	use := n
	if !withSig {
		newB := *n
		newB.GroupSig = nil
		use = &newB
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(use)
	if err != nil {
		// should never happen
		panic(err)
	}

	return buf.Bytes()
}

// Decode decodes the data into the notarization.
func (n *Notarization) Decode(d []byte) error {
	var use Notarization
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*n = use
	return nil
}

// Block is a notarized block proposal.
type Block struct {
	P *BlockProposal
	N *Notarization
}

// Chain is a single branch of the blockchain.
//
// There will be multiple chains if the blockchain has forks.
type Chain struct {
	// The weights of all unfinalized prefix chains, including
	// itself. In reverse order, e.g., Weights[0] is the weight of
	// itself if itself is not finalized.
	Weights   []float64
	Blocks    []*Block
	Proposals []*BlockProposal
}
