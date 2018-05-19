package consensus

// validator validates the data received from peers.
type validator struct {
	chain *Chain
}

func newValidator(chain *Chain) *validator {
	return &validator{chain: chain}
}

func (v *validator) ValidateBlock(b *Block) bool {
	return false
}

func (v *validator) ValidateBlockProposal(bp *BlockProposal) bool {
	return false
}

func (v *validator) ValidateNtShare(bp Hash) bool {
	return true
}
