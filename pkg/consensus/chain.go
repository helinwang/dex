package consensus

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

var errChainDataAlreadyExists = errors.New("chain data already exists")
var errBPParentNotFound = errors.New("block proposal parent not found")

type unNotarized struct {
	Weight float64
	BP     Hash

	Parent *notarized
}

type finalized struct {
	Block Hash
	BP    Hash
}

type notarized struct {
	Block  Hash
	Weight float64

	NtChildren    []*notarized
	NonNtChildren []*unNotarized

	BP *BlockProposal
}

type leader struct {
	Block    *Block
	State    State
	SysState *SysState
}

// Chain is the blockchain.
type Chain struct {
	roundInfo RoundInfo

	mu sync.RWMutex
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
	Fork                  []*notarized
	Leader                *leader
	HashToBlock           map[Hash]*Block
	HashToBP              map[Hash]*BlockProposal
	HashToNtShare         map[Hash]*NtShare
	BPToNtShares          map[Hash][]*NtShare
	bpNeedNotarize        map[Hash]bool
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

func (c *Chain) addBP(bp *BlockProposal, weight float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := bp.Hash()

	if _, ok := c.HashToBP[h]; ok {
		return errChainDataAlreadyExists
	}

	for _, notarized := range c.Fork {
		if notarized.Block == bp.PrevBlock {
			c.HashToBP[h] = bp
			u := &unNotarized{Weight: weight, BP: h, Parent: notarized}
			notarized.NonNtChildren = append(notarized.NonNtChildren, u)
			c.bpNeedNotarize[h] = true
			return nil
		}
	}

	return errBPParentNotFound
}

func (c *Chain) addNtShare(n *NtShare, groupID int) (*Block, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bp, ok := c.HashToBP[n.BP]
	if !ok {
		return nil, errors.New("block proposal not found")
	}

	if !c.bpNeedNotarize[n.BP] {
		return nil, errors.New("block proposal do not need notarization")
	}

	for _, s := range c.BPToNtShares[n.BP] {
		if s.Owner == n.Owner {
			return nil, errors.New("notarization share from the owner already received")
		}
	}

	c.BPToNtShares[n.BP] = append(c.BPToNtShares[n.BP], n)
	if len(c.BPToNtShares[n.BP]) >= groupThreshold {
		sig := recoverGroupSig(c.BPToNtShares[n.BP])
		if !c.validateGroupSig(sig, groupID, bp) {
			c.BPToNtShares[n.BP] = nil
			return nil, fmt.Errorf("fatal: group (%d) sig not valid", groupID)
		}

		b := &Block{
			Round:           bp.Round,
			StateRoot:       n.StateRoot,
			BlockProposal:   n.BP,
			PrevBlock:       bp.PrevBlock,
			SysTxns:         bp.SysTxns,
			NotarizationSig: sig.Serialize(),
		}

		delete(c.bpNeedNotarize, n.BP)
		for _, share := range c.BPToNtShares[n.BP] {
			delete(c.HashToNtShare, share.Hash())
		}
		delete(c.BPToNtShares, n.BP)
		return b, nil
	}

	c.HashToNtShare[n.Hash()] = n
	return nil, nil
}

func (c *Chain) addBlock(b *Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return nil
}

func (c *Chain) validateGroupSig(sig bls.Sign, groupID int, bp *BlockProposal) bool {
	msg := bp.Encode(true)
	return sig.Verify(&c.roundInfo.groups[groupID].PK, string(msg))
}
