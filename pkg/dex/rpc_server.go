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
	SendTxn([]byte) error
}

type ChainStater interface {
	ChainStatus() consensus.ChainStatus
	Graphviz(int) string
	TxnPoolSize() int
}

type RPCServer struct {
	sender TxnSender

	mu    sync.Mutex
	chain ChainStater
	s     *State
}

func NewRPCServer() *RPCServer {
	return &RPCServer{}
}

// SetSender sets the transaction sender, it must be called before
// Start.
func (r *RPCServer) SetSender(sender TxnSender) {
	r.sender = sender
}

// SetStater sets the chain stater, it must be called before Start.
func (r *RPCServer) SetStater(c ChainStater) {
	r.chain = c
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

type UserBalance struct {
	Token TokenID
	Balance
}

type WalletState struct {
	Balances         []UserBalance
	PendingOrders    []PendingOrder
	ExecutionReports []ExecutionReport
}

func (r *RPCServer) walletState(addr consensus.Addr, w *WalletState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.s == nil {
		return errors.New("waiting for reaching consensus")
	}

	acc := r.s.Account(addr)
	if acc == nil {
		return fmt.Errorf("account %v does not exist", addr)
	}

	keys := make([]TokenID, len(acc.balances))
	i := 0
	for k := range acc.balances {
		keys[i] = k
		i++
	}

	bs := make([]UserBalance, len(keys))
	for i := range bs {
		bs[i].Token = keys[i]
		b, _ := acc.Balance(keys[i])
		bs[i].Balance = b
	}

	// TODO: sort pending orders by key
	w.PendingOrders = acc.PendingOrders()
	w.ExecutionReports = acc.ExecutionReports()
	w.Balances = bs
	return nil
}

func (r *RPCServer) tokens(_ int, t *TokenState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.s == nil {
		return errors.New("waiting for reaching consensus")
	}

	t.Tokens = r.s.Tokens()
	return nil
}

func (r *RPCServer) sendTxn(t []byte, _ *int) error {
	return r.sender.SendTxn(t)
}

type NonceSlot struct {
	Idx uint8
	Val uint64
}

func (r *RPCServer) round(round *uint64) error {
	state := r.chain.ChainStatus()
	*round = state.Round
	return nil
}

func (r *RPCServer) graphviz(str *string) error {
	const maxFinalizeBlockPrint = 6
	*str = r.chain.Graphviz(maxFinalizeBlockPrint)
	return nil
}

func (r *RPCServer) txnPoolSize() int {
	return r.chain.TxnPoolSize()
}

func (r *RPCServer) chainStatus(state *consensus.ChainStatus) error {
	*state = r.chain.ChainStatus()
	return nil
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
		return fmt.Errorf("account %v does not exist", addr)
	}

	if len(acc.NonceVec()) > 0 {
		slot.Val = acc.NonceVec()[0]
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

func (s *WalletService) Round(_ int, r *uint64) error {
	return s.s.round(r)
}

func (s *WalletService) ChainStatus(_ int, state *consensus.ChainStatus) error {
	return s.s.chainStatus(state)
}

func (s *WalletService) Graphviz(_ int, str *string) error {
	return s.s.graphviz(str)
}

func (s *WalletService) TxnPoolSize(_ int, size *int) error {
	*size = s.s.txnPoolSize()
	return nil
}
