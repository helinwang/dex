package consensus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	log "github.com/helinwang/log15"
)

// syncer downloads blocks and block proposals, validates them
// and connect them to the chain.
//
// The synchronization steps:
// 1. got a new block hash
// 2. get the block B corresponding to the hash
// 3. get all prev block of the block, until connected to the chain,
// or reached the finalized block in the chain but can not connect to
// the chain, stop if can not connect to the chain
// 4. validate B and all it's prev blocks, then connect to the chain
// if valid
// 5. validate BP, then connect to the chain if validate
type syncer struct {
	v                 *validator
	chain             *Chain
	requester         requester
	invalidBlockCache *lru.Cache

	syncRandBeaconMu sync.Mutex
}

func newSyncer(v *validator, chain *Chain, requester requester) *syncer {
	c, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	return &syncer{
		v:                 v,
		chain:             chain,
		requester:         requester,
		invalidBlockCache: c,
	}
}

type requester interface {
	requestBlock(ctx context.Context, addr unicastAddr, hash Hash) (*Block, error)
	requestBlockProposal(ctx context.Context, addr unicastAddr, hash Hash) (*BlockProposal, error)
	requestRandBeaconSig(ctx context.Context, addr unicastAddr, round uint64) (*RandBeaconSig, error)
}

var errCanNotConnectToChain = errors.New("can not connect to chain")

func (s *syncer) SyncBlock(addr unicastAddr, hash Hash, round uint64) error {
	_, err := s.syncBlockAndConnectToChain(addr, hash, round)
	if err == errCanNotConnectToChain {
		s.invalidBlockCache.Add(hash, struct{}{})
	}
	return err
}

func (s *syncer) SyncNtShare(addr unicastAddr, hash Hash) {
	// TODO: don't proceed if the block is already notarized, or
	// can not connect to chain.
	panic("not implemented")
}

func (s *syncer) SyncRandBeaconSig(addr unicastAddr, round uint64) (bool, error) {
	log.Info("SyncRandBeaconSig", "round", round)
	if s.chain.RandomBeacon.Round() > round {
		return false, nil
	}

	s.syncRandBeaconMu.Lock()
	defer s.syncRandBeaconMu.Unlock()

	var sigs []*RandBeaconSig
	for s.chain.RandomBeacon.Round() < round {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		sig, err := s.requester.requestRandBeaconSig(ctx, addr, round)
		if err != nil {
			return false, err
		}
		sigs = append(sigs, sig)
		if sig.Round > 0 {
			round = sig.Round - 1
		} else {
			panic("syncing rand beacon sig of 0 round, should never happen")
		}
	}

	for i := len(sigs) - 1; i >= 0; i-- {
		sig := sigs[i]
		success := s.chain.RandomBeacon.AddRandBeaconSig(sig)
		if !success {
			return false, fmt.Errorf("failed to add rand beacon sig, round: %d, hash: %v", sig.Round, sig.Hash())
		}
	}

	return true, nil
}

type tradesResult struct {
	T *TrieBlob
	E error
}

type bpResult struct {
	BP *BlockProposal
	E  error
}

func (s *syncer) syncBlockAndConnectToChain(addr unicastAddr, hash Hash, round uint64) (State, error) {
	// TODO: validate block, get weight
	// TODO: prevent syncing the same block concurrently

	b := s.chain.Block(hash)
	if b != nil {
		// already connected to the chain
		return s.chain.BlockToState(hash), nil
	}

	if round <= s.chain.FinalizedRound() {
		return nil, errCanNotConnectToChain
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	b, err := s.requester.requestBlock(ctx, addr, hash)
	if err != nil {
		return nil, err
	}

	bpCh := make(chan bpResult, 1)
	go func() {
		bp, err := s.requester.requestBlockProposal(ctx, addr, b.BlockProposal)
		bpCh <- bpResult{BP: bp, E: err}
	}()

	var state State

	if round == 1 {
		if b.PrevBlock != s.chain.Genesis() {
			return nil, errCanNotConnectToChain
		}

		state = s.chain.BlockToState(b.PrevBlock)
	} else {
		state, err = s.syncBlockAndConnectToChain(addr, b.PrevBlock, round-1)
		if err != nil {
			if err == errCanNotConnectToChain {
				s.invalidBlockCache.Add(b.PrevBlock, struct{}{})
			}

			return nil, err
		}
	}

	bpr := <-bpCh
	if bpr.E != nil {
		return nil, bpr.E
	}

	bp := bpr.BP
	trans, err := getTransition(state, bp.Data, bp.Round)
	if err != nil {
		return nil, err
	}

	if trans.StateHash() != b.StateRoot {
		s.invalidBlockCache.Add(b.Hash(), struct{}{})
		return nil, errors.New("invalid state root")
	}

	err = s.chain.addBP(bp, 0)
	if err != nil && err != errChainDataAlreadyExists {
		log.Error("syncer: add block proposal error", "err", err)
	}

	state = trans.Commit()
	err = s.chain.addBlock(b, bp, state, 0)
	if err != nil {
		log.Error("syncer: add block error", "err", err)
	}

	return state, nil
}

/*

How does observer validate each block and update the state?

a. create token, send token, ICO:

  replay txns.

b. orders:

  replay each order txn to update the pending orders state, and then
  replay the trade receipts.

  observer does not need to do order matching, it can just replay the
  order matchin result according to the trade receipts.

  Order book: for the markets that the observer cares, he can
  reconstruct the order book of that market from the pending orders.

  Trade report: can be constructed from trade receipts.

steps:

  1. replay block proposal, but do not do order matching

  2. replay the trade receipts (order matching results)

  3. block proposals and trade receipts will be discarded after x
  blocks, we can have archiving nodes who persists them to disk or
  IPFS.

*/

/*

data structure related to state updates:

block:
  - state root hash
    state is a patricia merkle trie, it contains: token infos,
    accounts, pending orders.
  - receipt root hash
    receipt is a patricia merkle trie, it contains: trade receipts and
    token creation, send, freeze, burn receipts.

*/

/*

Stale client synchronization:

  a. download random beacon item from genesis to tip.

  b. download all key frames (contains group publications) from
  genesis to tip. The key frame is the first block of an epoch. L (a
  system parameter) consecutive blocks form an epoch. The genesis
  block is a key frame since it is the first block of the first
  epoch. Currently there is no open participation (groups are fixed),
  so only one key frame is necessary, L is set to infinity.

  c. download all the blocks, verify the block notarization. The block
  notarization is a threshold signature signed collected by a randomly
  selected group in each round. We can derive the group from the
  random beacon, and the group public key from the latest key frame.

  d. downloading the state of the (tip - n) block, replay the block
  proposal and trade receipts to tip, and verify that the state root
  hashes matches.

*/

/*

Do we need to shard block producers?

  Matching order should be way slower than collecting transactions:
  collecting transactions only involes transactions in the current
  block, while matching orders involves all past orders.

*/
