package dex

import (
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
)

type TxnPool struct {
	mu    sync.Mutex
	state *State
	txns  map[consensus.Hash][]byte
}

func NewTxnPool(state *State) *TxnPool {
	return &TxnPool{
		state: state,
		txns:  make(map[consensus.Hash][]byte),
	}
}

func (t *TxnPool) Add(b []byte) (hash consensus.Hash, boardcast bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	hash = consensus.SHA3(b)
	if t.txns[hash] != nil {
		return hash, false
	}

	_, _, _, valid := validateSigAndNonce(&t.state.state, b)
	if !valid {
		return hash, false
	}

	t.txns[hash] = b
	return hash, true
}

func (t *TxnPool) Need(h consensus.Hash) bool {
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
	delete(t.txns, hash)
}
