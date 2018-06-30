package dex

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
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

func (o *OrderID) Bytes() []byte {
	m := o.Market.Encode()
	buf := make([]byte, 64)
	binary.LittleEndian.PutUint64(buf, uint64(o.ID))
	return append(m, buf...)
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

// Account is a cached proxy to the account data inside the state
// trie.
type Account struct {
	state   *State
	addr    consensus.Addr
	pk      PK
	pkDirty bool
	// a vector of nonce that enables concurrent transactions.
	nonce          uint64
	nonceLoaded    bool
	nonceDirty     bool
	balances       map[TokenID]Balance
	balanceDirty   bool
	reportIdx      *uint32
	reportIdxDirty bool
}

func (a *Account) ExecutionReports() []ExecutionReport {
	return a.state.ExecutionReports(a.addr)
}

func (a *Account) AddExecutionReport(e ExecutionReport) {
	if a.reportIdx == nil {
		a.loadReportIdx()
	}
	a.state.AddExecutionReport(a.addr, e, *a.reportIdx)
	*a.reportIdx++
	a.reportIdxDirty = true
}

func (a *Account) loadReportIdx() {
	idx := a.state.ReportIdx(a.addr)
	a.reportIdx = &idx
}

func (a *Account) Nonce() uint64 {
	if !a.nonceLoaded {
		a.loadNonce()
	}
	return a.nonce
}

func (a *Account) UpdateNonce(n uint64) {
	a.nonce = n
	a.nonceLoaded = true
	a.nonceDirty = true
}

func (a *Account) loadNonce() {
	a.nonce = a.state.Nonce(a.addr)
	a.nonceLoaded = true
}

func (a *Account) PendingOrder(id OrderID) (PendingOrder, bool) {
	return a.state.PendingOrder(a.addr, id)
}

func (a *Account) UpdatePendingOrder(p PendingOrder) {
	a.state.UpdatePendingOrder(a.addr, p)
}

func (a *Account) RemovePendingOrder(id OrderID) {
	a.state.RemovePendingOrder(a.addr, id)
}

func (a *Account) PendingOrders() []PendingOrder {
	return a.state.PendingOrders(a.addr)
}

func (a *Account) Balance(tokenID TokenID) Balance {
	if a.balances == nil {
		a.loadBalances()
	}
	return a.balances[tokenID]
}

func (a *Account) loadBalances() {
	a.balances = make(map[TokenID]Balance)
	bs, ids := a.state.Balances(a.addr)
	for i, b := range bs {
		a.balances[ids[i]] = b
	}
}

func (a *Account) UpdateBalance(tokenID TokenID, balance Balance) {
	if a.balances == nil {
		a.loadBalances()
	}
	a.balances[tokenID] = balance
	a.balanceDirty = true
}

func (a *Account) PK() PK {
	return a.pk
}

func (a *Account) CommitCache(s *State) {
	if a.pkDirty {
		a.state.UpdatePK(a.pk)
		a.pkDirty = false
	}

	if a.nonceDirty {
		a.state.UpdateNonce(a.addr, a.nonce)
		a.nonceDirty = false
	}

	if a.balanceDirty {
		balances := make([]Balance, len(a.balances))
		ids := make([]TokenID, len(a.balances))
		i := 0
		for id := range a.balances {
			ids[i] = id
			i++
		}

		// make the resulting slice deterministic
		sort.Slice(ids, func(i, j int) bool {
			return ids[i] < ids[j]
		})

		for i := range ids {
			balances[i] = a.balances[ids[i]]
		}

		a.state.UpdateBalances(a.addr, balances, ids)
		a.balanceDirty = false
	}

	if a.reportIdxDirty {
		a.state.UpdateReportIdx(a.addr, *a.reportIdx)
		a.reportIdxDirty = false
	}
}
