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
	requester         blockRequester
	orphanBlockCache  *lru.Cache
	invalidBlockCache *lru.Cache
}

type blockRequester interface {
	RequestBlock(ctx context.Context, hash Hash) (*Block, error)
}

var errCanNotConnectToChain = errors.New("can not connect to chain")

func (s *blockSyncer) SyncBlock(hash Hash, round uint64) error {
	err := s.syncBlockAndConnectToChain(hash, round)
	if err == errCanNotConnectToChain {
		s.invalidBlockCache.Add(hash, struct{}{})
	}
	return err
}

func (s *blockSyncer) syncBlockAndConnectToChain(hash Hash, round uint64) error {
	b := s.chain.Block(hash)
	if b != nil {
		// already connected to the chain
		return nil
	}

	if round <= s.chain.FinalizedRound() {
		return errCanNotConnectToChain
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	b, err := s.requester.RequestBlock(ctx, hash)
	cancel()
	if err != nil {
		return err
	}

	if round == 1 {
		if b.PrevBlock == s.chain.Genesis() {
			return nil
		}
		return errCanNotConnectToChain
	}

	err = s.syncBlockAndConnectToChain(b.PrevBlock, round-1)
	if err != nil {
		if err == errCanNotConnectToChain {
			s.invalidBlockCache.Add(b.PrevBlock, struct{}{})
		}

		return err
	}

	// now prev block is connected to chain, validate the current
	// block

	return nil
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
  replay the trade records.

  observer does not need to do order matching, it can just replay the
  order matchin result recorded in the trade records.

  Order book: for the markets that the observer cares, he can
  reconstruct the order book of that market from the pending orders.

  Trade report: can be constructed from trade records.

steps:

  1. replay block proposal, but do not do order matching

  2. replay the trade records (order matching results)

  3. block proposals and trade blocks will be discarded after x
  blocks, we can have archiving nodes who persists them to disk or
  IPFS.

*/

/*

data structure related to state updates:

trade block:
trades happened in a given block.

block:
  - state root hash
    state is a patricia merkle trie, it contains: token infos,
    accounts, pending orders.
  - trade record hash: the hash for the block's trade records.

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
  proposal and trade records to tip, and verify that the state root
  hashes matches.

*/

/*

Do we need to shard block producers?

  Matching order should be way slower than collecting transactions:
  collecting transactions only involes transactions in the current
  block, while matching orders involves all past orders.

*/
