package consensus

// Transition is the transition from one State to another State.
type Transition interface {
	// Apply applies a transation to the state transition.
	//
	// returns true on success. The transition will not change if
	// false is returned.
	Apply(txn []byte) (valid, future bool)
	Commit() State
}

// State is the blockchain state.
type State interface {
	Hash() Hash
	Transition() Transition
}
