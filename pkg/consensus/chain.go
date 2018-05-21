package consensus

import (
	"errors"
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

	BP       *BlockProposal
	State    State
	SysState *SysState
}

// Chain is the blockchain.
type Chain struct {
	RandomBeacon *RandomBeacon

	mu sync.RWMutex
	// the finalized block burried deep enough becomes part of the
	// history. Its block proposal and state will be discarded to
	// save space.
	History             []Hash
	LastHistoryState    State
	LastHistorySysState *SysState
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
	HashToBlock           map[Hash]*Block
	HashToBP              map[Hash]*BlockProposal
	HashToNtShare         map[Hash]*NtShare
	BPToNtShares          map[Hash][]*NtShare
	bpNeedNotarize        map[Hash]bool
}

// NewChain creates a new chain.
func NewChain(genesis *Block, genesisState State, seed Rand) *Chain {
	sysState := NewSysState()
	t := sysState.Transition()
	for _, txn := range genesis.SysTxns {
		valid := t.Record(txn)
		if !valid {
			panic("sys txn in genesis is invalid")
		}
	}

	sysState = t.Apply()
	sysState.Finalized()

	gh := genesis.Hash()
	return &Chain{
		RandomBeacon:        NewRandomBeacon(seed, sysState.groups),
		History:             []Hash{gh},
		LastHistoryState:    genesisState,
		LastHistorySysState: sysState,
		HashToBlock:         map[Hash]*Block{gh: genesis},
		HashToBP:            make(map[Hash]*BlockProposal),
		HashToNtShare:       make(map[Hash]*NtShare),
		BPToNtShares:        make(map[Hash][]*NtShare),
		bpNeedNotarize:      make(map[Hash]bool),
	}
}

// Round returns the current round.
func (c *Chain) Round() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	round := len(c.History)
	round += len(c.Finalized)
	round += maxHeight(c.Fork)
	return round
}

func maxHeight(ns []*notarized) int {
	max := 0
	for _, n := range ns {
		h := maxHeight(n.NtChildren) + 1
		if max < h {
			max = h
		}
	}
	return max
}

func findPrevBlock(prevBlock Hash, ns []*notarized) *notarized {
	for _, notarized := range ns {
		if notarized.Block == prevBlock {
			return notarized
		}

		n := findPrevBlock(prevBlock, notarized.NtChildren)
		if n != nil {
			return n
		}
	}

	return nil
}

func (c *Chain) addBP(bp *BlockProposal, weight float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := bp.Hash()

	if _, ok := c.HashToBP[h]; ok {
		return errChainDataAlreadyExists
	}

	notarized := findPrevBlock(bp.PrevBlock, c.Fork)
	if notarized == nil {
		return errBPParentNotFound
	}

	c.HashToBP[h] = bp
	u := &unNotarized{Weight: weight, BP: h, Parent: notarized}
	notarized.NonNtChildren = append(notarized.NonNtChildren, u)
	c.bpNeedNotarize[h] = true
	return nil
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
		sig := recoverNtSig(c.BPToNtShares[n.BP])
		if !c.validateGroupSig(sig, groupID, bp) {
			panic("impossible: group sig not valid")
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

func (c *Chain) addBlock(b *Block, weight float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	h := b.Hash()
	if _, ok := c.HashToBlock[h]; ok {
		return errors.New("block already exists")
	}

	bp, ok := c.HashToBP[b.BlockProposal]
	if !ok {
		return errors.New("block's proposal not found")
	}

	var prevState State
	var prevSysState *SysState

	nt := &notarized{Block: h, Weight: weight, BP: bp}
	prev := findPrevBlock(b.PrevBlock, c.Fork)
	if prev != nil {
		prevState = prev.State
		prevSysState = prev.SysState
	} else if len(c.Finalized) > 0 && c.Finalized[len(c.Finalized)-1].Block == b.PrevBlock {
		prevState = c.LastFinalizedState
		prevSysState = c.LastFinalizedSysState
	} else if c.History[len(c.History)-1] == b.PrevBlock {
		prevState = c.LastHistoryState
		prevSysState = c.LastHistorySysState
	} else {
		return errors.New("can not connect block to the chain")
	}

	// TODO: update state
	nt.State = prevState

	// TODO: update sys state once need to support system txn.
	nt.SysState = prevSysState

	// TODO: independently generate the state root and verify state root hash

	if prev != nil {
		prev.NtChildren = append(prev.NtChildren, nt)
	} else {
		c.Fork = append(c.Fork, nt)
	}

	// TODO: finalize blocks

	c.HashToBlock[h] = b
	delete(c.bpNeedNotarize, b.BlockProposal)
	delete(c.BPToNtShares, b.BlockProposal)
	return nil
}

func (c *Chain) validateGroupSig(sig bls.Sign, groupID int, bp *BlockProposal) bool {
	msg := bp.Encode(true)
	return sig.Verify(&c.RandomBeacon.groups[groupID].PK, string(msg))
}
