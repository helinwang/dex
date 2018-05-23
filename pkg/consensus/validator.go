package consensus

// validator validates the data received from peers.
type validator struct {
	chain *Chain
}

func newValidator(chain *Chain) *validator {
	return &validator{chain: chain}
}

func (v *validator) ValidateBlock(b *Block) (float64, bool) {
	// TODO: validate sig, validate txns, validate sig
	return 0, true
}

func (v *validator) ValidateBlockProposal(bp *BlockProposal) (float64, bool) {
	// TODO: validate sig, validate txns, validate owner, validate
	// round is correct
	round := v.chain.RandomBeacon.Round() - 1
	if bp.Round != round {
		return 0, false
	}
	return 0, true
}

func (v *validator) ValidateNtShare(n *NtShare) (int, bool) {
	round := v.chain.RandomBeacon.Round() - 1
	_, _, nt := v.chain.RandomBeacon.Committees(round)
	// TODO: validate sig, validate owner, validate round is
	// correct, validate share is signed correctly.
	return nt, true
}

func (v *validator) ValidateRandBeaconSig(r *RandBeaconSig) bool {
	// TODO: validate sig, owner, round, share
	return true
}

func (v *validator) ValidateRandBeaconSigShare(r *RandBeaconSigShare) (int, bool) {
	round := v.chain.RandomBeacon.Round()
	rb, _, _ := v.chain.RandomBeacon.Committees(round)
	// TODO: validate sig, owner, round
	return rb, true
}
