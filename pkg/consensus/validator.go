package consensus

import (
	"math"

	"github.com/dfinity/go-dfinity-crypto/bls"
	log "github.com/helinwang/log15"
)

// validator validates the data received from peers.
type validator struct {
	chain *Chain
}

func newValidator(chain *Chain) *validator {
	return &validator{chain: chain}
}

func rankToWeight(rank int) float64 {
	if rank < 0 {
		panic(rank)
	}
	return math.Pow(0.5, float64(rank))
}

func (v *validator) ValidateBlock(b *Block) (float64, bool) {
	// TODO: validate txns
	if depth := v.chain.RandomBeacon.Depth(); b.Round > depth {
		// TODO: sync with the sender
		log.Warn("received block of too high round, can't validate", "round", b.Round, "depth", depth)
		return 0, false
	}

	if _, ok := v.chain.Block(b.Hash()); ok {
		log.Warn("block already received")
		return 0, false
	}

	prev, ok := v.chain.Block(b.PrevBlock)
	if !ok {
		log.Warn("ValidateBlock: prev block not found")
		return 0, false
	}

	if prev.Round != b.Round-1 {
		log.Warn("ValidateBlock: prev block round is not block round - 1", "prev round", prev.Round, "round", b.Round)
		return 0, false
	}

	var sign bls.Sign
	err := sign.Deserialize(b.NotarizationSig)
	if err != nil {
		log.Warn("validate block sig error", "err", err)
		return 0, false
	}

	msg := string(b.Encode(false))
	_, _, nt := v.chain.RandomBeacon.Committees(b.Round)
	success := sign.Verify(&v.chain.RandomBeacon.groups[nt].PK, msg)
	if !success {
		log.Warn("validate block group sig failed", "group", nt, "block", b.Hash())
		return 0, false
	}

	rank, err := v.chain.RandomBeacon.Rank(b.Owner, b.Round)
	if err != nil {
		log.Error("error get rank, but group sig is valid", "err", err)
		return 0, false
	}

	return rankToWeight(rank), true
}

func (v *validator) ValidateBlockProposal(bp *BlockProposal) (float64, bool) {
	// TODO: validate txns
	round := v.chain.Round()
	if bp.Round != round {
		if bp.Round > round {
			log.Warn("received block proposal of higher round", "round", bp.Round, "my round", round)
		} else {
			log.Debug("received block proposal of lower round", "round", bp.Round, "my round", round)
		}

		return 0, false
	}

	if _, ok := v.chain.BlockProposal(bp.Hash()); ok {
		log.Warn("block proposal already received")
		return 0, false
	}

	prev, ok := v.chain.Block(bp.PrevBlock)
	if !ok {
		log.Warn("ValidateBlockProposal: prev block not found")
		return 0, false
	}

	if prev.Round != bp.Round-1 {
		log.Warn("ValidateBlockProposal: prev block round is not block proposal round - 1", "prev round", prev.Round, "round", bp.Round)
		return 0, false
	}

	rank, err := v.chain.RandomBeacon.Rank(bp.Owner, bp.Round)
	if err != nil {
		log.Warn("error get rank", "err", err)
		return 0, false
	}

	var sign bls.Sign
	err = sign.Deserialize(bp.OwnerSig)
	if err != nil {
		log.Error("error recover block proposal signature", "err", err)
		return 0, false
	}

	pk, ok := v.chain.LastFinalizedSysState.addrToPK[bp.Owner]
	if !ok {
		log.Warn("block proposal owner not found", "owner", bp.Owner)
		return 0, false
	}

	if !sign.Verify(&pk, string(bp.Encode(false))) {
		log.Warn("invalid block proposal signature", "block", bp.Hash())
		return 0, false
	}

	return rankToWeight(rank), true
}

func (v *validator) ValidateNtShare(n *NtShare) (int, bool) {
	round := v.chain.Round()
	if n.Round != round {
		if n.Round > round {
			log.Warn("received nt share of higher round", "round", n.Round, "my round", round)
		} else {
			log.Debug("received nt share of lower round", "round", n.Round, "my round", round)
		}
		return 0, false
	}

	if _, ok := v.chain.NtShare(n.Hash()); ok {
		log.Warn("notarization share already received")
		return 0, false
	}

	bp, ok := v.chain.BlockProposal(n.BP)
	if !ok {
		log.Warn("ValidateNtShare: prev block not found")
		return 0, false
	}

	if bp.Round != n.Round {
		log.Warn("ValidateNtShare: notarization is in different round with block proposal", "bp round", bp.Round, "round", n.Round)
		return 0, false
	}

	_, _, nt := v.chain.RandomBeacon.Committees(round)
	group := v.chain.RandomBeacon.groups[nt]
	if !group.MemberExists[n.Owner] {
		log.Warn("ValidateNtShare: nt owner not a member of the nt cmte")
		return 0, false
	}

	var sign bls.Sign
	err := sign.Deserialize(n.OwnerSig)
	if err != nil {
		log.Warn("valid nt sig error", "err", err)
		return 0, false
	}

	pk, ok := v.chain.LastFinalizedSysState.addrToPK[n.Owner]
	if !ok {
		log.Warn("nt owner not found", "owner", n.Owner)
		return 0, false
	}

	if !sign.Verify(&pk, string(n.Encode(false))) {
		log.Warn("invalid nt signature", "nt", n.Hash())
		return 0, false
	}

	// TODO: validate share signature is valid.
	return nt, true
}

func (v *validator) ValidateRandBeaconSig(r *RandBeaconSig) bool {
	// TODO: validate sig, owner, round, share
	targetDepth := v.chain.RandomBeacon.Depth()
	if r.Round != targetDepth {
		if r.Round > targetDepth {
			log.Warn("received RandBeaconSig of higher round", "round", r.Round, "target depth", targetDepth)
		} else {
			log.Debug("received RandBeaconSig of lower round", "round", r.Round, "target depth", targetDepth)
		}
		return false
	}

	return true
}

func (v *validator) ValidateRandBeaconSigShare(r *RandBeaconSigShare) (int, bool) {
	targetDepth := v.chain.RandomBeacon.Depth()
	if r.Round != targetDepth {
		if r.Round > targetDepth {
			log.Warn("received RandBeaconSigShare of higher round", "round", r.Round, "target depth", targetDepth)
		} else {
			log.Debug("received RandBeaconSigShare of lower round", "round", r.Round, "target depth", targetDepth)
		}
		return 0, false
	}

	rb, _, _ := v.chain.RandomBeacon.Committees(targetDepth)
	group := v.chain.RandomBeacon.groups[rb]
	if !group.MemberExists[r.Owner] {
		log.Warn("ValidateNtShare: nt owner not a member of the nt cmte")
		return 0, false
	}

	var sign bls.Sign
	err := sign.Deserialize(r.OwnerSig)
	if err != nil {
		log.Warn("valid nt sig error", "err", err)
		return 0, false
	}

	pk, ok := v.chain.LastFinalizedSysState.addrToPK[r.Owner]
	if !ok {
		log.Warn("nt owner not found", "owner", r.Owner)
		return 0, false
	}

	if !sign.Verify(&pk, string(r.Encode(false))) {
		log.Warn("invalid rand beacon share signature", "rand beacon share", r.Hash())
		return 0, false
	}

	// TODO: validate share signature is valid.
	return rb, true
}
