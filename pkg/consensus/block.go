package consensus

import (
	"fmt"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	addrBytes = 20
)

var ZeroAddr = Addr{}

// Addr is the address of an account.
type Addr [addrBytes]byte

func (a Addr) String() string {
	return fmt.Sprintf("%x", a[:])
}

func (a Addr) Hex() string {
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
	Round       uint64
	LastSigHash Hash
	Sig         Sig
}

// Encode encodes the random beacon signature.
func (r *RandBeaconSig) Encode(withSig bool) []byte {
	en := *r
	if !withSig {
		en.Sig = nil
	}

	b, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return b
}

// Hash returns the hash of the random beacon signature.
func (r *RandBeaconSig) Hash() Hash {
	return SHA3(r.Encode(true))
}

// RandBeaconSigShare is one share of the random beacon signature.
type RandBeaconSigShare struct {
	Owner       Addr
	Round       uint64
	LastSigHash Hash
	Share       Sig
	OwnerSig    Sig
}

// Encode encodes the random beacon signature share.
func (r *RandBeaconSigShare) Encode(withSig bool) []byte {
	en := *r
	if !withSig {
		en.OwnerSig = nil
	}

	b, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return b
}

// Hash returns the hash of the random beacon signature share.
func (r *RandBeaconSigShare) Hash() Hash {
	return SHA3(r.Encode(true))
}

// NtShare is one share of the notarization. A notarization and its
// block proposal form a block.
//
// Each member of the notarization committee will broadcast its own
// notarization signature share. which threshold numbers of signature
// shares from different members, the notarization signature can be
// recovered, and a block can be made from the signature and the block
// proposal.
type NtShare struct {
	Round     uint64
	BP        Hash
	Receipts  Hash
	SigShare  Sig
	Owner     Addr
	StateRoot Hash
	Sig       Sig
}

// Encode encodes the notarization share.
func (n *NtShare) Encode(withSig bool) []byte {
	en := *n
	if !withSig {
		en.Sig = nil
	}

	b, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return b
}

// Hash returns the hash of the notarization share.
func (n *NtShare) Hash() Hash {
	return SHA3(n.Encode(true))
}

// BlockProposal is a block proposal.
type BlockProposal struct {
	Round     uint64
	PrevBlock Hash
	Data      []byte
	SysTxns   []SysTxn
	Owner     Addr
	// The signature of the gob serialized BlockProposal with
	// OwnerSig set to nil.
	OwnerSig Sig
}

// Encode encodes the block proposal.
func (bp *BlockProposal) Encode(withSig bool) []byte {
	en := *bp
	if !withSig {
		en.OwnerSig = nil
	}

	b, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return b
}

// Hash returns the hash of the block proposal.
func (bp *BlockProposal) Hash() Hash {
	return SHA3(bp.Encode(true))
}

type Genesis struct {
	Block Block
	State TrieBlob
}

// Block is generated from a block proposal collaboratively by the
// notarization committee, it is notarized by the notarization
// signature.
type Block struct {
	Rank          uint16
	Round         uint64
	StateRoot     Hash
	BlockProposal Hash
	PrevBlock     Hash
	SysTxns       []SysTxn
	// The signature of the gob serialized block with
	// NotarizationSig set to nil.
	NotarizationSig Sig
	Receipts        Hash
}

// Encode encodes the block.
func (b *Block) Encode(withSig bool) []byte {
	en := *b
	if !withSig {
		en.NotarizationSig = nil
	}

	d, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return d
}

// Hash returns the hash of the block.
func (b *Block) Hash() Hash {
	return SHA3(b.Encode(true))
}
