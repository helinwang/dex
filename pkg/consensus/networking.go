package consensus

// Peer is a peer node in the DEX network.
type Peer interface {
	Txn(sender Peer, txn []byte) error
	SysTxn(sender Peer, s *SysTxn) error
	Block(sender Peer, b *Block) error
	BlockProposal(sender Peer, b *BlockProposal) error
	NotarizationShare(sender Peer, bp Hash, sig []byte) error
	Inventory(sender Peer, items []ItemID) error
	GetData(requester Peer, items []ItemID) error
	PeerList(requester Peer) ([]string, error)
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
	Start(addr string, myself Peer) error
	Connect(addr string) (Peer, error)
}

// Networking is the component that enables the node to talk to its
// peers over the network.
type Networking struct {
	net Network
	v   *validator
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network, v *validator) *Networking {
	return &Networking{net: net, v: v}
}

// Start starts the networking component.
func (n *Networking) Start(addr string) {
	n.net.Start(addr, &receiver{n: n})
}

func (n *Networking) recvTxn(sender Peer, t []byte) {
}

func (n *Networking) recvSysTxn(sender Peer, t *SysTxn) {
}

func (n *Networking) recvBlock(sender Peer, b *Block) {
	n.v.ValidateBlock(b)
}

func (n *Networking) recvBlockProposal(sender Peer, b *BlockProposal) {
}

func (n *Networking) recvNtShare(sender Peer, bp Hash, sig []byte) {
}

func (n *Networking) recvInventory(sender Peer, ids []ItemID) {
}

func (n *Networking) serveData(requester Peer, ids []ItemID) {
	return
}

func (n *Networking) peerList(requester Peer) ([]string, error) {
	return nil, nil
}

type receiver struct {
	n *Networking
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

func (r *receiver) NotarizationShare(sender Peer, bp Hash, sig []byte) error {
	r.n.recvNtShare(sender, bp, sig)
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
