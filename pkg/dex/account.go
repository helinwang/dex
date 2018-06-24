package dex

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/helinwang/dex/pkg/consensus"
)

const (
	maxNonceIdx = 100
)

type Frozen struct {
	AvailableRound uint64
	Quant          uint64
}

type Balance struct {
	Available uint64
	Pending   uint64
	Frozen    []Frozen
}

type OrderID struct {
	ID     uint64
	Market MarketSymbol
}

func (o *OrderID) Encode() string {
	return fmt.Sprintf("%d_%d_%d", o.Market.Base, o.Market.Quote, o.ID)
}

func (o *OrderID) Decode(str string) error {
	ss := strings.Split(str, "_")
	if len(ss) != 3 {
		return errors.New("invalid order id format")
	}

	a, err := strconv.ParseUint(ss[0], 10, 64)
	if err != nil {
		return fmt.Errorf("error parsing order id: %v", err)
	}

	b, err := strconv.ParseUint(ss[1], 10, 64)
	if err != nil {
		return fmt.Errorf("error parsing order id: %v", err)
	}

	c, err := strconv.ParseUint(ss[2], 10, 64)
	if err != nil {
		return fmt.Errorf("error parsing order id: %v", err)
	}

	o.Market = MarketSymbol{Base: TokenID(a), Quote: TokenID(b)}
	o.ID = c
	return nil
}

type PendingOrder struct {
	ID       OrderID
	Executed uint64
	Order
}

type Account struct {
	PK consensus.PK
	// a vector of nonce that enables concurrent transactions.
	NonceVec         []uint64
	Balances         map[TokenID]*Balance
	PendingOrders    []PendingOrder
	ExecutionReports []ExecutionReport
}
