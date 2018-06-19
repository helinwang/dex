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
	v.chain.RandomBeacon.WaitUntil(b.Round)
	if b := v.chain.Block(b.Hash()); b != nil {
		log.Warn("block already received")
		return 0, false
	}

	prev := v.chain.Block(b.PrevBlock)
	if prev == nil {
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

	_, _, nt := v.chain.RandomBeacon.Committees(b.Round)
	success := b.NotarizationSig.Verify(v.chain.RandomBeacon.groups[nt].PK, b.Encode(false))
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

// TODO: validator should not check round information, and signature
// validation should be a method of the data type.
func (v *validator) ValidateNtShare(n *NtShare) (int, bool) {
	_, _, nt := v.chain.RandomBeacon.Committees(n.Round)
	group := v.chain.RandomBeacon.groups[nt]
	sharePK, ok := group.MemberPK[n.Owner]
	if !ok {
		log.Warn("ValidateNtShare: nt owner not a member of the nt cmte")
		return 0, false
	}

	pk, ok := v.chain.LastFinalizedSysState.addrToPK[n.Owner]
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
	if r.Round == 0 {
		log.Error("received RandBeaconSig of 0 round, should not happen")
		return 0, false
	}

	round := v.chain.RandomBeacon.Round()
	if round == r.Round {
		panic(r.Round)
	}

	v.chain.RandomBeacon.WaitUntil(r.Round - 1)

	round = v.chain.RandomBeacon.Round()
	if r.Round <= round {
		log.Debug("received RandBeaconSigShare of lower or same round", "round", r.Round, "target depth", round)
		return 0, false
	}

	if h := SHA3(v.chain.RandomBeacon.sigHistory[r.Round-1].Sig); h != r.LastSigHash {
		log.Warn("validate random beacon share last sig error", "hash", r.LastSigHash, "expected", h)
		return 0, false
	}

	rb, _, _ := v.chain.RandomBeacon.Committees(round)
	group := v.chain.RandomBeacon.groups[rb]
	sharePK, ok := group.MemberPK[r.Owner]
	if !ok {
		log.Warn("ValidateRandBeaconSigShare: owner not a member of the rb cmte")
		return 0, false
	}

	pk, ok := v.chain.LastFinalizedSysState.addrToPK[r.Owner]
	if !ok {
		log.Warn("rancom beacon sig shareowner not found", "owner", r.Owner)
		return 0, false
	}

	if !r.OwnerSig.Verify(pk, r.Encode(false)) {
		log.Warn("invalid rand beacon share signature", "rand beacon share", r.Hash())
		return 0, false
	}

	// TODO: validate share signature is valid.
	msg := randBeaconSigMsg(r.Round, r.LastSigHash)
	if !r.Share.Verify(sharePK, msg) {
		log.Warn("validate random beacon sig share error")
		return 0, false
	}

	return rb, true
}
