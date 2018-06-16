package consensus

import (
	"encoding/gob"
	"errors"
	"net"
	"sync"

	log "github.com/helinwang/log15"
)

type PacketType int

const (
	TxnPacket PacketType = iota
	RandBeaconSigSharePacket
	RandBeaconSigPacket
	BlockPacket
	BlockProposalPacket
	RequestItemPacket
	InventoryPacket
)

// init registers the types with gob, so that the gob will know how to
// encode and decode Packet.
func init() {
	var a []byte
	var b *RandBeaconSig
	var c *RandBeaconSigShare
	var d *Block
	var e *BlockProposal
	var f Item
	var g ItemRequest

	gob.Register(a)
	gob.Register(b)
	gob.Register(c)
	gob.Register(d)
	gob.Register(e)
	gob.Register(f)
	gob.Register(g)
}

type Packet interface{}

type PacketAndAddr struct {
	P Packet
	A UnicastAddr
}

// TODO: remove peer if send failed
// TODO: periodically ping peer
// TODO: handle two different connects: peer discovery, connect as a peer
// TODO: rename Networking
type UnicastAddr string

type NetAddr interface {
}

type Broadcast struct{}

type conn struct {
	addr UnicastAddr
	enc  *gob.Encoder
	dec  *gob.Decoder
	ch   chan<- PacketAndAddr
}

func newConn(c net.Conn) *conn {
	enc := gob.NewEncoder(c)
	dec := gob.NewDecoder(c)
	return &conn{enc: enc, dec: dec}
}

func (p *conn) Write(pac Packet) error {
	return p.enc.Encode(pac)
}

func (p *conn) Read() error {
	for {
		var pac Packet
		err := p.dec.Decode(&pac)
		if err != nil {
			return err
		}
		p.ch <- PacketAndAddr{P: pac, A: p.addr}
	}
}

type Network struct {
	ch chan PacketAndAddr

	mu    sync.Mutex
	conns map[UnicastAddr]*conn
}

func NewNetwork() *Network {
	return &Network{
		conns: make(map[UnicastAddr]*conn),
		ch:    make(chan PacketAndAddr, 300),
	}
}

func (n *Network) Start(addr UnicastAddr) error {
	return nil
}

func (n *Network) Connect(addr UnicastAddr) error {
	n.mu.Lock()
	if _, ok := n.conns[addr]; ok {
		n.mu.Unlock()
	}
	n.mu.Unlock()

	c, err := net.Dial("tcp", string(addr))
	if err != nil {
		return err
	}

	conn := newConn(c)
	n.mu.Lock()
	if _, ok := n.conns[addr]; !ok {
		n.conns[addr] = conn
		go func() {
			err := conn.Read()
			if err != nil {
				log.Warn("read peer conn error", "err", err)
			}

			n.mu.Lock()
			delete(n.conns, addr)
			n.mu.Unlock()
		}()
	} else {
		c.Close()
	}
	n.mu.Unlock()
	return nil
}

var ErrUnicastAddrNotFound = errors.New("unicast address not found")

func (n *Network) Send(addr NetAddr, p Packet) error {
	switch v := addr.(type) {
	case UnicastAddr:
		conn, ok := n.conns[v]
		if !ok {
			return ErrUnicastAddrNotFound
		}

		err := conn.Write(p)
		if err != nil {
			return err
		}
	default:
		panic(addr)
	}
	return nil
}

func (n *Network) Recv() (NetAddr, Packet) {
	p := <-n.ch
	return p.A, p.P
}
