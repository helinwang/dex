package consensus

import "log"

// Peer is a peer node in the DEX network.
type Peer interface {
	Txn(sender Peer, txn []byte) error
	SysTxn(sender Peer, s *SysTxn) error
	Block(sender Peer, b *Block) error
	BlockProposal(sender Peer, b *BlockProposal) error
	NotarizationShare(sender Peer, n *NtShare) error
	Inventory(sender Peer, items []ItemID) error
	GetData(requester Peer, items []ItemID) error
	PeerList(requester Peer) ([]string, error)
	Addr() string
}

// ItemType is the different type of items.
type ItemType int

// different types of items
const (
	TxnItem ItemType = iota
	SysTxnItem
	BlockItem
	BlockProposalItem
	NtShareItem
)

// ItemID is the identification of an item that the current node owns.
type ItemID struct {
	T    ItemType
	Hash Hash
}

// Network is used to connect to the peers.
type Network interface {
	Start(myself Peer) error
	Connect(addr string) (Peer, error)
}

// Networking is the component that enables the node to talk to its
// peers over the network.
type Networking struct {
	mySelf    Peer
	net       Network
	v         *validator
	roundInfo *RoundInfo
	chain     *Chain
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network, addr string, v *validator) *Networking {
	r := &receiver{addr: addr}
	n := &Networking{net: net, v: v, mySelf: r}
	r.n = n
	return n
}

// Start starts the networking component.
func (n *Networking) Start() {
	n.net.Start(n.mySelf)
}

func (n *Networking) recvTxn(sender Peer, t []byte) {
}

func (n *Networking) recvSysTxn(sender Peer, t *SysTxn) {
}

func (n *Networking) recvBlock(sender Peer, b *Block) {
	if !n.v.ValidateBlock(b) {
		return
	}

	err := n.chain.addBlock(b)
	if err != nil {
		log.Println(err)
		return
	}

	// TODO: broadcast
}

func (n *Networking) recvBlockProposal(sender Peer, bp *BlockProposal) {
	weight, valid := n.v.ValidateBlockProposal(bp)
	if !valid {
		return
	}

	err := n.chain.addBP(bp, weight)
	if err != nil {
		log.Println(err)
		return
	}

	// TODO: broadcast
}

func (n *Networking) recvNtShare(sender Peer, s *NtShare) {
	groupID, valid := n.v.ValidateNtShare(s)
	if !valid {
		return
	}

	b, err := n.chain.addNtShare(s, groupID)
	if err != nil {
		log.Println(err)
		return
	}

	if b != nil {
		go n.recvBlock(n.mySelf, b)
		return
	}

	// TODO: broadcast nt share
}

func (n *Networking) recvInventory(sender Peer, ids []ItemID) {
}

func (n *Networking) serveData(requester Peer, ids []ItemID) {
}

func (n *Networking) peerList(requester Peer) ([]string, error) {
	return nil, nil
}

type receiver struct {
	addr string
	n    *Networking
}

func (r *receiver) Addr() string {
	return r.addr
}

func (r *receiver) Txn(sender Peer, t []byte) error {
	r.n.recvTxn(sender, t)
	return nil
}

func (r *receiver) SysTxn(sender Peer, t *SysTxn) error {
	r.n.recvSysTxn(sender, t)
	return nil
}

func (r *receiver) Block(sender Peer, b *Block) error {
	r.n.recvBlock(sender, b)
	return nil
}

func (r *receiver) BlockProposal(sender Peer, bp *BlockProposal) error {
	r.n.recvBlockProposal(sender, bp)
	return nil
}

func (r *receiver) NotarizationShare(sender Peer, n *NtShare) error {
	r.n.recvNtShare(sender, n)
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

func (r *receiver) PeerList(requester Peer) ([]string, error) {
	return r.n.peerList(requester)
}
