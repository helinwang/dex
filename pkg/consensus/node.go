package consensus

import (
	"bytes"
	"encoding/gob"
	"errors"
	"log"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr       Addr
	sk         bls.SecretKey
	net        *Networking
	roundInfo  *RoundInfo
	nodeIDToPK map[int]bls.PublicKey
	addrToPK   map[Addr]bls.PublicKey
	idToGroup  map[int]*Group

	// the memberships of different groups
	memberships map[bls.PublicKey]membership
	chain       *Chain
	pendingTxns [][]byte
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
		nodeIDToPK:  make(map[int]bls.PublicKey),
		addrToPK:    make(map[Addr]bls.PublicKey),
		memberships: make(map[bls.PublicKey]membership),
		idToGroup:   make(map[int]*Group),
	}

	err := n.applySysTxns(genesis.SysTxns, true)
	if err != nil {
		// the sys txns are from the genesis block, should not
		// happen
		panic(err)
	}

	// advance from the 0th round to the 1st round
	n.roundInfo.Advance(Hash(seed))
	return n
}

type membership struct {
	skShare bls.SecretKey
	g       *Group
}

func (n *Node) applyReadyJoinGroup(t ReadyJoinGroupTxn) error {
	var pk bls.PublicKey
	err := pk.Deserialize(t.PK)
	if err != nil {
		return err
	}

	addr := hash(pk.Serialize()).Addr()
	n.nodeIDToPK[t.ID] = pk
	n.addrToPK[addr] = pk
	return nil
}

func (n *Node) applyRegGroup(t RegGroupTxn) error {
	var pk bls.PublicKey
	err := pk.Deserialize(t.PK)
	if err != nil {
		return err
	}

	g := NewGroup(pk)
	for _, id := range t.MemberIDs {
		pk, ok := n.nodeIDToPK[id]
		if !ok {
			return errors.New("node not found")
		}

		addr := hash(pk.Serialize()).Addr()
		g.MemberPK[addr] = pk
	}

	// TODO: parse vvec

	n.idToGroup[t.ID] = g
	return nil
}

func (n *Node) applyListGroups(t ListGroupsTxn) error {
	gs := make([]*Group, len(t.GroupIDs))
	for i, id := range t.GroupIDs {
		g, ok := n.idToGroup[id]
		if !ok {
			return errors.New("group not found")
		}
		gs[i] = g
	}
	n.roundInfo.groups = gs
	return nil
}

// nolint: gocyclo
func (n *Node) applySysTxns(txns []SysTxn, noCheck bool) error {
	// TODO: apply sys txn should be atomic. E.g., when error
	// happens in the middle, nothing will be changed.
	for _, txn := range txns {
		if !noCheck {
			// TODO: check signature, endorsement proof,
			// etc.
		}

		dec := gob.NewDecoder(bytes.NewReader(txn.Data))
		switch txn.Type {
		case ReadyJoinGroup:
			var t ReadyJoinGroupTxn
			err := dec.Decode(&t)
			if err != nil {
				return err
			}

			err = n.applyReadyJoinGroup(t)
			if err != nil {
				return err
			}
		case RegGroup:
			var t RegGroupTxn
			err := dec.Decode(&t)
			if err != nil {
				return err
			}

			err = n.applyRegGroup(t)
			if err != nil {
				return err
			}
		case ListGroups:
			var t ListGroupsTxn
			err := dec.Decode(&t)
			if err != nil {
				return err
			}

			err = n.applyListGroups(t)
			if err != nil {
				return err
			}
		}
	}

	return nil
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
