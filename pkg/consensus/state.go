package consensus

// State is the blockchain state.
type State interface {
	Hash() Hash
	Transition(round uint64) Transition
	Serialize() (TrieBlob, error)
	Deserialize(TrieBlob) error
	CommitCache()
}

// Transition is the transition from one State to another State.
type Transition interface {
	// Record records a transition to the state transition.
	Record(Txn) (valid, success bool)

	// RecordTxns records the serialized transactions.
	RecordTxns([]byte) (valid, success bool)

	// Txns returns the serialized recorded transactions.
	Txns() []byte

	// Commit commits the transition, creating a new state.
	Commit() State

	// StateHash returns the state root hash of the state after
	// applying the transition.
	StateHash() Hash
}

// Txn is a transaction.
type Txn interface {
	Hash() Hash
	Bytes() []byte
}

// TxnPool is the pool that stores the received transactions.
type TxnPool interface {
	// Add adds a transaction, the transaction pool should
	// validate the txn and return true if the transaction is
	// valid and not already in the pool. The caller should
	// broadcast the transaction if the return value is true.
	Add(txn []byte) (h Hash, broadcast bool)
	Get(hash Hash) Txn
	NotSeen(hash Hash) bool
	Txns() []Txn
	Remove(hash Hash)
	// RemoveTxns removes the given encoded transactions argument,
	// returns the number of transactions encoded in that
	// argument.
	RemoveTxns([]byte) int
	Size() int
}
