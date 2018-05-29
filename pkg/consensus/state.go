package consensus

// Transition is the transition from one State to another State.
type Transition interface {
	// Record records a transition to the state transition.
	Record(txn []byte) (valid, success bool)

	// Txns returns the recorded transactions.
	Txns() [][]byte

	// Commit commits the transition to the state root.
	Commit()
}

// State is the blockchain state.
type State interface {
	Accounts() Hash
	Tokens() Hash
	PendingOrders() Hash
	Reports() Hash
	MatchOrders()
	Transition() Transition
}
