package consensus

// Transition is the transition from one State to another State.
type Transition interface {
	// Apply applies a transation to the state transition.
	//
	// returns true on success. The transition will not change if
	// false is returned.
	Apply(txn []byte) bool
	Commit() State
}

// State is the blockchain state.
type State interface {
	Hash() Hash
	Transition() Transition
}

// StateStore stores all unfinalized States and the last finalized
// State.
type StateStore interface {
	Add(State)
	Finalize(State)
	Dead(State)
}
