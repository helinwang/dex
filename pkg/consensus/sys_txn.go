package consensus

// SysTxnType is the type of a SysTxn.
type SysTxnType int

// System transactions
const (
	ReadyJoinGroup SysTxnType = iota
	RegGroup
	ListGroups
)

// SysTxn is the consensus system transaction.
type SysTxn struct {
	Type SysTxnType
	Data []byte
	Sig  []byte
}

// ReadyJoinGroupTxn registers the node as ready to join group.
//
// The node has to submit a endorsement proof (e.g., proof of coin
// freezed) to registers itself as ready to join groups. Not all nodes
// have to join groups, but node in the notary committe group will
// receive transaction fee when notarizing a block. Group members are
// selected randomly by the random beacon, same as which group is the
// notary committe group.
type ReadyJoinGroupTxn struct {
	ID int
	PK []byte
}

// RegGroupTxn registers a group to the blockchain.
//
// Mebers of a group is selected by the random beacon, they will run a
// distributed key generation (DKG) protocol to generate the group
// public key and the secret key share of each member. If the DKG
// succeeds, a RegGroupTxn transaction will be submitted.
type RegGroupTxn struct {
	ID          int
	PK          []byte
	MemberIDs   []int
	MemberVVecs [][]byte
}

// ListGroupsTxn lists the groups in the current epoch.
//
// Epoch consists of l blocks, l is a constant system
// configuration. Key frame is the first block in an
// epoch. ListGroupsTxn will only appear in a key frame. The block
// proposer is not able to censor the groups because they are selected
// deterministically.
type ListGroupsTxn struct {
	GroupIDs int
}
