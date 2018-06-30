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
	mu                          sync.Mutex
	blocks                      map[Hash]*Block
	blockProposals              map[Hash]*BlockProposal
	randBeaconSigs              map[uint64]*RandBeaconSig
	lastBlockRound              uint64
	lastRoundBlock              map[Hash]*Block
	lastBPRound                 uint64
	lastRoundBP                 map[Hash]*BlockProposal
	lastNtShareRound            uint64
	lastRoundNtShare            map[Hash]*NtShare
	lastRandBeaconSigRound      uint64
	lastRoundRandBeaconSig      map[Hash]*RandBeaconSig
	lastRandBeaconSigShareRound uint64
	lastRoundRandBeaconSigShare map[Hash]*RandBeaconSigShare
}

func newStorage() *storage {
	return &storage{
		blocks:                      make(map[Hash]*Block),
		blockProposals:              make(map[Hash]*BlockProposal),
		randBeaconSigs:              make(map[uint64]*RandBeaconSig),
		lastRoundBlock:              make(map[Hash]*Block),
		lastRoundBP:                 make(map[Hash]*BlockProposal),
		lastRoundNtShare:            make(map[Hash]*NtShare),
		lastRoundRandBeaconSig:      make(map[Hash]*RandBeaconSig),
		lastRoundRandBeaconSigShare: make(map[Hash]*RandBeaconSigShare),
	}
}

func (s *storage) AddBlock(b *Block, h Hash) bool {
	s.mu.Lock()
	broadcast := false
	if _, ok := s.blocks[h]; !ok {
		s.blocks[h] = b
		broadcast = true
	}

	s.keepLastRoundBlock(b, h)
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

	s.keepLastRoundBlockProposal(bp, h)
	s.mu.Unlock()
	return true
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

func (s *storage) keepLastRoundBlock(b *Block, h Hash) {
	if b.Round < s.lastBlockRound {
		return
	}

	if b.Round > s.lastBlockRound {
		for k := range s.lastRoundBlock {
			delete(s.lastRoundBlock, k)
		}
		s.lastBlockRound = b.Round
	}

	s.lastRoundBlock[h] = b
}

func (s *storage) LastRoundBlocks() []*Block {
	s.mu.Lock()
	r := make([]*Block, len(s.lastRoundBlock))
	i := 0
	for _, b := range s.lastRoundBlock {
		r[i] = b
		i++
	}
	s.mu.Unlock()
	return r
}

func (s *storage) keepLastRoundBlockProposal(b *BlockProposal, h Hash) {
	if b.Round < s.lastBPRound {
		return
	}

	if b.Round > s.lastBPRound {
		for k := range s.lastRoundBP {
			delete(s.lastRoundBP, k)
		}
		s.lastBPRound = b.Round
	}

	s.lastRoundBP[h] = b
}

func (s *storage) LastRoundBlockProposals() []*BlockProposal {
	s.mu.Lock()
	r := make([]*BlockProposal, len(s.lastRoundBP))
	i := 0
	for _, b := range s.lastRoundBP {
		r[i] = b
		i++
	}
	s.mu.Unlock()
	return r
}

func (s *storage) KeepLastRoundNtShare(b *NtShare, h Hash) {
	s.mu.Lock()
	if b.Round < s.lastNtShareRound {
		s.mu.Unlock()
		return
	}

	if b.Round > s.lastNtShareRound {
		for k := range s.lastRoundNtShare {
			delete(s.lastRoundNtShare, k)
		}
		s.lastNtShareRound = b.Round
	}

	s.lastRoundNtShare[h] = b
	s.mu.Unlock()
}

func (s *storage) LastRoundNtShares() []*NtShare {
	s.mu.Lock()
	r := make([]*NtShare, len(s.lastRoundNtShare))
	i := 0
	for _, b := range s.lastRoundNtShare {
		r[i] = b
		i++
	}
	s.mu.Unlock()
	return r
}

func (s *storage) KeepLastRoundRandBeaconSigShare(b *RandBeaconSigShare) {
	h := b.Hash()
	s.mu.Lock()
	if b.Round < s.lastRandBeaconSigShareRound {
		s.mu.Unlock()
		return
	}

	if b.Round > s.lastRandBeaconSigShareRound {
		for k := range s.lastRoundRandBeaconSigShare {
			delete(s.lastRoundRandBeaconSigShare, k)
		}
		s.lastRandBeaconSigShareRound = b.Round
	}

	s.lastRoundRandBeaconSigShare[h] = b
	s.mu.Unlock()
}

func (s *storage) LastRoundRandBeaconSigShares() []*RandBeaconSigShare {
	s.mu.Lock()
	r := make([]*RandBeaconSigShare, len(s.lastRoundRandBeaconSigShare))
	i := 0
	for _, b := range s.lastRoundRandBeaconSigShare {
		r[i] = b
		i++
	}
	s.mu.Unlock()
	return r
}
