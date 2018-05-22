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
	sigHistory     []*RandBeaconSig
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
		sigHistory: []*RandBeaconSig{
			{Sig: []byte("DEX random beacon 0th signature")},
		},
	}
}

// RecvRandBeaconSigShare receives one share of the random beacon
// signature.
func (r *RandomBeacon) RecvRandBeaconSigShare(s *RandBeaconSigShare, groupID int) (*RandBeaconSig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.round() != s.Round {
		return nil, fmt.Errorf("unexpected RandBeaconSigShare.Round: %d, expected: %d", s.Round, r.round())
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

	if r.round() != s.Round {
		return fmt.Errorf("unexpected RandBeaconSig round: %d, expected: %d", s.Round, r.round())
	}

	r.deriveRand(hash(s.Sig))
	r.curRoundShares = nil
	r.sigHistory = append(r.sigHistory, s)
	return nil
}

func (r *RandomBeacon) round() int {
	return len(r.sigHistory)
}

// Round returns the round of the random beacon.
//
// This round will be always greater or equal to Chain.Round():
// - greater: when the node is synchronizing. It will synchronize the
// random beacon first, and then synchronize the chain's blocks.
// - equal: when the node is synchronized.
func (r *RandomBeacon) Round() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.round()
}

func (r *RandomBeacon) deriveRand(h Hash) {
	r.rbRand = r.rbRand.Derive(h[:])
	r.nextRBCmteHistory = append(r.nextRBCmteHistory, r.rbRand.Mod(len(r.groups)))
	r.ntRand = r.ntRand.Derive(h[:])
	r.nextNtCmteHistory = append(r.nextNtCmteHistory, r.ntRand.Mod(len(r.groups)))
	r.bpRand = r.bpRand.Derive(h[:])
	r.nextBPCmteHistory = append(r.nextBPCmteHistory, r.bpRand.Mod(len(r.groups)))
}

func (r *RandomBeacon) RandBeaconGroupID() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.nextRBCmteHistory[len(r.nextRBCmteHistory)-1]
}

// History returns the random beacon signature history.
func (r *RandomBeacon) History() []*RandBeaconSig {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.sigHistory
}
