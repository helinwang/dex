package dex

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"sync"

	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type TxnSender interface {
	SendTxn([]byte)
}

type RPCServer struct {
	sender TxnSender

	mu sync.Mutex
	s  *State
}

func NewRPCServer() *RPCServer {
	return &RPCServer{}
}

func (r *RPCServer) SetSender(sender TxnSender) {
	r.sender = sender
}

func (r *RPCServer) Update(state consensus.State) {
	s := state.(*State)
	r.mu.Lock()
	r.s = s
	r.mu.Unlock()
}

func (r *RPCServer) Start(addr string) error {
	w := &WalletService{s: r}

	err := rpc.Register(w)
	if err != nil {
		return err
	}

	rpc.HandleHTTP()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		err = http.Serve(l, nil)
		if err != nil {
			log.Error("error serving RPC server", "err", err)
		}
	}()
	return nil
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
	Balances []UserBalance
	Orders   []UserOrder
}

func (r *RPCServer) walletState(addr consensus.Addr, w *WalletState) error {
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

	markets := make(map[MarketSymbol]struct{})
	for _, market := range acc.OrderMarkets {
		markets[market] = struct{}{}
	}

	for market := range markets {
		orders := r.s.AccountOrders(acc, market)
		for _, o := range orders {
			w.Orders = append(w.Orders, UserOrder{Market: market, Order: o})
		}
	}

	w.Balances = bs
	return nil
}

func (r *RPCServer) tokens(_ int, t *TokenState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.s == nil {
		return errors.New("waiting for reaching consensus")
	}

	t.Tokens = r.s.tokenCache.Tokens()
	return nil
}

func (r *RPCServer) sendTxn(t []byte, _ *int) error {
	r.sender.SendTxn(t)
	return nil
}

type NonceSlot struct {
	Idx uint8
	Val uint64
}

func (r *RPCServer) nonce(addr consensus.Addr, slot *NonceSlot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// TODO: returns a slot that does not collide with the ones in
	// the pending txns.

	if r.s == nil {
		return errors.New("waiting for reaching consensus")
	}

	acc := r.s.Account(addr)
	if acc == nil {
		return fmt.Errorf("account %x does not exist", addr[:])
	}

	if len(acc.NonceVec) > 0 {
		slot.Val = acc.NonceVec[0]
	}

	return nil
}

// WalletService is the RPC service for wallet.
type WalletService struct {
	s *RPCServer
}

func (s *WalletService) WalletState(addr consensus.Addr, w *WalletState) error {
	return s.s.walletState(addr, w)
}

func (s *WalletService) Tokens(d int, t *TokenState) error {
	return s.s.tokens(d, t)
}

func (s *WalletService) SendTxn(t []byte, d *int) error {
	return s.s.sendTxn(t, d)
}

func (s *WalletService) Nonce(addr consensus.Addr, slot *NonceSlot) error {
	return s.s.nonce(addr, slot)
}
