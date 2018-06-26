package dex

import (
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
	state      *State
	addr       consensus.Addr
	pk         consensus.PK
	newAccount bool
	// a vector of nonce that enables concurrent transactions.
	nonceVec              []uint64
	nonceVecDirty         bool
	balances              map[TokenID]Balance
	balanceDirty          bool
	pendingOrders         map[OrderID]PendingOrder
	pendingOrdersDirty    bool
	executionReports      []ExecutionReport
	executionReportsDirty bool
}

func (a *Account) ExecutionReports() []ExecutionReport {
	if a.executionReports == nil {
		a.loadExecutionReports()
	}
	return a.executionReports
}

func (a *Account) loadExecutionReports() {
	if !a.newAccount {
		a.executionReports = a.state.ExecutionReports(a.addr)
	}

	if a.executionReports == nil {
		a.executionReports = make([]ExecutionReport, 0)
	}
}

func (a *Account) AddExecutionReport(e ExecutionReport) {
	if a.executionReports == nil {
		a.loadExecutionReports()
	}
	a.executionReports = append(a.executionReports, e)
	a.executionReportsDirty = true
}

func (a *Account) NonceVec() []uint64 {
	if a.nonceVec == nil {
		a.loadNonceVec()
	}
	return a.nonceVec
}

func (a *Account) loadNonceVec() {
	if !a.newAccount {
		a.nonceVec = a.state.NonceVec(a.addr)
	}

	if a.nonceVec == nil {
		a.nonceVec = make([]uint64, 0)
	}
}

func (a *Account) PendingOrder(id OrderID) (PendingOrder, bool) {
	if a.pendingOrders == nil {
		a.loadPendingOrders()
	}
	o, ok := a.pendingOrders[id]
	return o, ok
}

func (a *Account) loadPendingOrders() {
	a.pendingOrders = make(map[OrderID]PendingOrder)
	if !a.newAccount {
		ps := a.state.PendingOrders(a.addr)
		for _, p := range ps {
			a.pendingOrders[p.ID] = p
		}
	}
}

func (a *Account) UpdatePendingOrder(p PendingOrder) {
	if a.pendingOrders == nil {
		a.loadPendingOrders()
	}
	a.pendingOrders[p.ID] = p
	a.pendingOrdersDirty = true
}

func (a *Account) RemovePendingOrder(id OrderID) {
	if a.pendingOrders == nil {
		a.loadPendingOrders()
	}
	delete(a.pendingOrders, id)
	a.pendingOrdersDirty = true
}

func (a *Account) PendingOrders() []PendingOrder {
	if a.pendingOrders == nil {
		a.loadPendingOrders()
	}

	pendingOrders := make([]PendingOrder, len(a.pendingOrders))
	i := 0
	for _, v := range a.pendingOrders {
		pendingOrders[i] = v
		i++
	}
	return pendingOrders
}

func (a *Account) CheckAndIncrementNonce(idx int, val uint64) bool {
	if a.nonceVec == nil {
		a.loadNonceVec()
	}

	if len(a.nonceVec) <= idx {
		if val != 0 {
			return false
		}
		a.nonceVec = append(a.nonceVec, make([]uint64, idx-len(a.nonceVec)+1)...)
	}

	a.nonceVec[idx]++
	a.nonceVecDirty = true
	return true
}

func (a *Account) Balance(tokenID TokenID) (Balance, bool) {
	if a.balances == nil {
		a.loadBalances()
	}
	b, ok := a.balances[tokenID]
	return b, ok
}

func (a *Account) loadBalances() {
	a.balances = make(map[TokenID]Balance)

	if !a.newAccount {
		bs, ids := a.state.Balances(a.addr)
		for i, b := range bs {
			a.balances[ids[i]] = b
		}
	}
}

func (a *Account) UpdateBalance(tokenID TokenID, balance Balance) {
	if a.balances == nil {
		a.loadBalances()
	}
	a.balances[tokenID] = balance
	a.balanceDirty = true
}

func (a *Account) PK() consensus.PK {
	return a.pk
}

func (a *Account) CommitCache(s *State) {
	if a.newAccount {
		a.state.UpdatePK(a.pk)
		a.newAccount = false
	}

	if a.nonceVecDirty {
		a.state.UpdateNonceVec(a.addr, a.nonceVec)
		a.nonceVecDirty = false
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

	if a.pendingOrdersDirty {
		orders := make([]PendingOrder, len(a.pendingOrders))
		ids := make([]OrderID, len(a.pendingOrders))
		i := 0
		for id := range a.pendingOrders {
			ids[i] = id
			i++
		}

		// make the resulting slice deterministic
		sort.Slice(ids, func(i, j int) bool {
			a := ids[i]
			b := ids[j]
			if a.Market == b.Market {
				return a.ID < b.ID
			}

			if a.Market.Base < b.Market.Base {
				return true
			}

			if a.Market.Base > b.Market.Base {
				return false
			}

			return a.Market.Quote < b.Market.Quote
		})

		for i := range ids {
			orders[i] = a.pendingOrders[ids[i]]
		}
		a.state.UpdatePendingOrders(a.addr, orders)
		a.pendingOrdersDirty = false
	}

	if a.executionReportsDirty {
		a.state.UpdateExecutionReports(a.addr, a.executionReports)
		a.executionReportsDirty = false
	}
}
