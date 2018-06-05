package dex

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"io"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type TxnType uint8

const (
	PlaceOrder TxnType = iota
	CancelOrder
	IssueToken
	SendToken
)

type Txn struct {
	T          TxnType
	Data       []byte
	Owner      consensus.Addr
	NonceIdx   uint8
	NonceValue uint64
	Sig        []byte
}

func validateSigAndNonce(state *State, b []byte) (txn *Txn, acc *Account, ready, valid bool) {
	txn = &Txn{}
	err := rlp.DecodeBytes(b, &txn)
	if err != nil {
		log.Warn("error decode txn", "err", err)
		return
	}

	acc = state.Account(txn.Owner)
	if acc == nil {
		log.Warn("txn owner not found")
		return
	}

	var pk bls.PublicKey
	err = pk.Deserialize(acc.PK)
	if err != nil {
		log.Error("invalid account PK", "account", txn.Owner)
		return
	}

	// TODO: add helper that deserialize sign which handles crash
	// on nil.
	var sign bls.Sign
	err = sign.Deserialize(txn.Sig)
	if err != nil {
		log.Warn("txn signature deserialize failed", "err", err)
		return
	}

	if !sign.Verify(&pk, string(txn.Encode(false))) {
		log.Warn("invalid txn signature")
		return
	}

	if int(txn.NonceIdx) >= len(acc.NonceVec) {
		if txn.NonceValue > 0 {
			ready = false
			valid = true
			return
		}

		ready = true
		valid = true
		return
	}

	if acc.NonceVec[txn.NonceIdx] < txn.NonceValue {
		ready = false
		valid = true
		return
	} else if acc.NonceVec[txn.NonceIdx] > txn.NonceValue {
		valid = false
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

func (b *Txn) Hash() consensus.Hash {
	return consensus.SHA3(b.Encode(true))
}

type PlaceOrderTxn struct {
	Order
	Market MarketSymbol
	// the order is expired when ExpireHeight >= block height
	ExpireHeight uint64
}

type CancelOrderTxn struct {
	Order consensus.Hash
}

func MakeSendTokenTxn(from consensus.SK, to consensus.PK, tokenID TokenID, quant uint64, nonceIdx uint8, nonce uint64) []byte {
	send := SendTokenTxn{
		TokenID: tokenID,
		To:      to,
		Quant:   quant,
	}

	owner, err := from.PK()
	if err != nil {
		panic(err)
	}

	txn := Txn{
		T:          SendToken,
		Owner:      owner.Addr(),
		NonceIdx:   nonceIdx,
		NonceValue: nonce,
		Data:       gobEncode(send),
	}

	sk, err := from.Get()
	if err != nil {
		panic(err)
	}

	txn.Sig = sk.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func MakePlaceOrderTxn(sk consensus.SK, addr consensus.Addr, t PlaceOrderTxn) []byte {
	txn := Txn{
		T:     PlaceOrder,
		Owner: addr,
		Data:  gobEncode(t),
	}
	key, err := sk.Get()
	if err != nil {
		panic(err)
	}

	txn.Sig = key.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func MakeIssueTokenTxn(sk consensus.SK, info TokenInfo, nonceIdx uint8, nonceValue uint64) []byte {
	t := IssueTokenTxn{Info: info}
	owner, err := sk.PK()
	if err != nil {
		panic(err)
	}

	txn := Txn{
		T:          IssueToken,
		Data:       gobEncode(t),
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Owner:      owner.Addr(),
	}

	key, err := sk.Get()
	if err != nil {
		panic(err)
	}

	txn.Sig = key.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

type IssueTokenTxn struct {
	Info TokenInfo
}

type SendTokenTxn struct {
	TokenID TokenID
	To      consensus.PK
	Quant   uint64
}

type Order struct {
	SellSide bool
	Quant    uint64
	Price    float64
}

func (o *Order) EncodeRLP(w io.Writer) error {
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.LittleEndian, o.Price)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, buf.Bytes())
	if err != nil {
		return err
	}

	err = rlp.Encode(w, o.Quant)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, o.SellSide)
	if err != nil {
		return err
	}

	return nil
}

func (o *Order) DecodeRLP(s *rlp.Stream) error {
	var order Order
	b, err := s.Bytes()
	if err != nil {
		return err
	}

	var f float64
	err = binary.Read(bytes.NewReader(b), binary.LittleEndian, &f)
	if err != nil {
		return err
	}
	order.Price = f

	v, err := s.Uint()
	if err != nil {
		return err
	}
	order.Quant = v

	sell, err := s.Bool()
	if err != nil {
		return err
	}
	order.SellSide = sell

	*o = order
	return nil
}

// TODO: maybe move this func to common package
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
