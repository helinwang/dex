package dex

import (
	"github.com/helinwang/dex/pkg/consensus"
)

// TODO: does it needs threadsafety?
type TxnPool struct {
	state *State
	txns  map[consensus.Hash][]byte
}

func NewTxnPool(state *State) *TxnPool {
	return &TxnPool{
		state: state,
		txns:  make(map[consensus.Hash][]byte),
	}
}

func (t *TxnPool) Add(b []byte) (boardcast bool) {
	hash := consensus.SHA3(b)
	if t.txns[hash] != nil {
		return false
	}

	_, _, _, valid := validateSigAndNonce(&t.state.state, b)
	if !valid {
		return false
	}

	t.txns[hash] = b
	return true
}

func (t *TxnPool) Txns() [][]byte {
	r := make([][]byte, len(t.txns))
	i := 0
	for _, v := range t.txns {
		r[i] = v
		i++
	}
	return r
}

func (t *TxnPool) Remove(b []byte) {
	hash := consensus.SHA3(b)
	delete(t.txns, hash)
}
