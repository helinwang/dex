package consensus

import (
	"context"
	"log"
	"sync"
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
	PeerList() ([]string, error)
	Addr() string
	Ping(ctx context.Context) error
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
	SenderRound int
	T           ItemType
	Hash        Hash
}

// Network is used to connect to the peers.
type Network interface {
	Start(myself Peer) error
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
	peerAddrs []string
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network, v *validator, addr string) *Networking {
	return &Networking{
		net:   net,
		v:     v,
		peers: make(map[string]Peer),
	}
}

// Start starts the networking component.
func (n *Networking) Start(seedAddr string) error {
	err := n.net.Start(&receiver{addr: n.addr, n: n})
	if err != nil {
		return err
	}

	p, err := n.net.Connect(seedAddr)
	if err != nil {
		return err
	}

	peerAddrs, err := p.PeerList()
	if err != nil {
		return err
	}

	n.mu.Lock()
	n.peers[seedAddr] = p
	n.peerAddrs = peerAddrs
	n.mu.Unlock()

	// TODO: connect to more peers

	return nil
}

// BroadcastItem broadcast the item id to its peers.
func (n *Networking) BroadcastItem(item ItemID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, p := range n.peers {
		p := p
		go func() {
			p.Inventory(n.addr, []ItemID{item})
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
		log.Printf("ValidateRandBeaconSig failed, round: %d\n", r.Round)
		return
	}

	err := n.chain.RandomBeacon.RecvRandBeaconSig(r)
	if err != nil {
		log.Println(err)
		return
	}

	go n.BroadcastItem(ItemID{T: RandBeaconItem, Hash: r.Hash()})
}

func (n *Networking) recvRandBeaconSigShare(r *RandBeaconSigShare) {
	groupID, valid := n.v.ValidateRandBeaconSigShare(r)

	if !valid {
		log.Printf("ValidateRandBeaconSigShare failed, owner: %x, round: %d\n", r.Owner, r.Round)
		return
	}

	sig, err := n.chain.RandomBeacon.RecvRandBeaconSigShare(r, groupID)
	if err != nil {
		log.Println(err)
		return
	}

	if sig != nil {
		go n.recvRandBeaconSig(sig)
		return
	}

	go n.BroadcastItem(ItemID{T: RandBeaconShareItem, Hash: r.Hash()})
}

func (n *Networking) recvBlock(b *Block) {
	weight, valid := n.v.ValidateBlock(b)

	if !valid {
		log.Println("ValidateBlock failed")
		return
	}

	// TODO: make sure received all block's parents and block
	// proposal before processing this block.

	err := n.chain.addBlock(b, weight)
	if err != nil {
		log.Println(err)
		return
	}

	go n.BroadcastItem(ItemID{T: BlockItem, Hash: b.Hash()})
}

func (n *Networking) recvBlockProposal(bp *BlockProposal) {
	weight, valid := n.v.ValidateBlockProposal(bp)
	if !valid {
		log.Println("ValidateBlockProposal failed")
		return
	}

	err := n.chain.addBP(bp, weight)
	if err != nil {
		log.Println(err)
		return
	}

	go n.BroadcastItem(ItemID{T: BlockProposalItem, Hash: bp.Hash()})
}

func (n *Networking) recvNtShare(s *NtShare) {
	groupID, valid := n.v.ValidateNtShare(s)
	if !valid {
		log.Println("ValidateNtShare failed")
		return
	}

	b, err := n.chain.addNtShare(s, groupID)
	if err != nil {
		log.Println(err)
		return
	}

	if b != nil {
		go n.recvBlock(b)
		return
	}

	go n.BroadcastItem(ItemID{T: NtShareItem, Hash: s.Hash()})
}

func (n *Networking) recvInventory(sender string, ids []ItemID) {
	for _, id := range ids {
		switch id.T {
		case TxnItem:
			panic("not implemented")
		case SysTxnItem:
			panic("not implemented")
		case BlockItem:
		case BlockProposalItem:
		case NtShareItem:
		case RandBeaconShareItem:
		case RandBeaconItem:
		}
	}
}

func (n *Networking) serveData(requester string, ids []ItemID) {
}

func (n *Networking) peerList() []string {
	n.mu.Lock()
	defer n.mu.Unlock()

	// TODO: periodically verify the addrs in peerAddrs are valid
	// by using Ping.
	return n.peerAddrs
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

func (r *receiver) PeerList() ([]string, error) {
	return r.n.peerList(), nil
}

func (r *receiver) Ping(context.Context) error {
	return nil
}
