package consensus

import (
	"fmt"
)

// validator validates the data received from peers.
type validator struct {
	chain *Chain
}

func newValidator(chain *Chain) *validator {
	return &validator{chain: chain}
}

func (v *validator) ValidateBlock(b *Block) (float64, error) {
	// TODO: validate sig, validate txns, validate sig
	return 0, nil
}

func (v *validator) ValidateBlockProposal(bp *BlockProposal) (float64, error) {
	// TODO: validate sig, validate txns, validate owner, validate
	// round is correct
	round := v.chain.RandomBeacon.Depth() - 1
	if bp.Round != round {
		return 0, fmt.Errorf("round not match, round: %d, expected: %d", bp.Round, round)
	}
	return 0, nil
}

func (v *validator) ValidateNtShare(n *NtShare) (int, error) {
	round := v.chain.Round()
	if n.Round != round {
		return 0, fmt.Errorf("round not match, round: %d, expected: %d", n.Round, round)
	}

	_, _, nt := v.chain.RandomBeacon.Committees(round)
	// TODO: validate sig, validate owner, validate round is
	// correct, validate share is signed correctly.
	return nt, nil
}

func (v *validator) ValidateRandBeaconSig(r *RandBeaconSig) error {
	// TODO: validate sig, owner, round, share
	round := v.chain.RandomBeacon.Depth()
	if r.Round != round {
		return fmt.Errorf("round not match, round: %d, expected: %d", r.Round, round)
	}

	return nil
}

func (v *validator) ValidateRandBeaconSigShare(r *RandBeaconSigShare) (int, error) {
	round := v.chain.RandomBeacon.Depth()
	if r.Round != round {
		return 0, fmt.Errorf("round not match, round: %d, expected: %d", r.Round, round)
	}

	rb, _, _ := v.chain.RandomBeacon.Committees(round)
	// TODO: validate sig, owner, round
	return rb, nil
}
