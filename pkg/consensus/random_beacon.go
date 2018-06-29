package consensus

import (
	"fmt"
	"sync"

	log "github.com/helinwang/log15"
)

// RandomBeacon is the round information.
//
// The random beacon, block proposal, block notarization advance to
// the next round in lockstep.
type RandomBeacon struct {
	cfg               Config
	n                 *Node
	mu                sync.Mutex
	roundWaitCh       map[uint64]chan struct{}
	nextRBCmteHistory []int
	nextNtCmteHistory []int
	nextBPCmteHistory []int
	nextBPRandHistory []Rand
	groups            []*Group

	rbRand Rand
	ntRand Rand
	bpRand Rand

	sigHistory []*RandBeaconSig
}

// NewRandomBeacon creates a new random beacon
func NewRandomBeacon(seed Rand, groups []*Group, cfg Config) *RandomBeacon {
	rbRand := seed.Derive([]byte("random beacon committee rand seed"))
	bpRand := seed.Derive([]byte("block proposer committee rand seed"))
	ntRand := seed.Derive([]byte("notarization committee rand seed"))
	initRBGroup := 0
	initNtGroup := 0
	initBPGroup := 0

	if len(groups) > 0 {
		initRBGroup = rbRand.Mod(len(groups))
		initNtGroup = ntRand.Mod(len(groups))
		initBPGroup = bpRand.Mod(len(groups))
	}

	return &RandomBeacon{
		cfg:               cfg,
		groups:            groups,
		rbRand:            rbRand,
		bpRand:            bpRand,
		ntRand:            ntRand,
		roundWaitCh:       make(map[uint64]chan struct{}),
		nextRBCmteHistory: []int{initRBGroup},
		nextNtCmteHistory: []int{initNtGroup},
		nextBPCmteHistory: []int{initBPGroup},
		nextBPRandHistory: []Rand{bpRand},
		sigHistory: []*RandBeaconSig{
			{Sig: []byte("DEX random beacon 0th signature")},
		},
	}
}

func (r *RandomBeacon) AddRandBeaconSigShares(shares []*RandBeaconSigShare, groupID int) *RandBeaconSig {
	s := shares[0]
	log.Debug("add random beacon signature shares", "groupID", groupID, "share round", s.Round)
	r.mu.Lock()
	defer r.mu.Unlock()

	if round := r.round(); round+1 != s.Round {
		log.Debug("skipped the RandBeaconSigShare of different round than expected", "round", s.Round, "expected", round+1)
		return nil
	}

	sig, err := recoverRandBeaconSig(shares)
	if err != nil {
		log.Error("fatal: recoverRandBeaconSig error", "err", err)
		return nil
	}

	msg := randBeaconSigMsg(s.Round, s.LastSigHash)
	if !sig.Verify(r.groups[groupID].PK, msg) {
		panic("impossible: random beacon group signature verification failed")
	}

	var rbs RandBeaconSig
	rbs.Round = s.Round
	rbs.LastSigHash = s.LastSigHash
	rbs.Sig = sig
	return &rbs
}

// AddRandBeaconSig adds the random beacon signature.
func (r *RandomBeacon) AddRandBeaconSig(s *RandBeaconSig, syncDone bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Debug("add random beacon signature", "round", s.Round)

	if round := r.round(); round+1 != s.Round {
		if s.Round > round+1 {
			log.Warn("adding RandBeaconSig of higher round", "round", s.Round, "beacon round", round)
			return false
		}

		log.Debug("skipped RandBeaconSig of lower round", "round", s.Round, "beacon round", round)
		// still treat as success
		return true
	}

	r.deriveRand(SHA3(s.Sig))
	r.sigHistory = append(r.sigHistory, s)
	round := r.round()
	if ch, ok := r.roundWaitCh[round]; ok {
		close(ch)
		delete(r.roundWaitCh, round)
	}

	if syncDone {
		go r.n.StartRound(round)
	}
	return true
}

func (r *RandomBeacon) round() uint64 {
	return uint64(len(r.sigHistory) - 1)
}

// Round returns the round of the random beacon.
//
// Comparison of round with Chain.Round():
// - round >= Chain.Round() + 2: when the node is synchronizing. It
// will synchronize the random beacon first, and then synchronize the
// chain's blocks.
// - round >= Chain.Round() && round <= Chain.Round() + 1: when the
// node is synchronized.
// - round < Chain.Round(): random beacon is synchronizing.
func (r *RandomBeacon) Round() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.round()
}

// WaitUntil will return until the given round is reached.
func (r *RandomBeacon) WaitUntil(round uint64) {
	r.mu.Lock()
	curRound := r.round()
	if round <= curRound {
		r.mu.Unlock()
		return
	}

	ch, ok := r.roundWaitCh[round]
	if !ok {
		ch = make(chan struct{}, 0)
		r.roundWaitCh[round] = ch
	}
	r.mu.Unlock()

	<-ch
}

// Rank returns the rank for the given member in the current block
// proposal committee.
func (r *RandomBeacon) Rank(addr Addr, round uint64) (uint16, error) {
	if round < 1 {
		panic("should not happen")
	}

	r.mu.Lock()
	bp := r.nextBPCmteHistory[round]
	g := r.groups[bp]
	idx := -1
	for i := range g.Members {
		if addr == g.Members[i] {
			idx = i
			break
		}
	}

	if idx < 0 {
		r.mu.Unlock()
		return 0, fmt.Errorf("addr %v not in the current block proposal group %d, round: %d", addr, bp, round)
	}

	perm := r.nextBPRandHistory[round].Perm(idx+1, len(g.Members))
	r.mu.Unlock()
	return uint16(perm[idx]), nil
}

func (r *RandomBeacon) deriveRand(h Hash) {
	r.rbRand = r.rbRand.Derive(h[:])
	r.nextRBCmteHistory = append(r.nextRBCmteHistory, r.rbRand.Mod(len(r.groups)))
	r.ntRand = r.ntRand.Derive(h[:])
	r.nextNtCmteHistory = append(r.nextNtCmteHistory, r.ntRand.Mod(len(r.groups)))
	r.bpRand = r.bpRand.Derive(h[:])
	r.nextBPCmteHistory = append(r.nextBPCmteHistory, r.bpRand.Mod(len(r.groups)))
	r.nextBPRandHistory = append(r.nextBPRandHistory, r.bpRand)
}

// Committees returns the current random beacon, block proposal,
// notarization committees.
func (r *RandomBeacon) Committees(round uint64) (rb, bp, nt int) {
	r.mu.Lock()
	rb = r.nextRBCmteHistory[round]
	bp = r.nextBPCmteHistory[round]
	nt = r.nextNtCmteHistory[round]
	r.mu.Unlock()
	return
}

// History returns the random beacon signature history.
func (r *RandomBeacon) History() []*RandBeaconSig {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.sigHistory
}
