package consensus

import (
	"log"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr     Addr
	sk       bls.SecretKey
	net      *Networking
	round    uint64
	curRand  RandVal
	pastRand map[RandVal]struct{}
	addrToPK map[Addr]bls.PublicKey

	// the memberships of random beacon committees
	rbCommittee []membership
	// the memberships of noterization committees
	ntCommittee []membership

	curChain      *Chain
	curState      State
	curTransition Transition

	aliveChains []*Chain
	store       *StateStore

	pendingTxns   [][]byte
	pendingBlocks []*Block
}

type membership struct {
	skShare bls.SecretKey
	g       *Group
}

func (n *Node) recvTxn(txn []byte) {
	valid, future := n.curTransition.Apply(txn)
	if !valid {
		log.Printf("received invalid txn, len: %d\n", len(txn))
		return
	}

	if future {
		n.pendingTxns = append(n.pendingTxns, txn)
	}
}

func (n *Node) verifySig(b *BlockProposal) bool {
	pk, ok := n.addrToPK[b.Owner]
	if !ok {
		return false
	}

	if len(b.OwnerSig) == 0 {
		return false
	}

	var sign bls.Sign
	err := sign.Deserialize(b.OwnerSig)
	if err != nil {
		return false
	}

	d := b.Encode(false)
	return sign.Verify(&pk, string(d))
}

func (n *Node) recvBlockProposal(b *BlockProposal) {
	ok := n.verifySig(b)
	if !ok {
		log.Println("received block with invalid signature")
		return
	}

}
