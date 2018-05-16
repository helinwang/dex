package consensus

import (
	"bytes"
	"encoding/gob"
)

const (
	hashBytes = 32
	addrBytes = 20
)

// Hash is the hash of a piece of data.
type Hash [hashBytes]byte

// Addr is the address of an account.
type Addr [addrBytes]byte

// RandVal is a random value produced by the random beacon.
//
// It is the hash of the random beacon committee group signature.
type RandVal Hash

// BlockProposal is a block proposal, or a unnotarized block.
type BlockProposal struct {
	PrevNoterization Hash
	RandVal          RandVal
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

// Noterization is a noterization for a block proposal.
type Noterization struct {
	StateRoot     Hash
	BlockProposal Hash
	// The signature of the gob serialized Noterization with
	// GroupSig set to nil.
	GroupSig []byte
}

// Block is a noterized block proposal.
type Block struct {
	P *BlockProposal
	N *Noterization
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
