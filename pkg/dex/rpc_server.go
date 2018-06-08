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

type ChainStater interface {
	ChainStatus() consensus.ChainStatus
	Graphviz(int) string
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
	Balances      []UserBalance
	PendingOrders []PendingOrder
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

	keys := make([]TokenID, len(acc.Balances))
	i := 0
	for k := range acc.Balances {
		keys[i] = k
		i++
	}

	bs := make([]UserBalance, len(keys))
	for i := range bs {
		bs[i].Token = keys[i]
		bs[i].Balance = *acc.Balances[keys[i]]
	}

	w.PendingOrders = acc.PendingOrders
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
	state := r.chain.ChainStatus()
	if !state.InSync() {
		return fmt.Errorf("for your safety, please wait until the chain is synchronized before making any transaction. Current round: %d, random beacon depth: %d", state.Round, state.RandBeaconDepth)
	}
	r.sender.SendTxn(t)
	return nil
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

func (s *WalletService) Round(_ int, r *uint64) error {
	return s.s.round(r)
}

func (s *WalletService) ChainStatus(_ int, state *consensus.ChainStatus) error {
	return s.s.chainStatus(state)
}

func (s *WalletService) Graphviz(_ int, str *string) error {
	return s.s.graphviz(str)
}
