package dex

import (
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
)

type TxnPool struct {
	mu   sync.Mutex
	txns map[consensus.Hash][]byte
}

func NewTxnPool() *TxnPool {
	return &TxnPool{
		txns: make(map[consensus.Hash][]byte),
	}
}

func (t *TxnPool) Add(b []byte) (boardcast bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	hash := consensus.SHA3(b)
	if t.txns[hash] != nil {
		return false
	}

	// optimization TODOs:
	// 1. benchmark place order
	// 2. manually encode/decode order book/txn for speed
	// 3. validate txn signature, so does not need to validate later
	// 4. don't decode txn twice, take out from pool should be decoded
	t.txns[hash] = b
	return true
}

// TODO: remove txn which is no longer valid.

func (t *TxnPool) NotSeen(h consensus.Hash) bool {
	// TODO: return false for txn that are already in the block.
	t.mu.Lock()
	defer t.mu.Unlock()

	_, ok := t.txns[h]
	return !ok
}

func (t *TxnPool) Get(h consensus.Hash) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.txns[h]
}

func (t *TxnPool) Size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.txns)
}

func (t *TxnPool) Txns() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	r := make([][]byte, len(t.txns))
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
