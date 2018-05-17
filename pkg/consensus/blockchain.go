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

// Block is generated from a block proposal collaboratively by the
// notarization committee, it is notarized by the notarization
// signature.
type Block struct {
	Round         int
	StateRoot     Hash
	BlockProposal Hash
	PrevBlock     Hash
	SysTxns       []SysTxn
	// The signature of the gob serialized block with NtSig set to
	// nil.
	NtSig []byte
}

// Encode encodes the block.
func (n *Block) Encode(withSig bool) []byte {
	use := n
	if !withSig {
		newB := *n
		newB.NtSig = nil
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

type unNotarized struct {
	Weight float64
	BP     *BlockProposal

	Parent *notarized
}

type notarized struct {
	Block  *Block
	State  State
	Weight float64

	NtChildren    *notarized
	NonNtChildren *unNotarized

	// for a finalized block, its block proposal can be deleted to
	// save space.
	BP *BlockProposal
}

// Chain is a single branch of the blockchain.
//
// There will be multiple chains if the blockchain has forks.
type Chain struct {
	// Finalized notarizations, the block proposals are discarded
	// to save space.
	Finalized []*notarized
	Fork      []*notarized
	Leader    *notarized
}

// NewChain creates a new chain.
func NewChain() *Chain {
	n := &notarized{
		Block: &Genesis,
		State: GenesisState,
	}

	return &Chain{
		Finalized: []*notarized{n},
		Leader:    n,
	}
}
