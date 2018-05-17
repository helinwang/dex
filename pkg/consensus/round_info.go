package consensus

import (
	"errors"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

var errCommitteeNotSelected = errors.New("committee not selected yet")
var errAddrNotInCommittee = errors.New("addr not in committee")

// RoundInfo is the round information.
//
// The random beacon, block proposal, block notarization advance to
// the next round in lockstep.
type RoundInfo struct {
	Round             int
	nextRBCmteHistory []int
	nextNtCmteHistory []int
	nextBPCmteHistory []int
	groups            []*Group

	rbRand Rand
	ntRand Rand
	bpRand Rand
}

// NewRoundInfo creates a new round info.
func NewRoundInfo(seed Rand) *RoundInfo {
	r := &RoundInfo{}
	r.rbRand = seed.Derive([]byte("random beacon committee rand seed"))
	r.bpRand = seed.Derive([]byte("block maker committee rand seed"))
	r.ntRand = seed.Derive([]byte("notarization committee rand seed"))
	return r
}

// Advance advances round info into the next round.
func (r *RoundInfo) Advance(h Hash) {
	r.rbRand = r.rbRand.Derive(h[:])
	r.nextRBCmteHistory = append(r.nextRBCmteHistory, r.rbRand.Mod(len(r.groups)))
	r.ntRand = r.ntRand.Derive(h[:])
	r.nextNtCmteHistory = append(r.nextNtCmteHistory, r.ntRand.Mod(len(r.groups)))
	r.bpRand = r.bpRand.Derive(h[:])
	r.nextBPCmteHistory = append(r.nextBPCmteHistory, r.bpRand.Mod(len(r.groups)))
}

// BlockMakerPK returns the public key of the given block maker
func (r *RoundInfo) BlockMakerPK(addr Addr, round int) (bls.PublicKey, error) {
	idx := round - 1
	if idx >= len(r.nextBPCmteHistory) {
		return bls.PublicKey{}, errCommitteeNotSelected
	}

	g := r.groups[r.nextBPCmteHistory[idx]]
	pk, ok := g.MemberPK[addr]
	if !ok {
		return bls.PublicKey{}, errAddrNotInCommittee
	}

	return pk, nil
}

var errInvalidSig = errors.New("invalid signature")
var errRoundTooBig = errors.New("round too big")

// RecvNt handles the newly received notarization.
//
// return true if entered into next round.
func (r *RoundInfo) RecvNt(nt *Notarization) error {
	idx := nt.Round - 1
	if idx >= len(r.nextNtCmteHistory) {
		return errRoundTooBig
	}

	pk := r.groups[r.nextNtCmteHistory[idx]].PK
	if !verifySig(pk, nt.GroupSig, nt.Encode(false)) {
		return errInvalidSig
	}

	if nt.Round == r.Round {
		// enter next round
		r.Round++
	}

	// TODO: broadcast

	return nil
}

// ReceiveRandBeaconSig handles the newly received random value.
func (r *RoundInfo) ReceiveRandBeaconSig(rs *RandBeaconSig) error {
	idx := rs.Round - 1
	if idx >= len(r.nextRBCmteHistory) {
		// TODO: handle the case that random beacon sigature
		// received before the notarization.
		return errRoundTooBig
	}

	// rs.Round is the value for the new round, use the old round
	// number to get the group index.
	pk := r.groups[r.nextRBCmteHistory[idx]].PK
	if !verifySig(pk, rs.Sig, rs.Encode(false)) {
		return errInvalidSig
	}

	if rs.Round == r.Round {
		// select new groups
		h := hash(rs.Encode(true))
		r.Advance(h)
		return nil
	}

	return nil
}
