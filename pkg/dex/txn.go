package dex

import (
	"bytes"
	"encoding/gob"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/matching"
	log "github.com/helinwang/log15"
)

type TxnType int

const (
	PlaceOrder TxnType = iota
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

func validateSigAndNonce(state *state, b []byte) (txn *Txn, acc *Account, ready, valid bool) {
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

type PlaceOrderTxn struct {
	matching.Order
	Market MarketSymbol
	// the order is expired when ExpireHeight >= block height
	ExpireHeight uint64
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
	Quant   uint64
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
