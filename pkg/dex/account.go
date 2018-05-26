package dex

import "github.com/helinwang/dex/pkg/consensus"

const (
	maxNonceIdx = 100
)

type Balance struct {
	ID        TokenID
	Available int
	Pending   int
}

type Account struct {
	Addr consensus.Addr
	// a vector of nonce that enables concurrent transactions.
	NonceVec []int
	Balances map[TokenID]Balance
}
