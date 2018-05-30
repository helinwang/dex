package network

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"net"
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type packetType int

const (
	txnArg packetType = iota
	sysTxnArg
	randBeaconSigShareArg
	randBeaconSigArg
	blockArg
	blockProposalArg
	ntShareArg
	inventoryArg
	getDataArg
	peersArg
	peersRet
	updatePeersArg
	pingArg
	pingRet
	syncArg
	syncRet
)

type packet struct {
	T    packetType
	Data []byte
}

// Peer implements a rudimentary RPC mechanism for the methods of
// consensus.Peer.
//
// Peer will forwards all incoming RPC calls to Peer.myself. It
// assumes the methods of Peer.myself will not return error, other
// return values will be relayed back to the TCP
// connection. Concurrent calls of the same method are not supported
// (if called concurrently, the return values may be
// out-of-order). This simplification is intentional since methods
// with non-error type return values are only `Peers`, `Ping`, `Sync`
// and there is no need for calling them concurrently.
type Peer struct {
	myself     consensus.Peer
	conn       net.Conn
	enc        *gob.Encoder
	syncRetCh  chan syncRetData
	pingRetCh  chan struct{}
	peersRetCh chan []string

	mu  sync.Mutex
	err error
}

// NewPeer creates a peer.
func NewPeer(conn net.Conn, myself consensus.Peer) *Peer {
	syncRetCh := make(chan syncRetData, 10)
	pingRetCh := make(chan struct{}, 10)
	peersRetCh := make(chan []string, 10)

	p := &Peer{
		enc:        gob.NewEncoder(conn),
		conn:       conn,
		myself:     myself,
		syncRetCh:  syncRetCh,
		pingRetCh:  pingRetCh,
		peersRetCh: peersRetCh,
	}

	go p.read()
	return p
}

type syncRetData struct {
	A []*consensus.RandBeaconSig
	B []*consensus.Block
}

func (p *Peer) onErr(err error) {
	log.Info("Peer error, closing connection", "err", err)
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()

	err = p.conn.Close()
	if err != nil {
		log.Error("close TCP conn error", "err", err)
	}
}

// nolint: gocyclo
func (p *Peer) read() {
	dec := gob.NewDecoder(p.conn)
	for {
		var pac packet
		err := dec.Decode(&pac)
		if err != nil {
			p.onErr(err)
			return
		}

		dataDec := gob.NewDecoder(bytes.NewReader(pac.Data))
		switch pac.T {
		case txnArg:
			var d []byte
			err := dataDec.Decode(&d)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.Txn(d)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case sysTxnArg:
			var s *consensus.SysTxn
			err := dataDec.Decode(&s)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.SysTxn(s)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case randBeaconSigShareArg:
			var r *consensus.RandBeaconSigShare
			err := dataDec.Decode(&r)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.RandBeaconSigShare(r)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case randBeaconSigArg:
			var r *consensus.RandBeaconSig
			err := dataDec.Decode(&r)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.RandBeaconSig(r)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case blockArg:
			var b *consensus.Block
			err := dataDec.Decode(&b)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.Block(b)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case blockProposalArg:
			var b *consensus.BlockProposal
			err := dataDec.Decode(&b)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.BlockProposal(b)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case ntShareArg:
			var n *consensus.NtShare
			err := dataDec.Decode(&n)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.NotarizationShare(n)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case inventoryArg:
			var items []consensus.ItemID
			err = dataDec.Decode(&items)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.Inventory(p, items)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case getDataArg:
			var items []consensus.ItemID
			err = dataDec.Decode(&items)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.GetData(p, items)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case peersArg:
			peers, err := p.myself.Peers()
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
			d, err := gobEncode(peers)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.write(packet{T: peersRet, Data: d})
			if err != nil {
				p.onErr(err)
				return
			}
		case updatePeersArg:
			var peers []string
			err := dataDec.Decode(&peers)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.myself.UpdatePeers(peers)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
		case pingArg:
			err = p.myself.Ping(context.Background())
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}
			err = p.write(packet{T: pingRet})
			if err != nil {
				log.Error("write ping resp error", "err", err)
				p.onErr(err)
			}
		case syncArg:
			var start int
			err := dataDec.Decode(&start)
			if err != nil {
				p.onErr(err)
				return
			}

			a, b, err := p.myself.Sync(start)
			if err != nil {
				log.Error("Peer methods are not supposed to return error")
				continue
			}

			d, err := gobEncode(a, b)
			if err != nil {
				p.onErr(err)
				return
			}

			err = p.write(packet{T: syncRet, Data: d})
			if err != nil {
				p.onErr(err)
				return
			}

		case pingRet:
			p.pingRetCh <- struct{}{}
		case peersRet:
			var r []string
			err := dataDec.Decode(&r)
			if err != nil {
				p.onErr(err)
				return
			}
			p.peersRetCh <- r
		case syncRet:
			var a []*consensus.RandBeaconSig
			var b []*consensus.Block
			err = dataDec.Decode(&a)
			if err != nil {
				p.onErr(err)
				return
			}

			err = dataDec.Decode(&b)
			if err != nil {
				p.onErr(err)
				return
			}
			p.syncRetCh <- syncRetData{A: a, B: b}
		default:
			p.onErr(fmt.Errorf("unrecognized package type: %d", pac.T))
			return
		}
	}
}

func gobEncode(vs ...interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	for _, v := range vs {
		err := enc.Encode(v)
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (p *Peer) write(v interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// must use the same encoder instance, rather than create a
	// new encoder each time. Otherwise decode would get eror
	// "extra data in buffer".
	err := p.enc.Encode(v)
	if err != nil {
		return err
	}

	return nil
}

func (p *Peer) Peers() ([]string, error) {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.mu.Unlock()

	var pac packet
	pac.T = peersArg
	err := p.write(pac)
	if err != nil {
		p.onErr(err)
		return nil, err
	}

	r := <-p.peersRetCh
	return r, nil
}

func (p *Peer) Ping(ctx context.Context) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var pac packet
	pac.T = pingArg
	err := p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.pingRetCh:
		return nil
	}
}

func (p *Peer) Sync(start int) ([]*consensus.RandBeaconSig, []*consensus.Block, error) {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return nil, nil, err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = syncArg
	pac.Data, err = gobEncode(start)
	if err != nil {
		p.onErr(err)
		return nil, nil, err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return nil, nil, err
	}

	r := <-p.syncRetCh
	return r.A, r.B, nil
}

func (p *Peer) UpdatePeers(peers []string) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = updatePeersArg
	pac.Data, err = gobEncode(peers)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) Txn(txn []byte) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = txnArg
	pac.Data, err = gobEncode(txn)
	if err != nil {
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) SysTxn(txn *consensus.SysTxn) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = sysTxnArg
	pac.Data, err = gobEncode(txn)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) RandBeaconSigShare(r *consensus.RandBeaconSigShare) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = randBeaconSigShareArg
	pac.Data, err = gobEncode(r)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) RandBeaconSig(r *consensus.RandBeaconSig) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = randBeaconSigArg
	pac.Data, err = gobEncode(r)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) Block(b *consensus.Block) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = blockArg
	pac.Data, err = gobEncode(b)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) BlockProposal(b *consensus.BlockProposal) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = blockProposalArg
	pac.Data, err = gobEncode(b)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) NotarizationShare(n *consensus.NtShare) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = ntShareArg
	pac.Data, err = gobEncode(n)
	if err != nil {
		p.onErr(err)
		return err
	}

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) Inventory(sender consensus.Peer, items []consensus.ItemID) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = inventoryArg
	var d []byte
	d, err = gobEncode(items)
	if err != nil {
		p.onErr(err)
		return err
	}

	pac.Data = d

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}

func (p *Peer) GetData(requester consensus.Peer, items []consensus.ItemID) error {
	p.mu.Lock()
	if err := p.err; err != nil {
		p.mu.Unlock()
		return err
	}
	p.mu.Unlock()

	var err error
	var pac packet
	pac.T = getDataArg
	var d []byte
	d, err = gobEncode(items)
	if err != nil {
		p.onErr(err)
		return err
	}

	pac.Data = d

	err = p.write(pac)
	if err != nil {
		p.onErr(err)
		return err
	}

	return nil
}
