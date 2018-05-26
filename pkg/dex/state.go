package dex

import "github.com/helinwang/dex/pkg/consensus"

// State is the state of the DEX.
type State struct {
	Tokens        map[TokenID]TokenInfo
	Accounts      map[consensus.Addr]Account
	PendingOrders map[MarketSymbol][]OrderTxn
}
