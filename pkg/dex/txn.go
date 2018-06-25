package dex

import (
	"bytes"
	"encoding/gob"

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
	T          TxnType
	Data       []byte
	Owner      consensus.Addr
	NonceIdx   uint8
	NonceValue uint64
	Sig        consensus.Sig
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

	if !txn.Sig.Verify(acc.PK, txn.Encode(false)) {
		log.Warn("invalid txn signature")
		return
	}

	// if int(txn.NonceIdx) >= len(acc.NonceVec) {
	// 	if txn.NonceValue > 0 {
	// 		ready = false
	// 		valid = true
	// 		return
	// 	}

	// 	ready = true
	// 	valid = true
	// 	return
	// }

	// if acc.NonceVec[txn.NonceIdx] < txn.NonceValue {
	// 	ready = false
	// 	valid = true
	// 	return
	// } else if acc.NonceVec[txn.NonceIdx] > txn.NonceValue {
	// 	valid = false
	// 	return
	// }

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

type CancelOrderTxn struct {
	ID OrderID
}

func MakeCancelOrderTxn(sk consensus.SK, id OrderID, nonceIdx uint8, nonce uint64) []byte {
	t := CancelOrderTxn{
		ID: id,
	}

	txn := Txn{
		T:          CancelOrder,
		Owner:      sk.MustPK().Addr(),
		NonceIdx:   nonceIdx,
		NonceValue: nonce,
		Data:       gobEncode(t),
	}

	key := sk.MustGet()
	txn.Sig = key.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
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

	key := from.MustGet()
	txn.Sig = key.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func MakePlaceOrderTxn(sk consensus.SK, t PlaceOrderTxn, nonceIdx uint8, nonceValue uint64) []byte {
	txn := Txn{
		T:          PlaceOrder,
		Owner:      sk.MustPK().Addr(),
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Data:       gobEncode(t),
	}

	key := sk.MustGet()
	txn.Sig = key.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func MakeIssueTokenTxn(sk consensus.SK, info TokenInfo, nonceIdx uint8, nonceValue uint64) []byte {
	t := IssueTokenTxn{Info: info}
	txn := Txn{
		T:          IssueToken,
		Data:       gobEncode(t),
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Owner:      sk.MustPK().Addr(),
	}

	key := sk.MustGet()
	txn.Sig = key.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func MakeFreezeTokenTxn(sk consensus.SK, t FreezeTokenTxn, nonceIdx uint8, nonceValue uint64) []byte {
	txn := Txn{
		T:          FreezeToken,
		Data:       gobEncode(t),
		NonceIdx:   nonceIdx,
		NonceValue: nonceValue,
		Owner:      sk.MustPK().Addr(),
	}

	key := sk.MustGet()
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
