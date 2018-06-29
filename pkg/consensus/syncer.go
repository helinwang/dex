package consensus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	requestTimeout = time.Minute
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
	chain     *Chain
	requester requester
	store     *storage
	node      *Node

	mu                    sync.Mutex
	pendingSyncBlock      map[Hash][]chan syncBlockResult
	pendingSyncShardBlock map[Hash][]chan syncShardBlockResult
	pendingSyncBP         map[Hash][]chan syncBPResult
	pendingSyncRB         map[uint64][]chan syncRBResult
}

func newSyncer(chain *Chain, requester requester, store *storage) *syncer {
	return &syncer{
		chain:                 chain,
		store:                 store,
		requester:             requester,
		pendingSyncBlock:      make(map[Hash][]chan syncBlockResult),
		pendingSyncShardBlock: make(map[Hash][]chan syncShardBlockResult),
		pendingSyncBP:         make(map[Hash][]chan syncBPResult),
		pendingSyncRB:         make(map[uint64][]chan syncRBResult),
	}
}

type syncBlockResult struct {
	b         *Block
	broadcast bool
	err       error
}

type syncShardBlockResult struct {
	b         *ShardBlock
	broadcast bool
	err       error
}

type syncBPResult struct {
	bp        *ShardBlockProposal
	broadcast bool
	err       error
}

type syncRBResult struct {
	broadcast bool
	err       error
}

type requester interface {
	RequestShardBlock(ctx context.Context, addr unicastAddr, hash Hash) (*ShardBlock, error)
	RequestBlock(ctx context.Context, addr unicastAddr, hash Hash) (*Block, error)
	RequestShardBlockProposal(ctx context.Context, addr unicastAddr, hash Hash) (*ShardBlockProposal, error)
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

	prev, _, err := s.SyncBlock(addr, b.PrevBlock, b.Round-1)
	if err != nil {
		return
	}

	if prev.Round != b.Round-1 {
		err = fmt.Errorf("invalid block, prev round: %d, cur round: %d", prev.Round, b.Round)
		return
	}

	s.chain.randomBeacon.WaitUntil(b.Round)

	_, _, _, gNt := s.chain.randomBeacon.Committees(b.Round)
	success := b.Notarization.Verify(s.chain.randomBeacon.groups[gNt].PK, b.Encode(false))
	if !success {
		err = fmt.Errorf("validate shard block group sig failed, group: %d", gNt)
		return
	}

	broadcast = s.store.AddBlock(b, hash)
	return
}

func (s *syncer) SyncShardBlock(addr unicastAddr, hash Hash, round uint64) (b *ShardBlock, broadcast bool, err error) {
	s.mu.Lock()
	chs := s.pendingSyncShardBlock[hash]
	ch := make(chan syncShardBlockResult, 1)
	chs = append(chs, ch)
	s.pendingSyncShardBlock[hash] = chs
	if len(chs) == 1 {
		go func() {
			b, broadcast, err := s.syncShardBlock(addr, hash, round)
			result := syncShardBlockResult{b: b, broadcast: broadcast, err: err}
			s.mu.Lock()
			for _, ch := range s.pendingSyncShardBlock[hash] {
				ch <- result
			}
			delete(s.pendingSyncShardBlock, hash)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()

	r := <-ch
	return r.b, r.broadcast, r.err
}

func (s *syncer) syncShardBlock(addr unicastAddr, hash Hash, round uint64) (b *ShardBlock, broadcast bool, err error) {
	b = s.store.ShardBlock(hash)
	if b != nil {
		// already connected to the chain
		return
	}

	if round <= s.chain.FinalizedRound() {
		err = errCanNotConnectToChain
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	b, err = s.requester.RequestShardBlock(ctx, addr, hash)
	cancel()
	if err != nil {
		return
	}

	prev, _, err := s.SyncBlock(addr, b.PrevBlock, b.Round-1)
	if err != nil {
		return
	}

	if prev.Round != b.Round-1 {
		err = fmt.Errorf("invalid shard block, prev round: %d, cur round: %d", prev.Round, b.Round)
		return
	}

	s.chain.randomBeacon.WaitUntil(b.Round)
	_, _, nt, _ := s.chain.randomBeacon.Committees(b.Round)
	success := b.Notarization.Verify(s.chain.randomBeacon.groups[nt].PK, b.Encode(false))
	if !success {
		err = fmt.Errorf("validate shard block group sig failed, group: %d", nt)
		return
	}

	broadcast = s.store.AddShardBlock(b, hash)
	return
}

func (s *syncer) SyncShardBlockProposal(addr unicastAddr, hash Hash) (bp *ShardBlockProposal, broadcast bool, err error) {
	s.mu.Lock()
	chs := s.pendingSyncBP[hash]
	ch := make(chan syncBPResult, 1)
	chs = append(chs, ch)
	s.pendingSyncBP[hash] = chs
	if len(chs) == 1 {
		go func() {
			bp, broadcast, err := s.syncShardBlockProposal(addr, hash)
			result := syncBPResult{bp: bp, broadcast: broadcast, err: err}
			s.mu.Lock()
			for _, ch := range s.pendingSyncBP[hash] {
				ch <- result
			}
			delete(s.pendingSyncBP, hash)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()

	r := <-ch
	return r.bp, r.broadcast, r.err
}

func (s *syncer) syncShardBlockProposal(addr unicastAddr, hash Hash) (bp *ShardBlockProposal, broadcast bool, err error) {
	if bp = s.store.ShardBlockProposal(hash); bp != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	bp, err = s.requester.RequestShardBlockProposal(ctx, addr, hash)
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

	broadcast = s.store.AddShardBlockProposal(bp, hash)

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
