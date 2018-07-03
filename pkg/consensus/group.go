package consensus

// group is a sample of all the nodes in the consensus infrastructure.
//
// group can perform different roles:
// - random beacon group
// - block proposal group
// - notarization group
type group struct {
	Members  []Addr
	MemberPK map[Addr]PK
	PK       PK
}

// newGroup creates a new group.
func newGroup(pk PK) *group {
	return &group{
		PK:       pk,
		MemberPK: make(map[Addr]PK),
	}
}
