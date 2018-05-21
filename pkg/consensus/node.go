package consensus

import (
	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr Addr
	sk   bls.SecretKey
	net  *Networking

	// the memberships of different groups
	memberships map[bls.PublicKey]membership
	chain       *Chain
	pendingTxns [][]byte
}

type membership struct {
	skShare bls.SecretKey
	g       *Group
}

// NewNode creates a new node.
func NewNode(genesis *Block, genesisState State, sk bls.SecretKey, net *Networking, seed Rand) *Node {
	pk := sk.GetPublicKey()
	pkHash := hash(pk.Serialize())
	addr := pkHash.Addr()

	n := &Node{
		addr:        addr,
		sk:          sk,
		chain:       NewChain(genesis, genesisState, seed),
		memberships: make(map[bls.PublicKey]membership),
	}

	return n
}
