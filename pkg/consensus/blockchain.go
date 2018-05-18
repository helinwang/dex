package consensus

type unNotarized struct {
	Weight float64
	BP     *BlockProposal

	Parent *notarized
}

type finalized struct {
	Block *Block
	BP    *BlockProposal
}

type notarized struct {
	Block  *Block
	Weight float64

	NtChildren    *notarized
	NonNtChildren *unNotarized

	BP *BlockProposal
}

type leader struct {
	Block    *Block
	State    State
	SysState *SysState
}

// Chain is the blockchain.
type Chain struct {
	// the finalized block burried deep enough becomes part of the
	// history. Its block proposal and state will be discarded to
	// save space.
	History          []*Block
	LastHistoryState State

	// reorg will never happen to the finalized block, we will
	// discard its associated state. The block proposal will not
	// be discarded, so when a new client joins, he can replay the
	// block proposals starting from LastHistoryState, verify the
	// new state root hash against the one stored in the next
	// block.
	Finalized             []*finalized
	LastFinalizedState    State
	LastFinalizedSysState *SysState

	Fork   []*notarized
	Leader *leader
}

// NewChain creates a new chain.
func NewChain(genesis *Block, genesisState State) *Chain {
	return &Chain{
		History:          []*Block{genesis},
		LastHistoryState: genesisState,
		Leader: &leader{
			Block: genesis,
			State: genesisState,
		},
	}
}
