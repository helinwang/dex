package consensus

import (
	"context"
	"errors"
	"sync"

	log "github.com/helinwang/log15"
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
	Inventory(sender string, items []ItemID) error
	GetData(requester string, items []ItemID) error
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
	Start(addr string, myself Peer) error
	Connect(addr string) (Peer, error)
}

// Networking is the component that enables the node to talk to its
// peers over the network.
type Networking struct {
	net   Network
	addr  string
	v     *validator
	chain *Chain

	mu        sync.Mutex
	peers     map[string]Peer
	peerAddrs map[string]bool
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network, addr string, chain *Chain) *Networking {
	return &Networking{
		addr:      addr,
		net:       net,
		v:         newValidator(chain),
		peers:     make(map[string]Peer),
		peerAddrs: make(map[string]bool),
		chain:     chain,
	}
}

// Start starts the networking component.
// TODO: fix lint
// nolint: gocyclo
func (n *Networking) Start(seedAddr string) error {
	err := n.net.Start(n.addr, &receiver{addr: n.addr, n: n})
	if err != nil {
		return err
	}

	p, err := n.net.Connect(seedAddr)
	if err != nil {
		return err
	}

	peerAddrs, err := p.Peers()
	if err != nil {
		return err
	}

	n.mu.Lock()
	n.peers[seedAddr] = p
	for _, addr := range peerAddrs {
		// TODO: check peers is online
		n.peerAddrs[addr] = true
	}

	// TODO: limit the number of peers connected to
	for addr := range n.peerAddrs {
		_, err = n.findOrConnect(addr)
		if err != nil {
			log.Error("connect to peer", "err", err, "addr", addr)
		}
	}
	n.mu.Unlock()

	// TODO: sync random beacon from other peers rather than the
	// seed

	rb, bs, err := p.Sync(len(n.chain.RandomBeacon.History()))
	if err != nil {
		return err
	}

	for _, r := range rb {
		err = n.chain.RandomBeacon.AddRandBeaconSig(r)
		if err != nil {
			return err
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
			err := p.Inventory(n.addr, []ItemID{item})
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
		log.Debug("ValidateRandBeaconSig failed", "round", r.Round)
		return
	}

	err := n.chain.RandomBeacon.AddRandBeaconSig(r)
	if err != nil {
		log.Warn("add random beacon sig failed", "err", err)
		return
	}

	go n.BroadcastItem(ItemID{T: RandBeaconItem, Hash: r.Hash(), ItemRound: r.Round})
}

func (n *Networking) recvRandBeaconSigShare(r *RandBeaconSigShare) {
	groupID, valid := n.v.ValidateRandBeaconSigShare(r)

	if !valid {
		log.Debug("ValidateRandBeaconSigShare failed", "owner", r.Owner, "round", r.Round)
		return
	}

	sig, err := n.chain.RandomBeacon.AddRandBeaconSigShare(r, groupID)
	if err != nil {
		log.Error("add rand beacon sig share failed", "err", err)
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
		log.Warn("ValidateBlock failed")
		return
	}

	// TODO: make sure received all block's parents and block
	// proposal before processing this block.

	err := n.chain.addBlock(b, weight)
	if err == errChainDataAlreadyExists {
		return
	}

	if err != nil {
		log.Error("add block failed", "err", err)
		return
	}

	go n.BroadcastItem(ItemID{T: BlockItem, Hash: b.Hash(), ItemRound: b.Round, Ref: b.PrevBlock})
}

func (n *Networking) recvBlockProposal(bp *BlockProposal) {
	weight, valid := n.v.ValidateBlockProposal(bp)
	if !valid {
		log.Warn("ValidateBlockProposal failed")
		return
	}

	err := n.chain.addBP(bp, weight)
	if err != nil {
		log.Error("add block proposal failed", "err", err)
		return
	}

	go n.BroadcastItem(ItemID{T: BlockProposalItem, Hash: bp.Hash(), ItemRound: bp.Round, Ref: bp.PrevBlock})
}

func (n *Networking) recvNtShare(s *NtShare) {
	groupID, valid := n.v.ValidateNtShare(s)
	if !valid {
		log.Warn("ValidateNtShare failed")
		return
	}

	b, err := n.chain.addNtShare(s, groupID)
	if err != nil {
		log.Error("add notarization share failed", "err", err)
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
	for k, v := range n.peers {
		if v == p {
			delete(n.peers, k)
			return
		}
	}
}

// must be called with mutex held.
func (n *Networking) findOrConnect(addr string) (Peer, error) {
	if p, ok := n.peers[addr]; ok {
		return p, nil
	}

	p, err := n.net.Connect(addr)
	if err != nil {
		return nil, err
	}

	n.peers[addr] = p
	return p, nil
}

// TODO: fix lint
// nolint: gocyclo
func (n *Networking) recvInventory(sender string, ids []ItemID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	p, err := n.findOrConnect(sender)
	if err != nil {
		log.Error("connect to inventory sender failed", "err", err)
		return
	}

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
				err := p.GetData(n.addr, []ItemID{id})
				if err != nil {
					log.Error("get data from peer error", "err", err)
					n.removePeer(p)
				}
			}
		case BlockProposalItem:
			if id.ItemRound != round {
				log.Debug("received block proposal for a different round", "round", id.ItemRound, "expecting", round)
				continue
			}

			if _, ok := n.chain.BlockProposal(id.Hash); ok {
				continue
			}

			err := p.GetData(n.addr, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		case NtShareItem:
			if id.ItemRound != round {
				log.Debug("received notarization share for a different round", "round", id.ItemRound, "expecting", round)
				continue
			}

			if _, ok := n.chain.NtShare(id.Hash); ok {
				continue
			}

			if !n.chain.NeedNotarize(id.Ref) {
				continue
			}

			err := p.GetData(n.addr, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		case RandBeaconShareItem:
			if id.ItemRound != round {
				log.Debug("received random beacon share for a different round", "round", id.ItemRound, "expecting", round)
				continue
			}

			share := n.chain.RandomBeacon.GetShare(id.Hash)
			if share != nil {
				continue
			}
			err := p.GetData(n.addr, []ItemID{id})
			if err != nil {
				log.Error("get data from peer error", "err", err)
				n.removePeer(p)
			}
		case RandBeaconItem:
			if id.ItemRound != round {
				log.Debug("received random beacon share for a different round", "round", id.ItemRound, "expecting", round)
				continue
			}

			err := p.GetData(n.addr, []ItemID{id})
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
func (n *Networking) serveData(requester string, ids []ItemID) {
	p, err := n.findOrConnect(requester)
	if err != nil {
		log.Error("can not connect to the peer that asked for data", "err", err)
		return
	}

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
				log.Warn("peer requested random beacon of too high round, need to be smaller than random beacon round\n", "peer", requester, "requested round", id.ItemRound, "random beacon round", rbr)
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

	list := make([]string, 0, len(n.peerAddrs))
	for addr := range n.peerAddrs {
		list = append(list, addr)
	}

	// TODO: periodically verify the addrs in peerAddrs are valid
	// by using Ping.
	return list
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

func (r *receiver) Inventory(sender string, ids []ItemID) error {
	r.n.recvInventory(sender, ids)
	return nil
}

func (r *receiver) GetData(requester string, ids []ItemID) error {
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
