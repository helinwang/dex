package dex

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type pker interface {
	PK(addr consensus.Addr) PK
}

type TxnPool struct {
	pker pker

	mu   sync.Mutex
	txns map[consensus.Hash]*consensus.Txn
}

func NewTxnPool(pker pker) *TxnPool {
	return &TxnPool{
		pker: pker,
		txns: make(map[consensus.Hash]*consensus.Txn),
	}
}

func parseTxn(b []byte, pker pker) (*consensus.Txn, error) {
	var txn Txn
	err := rlp.DecodeBytes(b, &txn)
	if err != nil {
		return nil, fmt.Errorf("error decode txn: %v", err)
	}

	ret := &consensus.Txn{
		Raw:   b,
		Owner: txn.Owner,
		Nonce: txn.Nonce,
	}

	switch txn.T {
	case PlaceOrder:
		var t PlaceOrderTxn
		err := t.Decode(txn.Data)
		if err != nil {
			return nil, fmt.Errorf("PlaceOrderTxn decode failed: %v", err)
		}
		ret.Decoded = &t
	case CancelOrder:
		dec := gob.NewDecoder(bytes.NewReader(txn.Data))
		var txn CancelOrderTxn
		err := dec.Decode(&txn)
		if err != nil {
			return nil, fmt.Errorf("CancelOrderTxn decode failed: %v", err)
		}
		ret.Decoded = &txn
	case IssueToken:
		dec := gob.NewDecoder(bytes.NewReader(txn.Data))
		var txn IssueTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			return nil, fmt.Errorf("IssueTokenTxn decode failed: %v", err)
		}
		ret.Decoded = &txn
	case SendToken:
		dec := gob.NewDecoder(bytes.NewReader(txn.Data))
		var txn SendTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			return nil, fmt.Errorf("SendTokenTxn decode failed: %v", err)
		}
		ret.Decoded = &txn
	case FreezeToken:
		dec := gob.NewDecoder(bytes.NewReader(txn.Data))
		var txn FreezeTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			return nil, fmt.Errorf("FreezeTokenTxn decode failed: %v", err)
		}
		ret.Decoded = &txn
	default:
		return nil, fmt.Errorf("unknown txn type: %v", txn.T)
	}

	if !txn.Sig.Verify(txn.Encode(false), pker.PK(txn.Owner)) {
		return nil, fmt.Errorf("txn signature verification failed")
	}

	return ret, nil
}

func (t *TxnPool) Add(b []byte) (r *consensus.Txn, boardcast bool) {
	hash := consensus.SHA3(b)
	t.mu.Lock()
	if _, ok := t.txns[hash]; ok {
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()

	ret, err := parseTxn(b, t.pker)
	if err != nil {
		log.Error("error add txn to pool", "err", err)
		return nil, false
	}

	t.mu.Lock()
	t.txns[hash] = ret
	t.mu.Unlock()
	return ret, true
}

// TODO: remove txn which is no longer valid.

func (t *TxnPool) NotSeen(h consensus.Hash) bool {
	// TODO: return false for txn that are already in the block.
	t.mu.Lock()
	defer t.mu.Unlock()

	_, ok := t.txns[h]
	return !ok
}

func (t *TxnPool) Get(h consensus.Hash) *consensus.Txn {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.txns[h]
}

func (t *TxnPool) Size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.txns)
}

func (t *TxnPool) Txns() []*consensus.Txn {
	t.mu.Lock()
	defer t.mu.Unlock()

	txns := make([]*consensus.Txn, len(t.txns))
	i := 0
	for _, v := range t.txns {
		txns[i] = v
		i++
	}
	return txns
}

func (t *TxnPool) Remove(hash consensus.Hash) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.txns, hash)
}

func (t *TxnPool) RemoveTxns(b []byte) int {
	var txns [][]byte
	err := rlp.DecodeBytes(b, &txns)
	if err != nil {
		log.Error("error decode txns in RemoveTxns", "err", err)
		return 0
	}

	t.mu.Lock()
	for _, txn := range txns {
		h := consensus.SHA3(txn)
		delete(t.txns, h)
	}
	t.mu.Unlock()
	return len(txns)
}
