package consensus

import (
	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Group is a sample of all the nodes in the consensus infrastructure.
//
// Group can perform different roles:
// - random beacon committe
// - notarization committe
type Group struct {
	Members  []Addr
	MemberPK map[Addr]bls.PublicKey
	PK       bls.PublicKey
}

// NewGroup creates a new group.
func NewGroup(pk bls.PublicKey) *Group {
	return &Group{
		PK:       pk,
		MemberPK: make(map[Addr]bls.PublicKey),
	}
}
