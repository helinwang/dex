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
	addr      Addr
	sk        bls.SecretKey
	net       *Networking
	roundInfo *RoundInfo
	addrToPK  map[Addr]bls.PublicKey

	// the memberships of different groups
	memberships map[bls.PublicKey]membership
	chain       *Chain
	pendingTxns [][]byte
}

// NewNode creates a new node.
func NewNode(sk bls.SecretKey, net *Networking, seed Rand) *Node {
	pk := sk.GetPublicKey()
	pkHash := hash(pk.Serialize())
	addr := pkHash.Addr()

	n := &Node{
		addr:        addr,
		sk:          sk,
		roundInfo:   NewRoundInfo(seed),
		chain:       NewChain(),
		addrToPK:    make(map[Addr]bls.PublicKey),
		memberships: make(map[bls.PublicKey]membership),
	}

	return n
}

type membership struct {
	skShare bls.SecretKey
	g       *Group
}

// Sync synchronize the node with its peers.
func (n *Node) Sync() {
}

func (n *Node) recvTxn(txn []byte) {
	valid, future := n.chain.Leader.State.Transition().Apply(txn)
	if !valid {
		log.Printf("received invalid txn, len: %d\n", len(txn))
		return
	}

	if future {
		n.pendingTxns = append(n.pendingTxns, txn)
	}
}

func (n *Node) recvBlockProposal(b *BlockProposal) {
	if round := n.roundInfo.Round; b.Round < round {
		// stable block proposal, discard without broadcasting
		return
	} else if b.Round > round {
		log.Printf("discard block proposal, bp round: %d > round: %d\n", b.Round, n.roundInfo.Round)
		return
	}

	pk, err := n.roundInfo.BlockMakerPK(b.Owner, b.Round)
	if err == errCommitteeNotSelected {
		log.Printf("discard block proposal: %v, bp round: %d\n", err, b.Round)
		return
	} else if err != nil {
		log.Printf("discard block proposal: %v, bp round: %d\n", err, b.Round)
		return
	}

	if !verifySig(pk, b.OwnerSig, b.Encode(false)) {
		log.Printf("discard block proposal: sig verify failed, bp round: %d\n", b.Round)
		return
	}

	// TODO: broadcast
}
