package network

import (
	"net"

	"github.com/helinwang/dex/pkg/consensus"
)

// Network is a consensus.Network implementation.
type Network struct {
}

func (n *Network) Start(addr string, onPeerConnect func(p consensus.Peer), myself consensus.Peer) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				panic(err)
			}

			go func() {
				p := NewPeer(conn, myself)
				onPeerConnect(p)
			}()
		}
	}()

	return nil
}

func (n *Network) Connect(addr string, myself consensus.Peer) (consensus.Peer, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return NewPeer(conn, myself), nil
}
