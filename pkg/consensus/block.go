package consensus

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

const (
	addrBytes = 20
)

// Addr is the address of an account.
type Addr [addrBytes]byte

func (a Addr) String() string {
	return fmt.Sprintf("%x", a[:])
}

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
	LastSigHash Hash
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

// Hash returns the hash of the random beacon signature.
func (b *RandBeaconSig) Hash() Hash {
	return SHA3(b.Encode(true))
}

// RandBeaconSigShare is one share of the random beacon signature.
type RandBeaconSigShare struct {
	Owner       Addr
	Round       int
	LastSigHash Hash
	Share       []byte
	OwnerSig    []byte
}

// Encode encodes the random beacon signature share.
func (b *RandBeaconSigShare) Encode(withSig bool) []byte {
	use := b
	if !withSig {
		newB := *b
		newB.OwnerSig = nil
		use = &newB
	}

	return gobEncode(use)
}

// Decode decodes the data into the random beacon signature share.
func (b *RandBeaconSigShare) Decode(d []byte) error {
	var use RandBeaconSigShare
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*b = use
	return nil
}

// Hash returns the hash of the random beacon signature share.
func (b *RandBeaconSigShare) Hash() Hash {
	return SHA3(b.Encode(true))
}

// NtShare is one share of the notarization.
//
// Each member of the notarization committee will broadcast its own
// notarization signature share. which threshold numbers of signature
// shares from different members, the notarization signature can be
// recovered, and a block can be made from the signature and the block
// proposal.
type NtShare struct {
	Round     int
	BP        Hash
	StateRoot Hash
	SigShare  []byte
	Owner     Addr
	OwnerSig  []byte
}

// Encode encodes the notarization share.
func (n *NtShare) Encode(withSig bool) []byte {
	use := n
	if !withSig {
		newN := *n
		newN.OwnerSig = nil
		use = &newN
	}

	return gobEncode(use)
}

// Decode decodes the data into the notarization share.
func (n *NtShare) Decode(d []byte) error {
	var use NtShare
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*n = use
	return nil
}

// Hash returns the hash of the notarization share.
func (n *NtShare) Hash() Hash {
	return SHA3(n.Encode(true))
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

// Hash returns the hash of the block proposal.
func (b *BlockProposal) Hash() Hash {
	return SHA3(b.Encode(true))
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
func (b *Block) Encode(withSig bool) []byte {
	use := b
	if !withSig {
		newB := *b
		newB.NotarizationSig = nil
		use = &newB
	}
	return gobEncode(use)
}

// Decode decodes the data into the block.
func (b *Block) Decode(d []byte) error {
	var use Block
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*b = use
	return nil
}

// Hash returns the hash of the block.
func (b *Block) Hash() Hash {
	return SHA3(b.Encode(true))
}
