package consensus

import "errors"

// StateStore stores all unfinalized states and the last finalized
// state.
type StateStore struct {
}

// Add adds a new state.
func (s *StateStore) Add(State) {
}

// Finalize marks a state as finalized.
func (s *StateStore) Finalize(State) {
}

// Dead marks a state as dead.
func (s *StateStore) Dead(State) {
}

// Get gets a state given the state hash.
func (s *StateStore) Get(Hash) (State, error) {
	return nil, errors.New("not implemented")
}
