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
	mu             sync.Mutex
	blocks         map[Hash]*Block
	blockProposals map[Hash]*BlockProposal
	randBeaconSigs map[uint64]*RandBeaconSig
}

func newStorage() *storage {
	return &storage{
		blocks:         make(map[Hash]*Block),
		blockProposals: make(map[Hash]*BlockProposal),
		randBeaconSigs: make(map[uint64]*RandBeaconSig),
	}
}

func (s *storage) AddBlock(b *Block, h Hash) bool {
	s.mu.Lock()
	broadcast := false
	if _, ok := s.blocks[h]; !ok {
		s.blocks[h] = b
		broadcast = true
	}
	s.mu.Unlock()
	return broadcast
}

func (s *storage) AddBlockProposal(bp *BlockProposal, h Hash) bool {
	s.mu.Lock()
	if _, ok := s.blockProposals[h]; ok {
		s.mu.Unlock()
		return false
	}
	s.blockProposals[h] = bp
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

func (s *storage) BlockProposal(h Hash) *BlockProposal {
	s.mu.Lock()
	b := s.blockProposals[h]
	s.mu.Unlock()
	return b
}

func (s *storage) RandBeaconSig(round uint64) *RandBeaconSig {
	s.mu.Lock()
	b := s.randBeaconSigs[round]
	s.mu.Unlock()
	return b
}
