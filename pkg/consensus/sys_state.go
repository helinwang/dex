package consensus

// SysState is the system state, the system state can be changed by
// the SysTxn of each block.
type SysState struct {
}

// Transition returns the system state transition
func (s *SysState) Transition() *SysTransition {
	return &SysTransition{}
}

// SysTransition is the system transition used to change the system
// state.
type SysTransition struct {
}

// Record records the transaction as part of the transition.
func (s SysTransition) Record(txn SysTxn) bool {
	return true
}

// Txns returns the recorded transactions.
func (s *SysTransition) Txns() []SysTxn {
	return nil
}

// Apply applies the recorded transactions and creates a new system
// state.
func (s *SysTransition) Apply() *SysState {
	return nil
}
