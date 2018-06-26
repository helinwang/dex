package dex

import (
	"sync"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type TxnPool struct {
	mu   sync.Mutex
	txns map[consensus.Hash]*Txn
}

func NewTxnPool() *TxnPool {
	return &TxnPool{
		txns: make(map[consensus.Hash]*Txn),
	}
}

func (t *TxnPool) Add(b []byte) (h consensus.Hash, boardcast bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var txn Txn
	err := rlp.DecodeBytes(b, &txn)
	if err != nil {
		log.Warn("error decode txn", "err", err)
		return
	}

	hash := txn.Hash()
	if _, ok := t.txns[hash]; ok {
		return
	}

	// TODO: validate txn signature here.

	// optimization TODOs:
	// 1. benchmark place order
	// 2. manually encode/decode order book/txn for speed
	// 3. validate txn signature, so does not need to validate later
	// 4. don't decode txn twice, take out from pool should be decoded
	t.txns[hash] = &txn
	return h, true
}

// TODO: remove txn which is no longer valid.

func (t *TxnPool) NotSeen(h consensus.Hash) bool {
	// TODO: return false for txn that are already in the block.
	t.mu.Lock()
	defer t.mu.Unlock()

	_, ok := t.txns[h]
	return !ok
}

func (t *TxnPool) Get(h consensus.Hash) consensus.Txn {
	t.mu.Lock()
	defer t.mu.Unlock()

	txn, ok := t.txns[h]
	if !ok {
		// need to explicity check for nil, otherwise nil gets
		// wrapped inside a not nil interface value.
		return nil
	}
	return txn
}

func (t *TxnPool) Size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.txns)
}

func (t *TxnPool) Txns() []consensus.Txn {
	t.mu.Lock()
	defer t.mu.Unlock()

	r := make([]consensus.Txn, len(t.txns))
	i := 0
	for _, v := range t.txns {
		r[i] = v
		i++
	}
	return r
}

func (t *TxnPool) Remove(hash consensus.Hash) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.txns, hash)
}

func (t *TxnPool) RemoveTxns(b []byte) int {
	var txns []*Txn
	err := rlp.DecodeBytes(b, &txns)
	if err != nil {
		log.Error("error decode txns in RemoveTxns", "err", err)
		return 0
	}

	t.mu.Lock()
	for _, txn := range txns {
		delete(t.txns, txn.Hash())
	}
	t.mu.Unlock()
	return len(txns)
}
