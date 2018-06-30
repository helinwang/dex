package consensus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/helinwang/log15"
)

const (
	maxRoundMetric       = 9999
	sysTxnNotImplemented = "system transaction not implemented, will be implemented when open participation is necessary, however, the DEX is fully functional"
)

type blockNode struct {
	Block  Hash
	Weight float64

	// parent is nil if its parent is finalized
	parent        *blockNode
	blockChildren []*blockNode
}

// ChainStatus is the chain consensus state.
type ChainStatus struct {
	Round           uint64
	RandBeaconDepth uint64
	RoundMetrics    []RoundMetric
}

func (s *ChainStatus) InSync() bool {
	return s.Round >= s.RandBeaconDepth && s.Round <= s.RandBeaconDepth+1
}

type RoundMetric struct {
	Round     uint64
	BlockTime time.Duration
	TxnCount  int
}

// Chain is the blockchain.
type Chain struct {
	cfg          Config
	n            *Node
	randomBeacon *RandomBeacon
	store        *storage
	txnPool      TxnPool
	updater      Updater

	mu               sync.RWMutex
	roundMetrics     []RoundMetric
	lastEndRoundTime time.Time
	// reorg will never happen to the finalized block
	finalized             []Hash
	lastFinalizedState    State
	lastFinalizedSysState *SysState
	fork                  []*blockNode
	unFinalizedState      map[Hash]State
	roundWaitCh           map[uint64]chan struct{}
}

// Updater updates the application layer (DEX) about the current
// consensus.
type Updater interface {
	Update(s State)
}

// NewChain creates a new chain.
func NewChain(genesis *Block, genesisState State, seed Rand, cfg Config, txnPool TxnPool, u Updater, store *storage) *Chain {
	if genesisState.Hash() != genesis.StateRoot {
		panic(fmt.Errorf("genesis state hash and block state root does not match, state hash: %v, blocks state root: %v", genesisState.Hash(), genesis.StateRoot))
	}

	sysState := NewSysState()
	t := sysState.Transition()
	for _, txn := range genesis.SysTxns {
		valid := t.Record(txn)
		if !valid {
			panic("sys txn in genesis is invalid")
		}
	}

	u.Update(genesisState)
	sysState = t.Commit()
	gh := genesis.Hash()
	store.AddBlock(genesis, gh)
	return &Chain{
		cfg:                   cfg,
		store:                 store,
		updater:               u,
		txnPool:               txnPool,
		randomBeacon:          NewRandomBeacon(seed, sysState.groups, cfg),
		finalized:             []Hash{gh},
		lastFinalizedState:    genesisState,
		lastFinalizedSysState: sysState,
		unFinalizedState:      make(map[Hash]State),
		roundWaitCh:           make(map[uint64]chan struct{}),
		lastEndRoundTime:      time.Now(),
	}
}

// Genesis returns the hash of the genesis block.
func (c *Chain) Genesis() Hash {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finalized[0]
}

// ChainStatus returns the chain status.
func (c *Chain) ChainStatus() ChainStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := ChainStatus{}
	s.Round = c.round()
	s.RandBeaconDepth = c.randomBeacon.Round()
	s.RoundMetrics = make([]RoundMetric, len(c.roundMetrics))
	copy(s.RoundMetrics, c.roundMetrics)
	return s
}

func (c *Chain) TxnPoolSize() int {
	return c.txnPool.Size()
}

func (c *Chain) WaitUntil(round uint64) {
	c.mu.Lock()
	curRound := c.round()
	if round <= curRound {
		c.mu.Unlock()
		return
	}

	ch, ok := c.roundWaitCh[round]
	if !ok {
		ch = make(chan struct{}, 0)
		c.roundWaitCh[round] = ch
	}
	c.mu.Unlock()

	<-ch
}

// ProposeBlock proposes a new block proposal.
func (c *Chain) ProposeBlock(ctx context.Context, sk SK, round uint64) *BlockProposal {
	txns := c.txnPool.Txns()
	block, state, _ := c.Leader()
	if block.Round+1 < round {
		log.Info("proposing block skipped", "expected round", round-1, "block round", block.Round)
		return nil
	} else if block.Round+1 > round {
		log.Error("want to propose block, but does not find the suitable block", "expected round", round-1, "block round", block.Round)
		return nil
	}

	trans := state.Transition(round)
loop:
	for i := range txns {
		select {
		case <-ctx.Done():
			break loop
		default:
		}

		err := trans.Record(txns[i])
		if err != nil && err != ErrTxnNonceTooBig {
			log.Warn("error record txn", "err", err)
			// TODO: handle "lost" txn due to reorg.
			c.txnPool.Remove(SHA3(txns[i].Raw))
		}
	}

	pk := sk.MustPK()
	txnsBytes := trans.Txns()
	bp := BlockProposal{
		Round:     round,
		PrevBlock: block.Hash(),
		Txns:      txnsBytes,
		Owner:     pk.Addr(),
	}

	bp.OwnerSig = sk.Sign(bp.Encode(false))
	return &bp
}

// FinalizedRound returns the latest finalized round.
func (c *Chain) FinalizedRound() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return uint64(len(c.finalized) - 1)
}

func (c *Chain) round() uint64 {
	round := len(c.finalized)
	round += maxHeight(c.fork)
	return uint64(round)
}

// Round returns the current round.
func (c *Chain) Round() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.round()
}

func maxHeight(ns []*blockNode) int {
	max := 0
	for _, n := range ns {
		h := maxHeight(n.blockChildren) + 1
		if max < h {
			max = h
		}
	}
	return max
}

func weight(n *blockNode) float64 {
	w := n.Weight
	prev := n.parent
	for ; prev != nil; prev = prev.parent {
		w += prev.Weight
	}

	return w
}

func heaviestFork(fork []*blockNode, depth int) *blockNode {
	var nodes []*blockNode
	if depth == 0 {
		nodes = fork
	} else {
		for _, v := range fork {
			nodes = append(nodes, nodesAtDepth(v.blockChildren, depth-1)...)
		}
	}

	var maxWeight float64
	var r *blockNode
	for _, n := range nodes {
		w := weight(n)
		if w > maxWeight {
			r = n
			maxWeight = w
		}
	}

	return r
}

func nodesAtDepth(children []*blockNode, d int) []*blockNode {
	if d == 0 {
		if len(children) == 0 {
			return nil
		}

		return children
	}

	var nodes []*blockNode
	for _, child := range children {
		nodes = append(nodes, nodesAtDepth(child.blockChildren, d-1)...)
	}

	return nodes
}

func (c *Chain) leader() (*Block, State, *SysState) {
	if len(c.fork) == 0 {
		return c.store.Block(c.finalized[len(c.finalized)-1]), c.lastFinalizedState, c.lastFinalizedSysState
	}

	depth := maxHeight(c.fork) - 1
	n := heaviestFork(c.fork, depth)
	return c.store.Block(n.Block), c.unFinalizedState[n.Block], c.lastFinalizedSysState
}

// Leader returns the block of the current round whose chain is the
// heaviest.
func (c *Chain) Leader() (*Block, State, *SysState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.leader()
}

// BlockState returns the block's state given block's hash.
func (c *Chain) BlockState(h Hash) State {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.blockState(h)
}

func (c *Chain) blockState(h Hash) State {
	if h == c.finalized[len(c.finalized)-1] {
		return c.lastFinalizedState
	}

	return c.unFinalizedState[h]
}

func (c *Chain) addBlock(b *Block, s State, weight float64, txnCount int) (bool, error) {
	hash := b.Hash()
	log.Debug("add block to chain", "hash", hash)
	if saved := c.store.Block(hash); saved != nil {
		return false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	startingRound := c.round()
	finalizedRound := uint64(len(c.finalized) - 1)
	if b.Round <= finalizedRound {
		return false, fmt.Errorf("block's round is already finalized, round: %d, last finalized round: %d", b.Round, finalizedRound)
	}

	node := &blockNode{Block: hash, Weight: weight}
	if b.Round == finalizedRound+1 {
		if b.PrevBlock != c.finalized[len(c.finalized)-1] {
			return false, errors.New("block's prev round is finalized, but prev block is not the finalized block")
		}
		c.fork = append(c.fork, node)
		c.unFinalizedState[node.Block] = s
	} else {
		depth := int(b.Round - finalizedRound - 2)
		nodes := nodesAtDepth(c.fork, depth)

		var prev *blockNode
		for _, n := range nodes {
			if n.Block == b.PrevBlock {
				prev = n
				break
			}
		}

		if prev == nil {
			panic(fmt.Errorf("should never happen: can not find prev block %v, it should be already synced", b.PrevBlock))
		}

		node.parent = prev
		prev.blockChildren = append(prev.blockChildren, node)
	}

	c.store.AddBlock(b, hash)
	c.unFinalizedState[node.Block] = s
	_, leaderState, _ := c.leader()

	round := c.round()
	if startingRound == b.Round && startingRound+1 == round {
		// when round n ended, round n - 2 can be
		// finalized. See corollary 9.19 in page 15 of
		// https://arxiv.org/abs/1805.04548
		if startingRound > 2 {
			// TODO: use less aggressive finalize block count
			// (currently 2).
			c.finalize(startingRound - 2)
		}

		now := time.Now()
		metric := RoundMetric{
			Round:     startingRound,
			BlockTime: now.Sub(c.lastEndRoundTime),
			TxnCount:  txnCount,
		}

		if len(c.roundMetrics) < maxRoundMetric {
			c.roundMetrics = append(c.roundMetrics, metric)
		} else {
			copy(c.roundMetrics, c.roundMetrics[1:])
			c.roundMetrics[maxRoundMetric-1] = metric
		}
		c.lastEndRoundTime = now

		go c.n.EndRound(startingRound)
		if ch, ok := c.roundWaitCh[round]; ok {
			close(ch)
			delete(c.roundWaitCh, round)
		}
	}
	go c.updater.Update(leaderState)
	return true, nil
}

func widthAtDepth(n *blockNode, d int) int {
	if d == 0 {
		return len(n.blockChildren)
	}

	width := 0
	for _, child := range n.blockChildren {
		width += widthAtDepth(child, d-1)
	}
	return width
}

func forkWidth(fork []*blockNode, depth int) int {
	if depth == 0 {
		return len(fork)
	}

	width := 0
	for _, branch := range fork {
		width += widthAtDepth(branch, depth-1)
	}
	return width
}

func nodeAtDepth(n *blockNode, d int) *blockNode {
	if d == 0 {
		if len(n.blockChildren) == 0 {
			return nil
		}

		return n.blockChildren[0]
	}

	for _, child := range n.blockChildren {
		r := nodeAtDepth(child, d-1)
		if r != nil {
			return r
		}
	}

	return nil
}

func nodeAtDepthInFork(fork []*blockNode, depth int) *blockNode {
	if depth == 0 {
		return fork[0]
	}

	for _, branch := range fork {
		r := nodeAtDepth(branch, depth-1)
		if r != nil {
			return r
		}
	}

	return nil
}

// must be called with mutex held
func (c *Chain) finalize(round uint64) {
	count := uint64(len(c.finalized))
	if round < count {
		return
	}

	depth := int(round - count)

	// TODO: release finalized state/bp/block from memory, since
	// its persisted on disk, peers can still ask for them.

	if forkWidth(c.fork, depth) > 1 {
		// more than one block in the finalized round,
		// wait for next time to determin which fork
		// is finalized.
		return
	}

	root := nodeAtDepthInFork(c.fork, depth)
	for i := depth; i > 0; i-- {
		root = root.parent
	}

	found := false
	for _, b := range c.fork {
		if b == root {
			found = true
			break
		}
	}

	if !found {
		panic("should not happen: the node to be finalized is not on fork")
	}

	c.finalized = append(c.finalized, root.Block)
	c.lastFinalizedState = c.unFinalizedState[root.Block]
	delete(c.unFinalizedState, root.Block)
	c.fork = root.blockChildren

	for i := range c.fork {
		c.fork[i].parent = nil
	}

	// TODO: delete the state/block/bp of the removed branches from the map
}

// Graphviz returns the Graphviz format encoded chain visualization.
//
// only maxFinalized number of blocks will be shown, the rest will be
// hidden to save graph space.
func (c *Chain) Graphviz(maxFinalized int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.graphviz(maxFinalized)
}

func (c *Chain) graphviz(maxFinalized int) string {

	const (
		arrow = " -> "
		begin = `digraph chain {
rankdir=LR;
size="12,8"`
		end = `}
`
		finalizedNode    = `node [shape = rect, style=filled, color = chartreuse2];`
		notFinalizedNode = `node [shape = rect, style=filled, color = aquamarine];`
	)

	finalized := finalizedNode
	notFinalized := notFinalizedNode

	var start string
	var graph string

	dotIdx := 0
	finalizedSlice := c.finalized
	omitted := len(finalizedSlice) - maxFinalized
	if maxFinalized > 0 && len(finalizedSlice) > maxFinalized {
		dotIdx = maxFinalized / 2
		finalizedSlice = append(finalizedSlice[:dotIdx], finalizedSlice[len(finalizedSlice)-(maxFinalized-dotIdx):]...)
	}

	for i, f := range finalizedSlice {
		str := fmt.Sprintf("block_%x", f[:2])
		start = str
		finalized += " " + str

		if i > 0 {
			graph += arrow + str
		} else {
			graph = str
		}

		if dotIdx > 0 && i == dotIdx-1 {
			omitBlockName := fmt.Sprintf("num_blocks_omitted_to_save_space_%d", omitted)
			graph += arrow + omitBlockName
			finalized += " " + omitBlockName
		}
	}

	graph += "\n"

	graph, notFinalized = graphUpdateBlock(c.fork, start, graph, notFinalized)
	return strings.Join([]string{begin, finalized, notFinalized, graph, end}, "\n")
}

func graphUpdateBlock(ns []*blockNode, start, graph, block string) (string, string) {
	for _, u := range ns {
		str := fmt.Sprintf("block_%x", u.Block[:2])
		block += " " + str
		graph += start + " -> " + str + "\n"

		if len(u.blockChildren) > 0 {
			graph, block = graphUpdateBlock(u.blockChildren, str, graph, block)
		}
	}
	return graph, block
}
