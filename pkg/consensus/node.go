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
	memberships []membership
	chain       *Chain
	pendingTxns [][]byte
}

type membership struct {
	skShare bls.SecretKey
	groupID int
}

// NewNode creates a new node.
func NewNode(chain *Chain, sk bls.SecretKey, net *Networking) *Node {
	pk := sk.GetPublicKey()
	pkHash := hash(pk.Serialize())
	addr := pkHash.Addr()
	n := &Node{
		addr:  addr,
		sk:    sk,
		chain: chain,
		net:   net,
	}
	chain.n = n
	return n
}

func (n *Node) StartRound(round int) {
	lastSigHash := hash(n.chain.RandomBeacon.History()[round-1].Sig)
	rbGroup := n.chain.RandomBeacon.RandBeaconGroupID()
	for _, m := range n.memberships {
		if m.groupID == rbGroup {
			// Current node is a member of the random
			// beacon committee, members collatively
			// produce the random beacon signature using
			// BLS threshold signature scheme. There are
			// multiple committees, which committee will
			// produce the next random beacon signature is
			// derived from the current random beacon
			// signature.
			keyShare := m.skShare
			s := signRandBeaconShare(n.sk, keyShare, round, lastSigHash)
			go n.net.recvRandBeaconSigShare(s)
		}
	}
}
