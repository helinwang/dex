package dex

import (
	"errors"
	"fmt"
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type RPCServer struct {
	mu sync.Mutex
	s  *state
}

func (r *RPCServer) updateConsensus(s *state) {
	r.mu.Lock()
	r.s = s
	r.mu.Unlock()
}

type TokenState struct {
	Tokens []Token
}

type UserOrder struct {
	Market MarketSymbol
	Order
}

type UserBalance struct {
	Token TokenID
	Balance
}

type WalletState struct {
	Balances      []UserBalance
	PendingOrders []UserOrder
}

func (r *RPCServer) WalletState(addr consensus.Addr, w *WalletState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.s == nil {
		return errors.New("waiting for reaching consensus")
	}

	acc := r.s.Account(addr)
	if acc == nil {
		return fmt.Errorf("account %x does not exist", addr[:])
	}

	bs := make([]UserBalance, len(acc.Balances))
	i := 0
	for k, v := range acc.Balances {
		bs[i].Token = k
		bs[i].Balance = *v
		i++
	}

	var pos []UserOrder
	for _, m := range acc.PendingOrderMarkets {
		po := r.s.AccountPendingOrders(m, addr)
		if len(po) == 0 {
			log.Error("failed to get pending order for wallet in given market", "account", addr, "market", m)
			continue
		}

		for _, o := range po {
			pos = append(pos, UserOrder{Market: m, Order: o.Order})
		}
	}

	w.Balances = bs
	w.PendingOrders = pos
	return nil
}

func (r *RPCServer) Tokens(_ int, t *TokenState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.s == nil {
		return errors.New("waiting for reaching consensus")
	}

	t.Tokens = r.s.tokenCache.Tokens()
	return nil
}
