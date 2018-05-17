package consensus

import (
	"github.com/dfinity/go-dfinity-crypto/bls"
)

const (
	groupSize      = 401
	groupThreshold = 200
)

// Group is a sample of all the nodes in the consensus infrastructure.
//
// Group can perform different roles:
// - random beacon committe
// - notarization committe
type Group struct {
	MemberPK   map[Addr]bls.PublicKey
	MemberVVec map[Addr]bls.SecretKey
	PK         bls.PublicKey
}

// NewGroup creates a new group.
func NewGroup(pk bls.PublicKey) *Group {
	return &Group{
		PK:         pk,
		MemberPK:   make(map[Addr]bls.PublicKey),
		MemberVVec: make(map[Addr]bls.SecretKey),
	}
}
