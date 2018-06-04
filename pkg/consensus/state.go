package consensus

// Transition is the transition from one State to another State.
type Transition interface {
	// Record records a transition to the state transition.
	Record(txn []byte) (valid, success bool)

	// Txns returns the recorded transactions.
	Txns() [][]byte

	// Commit commits the transition, creating a new state.
	Commit() State

	// StateHash returns the state root hash of the state after
	// applying the transition.
	StateHash() Hash

	ApplyTrades([]byte) error
}

// State is the blockchain state.
type State interface {
	Hash() Hash
	MatchOrders() Hash
	Transition() Transition
}

// TxnPool is the pool that stores the received transactions.
type TxnPool interface {
	// Add adds a transaction, the transaction pool should
	// validate the txn and return true if the transaction is
	// valid and not already in the pool. The caller should
	// broadcast the transaction if the return value is true.
	Add(txn []byte) (broadcast bool)
	Get(hash Hash) []byte
	NotSeen(hash Hash) bool
	Txns() [][]byte
	Remove(hash Hash)
}
