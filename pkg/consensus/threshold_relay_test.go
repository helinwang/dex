package consensus

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
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

func decodeFromFile(path string, v interface{}) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	dec := gob.NewDecoder(bytes.NewReader(b))
	err = dec.Decode(v)
	if err != nil {
		panic(err)
	}
}

func setupNodes() []*Node {
	const (
		groupSize = 5
		threshold = 3
	)

	var genesis Block
	decodeFromFile("test_data/credentials/genesis.gob", &genesis)
	files, err := ioutil.ReadDir("test_data/credentials/nodes")
	if err != nil {
		panic(err)
	}

	var nodeCredentials []NodeCredentials
	for _, f := range files {
		var n NodeCredentials
		decodeFromFile("test_data/credentials/nodes/"+f.Name(), &n)
		nodeCredentials = append(nodeCredentials, n)
	}

	net := &LocalNet{}
	nodes := make([]*Node, len(nodeCredentials))

	cfg := Config{
		BlockTime:      200 * time.Millisecond,
		GroupSize:      groupSize,
		GroupThreshold: threshold,
	}

	for i := range nodes {
		nodes[i] = MakeNode(nodeCredentials[i], net, cfg, &genesis, &emptyState{}, nil)

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
		go net.Start(fmt.Sprintf("node-%d", i%len(nodes)), fmt.Sprintf("node-%d", (i+len(nodes)-1)%len(nodes)))
	}

	time.Sleep(30 * time.Millisecond)
	return nodes
}

func TestThresholdRelay(t *testing.T) {
	nodes := setupNodes()
	assert.Equal(t, 30, len(nodes))
	assert.Equal(t, 20, len(nodes[0].chain.RandomBeacon.groups))

	for _, n := range nodes {
		n.StartRound(1)
	}

	time.Sleep(790 * time.Millisecond)
	for _, n := range nodes {
		round := n.chain.Round()
		assert.Equal(t, uint64(4), round)
		assert.Equal(t, uint64(5), n.chain.RandomBeacon.Depth())
	}

	fmt.Println(nodes[0].chain.Graphviz())
	fmt.Println(nodes[1].chain.Graphviz())
	fmt.Println(nodes[2].chain.Graphviz())
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

func (e *emptyState) Accounts() Hash {
	return SHA3([]byte("abc"))
}

func (e *emptyState) MatchOrders() {
}

func (e *emptyState) Tokens() Hash {
	return SHA3([]byte("abc"))
}

func (e *emptyState) PendingOrders() Hash {
	return SHA3([]byte("abc"))
}

func (e *emptyState) Reports() Hash {
	return SHA3([]byte("abc"))
}

func (e *emptyState) Transition() Transition {
	return &emptyTransition{}
}

type emptyTransition struct {
}

func (e *emptyTransition) Record(txn []byte) (valid, future bool) {
	return true, false
}

func (e *emptyTransition) Txns() [][]byte {
	return nil
}

func (e *emptyTransition) Commit() {
}
