package local

import (
	"fmt"
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
)

// Network is a local network implementation
type Network struct {
	mu    sync.Mutex
	peers map[string]consensus.Peer
}

// Start starts the network for the given address.
func (n *Network) Start(addr string, p consensus.Peer) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.peers == nil {
		n.peers = make(map[string]consensus.Peer)
	}

	n.peers[addr] = p
}

// Connect connects to the peer.
func (n *Network) Connect(addr string) (consensus.Peer, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.peers == nil {
		return nil, fmt.Errorf("peer not found: %s", addr)
	}

	p, ok := n.peers[addr]
	if !ok {
		return nil, fmt.Errorf("peer not found: %s", addr)
	}

	return p, nil
}
