package consensus

// Peer is a peer node in the DEX network.
type Peer interface {
	SendBlock(b *Block) error
	SendBlockProposal(b *BlockProposal) error
	SendNotarizationShare(bp Hash, sig []byte) error

	GetBlock(block Hash) (*Block, error)
	GetBlockProposal(bp Hash) (*BlockProposal, error)
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
}

// NewNetworking creates a new networking component.
func NewNetworking(net Network) *Networking {
	return &Networking{net: net}
}

// Start starts the networking component.
func (n *Networking) Start(addr string) {
	n.net.Start(addr, &receiver{n: n})
}

func (n *Networking) recvBlock(b *Block) {
}

func (n *Networking) recvBlockProposal(b *BlockProposal) {
}

func (n *Networking) recvNtShare(bp Hash, sig []byte) {
}

func (n *Networking) getBlock(block Hash) (*Block, error) {
	return nil, nil
}

func (n *Networking) getBlockProposal(bp Hash) (*BlockProposal, error) {
	return nil, nil
}

type receiver struct {
	n *Networking
}

func (r *receiver) SendBlock(b *Block) error {
	r.n.recvBlock(b)
	return nil
}

func (r *receiver) SendBlockProposal(bp *BlockProposal) error {
	r.n.recvBlockProposal(bp)
	return nil
}

func (r *receiver) SendNotarizationShare(bp Hash, sig []byte) error {
	r.n.recvNtShare(bp, sig)
	return nil
}

func (r *receiver) GetBlock(b Hash) (*Block, error) {
	return r.n.getBlock(b)
}

func (r *receiver) GetBlockProposal(bp Hash) (*BlockProposal, error) {
	return r.n.getBlockProposal(bp)
}
