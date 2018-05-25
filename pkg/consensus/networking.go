package consensus

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"

	log "github.com/helinwang/log15"
)

const (
	connectPeerCount = 8
)

// Peer is a peer node in the DEX network.
type Peer interface {
	Txn(txn []byte) error
	SysTxn(s *SysTxn) error
	RandBeaconSigShare(r *RandBeaconSigShare) error
	RandBeaconSig(r *RandBeaconSig) error
	Block(b *Block) error
	BlockProposal(b *BlockProposal) error
	NotarizationShare(n *NtShare) error
	Inventory(sender Peer, items []ItemID) error
	GetData(requester Peer, items []ItemID) error
	Peers() ([]string, error)
	UpdatePeers([]string) error
	Ping(ctx context.Context) error
	Sync(start int) ([]*RandBeaconSig, []*Block, error)
}

// TODO: networking should ensure that adding things to the chain is
// in order: from lower round to higher round.

// ItemType is the different type of items.
type ItemType int

// different types of items
const (
	TxnItem ItemType = iota
	SysTxnItem
	BlockItem
	BlockProposalItem
	NtShareItem
	RandBeaconShareItem
	RandBeaconItem
)

// ItemID is the identification of an item that the current node owns.
type ItemID struct {
	T         ItemType
	ItemRound int
	Ref       Hash
	Hash      Hash
}

// Network is used to connect to the peers.
type Network interface {
	Start(addr string, onPeerConnect func(p Peer), myself Peer) error
	Connect(addr string, myself Peer) (Peer, error)
}

// Networking is the component that enables the node to talk to its
// peers over the network.
type Networking struct {
	net    Network
	myself Peer
	addr   string
	v      *validator
	chain  *Chain

	mu        sync.Mutex
	peers     []Peer
	peerAddrs []string
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network, addr string, chain *Chain) *Networking {
	r := &receiver{addr: addr}
	n := &Networking{
		myself: r,
		addr:   addr,
		net:    net,
		v:      newValidator(chain),
		chain:  chain,
	}
	r.n = n
	return n
}

func (n *Networking) onPeerConnect(p Peer) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.peers = append(n.peers, p)
}

// must be called with mutex held
func (n *Networking) addAddrs(addrs []string) {
	m := make(map[string]bool)
	for _, a := range n.peerAddrs {
		m[a] = true
	}
	for _, addr := range addrs {
		if m[addr] {
			continue
		}

		// TODO: check peers is online
		n.peerAddrs = append(n.peerAddrs, addr)
	}
}

// Start starts the networking component.
// TODO: fix lint
// nolint: gocyclo
func (n *Networking) Start(seedAddr string) error {
	err := n.net.Start(n.addr, n.onPeerConnect, n.myself)
	if err != nil {
		return err
	}

	p, err := n.net.Connect(seedAddr, n.myself)
	if err != nil {
		return err
	}

	peerAddrs, err := p.Peers()
	if err != nil {
		return err
	}

	n.mu.Lock()
	n.addAddrs(peerAddrs)
	n.peers = append(n.peers, p)

	dest := make([]string, len(n.peerAddrs))
	perm := rand.Perm(len(n.peerAddrs))
	for i, v := range perm {
		dest[v] = n.peerAddrs[i]
	}

	for i, addr := range dest {
		if i >= connectPeerCount {
			break
		}

		p, err = n.net.Connect(addr, n.myself)
		if err != nil {
			log.Error("connect to peer", "err", err, "addr", addr)
		}
		n.peers = append(n.peers, p)
	}
	n.mu.Unlock()

	// TODO: sync random beacon from other peers rather than the
	// seed

	rb, bs, err := p.Sync(len(n.chain.RandomBeacon.History()))
	if err != nil {
		return err
	}

	for _, r := range rb {
		success := n.chain.RandomBeacon.AddRandBeaconSig(r)
		if !success {
			return fmt.Errorf("add RandBeaconSig failed, round: %d, beacon depth: %d", r.Round, n.chain.RandomBeacon.Depth())
		}
	}

	for _, b := range bs {
		weight, valid := n.v.ValidateBlock(b)
		if !valid {
			return errors.New("invalid block when syncing")
		}
		err = n.chain.addBlock(b, weight)
		if err != nil {
			return err
		}
	}

	return nil
}

// TODO: don't broadcast when syncing.

// BroadcastItem broadcast the item id to its peers.
func (n *Networking) BroadcastItem(item ItemID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, p := range n.peers {
		p := p

		go func() {
			err := p.Inventory(n.myself, []ItemID{item})
			if err != nil {
				log.Error("send inventory error", "err", err)
				n.removePeer(p)
			}

		}()
	}
}

func (n *Networking) recvTxn(t []byte) {
	panic("not implemented")
}

func (n *Networking) recvSysTxn(t *SysTxn) {
	panic("not implemented")
}

func (n *Networking) recvRandBeaconSig(r *RandBeaconSig) {
	if !n.v.ValidateRandBeaconSig(r) {
		return
	}

	success := n.chain.RandomBeacon.AddRandBeaconSig(r)
	if !success {
		log.Warn("add random beacon sig failed", "round", r.Round, "beacon depth", n.chain.RandomBeacon.Depth())
		return
	}

	go n.BroadcastItem(ItemID{T: RandBeaconItem, Hash: r.Hash(), ItemRound: r.Round})
}

func (n *Networking) recvRandBeaconSigShare(r *RandBeaconSigShare) {
	groupID, valid := n.v.ValidateRandBeaconSigShare(r)

	if !valid {
		return
	}

	sig, success := n.chain.RandomBeacon.AddRandBeaconSigShare(r, groupID)
	if !success {
		return
	}

	if sig != nil {
		go n.recvRandBeaconSig(sig)
		return
	}

	go n.BroadcastItem(ItemID{T: RandBeaconShareItem, Hash: r.Hash(), ItemRound: r.Round})
}

func (n *Networking) recvBlock(b *Block) {
	weight, valid := n.v.ValidateBlock(b)

	if !valid {
		return
	}

	// TODO: make sure received all block's parents and block
	// proposal before processing this block.

	err := n.chain.addBlock(b, weight)
	if err == errChainDataAlreadyExists {
		return
	}

	if err != nil {
		log.Warn("add block failed", "err", err)
		return
	}

	go n.BroadcastItem(ItemID{T: BlockItem, Hash: b.Hash(), ItemRound: b.Round, Ref: b.PrevBlock})
}

func (n *Networking) recvBlockProposal(bp *BlockProposal) {
	weight, valid := n.v.ValidateBlockProposal(bp)
	if !valid {
		return
	}

	err := n.chain.addBP(bp, weight)
	if err != nil {
		log.Warn("add block proposal failed", "err", err)
		return
	}

	go n.BroadcastItem(ItemID{T: BlockProposalItem, Hash: bp.Hash(), ItemRound: bp.Round, Ref: bp.PrevBlock})
}

func (n *Networking) recvNtShare(s *NtShare) {
	groupID, valid := n.v.ValidateNtShare(s)
	if !valid {
		return
	}

	b, success := n.chain.addNtShare(s, groupID)
	if !success {
		return
	}

	if b != nil {
		go n.recvBlock(b)
		return
	}

	// TODO: use multicast rather than broadcast
	go n.BroadcastItem(ItemID{T: NtShareItem, Hash: s.Hash(), ItemRound: s.Round, Ref: s.BP})
}

// RemovePeer removes the peer.
func (n *Networking) RemovePeer(p Peer) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.removePeer(p)
}

func (n *Networking) removePeer(p Peer) {
	for i, v := range n.peers {
		if v == p {
			n.peers = append(n.peers[:i], n.peers[i+1:]...)
			return
		}
	}
}

// TODO: fix lint
// nolint: gocyclo
func (n *Networking) recvInventory(p Peer, ids []ItemID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	round := n.chain.Round()
	for _, id := range ids {
		switch id.T {
		case TxnItem:
			panic("not implemented")
		case SysTxnItem:
			panic("not implemented")
		case BlockItem:
			// TODO: improve logic of what to get, e.g., using id.Ref
			if _, ok := n.chain.Block(id.Hash); !ok {
				err := p.GetData(n.myself, []ItemID{id})
				if err != nil {
					log.Error("get data from peer error", "err", err)
					n.removePeer(p)
				}
			}
		case BlockProposalItem:
			if id.ItemRound != round {
				if id.ItemRound > round {
					log.Warn("received block proposal for a bigger round", "round", id.ItemRound, "expecting", round)
				}
				continue
			}

			if _, ok := n.chain.BlockProposal(id.Hash); ok {
				continue
			}

			err := p.GetData(n.myself, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		case NtShareItem:
			if id.ItemRound != round {
				if id.ItemRound > round {
					// TODO: when received from bigger round, sync the smaller round
					log.Warn("received notarization share for a bigger round", "round", id.ItemRound, "expecting", round)
				}
				continue
			}

			if _, ok := n.chain.NtShare(id.Hash); ok {
				continue
			}

			if !n.chain.NeedNotarize(id.Ref) {
				continue
			}

			err := p.GetData(n.myself, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		case RandBeaconShareItem:
			if id.ItemRound != round {
				if id.ItemRound > round {
					log.Warn("received random beacon share for bigger round", "round", id.ItemRound, "expecting", round)
				}
				continue
			}

			share := n.chain.RandomBeacon.GetShare(id.Hash)
			if share != nil {
				continue
			}
			err := p.GetData(n.myself, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		case RandBeaconItem:
			if id.ItemRound != round {
				if id.ItemRound > round {
					log.Warn("received random beacon share for a bigger round", "round", id.ItemRound, "expecting", round)
				}
				continue
			}

			err := p.GetData(n.myself, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		}
	}
}

func (n *Networking) getSyncData(start int) ([]*RandBeaconSig, []*Block) {
	n.mu.Lock()
	defer n.mu.Unlock()
	history := n.chain.RandomBeacon.History()
	if len(history) <= start {
		return nil, nil
	}

	blocks := n.chain.FinalizedChain()
	if len(blocks) <= start {
		blocks = nil
	} else {
		blocks = blocks[start:]
	}

	return history[start:], blocks
}

// TODO: fix lint
// nolint: gocyclo
func (n *Networking) serveData(p Peer, ids []ItemID) {
	for _, id := range ids {
		switch id.T {
		case TxnItem:
			panic("not implemented")
		case SysTxnItem:
			panic("not implemented")
		case BlockItem:
			b, ok := n.chain.Block(id.Hash)
			if !ok {
				continue
			}
			err := p.Block(b)
			if err != nil {
				log.Error("send block to peer error", "err", err)
				n.removePeer(p)
			}
		case BlockProposalItem:
			bp, ok := n.chain.BlockProposal(id.Hash)
			if !ok {
				continue
			}
			err := p.BlockProposal(bp)
			if err != nil {
				log.Error("send block proposal to peer error", "err", err)
				n.removePeer(p)
			}
		case NtShareItem:
			nts, ok := n.chain.NtShare(id.Hash)
			if !ok {
				continue
			}
			err := p.NotarizationShare(nts)
			if err != nil {
				log.Error("send notarization share to peer error", "err", err)
				n.removePeer(p)
			}
		case RandBeaconShareItem:
			share := n.chain.RandomBeacon.GetShare(id.Hash)
			if share == nil {
				continue
			}

			err := p.RandBeaconSigShare(share)
			if err != nil {
				log.Error("send random beacon sig share to peer error", "err", err)
				n.removePeer(p)
			}
		case RandBeaconItem:
			if rbr := n.chain.RandomBeacon.Depth(); id.ItemRound >= rbr {
				log.Warn("peer requested random beacon of too high depth, need to be smaller than random beacon round\n", "requested round", id.ItemRound, "random beacon round", rbr)
				continue
			}

			history := n.chain.RandomBeacon.History()
			err := p.RandBeaconSig(history[id.ItemRound])
			if err != nil {
				log.Error("send rand beacon sig to peer error", "err", err)
				n.removePeer(p)
			}
		}
	}
}

func (n *Networking) peerList() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	// TODO: periodically verify the addrs in peerAddrs are valid
	// by using Ping.
	return n.peerAddrs
}

func (n *Networking) updatePeers([]string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// TODO: validate, dedup the peer list
}

// receiver implements the Peer interface. It forwards the peers'
// queries to the networking component.
type receiver struct {
	addr string
	n    *Networking
}

func (r *receiver) Addr() string {
	return r.addr
}

func (r *receiver) Txn(t []byte) error {
	r.n.recvTxn(t)
	return nil
}

func (r *receiver) SysTxn(t *SysTxn) error {
	r.n.recvSysTxn(t)
	return nil
}

func (r *receiver) RandBeaconSigShare(s *RandBeaconSigShare) error {
	r.n.recvRandBeaconSigShare(s)
	return nil
}

func (r *receiver) RandBeaconSig(s *RandBeaconSig) error {
	r.n.recvRandBeaconSig(s)
	return nil
}

func (r *receiver) Block(b *Block) error {
	r.n.recvBlock(b)
	return nil
}

func (r *receiver) BlockProposal(bp *BlockProposal) error {
	r.n.recvBlockProposal(bp)
	return nil
}

func (r *receiver) NotarizationShare(n *NtShare) error {
	r.n.recvNtShare(n)
	return nil
}

func (r *receiver) Inventory(sender Peer, ids []ItemID) error {
	r.n.recvInventory(sender, ids)
	return nil
}

func (r *receiver) GetData(requester Peer, ids []ItemID) error {
	r.n.serveData(requester, ids)
	return nil
}

func (r *receiver) Sync(start int) ([]*RandBeaconSig, []*Block, error) {
	rb, bs := r.n.getSyncData(start)
	return rb, bs, nil
}

func (r *receiver) Peers() ([]string, error) {
	return r.n.peerList(), nil
}

func (r *receiver) UpdatePeers(peers []string) error {
	r.n.updatePeers(peers)
	return nil
}

func (r *receiver) Ping(ctx context.Context) error {
	return nil
}
