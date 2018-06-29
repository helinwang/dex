package consensus

import (
	"context"
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

// TODO: periodically sync with peer about the public nodes it knows
// TODO: periodically ping peer
// TODO: rename Networking
type unicastAddr struct {
	Addr  string
	PKStr string
}

type netAddr interface {
}

type broadcast struct{}

type shardBroadcast struct{}

type packetAndAddr struct {
	P packet
	A unicastAddr
}

type network struct {
	sk         SK
	port       uint16
	ch         chan packetAndAddr
	shardIdx   uint16
	shardCount int

	mu     sync.Mutex
	conns  map[unicastAddr]*conn
	shards []map[unicastAddr]*conn
	// nodes with a public IP
	publicNodes []unicastAddr
}

func newNetwork(sk SK, shardIdx uint16, shardCount int) *network {
	shards := make([]map[unicastAddr]*conn, shardCount)
	for i := range shards {
		shards[i] = make(map[unicastAddr]*conn)
	}

	return &network{
		sk:         sk,
		shardIdx:   shardIdx,
		shardCount: shardCount,
		ch:         make(chan packetAndAddr, 100),
		conns:      make(map[unicastAddr]*conn),
		shards:     shards,
	}
}

func (n *network) acceptPeerOrDisconnect(c net.Conn) {
	conn := newConn(c)
	pac, err := conn.Read()
	if err != nil {
		log.Warn("err read from newly accepted conn", "err", err)
		return
	}

	var recv *connectRequest
	switch v := pac.Data.(type) {
	case *connectRequest:
		if !v.Sig.Verify(v.PK, v.ByteToSign()) {
			log.Warn("connect request signature validation failed")
			return
		}

		recv = v
	case ack:
		conn.Write(packet{Data: ack{}})
		conn.Close()
		return
	default:
		log.Warn("first received packet should be a connect request or an ack")
		return
	}

	ip := strings.Split(c.RemoteAddr().String(), ":")[0]
	addrStr := fmt.Sprintf("%s:%d", ip, recv.Port)
	addr := unicastAddr{Addr: addrStr, PKStr: string(recv.PK)}
	go func() {
		// check if the connecting node is a public node
		ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
		isPubAddr := n.isPubAddr(ctx, addrStr)
		cancel()
		if isPubAddr {
			n.mu.Lock()
			n.publicNodes = append(n.publicNodes, addr)
			n.mu.Unlock()
		}
	}()

	n.mu.Lock()
	pubNodes := n.publicNodes
	n.mu.Unlock()

	conn.Write(packet{Data: pubNodes})

	// send a connect reuqest just to tell the other node about my
	// public key.
	req := &connectRequest{}
	req.PK = n.sk.MustPK()
	req.Sig = n.sk.Sign(req.ByteToSign())
	conn.Write(packet{Data: req})

	if recv.GetNodesOnly {
		conn.Close()
		return
	}

	peerShard := recv.PK.Shard(n.shardCount)
	n.mu.Lock()
	n.conns[addr] = conn
	n.shards[peerShard][addr] = conn
	go n.readConn(addr, conn, peerShard)
	n.mu.Unlock()
}

func (n *network) Start(host string, port int) (unicastAddr, error) {
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
	return unicastAddr{Addr: addr, PKStr: string(n.sk.MustPK())}, nil
}

func dedup(nodes []unicastAddr) []unicastAddr {
	m := make(map[string]bool)
	r := make([]unicastAddr, 0, len(nodes))
	for _, n := range nodes {
		if m[n.PKStr] {
			continue
		}

		m[n.PKStr] = true
		r = append(r, n)
	}
	return r
}

func (n *network) ConnectSeed(addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
	pk, nodes, err := n.getAddrsFromSeed(ctx, addr)
	cancel()
	if err != nil {
		return err
	}

	myPKStr := string(n.sk.MustPK())
	if len(nodes) == 0 {
		nodes = []unicastAddr{{PKStr: string(pk), Addr: addr}}
	} else if len(nodes) == 1 && nodes[0].PKStr == myPKStr {
		nodes = append(nodes, unicastAddr{PKStr: string(pk), Addr: addr})
	}
	nodes = dedup(nodes)
	perm := rand.Perm(len(nodes))

	log.Info("received nodes", "count", len(nodes))

	connected := 0
	for _, idx := range perm {
		addr := nodes[idx]
		if addr.PKStr == myPKStr {
			continue
		}

		go n.connect(addr, PK([]byte(addr.PKStr)))
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

func (n *network) isPubAddr(ctx context.Context, addr string) bool {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}

	conn := newConn(c)
	err = conn.Write(packet{Data: ack{}})
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

func (n *network) getAddrsFromSeed(ctx context.Context, addr string) (PK, []unicastAddr, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	conn := newConn(c)
	req := &connectRequest{GetNodesOnly: true, Port: n.port}
	req.PK = n.sk.MustPK()
	req.Sig = n.sk.Sign(req.ByteToSign())
	err = conn.Write(packet{Data: req})
	if err != nil {
		return nil, nil, err
	}
	type result struct {
		addrs []unicastAddr
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

		addrs, ok := pac.Data.([]unicastAddr)
		if !ok {
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

		if !req.Sig.Verify(req.PK, req.ByteToSign()) {
			ch <- result{err: errors.New("peer signature validation failed")}
			return
		}

		ch <- result{addrs: addrs, pk: req.PK}
	}()

	select {
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("get public node addresses err: %v", ctx.Err())
	case r := <-ch:
		return r.pk, r.addrs, r.err
	}
}

func (n *network) connect(addr unicastAddr, pk PK) error {
	log.Info("connecting to peer", "addr", addr.Addr)
	peerShard := pk.Shard(n.shardCount)

	n.mu.Lock()
	if _, ok := n.conns[addr]; ok {
		n.mu.Unlock()
		return nil
	}
	n.mu.Unlock()

	c, err := net.Dial("tcp", addr.Addr)
	if err != nil {
		return err
	}

	conn := newConn(c)
	req := &connectRequest{Port: n.port}
	req.PK = n.sk.MustPK()
	req.Sig = n.sk.Sign(req.ByteToSign())
	err = conn.Write(packet{Data: req})
	if err != nil {
		return err
	}

	n.mu.Lock()
	if _, ok := n.conns[addr]; !ok {
		n.conns[addr] = conn
		n.shards[peerShard][addr] = conn
		go n.readConn(addr, conn, peerShard)
	} else {
		c.Close()
	}
	n.mu.Unlock()
	return nil
}

func (n *network) readConn(addr unicastAddr, conn *conn, peerShard uint16) {
	for {
		pac, err := conn.Read()
		if err != nil {
			log.Warn("read peer conn error", "err", err)
			conn.Close()
			break
		}

		switch v := pac.Data.(type) {
		case []unicastAddr:
			// TODO: update public node list
			_ = v
		case *connectRequest:
			// connection already established, discard
		default:
			n.ch <- packetAndAddr{A: addr, P: pac}
		}
	}

	n.mu.Lock()
	delete(n.shards[peerShard], addr)
	delete(n.conns, addr)
	n.mu.Unlock()
}

func (n *network) Send(addr netAddr, p packet) error {
	switch v := addr.(type) {
	case unicastAddr:
		n.mu.Lock()
		conn, ok := n.conns[v]
		n.mu.Unlock()
		if !ok {
			log.Warn("sending to unknown address", "addr", v.Addr)
			return errors.New("can not find the send address")
		}

		err := conn.Write(p)
		if err != nil {
			log.Warn("send failed, removing this peer", "err", err)
			n.mu.Lock()
			if conn, ok = n.conns[v]; ok {
				delete(n.conns, v)

				for _, shard := range n.shards {
					if _, ok := shard[v]; ok {
						delete(shard, v)
						break
					}
				}
				conn.Close()
			}
			n.mu.Unlock()
			return err
		}
	case broadcast:
		n.mu.Lock()
		for addr := range n.conns {
			go n.Send(addr, p)
		}
		n.mu.Unlock()
	case shardBroadcast:
		n.mu.Lock()
		for addr := range n.shards[n.shardIdx] {
			go n.Send(addr, p)
		}
		n.mu.Unlock()
	default:
		panic(addr)
	}
	return nil
}

func (n *network) Recv() (unicastAddr, packet) {
	p := <-n.ch
	return p.A, p.P
}

type connectRequest struct {
	Port         uint16
	GetNodesOnly bool
	PK           PK
	Sig          Sig
}

// TODO: create similar functions for other types that needs
// signature.

func (c *connectRequest) ByteToSign() []byte {
	dup := *c
	dup.Sig = nil
	msg, err := rlp.EncodeToBytes(dup)
	if err != nil {
		panic(err)
	}
	return msg
}

type ack struct {
}
