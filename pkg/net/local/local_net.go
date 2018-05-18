package local

import (
	"bytes"
	"encoding/gob"
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

	n.peers[addr] = &wire{p: p}
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

// wire serialize and deserialize the data sent between peers using
// the gob package, to simulate the communication process.
type wire struct {
	p consensus.Peer
}

func (w *wire) SendBlock(b *consensus.Block) error {
	d := gobEncode(b)
	var n consensus.Block
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&n)
	if err != nil {
		// should not happen
		panic(err)
	}

	return w.p.SendBlock(&n)
}

func (w *wire) SendBlockProposal(bp *consensus.BlockProposal) error {
	d := gobEncode(bp)
	var n consensus.BlockProposal
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&n)
	if err != nil {
		// should not happen
		panic(err)
	}

	return w.p.SendBlockProposal(&n)
}

func (w *wire) SendNotarizationShare(bp consensus.Hash, sig []byte) error {
	return w.p.SendNotarizationShare(bp, sig)
}

func (w *wire) GetBlock(h consensus.Hash) (*consensus.Block, error) {
	b, err := w.p.GetBlock(h)
	if err != nil || b == nil {
		return b, err
	}

	d := gobEncode(b)
	var n consensus.Block
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err = dec.Decode(&n)
	if err != nil {
		// should not happen
		panic(err)
	}

	return &n, nil
}

func (w *wire) GetBlockProposal(h consensus.Hash) (*consensus.BlockProposal, error) {
	b, err := w.p.GetBlockProposal(h)
	if err != nil || b == nil {
		return b, err
	}

	d := gobEncode(b)
	var n consensus.BlockProposal
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err = dec.Decode(&n)
	if err != nil {
		// should not happen
		panic(err)
	}

	return &n, nil
}

func gobEncode(v interface{}) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		// should not happen
		panic(err)
	}
	return buf.Bytes()
}
