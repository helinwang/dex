package consensus

import (
	"sync"
)

// DB is the database used to save the blockchain data.
type DB interface {
	Put(Hash, []byte)
	Get(Hash) []byte
}

// storage stores the blockchain data.
type storage struct {
	mu                  sync.Mutex
	blocks              map[Hash]*Block
	shardBlocks         map[Hash]*ShardBlock
	shardBlockProposals map[Hash]*ShardBlockProposal
	randBeaconSigs      map[uint64]*RandBeaconSig
}

func newStorage() *storage {
	return &storage{
		blocks:              make(map[Hash]*Block),
		shardBlocks:         make(map[Hash]*ShardBlock),
		shardBlockProposals: make(map[Hash]*ShardBlockProposal),
		randBeaconSigs:      make(map[uint64]*RandBeaconSig),
	}
}

func (s *storage) AddBlock(b *Block, h Hash) bool {
	s.mu.Lock()
	broadcast := false
	if _, ok := s.blocks[h]; !ok {
		broadcast = true
		s.blocks[h] = b
	}
	s.mu.Unlock()
	return broadcast
}

func (s *storage) AddShardBlock(b *ShardBlock, h Hash) bool {
	s.mu.Lock()
	broadcast := false
	if _, ok := s.shardBlocks[h]; !ok {
		s.shardBlocks[h] = b
		broadcast = true
	}
	s.mu.Unlock()
	return broadcast
}

func (s *storage) AddShardBlockProposal(bp *ShardBlockProposal, h Hash) bool {
	s.mu.Lock()
	if _, ok := s.shardBlockProposals[h]; ok {
		s.mu.Unlock()
		return false
	}
	s.shardBlockProposals[h] = bp
	s.mu.Unlock()
	return true
}

func (s *storage) AddRandBeaconSig(r *RandBeaconSig, round uint64) {
	s.mu.Lock()
	s.randBeaconSigs[round] = r
	s.mu.Unlock()
}

func (s *storage) Block(h Hash) *Block {
	s.mu.Lock()
	b := s.blocks[h]
	s.mu.Unlock()
	return b
}

func (s *storage) ShardBlock(h Hash) *ShardBlock {
	s.mu.Lock()
	b := s.shardBlocks[h]
	s.mu.Unlock()
	return b
}

func (s *storage) ShardBlockProposal(h Hash) *ShardBlockProposal {
	s.mu.Lock()
	b := s.shardBlockProposals[h]
	s.mu.Unlock()
	return b
}

func (s *storage) RandBeaconSig(round uint64) *RandBeaconSig {
	s.mu.Lock()
	b := s.randBeaconSigs[round]
	s.mu.Unlock()
	return b
}
