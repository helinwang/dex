package consensus

import (
	"errors"
	"sync"
	"time"

	log "github.com/helinwang/log15"
)

// RandomBeacon is the round information.
//
// The random beacon, block proposal, block notarization advance to
// the next round in lockstep.
type RandomBeacon struct {
	cfg               Config
	mu                sync.Mutex
	nextRBCmteHistory []int
	nextNtCmteHistory []int
	nextBPCmteHistory []int
	nextBPRandHistory []Rand
	groups            []*Group

	rbRand Rand
	ntRand Rand
	bpRand Rand

	curRoundShares map[Hash]*RandBeaconSigShare
	curRoundItems  []ItemID
	sigHistory     []*RandBeaconSig
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
		nextRBCmteHistory: []int{initRBGroup},
		nextNtCmteHistory: []int{initNtGroup},
		nextBPCmteHistory: []int{initBPGroup},
		nextBPRandHistory: []Rand{bpRand},
		curRoundShares:    make(map[Hash]*RandBeaconSigShare),
		sigHistory: []*RandBeaconSig{
			{Sig: []byte("DEX random beacon 0th signature")},
		},
	}
}

// GetShare returns the randome beacon signature share of the current
// round.
func (r *RandomBeacon) GetShare(h Hash) *RandBeaconSigShare {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.curRoundShares[h]
}

// AddRandBeaconSigShare receives one share of the random beacon
// signature.
func (r *RandomBeacon) AddRandBeaconSigShare(s *RandBeaconSigShare, groupID int) (*RandBeaconSig, bool) {
	log.Info("AddRandBeaconSigShare", "groupID", groupID, "hash", s.Hash())
	r.mu.Lock()
	defer r.mu.Unlock()

	if depth := r.depth(); depth != s.Round {
		if s.Round > depth {
			log.Warn("failed to add RandBeaconSigShare that has bigger round than depth", "round", s.Round, "depth", depth)
			return nil, false
		}

		log.Debug("skipped the RandBeaconSigShare that has smaller round than depth", "round", s.Round, "depth", depth)
		return nil, true
	}

	if _, ok := r.curRoundShares[s.Hash()]; ok {
		return nil, false
	}

	r.curRoundShares[s.Hash()] = s
	r.curRoundItems = append(r.curRoundItems, ItemID{T: RandBeaconShareItem, Hash: s.Hash(), ItemRound: s.Round})
	if len(r.curRoundShares) >= r.cfg.GroupThreshold {
		sig, err := recoverRandBeaconSig(r.curRoundShares)
		if err != nil {
			log.Error("fatal: recoverRandBeaconSig error", "err", err)
			return nil, false
		}

		// TODO: get last sig hash locally
		msg := randBeaconSigMsg(s.Round, s.LastSigHash)
		if !sig.Verify(&r.groups[groupID].PK, string(msg)) {
			panic("impossible: random beacon group signature verification failed")
		}

		var rbs RandBeaconSig
		rbs.Round = s.Round
		rbs.LastSigHash = s.LastSigHash
		rbs.Sig = sig.Serialize()
		return &rbs, true
	}
	return nil, true
}

// AddRandBeaconSig adds the random beacon signature.
func (r *RandomBeacon) AddRandBeaconSig(s *RandBeaconSig) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	log.Info("AddRandBeaconSig", "round", s.Round, "hash", s.Hash())

	if depth := r.depth(); depth != s.Round {
		if s.Round > depth {
			log.Warn("adding RandBeaconSig of higher round", "round", s.Round, "beacon depth", depth)
			return false
		}

		log.Debug("skipped RandBeaconSig of lower round", "round", s.Round, "beacon depth", depth)
		// still treat as success
		return true
	}

	r.deriveRand(SHA3(s.Sig))
	r.curRoundShares = make(map[Hash]*RandBeaconSigShare)
	r.curRoundItems = nil
	r.sigHistory = append(r.sigHistory, s)
	return true
}

func (r *RandomBeacon) Inventory() []ItemID {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.curRoundItems) == 0 {
		return nil
	}

	ret := make([]ItemID, len(r.curRoundItems))
	copy(ret, r.curRoundItems)
	return ret
}

func (r *RandomBeacon) depth() uint64 {
	return uint64(len(r.sigHistory))
}

// Depth returns the depth of the random beacon.
//
// This round will be always greater or equal to Chain.Round():
// - greater: when the node is synchronizing. It will synchronize the
// random beacon first, and then synchronize the chain's blocks.
// - equal: when the node is synchronized.
func (r *RandomBeacon) Depth() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.depth()
}

// Rank returns the rank for the given member in the current block
// proposal committee.
func (r *RandomBeacon) Rank(addr Addr, round uint64) (int, error) {
	if round < 1 {
		panic("should not happen")
	}

	r.mu.Lock()
	i := round - 1
	for i >= uint64(len(r.nextBPCmteHistory)) {
		r.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		r.mu.Lock()
	}
	bp := r.nextBPCmteHistory[i]
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
		return 0, errors.New("addr not in the current block proposal committee")
	}

	perm := r.nextBPRandHistory[i].Perm(idx+1, len(g.Members))
	r.mu.Unlock()
	return perm[idx], nil
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
	if round < 1 {
		panic("should not happen")
	}
	r.mu.Lock()

	idx := round - 1
	for idx >= uint64(len(r.nextRBCmteHistory)) {
		r.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		r.mu.Lock()
	}

	// TODO: check idx
	rb = r.nextRBCmteHistory[idx]
	bp = r.nextBPCmteHistory[idx]
	nt = r.nextNtCmteHistory[idx]
	r.mu.Unlock()
	return
}

// History returns the random beacon signature history.
func (r *RandomBeacon) History() []*RandBeaconSig {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.sigHistory
}
