package consensus

// Group is a sample of all the nodes in the consensus infrastructure.
//
// Group can perform different roles:
// - random beacon committe
// - notarization committe
type Group struct {
	Members  []Addr
	MemberPK map[Addr]PK
	PK       PK
}

// NewGroup creates a new group.
func NewGroup(pk PK) *Group {
	return &Group{
		PK:       pk,
		MemberPK: make(map[Addr]PK),
	}
}
