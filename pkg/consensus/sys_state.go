package consensus

import (
	"bytes"
	"encoding/gob"
	"errors"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// SysState is the system state, the system state can be changed by
// the SysTxn of each block.
type SysState struct {
	nodeIDToPK map[int]bls.PublicKey
	addrToPK   map[Addr]bls.PublicKey
	idToGroup  map[int]*Group
	groups     []*Group
}

// NewSysState creates a new system state.
func NewSysState() *SysState {
	return &SysState{
		nodeIDToPK: make(map[int]bls.PublicKey),
		addrToPK:   make(map[Addr]bls.PublicKey),
		idToGroup:  make(map[int]*Group),
	}
}

// SysTransition is the system transition used to change the system
// state.
type SysTransition struct {
	s    *SysState
	txns []SysTxn
}

// Record records the transaction as part of the transition.
func (s *SysTransition) Record(txn SysTxn) bool {
	s.txns = append(s.txns, txn)
	return true
}

// Txns returns the recorded transactions.
func (s *SysTransition) Txns() []SysTxn {
	return s.txns
}

// Apply applies the recorded transactions and creates a new system
// state.
func (s *SysTransition) Apply() *SysState {
	// TODO: this is assuming that there will be no more sys txn
	// after genesis. This is not true after we support open
	// participation though DKG.
	err := s.s.applySysTxns(s.txns)
	if err != nil {
		// TODO: handle error when open participation is
		// supported.
		panic(err)
	}

	return s.s
}

// Clear clears the recorded transactions.
func (s *SysTransition) Clear() *SysState {
	return nil
}

// Transition returns the system state transition
func (s *SysState) Transition() *SysTransition {
	return &SysTransition{s: s}
}

// Finalized marks the system state as finalized.
//
// System state references its parent state, only records the diff
// state, to save memory. All finalized system state execept the last
// one will never be used, so they can be squashed into the last
// finalized system state.
func (s *SysState) Finalized() {
	// TODO
}

func (s *SysState) applyReadyJoinGroup(t ReadyJoinGroupTxn) error {
	var pk bls.PublicKey
	err := pk.Deserialize(t.PK)
	if err != nil {
		return err
	}

	addr := SHA3(pk.Serialize()).Addr()
	s.nodeIDToPK[t.ID] = pk
	s.addrToPK[addr] = pk
	return nil
}

func (s *SysState) applyRegGroup(t RegGroupTxn) error {
	var pk bls.PublicKey
	err := pk.Deserialize(t.PK)
	if err != nil {
		return err
	}

	g := NewGroup(pk)
	for _, id := range t.MemberIDs {
		pk, ok := s.nodeIDToPK[id]
		if !ok {
			return errors.New("node not found")
		}

		addr := SHA3(pk.Serialize()).Addr()
		g.Members = append(g.Members, addr)
	}

	// TODO: parse vvec

	s.idToGroup[t.ID] = g
	return nil
}

func (s *SysState) applyListGroups(t ListGroupsTxn) error {
	gs := make([]*Group, len(t.GroupIDs))
	for i, id := range t.GroupIDs {
		g, ok := s.idToGroup[id]
		if !ok {
			return errors.New("group not found")
		}
		gs[i] = g
	}
	s.groups = gs
	return nil
}

// nolint: gocyclo
func (s *SysState) applySysTxns(txns []SysTxn) error {
	for _, txn := range txns {
		// TODO: check signature, endorsement proof, etc.
		dec := gob.NewDecoder(bytes.NewReader(txn.Data))
		switch txn.Type {
		case ReadyJoinGroup:
			var t ReadyJoinGroupTxn
			err := dec.Decode(&t)
			if err != nil {
				return err
			}

			err = s.applyReadyJoinGroup(t)
			if err != nil {
				return err
			}
		case RegGroup:
			var t RegGroupTxn
			err := dec.Decode(&t)
			if err != nil {
				return err
			}

			err = s.applyRegGroup(t)
			if err != nil {
				return err
			}
		case ListGroups:
			var t ListGroupsTxn
			err := dec.Decode(&t)
			if err != nil {
				return err
			}

			err = s.applyListGroups(t)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
