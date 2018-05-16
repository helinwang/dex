package consensus

import "github.com/dfinity/go-dfinity-crypto/bls"

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr  Addr
	sk    bls.SecretKey
	round uint64
	// the memberships of random beacon committees
	rbCommittee []membership
	// the memberships of noterization committees
	ntCommittee []membership
	aliveChains []*Chain
	store       StateStore
}

type membership struct {
	skShare bls.SecretKey
	g       *Group
}
