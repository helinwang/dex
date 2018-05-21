package consensus

import "log"

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
	PeerList() ([]string, error)
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
	net       Network
	v         *validator
	roundInfo *RoundInfo
	chain     *Chain
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network, v *validator) *Networking {
	return &Networking{net: net, v: v}
}

// Start starts the networking component.
func (n *Networking) Start(addr string) {
	n.net.Start(&receiver{addr: addr, n: n})
}

func (n *Networking) recvTxn(t []byte) {
}

func (n *Networking) recvSysTxn(t *SysTxn) {
}

func (n *Networking) recvRandBeaconSig(r *RandBeaconSig) {
	if !n.v.ValidateRandBeaconSig(r) {
		log.Printf("ValidateRandBeaconSig failed, round: %d\n", r.Round)
		return
	}

	err := n.chain.addRandBeaconSig(r)
	if err != nil {
		log.Println(err)
		return
	}

	// TODO: broadcast
}

func (n *Networking) recvRandBeaconSigShare(r *RandBeaconSigShare) {
	if !n.v.ValidateRandBeaconSigShare(r) {
		log.Printf("ValidateRandBeaconSigShare failed, owner: %x, round: %d\n", r.Owner, r.Round)
		return
	}

	sig, err := n.chain.addRandBeaconSigShare(r)
	if err != nil {
		log.Println(err)
		return
	}

	if sig != nil {
		go n.recvRandBeaconSig(sig)
		return
	}

	// TODO: broadcast
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

	// TODO: broadcast
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

	// TODO: broadcast
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

	// TODO: broadcast nt share
}

func (n *Networking) recvInventory(sender Peer, ids []ItemID) {
}

func (n *Networking) serveData(requester Peer, ids []ItemID) {
}

func (n *Networking) peerList() ([]string, error) {
	return nil, nil
}

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

func (r *receiver) PeerList() ([]string, error) {
	return r.n.peerList()
}
