package consensus

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dfinity/go-dfinity-crypto/bls"
	log "github.com/helinwang/log15"

	"github.com/stretchr/testify/assert"
)

func init() {
	bls.Init(int(bls.CurveFp254BNb))
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StdoutHandler))
}

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

	cfg := Config{
		BlockTime:      100 * time.Millisecond,
		NtWaitTime:     120 * time.Millisecond,
		GroupSize:      groupSize,
		GroupThreshold: threshold,
	}

	for i := range nodes {
		chain := NewChain(genesis, &emptyState{}, nodeSeed, cfg)
		networking := NewNetworking(net, fmt.Sprintf("node-%d", (i+len(nodes)-1)%len(nodes)), chain)
		nodes[i] = NewNode(chain, nodeSKs[i], networking, cfg)

		peers := make([]string, len(nodes))
		for i := range peers {
			peers[i] = fmt.Sprintf("node-%d", i)
		}

		nodes[i].net.mu.Lock()
		nodes[i].net.peerAddrs = peers
		nodes[i].net.mu.Unlock()
	}

	for i := range nodes {
		net := nodes[i].net
		addr := nodes[(i-1+len(nodes))%len(nodes)].net.addr
		go net.Start(addr)
	}

	time.Sleep(30 * time.Millisecond)

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

	time.Sleep(400 * time.Millisecond)
	for _, n := range nodes {
		round := n.chain.Round()
		assert.Equal(t, 4, round)
		assert.Equal(t, 5, n.chain.RandomBeacon.Depth())
	}

	fmt.Println(nodes[0].chain.Graphviz())
}

// LocalNet is a local network implementation
type LocalNet struct {
	mu              sync.Mutex
	addrToOnConnect map[string]func(p Peer)
	addrToPeer      map[string]Peer
}

// Start starts the network for the given address.
func (n *LocalNet) Start(addr string, onPeerConnect func(p Peer), p Peer) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.addrToOnConnect == nil {
		n.addrToOnConnect = make(map[string]func(p Peer))
		n.addrToPeer = make(map[string]Peer)
	}

	n.addrToOnConnect[addr] = onPeerConnect
	n.addrToPeer[addr] = p
	return nil
}

// Connect connects to the peer.
func (n *LocalNet) Connect(addr string, myself Peer) (Peer, error) {
	n.mu.Lock()

	if n.addrToOnConnect == nil {
		time.Sleep(10 * time.Millisecond)
		n.mu.Unlock()
		return n.Connect(addr, myself)
	}

	f, ok := n.addrToOnConnect[addr]
	if !ok {
		time.Sleep(10 * time.Millisecond)
		n.mu.Unlock()
		return n.Connect(addr, myself)
	}

	p := n.addrToPeer[addr]
	n.mu.Unlock()

	go f(myself)
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
