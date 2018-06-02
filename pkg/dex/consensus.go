package dex

import (
	"net"
	"net/http"
	"net/rpc"
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type Consensus struct {
	mu     sync.Mutex
	s      *State
	server *RPCServer
}

func NewConsensus() *Consensus {
	return &Consensus{
		server: &RPCServer{},
	}
}

func (c *Consensus) Update(s consensus.State) {
	state := s.(*State)
	c.server.updateConsensus(&state.state)

	c.mu.Lock()
	c.s = state
	c.mu.Unlock()
}

func (c *Consensus) StartServer(addr string) error {
	err := rpc.Register(c.server)
	if err != nil {
		return err
	}

	rpc.HandleHTTP()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		err = http.Serve(l, nil)
		if err != nil {
			log.Error("error serving RPC server", "err", err)
		}
	}()
	return nil
}
