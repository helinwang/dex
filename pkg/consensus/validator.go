package consensus

import (
	"math"

	log "github.com/helinwang/log15"
)

// validator validates the data received from peers.
type validator struct {
	chain *Chain
	store *storage
}

func newValidator(chain *Chain, store *storage) *validator {
	return &validator{chain: chain}
}

func rankToWeight(rank uint16) float64 {
	if rank < 0 {
		panic(rank)
	}
	return math.Pow(0.5, float64(rank))
}

func (v *validator) ValidateShardBlock(b *ShardBlock) (uint16, bool) {
	// TODO: validate txns
	v.chain.randomBeacon.WaitUntil(b.Round)

	prev := v.store.Block(b.PrevBlock)
	if prev == nil {
		log.Warn("ValidateBlock: prev block not found")
		return 0, false
	}

	if prev.Round != b.Round-1 {
		log.Warn("ValidateBlock: prev block round is not block round - 1", "prev round", prev.Round, "round", b.Round)
		return 0, false
	}

	rank, err := v.chain.randomBeacon.Rank(b.Owner, b.Round)
	if err != nil {
		log.Warn("shard block's proposer is not selected for proposal")
		return 0, false
	}

	_, _, nt := v.chain.randomBeacon.Committees(b.Round)
	success := b.Notarization.Verify(v.chain.randomBeacon.groups[nt].PK, b.Encode(false))
	if !success {
		log.Warn("validate block group sig failed", "group", nt, "block", b.Hash())
		return 0, false
	}

	return rank, true
}

// TODO: validator should not check round information, and signature
// validation should be a method of the data type.
func (v *validator) ValidateNtShare(n *ShardNtShare) (int, bool) {
	_, _, nt := v.chain.randomBeacon.Committees(n.Round)
	group := v.chain.randomBeacon.groups[nt]
	sharePK, ok := group.MemberPK[n.Owner]
	if !ok {
		log.Warn("ValidateNtShare: nt owner not a member of the nt cmte")
		return 0, false
	}

	pk, ok := v.chain.lastFinalizedSysState.addrToPK[n.Owner]
	if !ok {
		log.Warn("nt owner not found", "owner", n.Owner)
		return 0, false
	}

	if !n.Sig.Verify(pk, n.Encode(false)) {
		log.Warn("invalid nt signature", "nt", n.Hash())
		return 0, false
	}

	// TODO: validate share signature is valid.
	_ = sharePK
	return nt, true
}

func (v *validator) ValidateRandBeaconSigShare(r *RandBeaconSigShare) (int, bool) {
	if h := SHA3(v.chain.randomBeacon.sigHistory[r.Round-1].Sig); h != r.LastSigHash {
		log.Warn("validate random beacon share last sig error", "hash", r.LastSigHash, "expected", h)
		return 0, false
	}

	rb, _, _ := v.chain.randomBeacon.Committees(r.Round - 1)
	group := v.chain.randomBeacon.groups[rb]
	sharePK, ok := group.MemberPK[r.Owner]
	if !ok {
		log.Warn("ValidateRandBeaconSigShare: owner not a member of the rb cmte")
		return 0, false
	}

	pk, ok := v.chain.lastFinalizedSysState.addrToPK[r.Owner]
	if !ok {
		log.Warn("rancom beacon sig shareowner not found", "owner", r.Owner)
		return 0, false
	}

	if !r.OwnerSig.Verify(pk, r.Encode(false)) {
		log.Warn("invalid rand beacon share signature", "rand beacon share", r.Hash())
		return 0, false
	}

	// TODO: validate share signature is valid according to the
	// member group public key share
	msg := randBeaconSigMsg(r.Round, r.LastSigHash)
	if !r.Share.Verify(sharePK, msg) {
		log.Warn("validate random beacon sig share error")
		return 0, false
	}

	return rb, true
}
