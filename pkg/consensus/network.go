package consensus

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	log "github.com/helinwang/log15"
)

const (
	timeoutDur = 5 * time.Second
	intialConn = 8
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
	var h *connectRequest
	var i []UnicastAddr
	var j ack

	gob.Register(a)
	gob.Register(b)
	gob.Register(c)
	gob.Register(d)
	gob.Register(e)
	gob.Register(f)
	gob.Register(g)
	gob.Register(h)
	gob.Register(i)
	gob.Register(j)
}

type Packet struct {
	Data interface{}
}

// TODO: remove peer if send failed
// TODO: periodically ping peer
// TODO: handle two different connects: peer discovery, connect as a peer
// TODO: rename Networking
type UnicastAddr struct {
	Addr  string
	PKStr string
}

type NetAddr interface {
}

type Broadcast struct{}

type conn struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func newConn(c net.Conn) *conn {
	enc := gob.NewEncoder(c)
	dec := gob.NewDecoder(c)
	return &conn{
		enc:  enc,
		dec:  dec,
		conn: c,
	}
}

func (p *conn) Write(pac Packet) error {
	return p.enc.Encode(pac)
}

func (p *conn) Read() (pac Packet, err error) {
	err = p.dec.Decode(&pac)
	if err != nil {
		return
	}

	return
}

func (p *conn) Close() {
	err := p.conn.Close()
	if err != nil {
		log.Warn("error close connection", "err", err)
	}
}

type PacketAndAddr struct {
	P Packet
	A UnicastAddr
}

type Network struct {
	sk   SK
	port uint16
	ch   chan PacketAndAddr

	mu    sync.Mutex
	conns map[UnicastAddr]*conn
	// nodes with a public IP
	publicNodes []UnicastAddr
}

func NewNetwork(sk SK) *Network {
	return &Network{
		sk:    sk,
		ch:    make(chan PacketAndAddr, 100),
		conns: make(map[UnicastAddr]*conn),
	}
}

func (n *Network) acceptPeerOrDisconnect(c net.Conn) {
	conn := newConn(c)
	pac, err := conn.Read()
	if err != nil {
		log.Warn("err read from newly accepted conn", "err", err)
		return
	}

	var recv *connectRequest
	switch v := pac.Data.(type) {
	case *connectRequest:
		if !v.ValidateSig() {
			log.Warn("connect request signature validation failed")
			return
		}

		recv = v
	case ack:
		conn.Write(Packet{Data: ack{}})
		conn.Close()
		return
	default:
		log.Warn("first received packet should be a connect request or an ack")
		return
	}

	// check if the connecting node is a public node
	ip := strings.Split(c.RemoteAddr().String(), ":")[0]
	addr := fmt.Sprintf("%s:%d", ip, recv.Port)
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
	isPubNode := n.checkPubIP(ctx, addr)
	cancel()

	n.mu.Lock()
	if isPubNode {
		n.publicNodes = append(n.publicNodes, UnicastAddr{Addr: addr, PKStr: string(recv.PK)})
	}
	pubNodes := n.publicNodes
	n.mu.Unlock()

	conn.Write(Packet{Data: pubNodes})

	// send a connect reuqest just to tell the other node about my
	// public key.
	req := &connectRequest{}
	req.Sign(n.sk)
	conn.Write(Packet{Data: req})

	if recv.GetNodesOnly {
		conn.Close()
		return
	}
}

func (n *Network) Start(host string, port int) (UnicastAddr, error) {
	n.port = uint16(port)
	addr := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				log.Error("error accepting connection", "err", err)
				return
			}

			go n.acceptPeerOrDisconnect(c)
		}
	}()
	return UnicastAddr{Addr: addr, PKStr: string(n.sk.MustPK())}, nil
}

func (n *Network) ConnectSeed(addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
	pk, nodes, err := n.getAddrsFromSeed(ctx, addr)
	cancel()
	if err != nil {
		return err
	}

	myPKStr := string(n.sk.MustPK())
	if len(nodes) == 0 {
		nodes = []UnicastAddr{{PKStr: string(pk), Addr: addr}}
	} else if len(nodes) == 1 && nodes[0].PKStr == myPKStr {
		nodes = append(nodes, UnicastAddr{PKStr: string(pk), Addr: addr})
	}
	perm := rand.Perm(len(nodes))

	connected := 0
	for _, idx := range perm {
		addr := nodes[idx]
		if addr.PKStr == myPKStr {
			continue
		}

		go n.connect(addr)
		connected++
		if connected >= intialConn {
			break
		}
	}

	n.mu.Lock()
	n.publicNodes = nodes
	n.mu.Unlock()

	return nil
}

func (n *Network) checkPubIP(ctx context.Context, addr string) bool {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}

	conn := newConn(c)
	err = conn.Write(Packet{Data: ack{}})
	if err != nil {
		return false
	}

	ch := make(chan bool, 1)

	go func() {
		pac, err := conn.Read()
		if err != nil {
			ch <- false
			return
		}

		if _, ok := pac.Data.(ack); !ok {
			ch <- false
			return
		}

		ch <- true
	}()

	select {
	case <-ctx.Done():
		return false
	case r := <-ch:
		return r
	}
}

func (n *Network) getAddrsFromSeed(ctx context.Context, addr string) (PK, []UnicastAddr, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	conn := newConn(c)
	req := &connectRequest{GetNodesOnly: true, Port: n.port}
	req.Sign(n.sk)
	err = conn.Write(Packet{Data: req})
	if err != nil {
		return nil, nil, err
	}
	type result struct {
		addrs []UnicastAddr
		pk    PK
		err   error
	}

	ch := make(chan result, 1)

	go func() {
		pac, err := conn.Read()
		if err != nil {
			ch <- result{err: err}
			return
		}

		addrs, ok := pac.Data.([]UnicastAddr)
		if !ok {
			fmt.Printf("%T\n", pac.Data)
			ch <- result{err: errors.New("the first packet should be of type []UnicastAddr")}
			return
		}

		pac, err = conn.Read()
		if err != nil {
			ch <- result{err: err}
			return
		}

		req, ok := pac.Data.(*connectRequest)
		if !ok {
			ch <- result{err: errors.New("the second packet should be of type *connectRequest")}
			return
		}

		if !req.ValidateSig() {
			ch <- result{err: errors.New("peer signature validation failed")}
			return
		}

		ch <- result{addrs: addrs, pk: req.PK}
	}()

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case r := <-ch:
		return r.pk, r.addrs, r.err
	}
}

func (n *Network) connect(addr UnicastAddr) error {
	n.mu.Lock()
	if _, ok := n.conns[addr]; ok {
		n.mu.Unlock()
	}
	n.mu.Unlock()

	c, err := net.Dial("tcp", addr.Addr)
	if err != nil {
		return err
	}

	conn := newConn(c)
	req := &connectRequest{}
	req.Sign(n.sk)
	err = conn.Write(Packet{Data: req})
	if err != nil {
		return err
	}

	n.mu.Lock()
	if _, ok := n.conns[addr]; !ok {
		n.conns[addr] = conn
		go func() {
			for {
				pac, err := conn.Read()
				if err != nil {
					log.Warn("read peer conn error", "err", err)
					conn.Close()
					break
				}

				_ = pac
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

func (n *Network) Send(addr NetAddr, p Packet) error {
	switch v := addr.(type) {
	case UnicastAddr:
		n.mu.Lock()
		conn, ok := n.conns[v]
		n.mu.Unlock()
		if !ok {
			log.Warn("sending to unknown address", "addr", addr)
			return errors.New("can not find the send address")
		}

		err := conn.Write(p)
		if err != nil {
			log.Warn("send failed, removing this peer", "err", err)
			n.mu.Lock()
			if conn, ok = n.conns[v]; ok {
				delete(n.conns, v)
			}
			n.mu.Unlock()
			conn.Close()
			return err
		}
	case Broadcast:
		n.mu.Lock()
		for addr := range n.conns {
			go n.Send(addr, p)
		}
		n.mu.Lock()
	default:
		panic(addr)
	}
	return nil
}

func (n *Network) Recv() (NetAddr, Packet) {
	p := <-n.ch
	return p.A, p.P
}

type connectRequest struct {
	Port         uint16
	GetNodesOnly bool
	PK           PK
	Sig          Sign
}

// TODO: create similar functions for other types that needs
// signature.

func (c *connectRequest) ValidateSig() bool {
	return c.Sig.Verify(c.PK, c.msg())
}

func (c *connectRequest) msg() []byte {
	dup := *c
	dup.Sig = nil
	msg, err := rlp.EncodeToBytes(dup)
	if err != nil {
		panic(err)
	}
	return msg
}

func (c *connectRequest) Sign(sk SK) {
	c.PK = sk.MustPK()
	c.Sig = sk.Sign(c.msg())
}

type ack struct {
}
