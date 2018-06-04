package consensus

import (
	"context"
	"errors"
	"time"

	lru "github.com/hashicorp/golang-lru"
)

// blockSyncer downloads blocks and block proposals, validates them
// and connect them to the chain.
//
// The synchronization steps:
// 1. got a new block proposal BP, if got a new block, skip to 3
// 2. ask for block proposal's prev block
// 3. got a new block B
// 4. get all prev block of the block, until connected to the chain,
// or reached the finalized block in the chain but can not connect to
// the chain, stop if can not connect to the chain
// 5. validate B and all it's prev blocks, then connect to the chain
// if valid
// 6. validate BP, then connect to the chain if validate
type blockSyncer struct {
	v                 *validator
	chain             *Chain
	requester         requester
	invalidBlockCache *lru.Cache
}

func newBlockSyncer(v *validator, chain *Chain, requester requester) *blockSyncer {
	c, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	return &blockSyncer{
		v:                 v,
		chain:             chain,
		requester:         requester,
		invalidBlockCache: c,
	}
}

type requester interface {
	RequestBlock(ctx context.Context, hash Hash) (*Block, error)
	RequestBlockProposal(ctx context.Context, hash Hash) (*BlockProposal, error)
	RequestTrades(ctx context.Context, hash Hash) ([]byte, error)
}

var errCanNotConnectToChain = errors.New("can not connect to chain")

func (s *blockSyncer) SyncBlock(hash Hash, round uint64) error {
	_, err := s.syncBlockAndConnectToChain(hash, round)
	if err == errCanNotConnectToChain {
		s.invalidBlockCache.Add(hash, struct{}{})
	}
	return err
}

type tradesResult struct {
	T []byte
	E error
}

type bpResult struct {
	BP *BlockProposal
	E  error
}

func (s *blockSyncer) syncBlockAndConnectToChain(hash Hash, round uint64) (State, error) {
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

	b, err := s.requester.RequestBlock(ctx, hash)
	if err != nil {
		return nil, err
	}

	tCh := make(chan tradesResult, 1)
	go func() {
		t, err := s.requester.RequestTrades(ctx, b.Trades)
		tCh <- tradesResult{T: t, E: err}
	}()

	bpCh := make(chan bpResult, 1)
	go func() {
		bp, err := s.requester.RequestBlockProposal(ctx, b.BlockProposal)
		bpCh <- bpResult{BP: bp, E: err}
	}()

	var state State

	if round == 1 {
		if b.PrevBlock != s.chain.Genesis() {
			return nil, errCanNotConnectToChain
		}

		state = s.chain.BlockToState(b.PrevBlock)
	} else {
		state, err = s.syncBlockAndConnectToChain(b.PrevBlock, round-1)
		if err != nil {
			if err == errCanNotConnectToChain {
				s.invalidBlockCache.Add(b.PrevBlock, struct{}{})
			}

			return nil, err
		}
	}

	tr := <-tCh
	if tr.E != nil {
		return nil, tr.E
	}

	bpr := <-bpCh
	if bpr.E != nil {
		return nil, bpr.E
	}

	bp := bpr.BP
	trans, err := getTransition(state, bp.Data)
	if err != nil {
		return nil, err
	}

	err = trans.ApplyTrades(tr.T)
	if err != nil {
		return nil, err
	}

	if trans.StateHash() != b.StateRoot {
		s.invalidBlockCache.Add(b.Hash(), struct{}{})
		return nil, errors.New("invalid state root")
	}

	state = trans.Commit()
	s.chain.addBlock(b, bp, state, tr.T, 0)
	return state, nil
}

func (s *blockSyncer) SyncBlockProposal(item ItemID) error {
	return nil
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
