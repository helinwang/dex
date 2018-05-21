package consensus

import (
	"errors"
	"fmt"
	"sync"
)

var errCommitteeNotSelected = errors.New("committee not selected yet")
var errAddrNotInCommittee = errors.New("addr not in committee")

// RandomBeacon is the round information.
//
// The random beacon, block proposal, block notarization advance to
// the next round in lockstep.
type RandomBeacon struct {
	mu                sync.Mutex
	nextRBCmteHistory []int
	nextNtCmteHistory []int
	nextBPCmteHistory []int
	groups            []*Group
	sigHash           Hash

	rbRand Rand
	ntRand Rand
	bpRand Rand

	curRoundShares []*RandBeaconSigShare
}

// NewRandomBeacon creates a new random beacon
func NewRandomBeacon(seed Rand, groups []*Group) *RandomBeacon {
	rbRand := seed.Derive([]byte("random beacon committee rand seed"))
	bpRand := seed.Derive([]byte("block proposer committee rand seed"))
	ntRand := seed.Derive([]byte("notarization committee rand seed"))
	return &RandomBeacon{
		groups:            groups,
		rbRand:            rbRand,
		bpRand:            bpRand,
		ntRand:            ntRand,
		sigHash:           Hash(seed),
		nextRBCmteHistory: []int{rbRand.Mod(len(groups))},
		nextNtCmteHistory: []int{ntRand.Mod(len(groups))},
		nextBPCmteHistory: []int{bpRand.Mod(len(groups))},
	}
}

// RecvRandBeaconSigShare receives one share of the random beacon
// signature.
func (r *RandomBeacon) RecvRandBeaconSigShare(s *RandBeaconSigShare, groupID int) (*RandBeaconSig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.randRound() != s.Round {
		return nil, fmt.Errorf("unexpected RandBeaconSigShare.Round: %d, expected: %d", s.Round, r.randRound())
	}

	if r.sigHash != s.LastSigHash {
		return nil, fmt.Errorf("unexpected RandBeaconSigShare.LastSigHash: %x, expected: %x", s.LastSigHash, r.sigHash)
	}

	r.curRoundShares = append(r.curRoundShares, s)
	if len(r.curRoundShares) >= groupThreshold {
		sig := recoverRandBeaconSig(r.curRoundShares)
		var rbs RandBeaconSig
		rbs.LastRandVal = s.LastSigHash
		rbs.Round = s.Round
		msg := rbs.Encode(false)
		if !sig.Verify(&r.groups[groupID].PK, string(msg)) {
			panic("impossible: random beacon group signature verification failed")
		}

		rbs.Sig = sig.Serialize()
		return &rbs, nil
	}
	return nil, nil
}

// RecvRandBeaconSig adds the random beacon signature.
func (r *RandomBeacon) RecvRandBeaconSig(s *RandBeaconSig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.randRound() != s.Round {
		return fmt.Errorf("unexpected RandBeaconSig round: %d, expected: %d", s.Round, r.randRound())
	}

	r.deriveRand(hash(s.Sig))
	r.curRoundShares = nil
	return nil
}

func (r *RandomBeacon) randRound() int {
	return len(r.nextRBCmteHistory)
}

func (r *RandomBeacon) deriveRand(h Hash) {
	r.rbRand = r.rbRand.Derive(h[:])
	r.nextRBCmteHistory = append(r.nextRBCmteHistory, r.rbRand.Mod(len(r.groups)))
	r.ntRand = r.ntRand.Derive(h[:])
	r.nextNtCmteHistory = append(r.nextNtCmteHistory, r.ntRand.Mod(len(r.groups)))
	r.bpRand = r.bpRand.Derive(h[:])
	r.nextBPCmteHistory = append(r.nextBPCmteHistory, r.bpRand.Mod(len(r.groups)))
}
