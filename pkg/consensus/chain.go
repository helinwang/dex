package consensus

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/rlp"
	log "github.com/helinwang/log15"
)

var errChainDataAlreadyExists = errors.New("chain data already exists")

type unNotarized struct {
	BP     Hash
	Weight float64
}

type notarized struct {
	Block  Hash
	Weight float64

	NtChildren    []*notarized
	NonNtChildren []*unNotarized

	BP Hash
}

// Chain is the blockchain.
type Chain struct {
	cfg          Config
	RandomBeacon *RandomBeacon
	n            *Node
	TxnPool      TxnPool
	updater      Updater

	mu sync.RWMutex
	// reorg will never happen to the finalized block, we will
	// discard its associated state and block proposal.
	Finalized             []Hash
	LastFinalizedState    State
	LastFinalizedSysState *SysState
	Fork                  []*notarized
	UnNotarizedNotOnFork  []*unNotarized
	unFinalizedState      map[Hash]State
	unFinalizedSysState   map[Hash]*SysState
	hashToBlock           map[Hash]*Block
	hashToBP              map[Hash]*BlockProposal
	hashToNtShare         map[Hash]*NtShare
	hashToInventory       map[Hash]ItemID
	bpToNtShares          map[Hash][]*NtShare
	bpNeedNotarize        map[Hash]bool
}

// Updater updates the application layer (DEX) about the current
// consensus.
type Updater interface {
	Update(s State)
}

// NewChain creates a new chain.
func NewChain(genesis *Block, genesisState State, seed Rand, cfg Config, txnPool TxnPool, u Updater) *Chain {
	sysState := NewSysState()
	t := sysState.Transition()
	for _, txn := range genesis.SysTxns {
		valid := t.Record(txn)
		if !valid {
			panic("sys txn in genesis is invalid")
		}
	}

	u.Update(genesisState)
	sysState = t.Apply()
	sysState.Finalized()
	gh := genesis.Hash()
	return &Chain{
		cfg:                   cfg,
		updater:               u,
		TxnPool:               txnPool,
		RandomBeacon:          NewRandomBeacon(seed, sysState.groups, cfg),
		Finalized:             []Hash{gh},
		LastFinalizedState:    genesisState,
		LastFinalizedSysState: sysState,
		unFinalizedState:      make(map[Hash]State),
		unFinalizedSysState:   make(map[Hash]*SysState),
		hashToInventory:       make(map[Hash]ItemID),
		hashToBlock:           map[Hash]*Block{gh: genesis},
		hashToBP:              make(map[Hash]*BlockProposal),
		hashToNtShare:         make(map[Hash]*NtShare),
		bpToNtShares:          make(map[Hash][]*NtShare),
		bpNeedNotarize:        make(map[Hash]bool),
	}
}

func (c *Chain) Genesis() Hash {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Finalized[0]
}

func (c *Chain) ProposeBlock(sk SK) *BlockProposal {
	txns := c.TxnPool.Txns()
	block, state, _ := c.Leader()

	trans := state.Transition()
	for _, txn := range txns {
		valid, _ := trans.Record(txn)
		if !valid {
			// TODO: handle "lost" txn due to reorg.
			c.TxnPool.Remove(SHA3(txn))
		}
	}

	txns = trans.Txns()
	b, err := rlp.EncodeToBytes(txns)
	if err != nil {
		panic(err)
	}

	var bp BlockProposal
	bp.PrevBlock = SHA3(block.Encode(true))
	bp.Round = block.Round + 1
	pk, err := sk.PK()
	if err != nil {
		panic(err)
	}

	bp.Owner = pk.Addr()
	// TODO: support SysTxn when needed (e.g., open participation)
	bp.Data = b
	key, err := sk.Get()
	if err != nil {
		panic(err)
	}

	bp.OwnerSig = key.Sign(string(bp.Encode(false))).Serialize()
	return &bp
}

// Block returns the block of the given hash.
func (c *Chain) Block(h Hash) *Block {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.hashToBlock[h]
}

// BlockProposal returns the block of the given hash.
func (c *Chain) BlockProposal(h Hash) *BlockProposal {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.hashToBP[h]
}

// NtShare returns the notarization share of the given hash.
func (c *Chain) NtShare(h Hash) *NtShare {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.hashToNtShare[h]
}

// NeedNotarize returns if the block proposal of the given hash needs
// to be notarized.
func (c *Chain) NeedNotarize(h Hash) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, ok := c.bpNeedNotarize[h]
	if ok {
		return b
	}

	// treat bp that has not arrived yet as need notarize.
	return true
}

// FinalizedChain returns the finalized block chain.
func (c *Chain) FinalizedChain() []*Block {
	var bs []*Block
	for _, h := range c.Finalized {
		bs = append(bs, c.hashToBlock[h])
	}

	return bs
}

func (c *Chain) FinalizedRound() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return uint64(len(c.Finalized))
}

func (c *Chain) round() uint64 {
	round := len(c.Finalized)
	round += maxHeight(c.Fork)
	return uint64(round)
}

// Round returns the current round.
func (c *Chain) Round() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.round()
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

func (c *Chain) heaviestFork() *notarized {
	// TODO: implement correctly
	n := c.Fork[0]
	for len(n.NtChildren) > 0 {
		n = n.NtChildren[0]
	}

	return n
}

func (c *Chain) leader() (*Block, State, *SysState) {
	if len(c.Fork) == 0 {
		return c.hashToBlock[c.Finalized[len(c.Finalized)-1]], c.LastFinalizedState, c.LastFinalizedSysState
	}

	n := c.heaviestFork()
	return c.hashToBlock[n.Block], c.unFinalizedState[n.Block], c.unFinalizedSysState[n.Block]

}

// Leader returns the notarized block of the current round whose chain
// is the heaviest.
func (c *Chain) Leader() (*Block, State, *SysState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.leader()
}

func findPrevBlock(prevBlock Hash, ns []*notarized) (*notarized, int) {
	for i, notarized := range ns {
		if notarized.Block == prevBlock {
			return notarized, i
		}

		n, idx := findPrevBlock(prevBlock, notarized.NtChildren)
		if n != nil {
			return n, idx
		}
	}

	return nil, 0
}

func (c *Chain) addBP(bp *BlockProposal, weight float64) error {
	log.Info("addBP called", "hash", bp.Hash(), "weight", weight)
	c.mu.Lock()
	defer c.mu.Unlock()
	h := bp.Hash()

	if _, ok := c.hashToBP[h]; ok {
		return errChainDataAlreadyExists
	}

	notarized, _ := findPrevBlock(bp.PrevBlock, c.Fork)
	if notarized == nil {
		if c.Finalized[len(c.Finalized)-1] != bp.PrevBlock {
			return fmt.Errorf("block proposal's parent not found: %x, round: %d", bp.PrevBlock, bp.Round)
		}
	}

	c.registerBP(bp)
	u := &unNotarized{Weight: weight, BP: h}

	if notarized != nil {
		notarized.NonNtChildren = append(notarized.NonNtChildren, u)
	} else {
		c.UnNotarizedNotOnFork = append(c.UnNotarizedNotOnFork, u)
	}
	c.bpNeedNotarize[h] = true
	go c.n.RecvBlockProposal(bp)
	return nil
}

func (c *Chain) Inventory() []ItemID {
	c.mu.Lock()
	defer c.mu.Unlock()

	ret := make([]ItemID, len(c.hashToInventory))
	i := 0
	for _, v := range c.hashToInventory {
		ret[i] = v
		i++
	}

	return ret
}

// must be called with mutex held.
func (c *Chain) registerBP(bp *BlockProposal) {
	h := bp.Hash()
	c.hashToBP[h] = bp
	c.hashToInventory[h] = ItemID{
		T:         BlockProposalItem,
		Hash:      h,
		ItemRound: bp.Round,
		Ref:       bp.PrevBlock,
	}
}

// must be called with mutex held.
func (c *Chain) unregisterBP(h Hash) {
	delete(c.hashToBP, h)
	delete(c.hashToInventory, h)
}

// must be called with mutex held.
func (c *Chain) registerBlock(b *Block) {
	h := b.Hash()
	c.hashToBlock[h] = b
	c.hashToInventory[h] = ItemID{
		T:         BlockItem,
		Hash:      h,
		ItemRound: b.Round,
		Ref:       b.PrevBlock,
	}
}

// must be called with mutex held.
func (c *Chain) unregisterBlock(h Hash) {
	delete(c.hashToBlock, h)
	delete(c.hashToInventory, h)
}

// must be called with mutex held.
func (c *Chain) registerNtShare(s *NtShare) {
	h := s.Hash()
	c.hashToNtShare[h] = s
	c.hashToInventory[h] = ItemID{T: NtShareItem, Hash: h, ItemRound: s.Round, Ref: s.BP}
}

// must be called with mutex held.
func (c *Chain) unregisterNtShare(h Hash) {
	delete(c.hashToNtShare, h)
	delete(c.hashToInventory, h)
}

func (c *Chain) addNtShare(n *NtShare, groupID int) (*Block, bool) {
	log.Info("addNtShare called", "hash", n.Hash(), "group", groupID)
	c.mu.Lock()
	defer c.mu.Unlock()

	bp, ok := c.hashToBP[n.BP]
	if !ok {
		log.Warn("add nt share but block proposal not found")
		return nil, false
	}

	if !c.bpNeedNotarize[n.BP] {
		return nil, true
	}

	for _, s := range c.bpToNtShares[n.BP] {
		if s.Owner == n.Owner {
			log.Warn("notarization share from the owner already received")
			return nil, true
		}
	}

	c.bpToNtShares[n.BP] = append(c.bpToNtShares[n.BP], n)

	if len(c.bpToNtShares[n.BP]) >= c.cfg.GroupThreshold {
		sig, err := recoverNtSig(c.bpToNtShares[n.BP])
		if err != nil {
			// should not happen
			panic(err)
		}

		if !c.validateGroupSig(sig, groupID, bp) {
			panic("impossible: group sig not valid")
		}

		b, err := c.BPToBlock(bp)
		if err != nil {
			panic(err)
		}

		//		b.StateRoot = n.StateRoot
		b.NotarizationSig = sig.Serialize()

		delete(c.bpNeedNotarize, n.BP)
		for _, share := range c.bpToNtShares[n.BP] {
			c.unregisterNtShare(share.Hash())
		}
		delete(c.bpToNtShares, n.BP)
		return b, true
	}

	c.registerNtShare(n)
	return nil, true
}

func (c *Chain) BPToBlock(bp *BlockProposal) (*Block, error) {
	trans, err := c.bpTransition(bp)
	if err != nil {
		return nil, err
	}

	return &Block{
		Owner:         bp.Owner,
		Round:         bp.Round,
		BlockProposal: bp.Hash(),
		PrevBlock:     bp.PrevBlock,
		SysTxns:       bp.SysTxns,
		StateRoot:     trans.Account(),
	}, nil
}

func getTransition(state State, txnData []byte) (trans Transition, err error) {
	trans = state.Transition()

	if len(txnData) == 0 {
		return
	}

	var txns [][]byte
	err = rlp.DecodeBytes(txnData, &txns)
	if err != nil {
		err = fmt.Errorf("invalid transactions: %v", err)
		return
	}

	for _, t := range txns {
		valid, success := trans.Record(t)
		if !valid || !success {
			err = errors.New("failed to apply transactions")
			return
		}
	}

	return
}

func (c *Chain) bpTransition(bp *BlockProposal) (Transition, error) {
	var prevState State

	prev := c.hashToBlock[bp.PrevBlock]
	if prev == nil {
		panic("TODO")
	}

	if bp.PrevBlock == c.Finalized[len(c.Finalized)-1] {
		prevState = c.LastFinalizedState
	} else {
		prevState = c.unFinalizedState[bp.PrevBlock]
		if prevState == nil {
			panic("TODO")
		}
	}

	return getTransition(prevState, bp.Data)
}

func (c *Chain) addBlock(b *Block, weight float64) error {
	// TODO: remove txn from the txn pool
	log.Info("addBlock called", "hash", b.Hash(), "weight", weight)
	c.mu.Lock()
	defer c.mu.Unlock()

	prevRound := c.round()

	h := b.Hash()
	if _, ok := c.hashToBlock[h]; ok {
		return errChainDataAlreadyExists
	}

	bp, ok := c.hashToBP[b.BlockProposal]
	if !ok {
		panic("TODO: iteratively get all dependent BP")
	}

	trans, err := c.bpTransition(bp)
	if err != nil {
		return err
	}

	if root := trans.Account(); root != b.StateRoot {
		return fmt.Errorf("fatal: block state not valid, expected: %x, got: %x", root[:], b.StateRoot[:])
	}

	nt := &notarized{Block: h, Weight: weight, BP: b.BlockProposal}
	c.unFinalizedState[nt.Block] = trans.Commit()

	var prevSysState *SysState
	prevFinalized := false
	if bp.PrevBlock == c.Finalized[len(c.Finalized)-1] {
		prevSysState = c.LastFinalizedSysState
		prevFinalized = true
	} else {
		prevSysState = c.unFinalizedSysState[bp.PrevBlock]
		if prevSysState == nil {
			panic("TODO")
		}
	}
	// TODO: update sys state once need to support system txn.
	c.unFinalizedSysState[nt.Block] = prevSysState

	if prevFinalized {
		c.Fork = append(c.Fork, nt)
		removeIdx := -1
		for i, e := range c.UnNotarizedNotOnFork {
			if e.BP == nt.BP {
				removeIdx = i
			}
		}

		if removeIdx <= 0 {
			log.Info("block's proposal not found on chain", "block", h)
		} else {
			c.UnNotarizedNotOnFork = append(c.UnNotarizedNotOnFork[:removeIdx], c.UnNotarizedNotOnFork[removeIdx+1:]...)
		}
	} else {
		prev, removeIdx := findPrevBlock(bp.PrevBlock, c.Fork)
		prev.NtChildren = append(prev.NtChildren, nt)
		prev.NonNtChildren = append(prev.NonNtChildren[:removeIdx], prev.NonNtChildren[removeIdx+1:]...)
	}

	c.registerBlock(b)
	delete(c.bpNeedNotarize, b.BlockProposal)
	delete(c.bpToNtShares, b.BlockProposal)

	round := c.round()
	// when round n is started, round n - 3 can be finalized. See
	// corollary 9.19 in https://arxiv.org/abs/1805.04548
	if round > 3 {
		// TODO: use less aggressive finalize block count
		// (currently 3).
		c.finalize(round - 3)
	}

	_, s, _ := c.leader()
	go c.updater.Update(s)

	if round == prevRound+1 {
		// TODO: make it more robust
		go func() {
			for {
				if c.RandomBeacon.Depth() >= round {
					c.n.StartRound(round)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}
	return nil
}

// must be called with mutex held
func (c *Chain) releaseBPs(s []*unNotarized) {
	for _, e := range s {
		c.unregisterBP(e.BP)
	}
}

// must be called with mutex held
func (c *Chain) finalize(round uint64) {
	depth := round
	var count uint64
	count += uint64(len(c.Finalized))
	if depth < count {
		return
	}

	depth -= count

	c.releaseBPs(c.UnNotarizedNotOnFork)
	c.UnNotarizedNotOnFork = nil

	if depth == 0 {
		if len(c.Fork) > 1 {
			// more than one notarized in the finalized round,
			// wait for next time to determin which fork is
			// finalized.
			return
		}

		f := c.Fork[0]
		c.Finalized = append(c.Finalized, f.Block)
		// TODO: compact not used state
		c.LastFinalizedState = c.unFinalizedState[f.Block]
		delete(c.unFinalizedState, f.Block)
		c.LastFinalizedSysState = c.unFinalizedSysState[f.Block]
		delete(c.unFinalizedSysState, f.Block)
		c.Fork = f.NtChildren
		c.UnNotarizedNotOnFork = f.NonNtChildren
		return
	}

	// TODO: add to history if condition met

	// TODO: delete removed states from map

	// TODO: handle condition of not normal operation. E.g, remove
	// the peer of the finalized parents

	panic("not under normal operation, not implemented")
}

func (c *Chain) validateGroupSig(sig bls.Sign, groupID int, bp *BlockProposal) bool {
	b, err := c.BPToBlock(bp)
	if err != nil {
		log.Error("error reconstruct block during validateGroupSig", "err", err)
		return false
	}

	msg := b.Encode(false)
	return sig.Verify(&c.RandomBeacon.groups[groupID].PK, string(msg))
}

// Graphviz returns the Graphviz dot formate encoded chain
// visualization.
func (c *Chain) Graphviz() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	const (
		arrow = " -> "
		begin = `digraph chain {
rankdir=LR;
size="8,5"`
		end = `}
`
		finalizedNode   = `node [shape = rect, style=filled, color = chartreuse2];`
		notarizedNode   = `node [shape = rect, style=filled, color = aquamarine];`
		unNotarizedNode = `node [shape = octagon, style=filled, color = aliceblue];`
	)

	finalized := finalizedNode
	notarized := notarizedNode
	unNotarized := unNotarizedNode

	var start string
	var graph string

	for i, f := range c.Finalized {
		str := fmt.Sprintf("block_%x", f[:2])
		start = str
		finalized += " " + str

		if i > 0 {
			graph += arrow + str
		} else {
			graph = str
		}
	}

	graph += "\n"

	graph, unNotarized = updateUnNt(c.UnNotarizedNotOnFork, start, graph, unNotarized)
	graph, notarized, unNotarized = updateNt(c.Fork, start, graph, notarized, unNotarized)
	return strings.Join([]string{begin, finalized, notarized, unNotarized, graph, end}, "\n")
}

func updateUnNt(ns []*unNotarized, start, graph, unNotarized string) (string, string) {
	for _, u := range ns {
		str := fmt.Sprintf("proposal_%x", u.BP[:2])
		unNotarized += " " + str
		graph += start + " -> " + str + "\n"
	}
	return graph, unNotarized
}

func updateNt(ns []*notarized, start, graph, notarized, unNotarized string) (string, string, string) {
	for _, u := range ns {
		str := fmt.Sprintf("block_%x", u.Block[:2])
		notarized += " " + str
		graph += start + " -> " + str + "\n"

		if len(u.NtChildren) > 0 {
			graph, notarized, unNotarized = updateNt(u.NtChildren, str, graph, notarized, unNotarized)
		}

		if len(u.NonNtChildren) > 0 {
			graph, unNotarized = updateUnNt(u.NonNtChildren, str, graph, unNotarized)
		}
	}
	return graph, notarized, unNotarized
}
