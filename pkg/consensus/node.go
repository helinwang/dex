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
		roundInfo:   NewRoundInfo(seed),
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
	n.roundInfo.groups = sysState.groups

	// advance from the 0th round to the 1st round
	n.roundInfo.Advance(Hash(seed))
	return n
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
