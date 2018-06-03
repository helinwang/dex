package dex

import (
	"sync"

	"github.com/ethereum/go-ethereum/rlp"
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

func (t *TxnPool) Get(h consensus.Hash) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.txns[h]
}

func (t *TxnPool) Txns() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	r := make([][]byte, len(t.txns))
	i := 0
	for _, v := range t.txns {
		r[i] = v
		i++
	}
	b, err := rlp.EncodeToBytes(r)
	if err != nil {
		panic(err)
	}

	return b
}

func (t *TxnPool) Remove(hash consensus.Hash) {
	delete(t.txns, hash)
}
