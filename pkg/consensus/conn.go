package consensus

import (
	"encoding/gob"
	"net"

	log "github.com/helinwang/log15"
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
	var g itemRequest
	var h *connectRequest
	var i []unicastAddr
	var j ack
	var k *NtShare

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
	gob.Register(k)
}

type packet struct {
	Data interface{}
}

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

func (p *conn) Write(pac packet) error {
	return p.enc.Encode(pac)
}

func (p *conn) Read() (pac packet, err error) {
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
