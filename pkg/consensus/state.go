package consensus

// Transition is the transition from one State to another State.
type Transition interface {
	// Apply applies a transition to the state transition.
	//
	// returns true on success. The transition will not change if
	// false is returned.
	Apply(txn []byte) (valid, future bool)

	// Clear clears the accumulated transactions.
	Clear() [][]byte

	// Encode encodes the state transition, used to generate the
	// block proposal.
	Encode() []byte
}

// State is the blockchain state.
type State interface {
	Hash() Hash
	Transition() Transition
}
