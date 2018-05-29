package dex

import (
	"bytes"
	"encoding/gob"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/prometheus/common/log"
)

type TxnType int

const (
	Order TxnType = iota
	CancelOrder
	CreateToken
	SendToken
)

type Txn struct {
	T          TxnType
	Data       []byte
	Owner      consensus.Addr
	NonceIdx   int
	NonceValue uint64
	Sig        []byte
}

func validateSigAndNonce(state *State, b []byte) (txn *Txn, acc *Account, ready, valid bool) {
	txn = &Txn{}
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	err := dec.Decode(&txn)
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

	if txn.NonceIdx >= len(acc.NonceVec) {
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
	use := b
	if !withSig {
		newB := *b
		newB.Sig = nil
		use = &newB
	}

	return gobEncode(use)
}

func (b *Txn) Decode(d []byte) error {
	var use Txn
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	err := dec.Decode(&use)
	if err != nil {
		return err
	}

	*b = use
	return nil
}

func (b *Txn) Hash() consensus.Hash {
	return consensus.SHA3(b.Encode(true))
}

// MarketSymbol is the symbol of a trading pair.
//
// A must be <= B for the symbol to be valid. This is to ensure there
// is only one market symbol per tranding pair. E.g., MarketSymbol{A:
// "BTC", B: "ETH"} is valid, MarketSymbol{A: "ETH", B: "BTC"} is
// invalid.
type MarketSymbol struct {
	A string
	B string
}

type OrderTxn struct {
	Sell TokenSymbol
	// the user must own SellQuant of the Sell token for the order
	// to be valid
	SellQuant int
	Buy       TokenSymbol
	BuyQuant  int
	// the order is expired when ExpireHeight >= block height
	ExpireHeight int
}

type CancelOrderTxn struct {
	Order consensus.Hash
}

type CreateTokenTxn struct {
	Info TokenInfo
}

type SendTokenTxn struct {
	TokenID TokenID
	To      consensus.PK
	Quant   int
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
