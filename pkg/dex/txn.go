package dex

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

const (
	OrderPriceDecimals = 8
)

type TxnType uint8

const (
	PlaceOrder TxnType = iota
	CancelOrder
	IssueToken
	SendToken
	FreezeToken
	BurnToken
)

type Txn struct {
	T     TxnType
	Data  []byte
	Nonce uint64
	Owner consensus.Addr
	Sig   Sig
}

func validateNonce(state *State, txn *consensus.Txn) (acc *Account, ready, valid bool) {
	acc = state.Account(txn.Owner)
	if acc == nil {
		log.Warn("txn owner not found")
		return
	}

	if txn.Nonce < acc.Nonce() {
		return
	}

	if txn.Nonce > acc.Nonce() {
		ready = false
		valid = true
		return
	}

	ready = true
	valid = true
	return
}

func (b *Txn) Encode(withSig bool) []byte {
	en := *b
	if !withSig {
		en.Sig = nil
	}

	d, err := rlp.EncodeToBytes(en)
	if err != nil {
		panic(err)
	}

	return d
}

func (b *Txn) Bytes() []byte {
	return b.Encode(true)
}

type PlaceOrderTxn struct {
	SellSide bool
	// quant step size is the decimals of the token, specific when
	// the token is issued, e.g., quant = Quant * 10^-(decimals)
	Quant uint64
	// price tick size is 10^-8, e.g,. price = Price * 10^-8
	Price uint64
	// the order is expired when ExpireRound >= block height
	ExpireRound uint64
	Market      MarketSymbol
}

func (p *PlaceOrderTxn) Encode() []byte {
	var buf bytes.Buffer
	b := make([]byte, 64)
	n := binary.PutUvarint(b, p.Quant)
	buf.Write(b[:n])
	n = binary.PutUvarint(b, p.Price)
	buf.Write(b[:n])
	n = binary.PutUvarint(b, p.ExpireRound)
	buf.Write(b[:n])
	buf.Write(p.Market.Encode())
	if p.SellSide {
		buf.Write([]byte{1})
	}
	return buf.Bytes()
}

func (p *PlaceOrderTxn) Decode(b []byte) error {
	var t PlaceOrderTxn
	v, n := binary.Uvarint(b)
	t.Quant = v
	b = b[n:]

	v, n = binary.Uvarint(b)
	t.Price = v
	b = b[n:]

	v, n = binary.Uvarint(b)
	t.ExpireRound = v
	b = b[n:]

	n, err := t.Market.Decode(b)
	if err != nil {
		return err
	}

	b = b[n:]
	if len(b) == 1 {
		t.SellSide = true
	} else if len(b) > 1 {
		return fmt.Errorf("unexpected bytes remaining, count: %d", len(b))
	}

	*p = t
	return nil
}

type CancelOrderTxn struct {
	ID OrderID
}

func MakeCancelOrderTxn(sk SK, owner consensus.Addr, id OrderID, nonce uint64) []byte {
	t := CancelOrderTxn{
		ID: id,
	}

	txn := &Txn{
		T:     CancelOrder,
		Owner: owner,
		Nonce: nonce,
		Data:  gobEncode(t),
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakeSendTokenTxn(from SK, owner consensus.Addr, to PK, tokenID TokenID, quant uint64, nonce uint64) []byte {
	send := SendTokenTxn{
		TokenID: tokenID,
		To:      to,
		Quant:   quant,
	}

	txn := &Txn{
		T:     SendToken,
		Owner: owner,
		Nonce: nonce,
		Data:  gobEncode(send),
	}

	txn.Sig = from.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakePlaceOrderTxn(sk SK, owner consensus.Addr, t PlaceOrderTxn, nonce uint64) []byte {
	txn := &Txn{
		T:     PlaceOrder,
		Owner: owner,
		Nonce: nonce,
		Data:  t.Encode(),
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakeIssueTokenTxn(sk SK, owner consensus.Addr, info TokenInfo, nonce uint64) []byte {
	t := IssueTokenTxn{Info: info}
	txn := &Txn{
		T:     IssueToken,
		Data:  gobEncode(t),
		Nonce: nonce,
		Owner: owner,
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

func MakeFreezeTokenTxn(sk SK, owner consensus.Addr, t FreezeTokenTxn, nonce uint64) []byte {
	txn := &Txn{
		T:     FreezeToken,
		Data:  gobEncode(t),
		Nonce: nonce,
		Owner: owner,
	}

	txn.Sig = sk.Sign(txn.Encode(false))
	return txn.Encode(true)
}

type IssueTokenTxn struct {
	Info TokenInfo
}

type SendTokenTxn struct {
	TokenID TokenID
	To      PK
	Quant   uint64
}

type FreezeTokenTxn struct {
	TokenID        TokenID
	AvailableRound uint64
	Quant          uint64
}

func gobEncode(v interface{}) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		// should not happen
		panic(err)
	}
	return buf.Bytes()
}
