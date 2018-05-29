package consensus

// Transition is the transition from one State to another State.
type Transition interface {
	// Record records a transition to the state transition.
	Record(txn []byte) (valid, success bool)

	// Clear clears the accumulated transactions.
	Clear() [][]byte

	// Commit commits the transition to the state root.
	Commit()
}

// State is the blockchain state.
type State interface {
	Accounts() Hash
	Tokens() Hash
	PendingOrders() Hash
	Reports() Hash
	Transition() Transition
}
