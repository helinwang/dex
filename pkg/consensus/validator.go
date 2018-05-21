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
	return 0, false
}

func (v *validator) ValidateBlockProposal(bp *BlockProposal) (float64, bool) {
	// TODO: validate sig, validate txns, validate owner, validate round is correct
	return 0, false
}

func (v *validator) ValidateNtShare(n *NtShare) (int, bool) {
	// TODO: validate sig, validate owner, validate round is correct
	return 0, true
}

func (v *validator) ValidateRandBeaconSig(r *RandBeaconSig) bool {
	// TODO: validate sig, owner, round
	return true
}

func (v *validator) ValidateRandBeaconSigShare(r *RandBeaconSigShare) bool {
	// TODO: validate sig, owner, round
	return true
}
