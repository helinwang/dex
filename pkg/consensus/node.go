package consensus

import (
	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr         Addr
	sk           bls.SecretKey
	net          *Networking
	randomBeacon *RandomBeacon

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
		chain:       NewChain(genesis, genesisState),
		memberships: make(map[bls.PublicKey]membership),
	}

	sysState := NewSysState()
	t := sysState.Transition()
	for _, txn := range genesis.SysTxns {
		valid := t.Record(txn)
		if !valid {
			panic("sys txn in genesis is invalid")
		}
	}

	sysState = t.Apply()
	sysState.Finalized()
	n.chain.LastFinalizedSysState = sysState
	n.randomBeacon = NewRandomBeacon(seed, sysState.groups)
	return n
}
