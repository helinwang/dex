package dex

import "github.com/helinwang/dex/pkg/consensus"

const (
	maxNonceIdx = 100
)

type Balance struct {
	Available int
	Pending   int
}

type Account struct {
	PK consensus.PK
	// a vector of nonce that enables concurrent transactions.
	NonceVec []uint64
	Balances map[TokenID]*Balance
}
