package consensus

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

func makeShares(t int, idVec []bls.ID, rand Rand) (bls.PublicKey, []bls.SecretKey, Rand) {
	sk := rand.SK()
	rand = rand.Derive(rand[:])

	msk := sk.GetMasterSecretKey(t)
	skShares := make([]bls.SecretKey, len(idVec))

	for i := range skShares {
		skShares[i].Set(msk, &idVec[i])
	}

	return *sk.GetPublicKey(), skShares, rand
}

func setupNodes() []*Node {
	const (
		numNode   = 10
		numGroup  = 20
		groupSize = 5
		threshold = 3
	)

	rand := Rand(hash([]byte("seed")))
	nodeSeed := rand.Derive([]byte("node"))

	genesis := &Block{}
	nodeSKs := make([]bls.SecretKey, numNode)
	for i := range nodeSKs {
		nodeSKs[i] = rand.SK()
		rand = rand.Derive(rand[:])
		txn := ReadyJoinGroupTxn{
			ID: i,
			PK: nodeSKs[i].GetPublicKey().Serialize(),
		}
		genesis.SysTxns = append(genesis.SysTxns, SysTxn{
			Type: ReadyJoinGroup,
			Data: gobEncode(txn),
		})
	}

	gs := make([]*Group, numGroup)
	groupIDs := make([]int, numGroup)
	sharesVec := make([][]bls.SecretKey, numGroup)
	perms := make([][]int, numGroup)
	for i := range groupIDs {
		perm := rand.Perm(groupSize, numNode)
		perms[i] = perm
		rand = rand.Derive(rand[:])
		idVec := make([]bls.ID, groupSize)
		for i := range idVec {
			pk := nodeSKs[perm[i]].GetPublicKey()
			idVec[i] = hash(pk.Serialize()).Addr().ID()
		}

		var groupPK bls.PublicKey
		groupPK, sharesVec[i], rand = makeShares(threshold, idVec, rand)
		gs[i] = NewGroup(groupPK)
		txn := RegGroupTxn{
			ID:        i,
			PK:        groupPK.Serialize(),
			MemberIDs: perm,
		}
		genesis.SysTxns = append(genesis.SysTxns, SysTxn{
			Type: RegGroup,
			Data: gobEncode(txn),
		})
		groupIDs[i] = i
	}

	l := ListGroupsTxn{
		GroupIDs: groupIDs,
	}

	genesis.SysTxns = append(genesis.SysTxns, SysTxn{
		Type: ListGroups,
		Data: gobEncode(l),
	})

	net := &LocalNet{}
	nodes := make([]*Node, numNode)

	peers := make(map[string]bool)
	for i := range nodes {
		peers[fmt.Sprintf("node-%d", i)] = true
	}

	cfg := Config{
		BlockTime:      100 * time.Millisecond,
		NtWaitTime:     150 * time.Millisecond,
		GroupSize:      groupSize,
		GroupThreshold: threshold,
	}

	for i := range nodes {
		chain := NewChain(genesis, &emptyState{}, nodeSeed, cfg)
		networking := NewNetworking(net, newValidator(chain), fmt.Sprintf("node-%d", (i+len(nodes)-1)%len(nodes)), chain)
		nodes[i] = NewNode(chain, nodeSKs[i], networking, cfg)
		nodes[i].net.peerAddrs = peers
	}

	for i := range nodes {
		go nodes[i].net.Start(nodes[(i-1+len(nodes))%len(nodes)].net.addr)
	}

	time.Sleep(50 * time.Millisecond)

	for i, p := range perms {
		for j, nodeIdx := range p {
			m := membership{groupID: i, skShare: sharesVec[i][j]}
			nodes[nodeIdx].memberships = append(nodes[nodeIdx].memberships, m)
		}
	}

	return nodes
}

func TestThresholdRelay(t *testing.T) {
	nodes := setupNodes()
	for _, n := range nodes {
		n.StartRound(1)
	}

	time.Sleep(2200 * time.Millisecond)
	for _, n := range nodes {
		round := n.chain.Round()
		rb, bp, nt := n.chain.RandomBeacon.Committees(round)
		fmt.Println(round, n.chain.RandomBeacon.Round(), rb, bp, nt)
	}
}

// LocalNet is a local network implementation
type LocalNet struct {
	mu    sync.Mutex
	peers map[string]Peer
}

// Start starts the network for the given address.
func (n *LocalNet) Start(addr string, p Peer) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.peers == nil {
		n.peers = make(map[string]Peer)
	}

	n.peers[addr] = p
	return nil
}

// Connect connects to the peer.
func (n *LocalNet) Connect(addr string) (Peer, error) {
	n.mu.Lock()

	if n.peers == nil {
		time.Sleep(10 * time.Millisecond)
		n.mu.Unlock()
		return n.Connect(addr)
	}

	p, ok := n.peers[addr]
	if !ok {
		time.Sleep(10 * time.Millisecond)
		n.mu.Unlock()
		return n.Connect(addr)
	}

	n.mu.Unlock()
	return p, nil
}

type emptyState struct {
}

func (e *emptyState) Hash() Hash {
	return hash([]byte("abc"))
}

func (e *emptyState) Transition() Transition {
	return &emptyTransition{}
}

type emptyTransition struct {
}

func (e *emptyTransition) Record(txn []byte) (valid, future bool) {
	return true, false
}

func (e *emptyTransition) Clear() [][]byte {
	return nil
}

func (e *emptyTransition) Encode() []byte {
	return nil
}
