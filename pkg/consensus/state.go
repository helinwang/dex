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

// TxnPool is the pool that stores the received transactions.
type TxnPool interface {
	// Add adds a transaction, the transaction pool should
	// validate the txn and return true if the transaction is
	// valid and not already in the pool. The caller should
	// broadcast the transaction if the return value is true.
	Add(txn []byte) (broadcast bool)
	Txns() [][]byte
	Remove(txn []byte)
}
