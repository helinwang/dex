package dex

import "github.com/helinwang/dex/pkg/consensus"

type TxnType int

const (
	Order TxnType = iota
	CancelOrder
	CreateToken
	SendToken
)

type Txn struct {
	T          TxnType
	Data       []byte
	Owner      consensus.Addr
	NonceIdx   int
	NonceValue int
	Sig        []byte
}

// MarketSymbol is the symbol of a trading pair.
//
// A must be <= B for the symbol to be valid. This is to ensure there
// is only one market symbol per tranding pair. E.g., MarketSymbol{A:
// "BTC", B: "ETH"} is valid, MarketSymbol{A: "ETH", B: "BTC"} is
// invalid.
type MarketSymbol struct {
	A string
	B string
}

type OrderTxn struct {
	Sell TokenSymbol
	// the user must own SellQuant of the Sell token for the order
	// to be valid
	SellQuant int
	Buy       TokenSymbol
	BuyQuant  int
	// the order is expired when ExpireHeight >= block height
	ExpireHeight int
}

type CancelOrderTxn struct {
	Order consensus.Hash
}

type CreateTokenTxn struct {
	Info TokenInfo
}

type SendTokenTxn struct {
	TokenID TokenID
	// sends token to the list of receiving addresses with the
	// corresponding quantities.
	To    []consensus.Addr
	Quant []int
}
