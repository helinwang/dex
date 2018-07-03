package consensus

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	requestTimeout = time.Minute
)

// syncer downloads data using the gateway, and validates them and
// connect them to the chain.
type syncer struct {
	chain     *Chain
	requester requester
	store     *storage
	node      *Node

	mu               sync.Mutex
	pendingSyncBlock map[Hash][]chan syncBlockResult
	pendingSyncBP    map[Hash][]chan syncBPResult
	pendingSyncRB    map[uint64][]chan syncRBResult
}

func newSyncer(chain *Chain, requester requester, store *storage) *syncer {
	return &syncer{
		chain:            chain,
		store:            store,
		requester:        requester,
		pendingSyncBlock: make(map[Hash][]chan syncBlockResult),
		pendingSyncBP:    make(map[Hash][]chan syncBPResult),
		pendingSyncRB:    make(map[uint64][]chan syncRBResult),
	}
}

type syncBlockResult struct {
	b         *Block
	broadcast bool
	err       error
}

type syncBPResult struct {
	bp        *BlockProposal
	broadcast bool
	err       error
}

type syncRBResult struct {
	broadcast bool
	err       error
}

type requester interface {
	RequestBlock(ctx context.Context, addr unicastAddr, hash Hash) (*Block, error)
	RequestBlockProposal(ctx context.Context, addr unicastAddr, hash Hash) (*BlockProposal, error)
	RequestRandBeaconSig(ctx context.Context, addr unicastAddr, round uint64) (*RandBeaconSig, error)
}

var errCanNotConnectToChain = errors.New("can not connect to chain")

func (s *syncer) SyncBlock(addr unicastAddr, hash Hash, round uint64) (b *Block, broadcast bool, err error) {
	s.mu.Lock()
	chs := s.pendingSyncBlock[hash]
	ch := make(chan syncBlockResult, 1)
	chs = append(chs, ch)
	s.pendingSyncBlock[hash] = chs
	if len(chs) == 1 {
		go func() {
			b, broadcast, err := s.syncBlock(addr, hash, round)
			result := syncBlockResult{b: b, broadcast: broadcast, err: err}
			s.mu.Lock()
			for _, ch := range s.pendingSyncBlock[hash] {
				ch <- result
			}
			delete(s.pendingSyncBlock, hash)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()

	r := <-ch
	return r.b, r.broadcast, r.err
}

func (s *syncer) syncBlock(addr unicastAddr, hash Hash, round uint64) (b *Block, broadcast bool, err error) {
	b = s.store.Block(hash)
	if b != nil {
		// already connected to the chain
		return
	}

	if round <= s.chain.FinalizedRound() {
		err = errCanNotConnectToChain
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	b, err = s.requester.RequestBlock(ctx, addr, hash)
	cancel()
	if err != nil {
		return
	}

	bp, _, err := s.SyncBlockProposal(addr, b.BlockProposal)
	if err != nil {
		return
	}

	var weight float64
	s.chain.randomBeacon.WaitUntil(b.Round)
	prev := s.store.Block(b.PrevBlock)
	if prev == nil {
		err = errors.New("impossible: prev block not found")
		return
	}

	if prev.Round != b.Round-1 {
		err = fmt.Errorf("invalid block, prev round: %d, cur round: %d", prev.Round, b.Round)
		return
	}

	_, _, nt := s.chain.randomBeacon.Committees(b.Round)
	success := b.Notarization.Verify(s.chain.randomBeacon.groups[nt].PK, b.Encode(false))
	if !success {
		err = fmt.Errorf("validate block group sig failed, group:%d", nt)
		return
	}

	rank, err := s.chain.randomBeacon.Rank(b.Owner, b.Round)
	if err != nil {
		err = fmt.Errorf("error get rank, but group sig is valid: %v", err)
		return
	}
	weight = rankToWeight(rank)

	state := s.chain.BlockState(b.PrevBlock)
	newState, count, err := state.CommitTxns(bp.Txns, s.chain.txnPool, bp.Round)
	if err != nil {
		return
	}

	if newState.Hash() != b.StateRoot {
		err = errors.New("invalid state root")
		return
	}

	broadcast, err = s.chain.AddBlock(b, newState, weight, count)
	if err != nil {
		return
	}

	return
}

func rankToWeight(rank uint16) float64 {
	if rank < 0 {
		panic(rank)
	}
	return math.Pow(0.5, float64(rank))
}

func (s *syncer) SyncBlockProposal(addr unicastAddr, hash Hash) (bp *BlockProposal, broadcast bool, err error) {
	if bp = s.store.BlockProposal(hash); bp != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	bp, err = s.requester.RequestBlockProposal(ctx, addr, hash)
	cancel()
	if err != nil {
		return
	}

	var prev *Block
	if bp.Round == 1 {
		if bp.PrevBlock != s.chain.Genesis() {
			err = errCanNotConnectToChain
			return
		}
		prev = s.store.Block(s.chain.Genesis())
	} else {
		prev, _, err = s.SyncBlock(addr, bp.PrevBlock, bp.Round-1)
		if err != nil {
			return
		}
	}

	s.chain.randomBeacon.WaitUntil(bp.Round)

	if prev.Round != bp.Round-1 {
		err = errors.New("prev block round is not block proposal round - 1")
		return
	}

	// make sure proposer is in the current proposal group
	_, err = s.chain.randomBeacon.Rank(bp.Owner, bp.Round)
	if err != nil {
		return
	}

	pk, ok := s.chain.lastFinalizedSysState.addrToPK[bp.Owner]
	if !ok {
		err = errors.New("block proposal owner not found")
		return
	}

	if !bp.OwnerSig.Verify(pk, bp.Encode(false)) {
		err = errors.New("invalid block proposal signature")
		return
	}

	broadcast = s.store.AddBlockProposal(bp, hash)

	if broadcast {
		go s.node.recvBPForNotary(bp)
	}
	return
}

func (s *syncer) SyncRandBeaconSig(addr unicastAddr, round uint64) (bool, error) {
	return s.syncRandBeaconSig(addr, round, true)
}

func (s *syncer) syncRandBeaconSig(addr unicastAddr, round uint64, syncDone bool) (bool, error) {
	s.mu.Lock()
	chs := s.pendingSyncRB[round]
	ch := make(chan syncRBResult, 1)
	chs = append(chs, ch)
	s.pendingSyncRB[round] = chs
	if len(chs) == 1 {
		go func() {
			broadcast, err := s.syncRandBeaconSigImpl(addr, round, syncDone)
			result := syncRBResult{broadcast: broadcast, err: err}
			s.mu.Lock()
			for _, ch := range s.pendingSyncRB[round] {
				ch <- result
			}
			delete(s.pendingSyncRB, round)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()

	r := <-ch
	return r.broadcast, r.err
}

func (s *syncer) syncRandBeaconSigImpl(addr unicastAddr, round uint64, syncDone bool) (bool, error) {
	if s.chain.randomBeacon.Round() >= round {
		return false, nil
	}

	if s.chain.randomBeacon.Round()+1 < round {
		_, err := s.syncRandBeaconSig(addr, round-1, false)
		if err != nil {
			return false, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	sig, err := s.requester.RequestRandBeaconSig(ctx, addr, round)
	cancel()
	if err != nil {
		return false, err
	}

	success := s.chain.randomBeacon.AddRandBeaconSig(sig, syncDone)
	if !success {
		return false, fmt.Errorf("failed to add rand beacon sig, round: %d, hash: %v", sig.Round, sig.Hash())

	}

	return true, nil
}
