package consensus

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/rlp"
	log "github.com/helinwang/log15"
)

const (
	sysTxnNotImplemented = "system transaction not implemented, will be implemented when open participation is necessary, however, the DEX is fully functional"
)

type bpNode struct {
	BP     Hash
	Weight float64
}

type blockNode struct {
	Block  Hash
	BP     Hash
	Weight float64

	blockChildren []*blockNode
	bpChildren    []*bpNode
}

// ChainStatus is the chain consensus state.
type ChainStatus struct {
	Round           uint64
	RandBeaconDepth uint64
}

func (s *ChainStatus) InSync() bool {
	return s.Round >= s.RandBeaconDepth && s.Round <= s.RandBeaconDepth+1
}

// Chain is the blockchain.
type Chain struct {
	cfg          Config
	randomBeacon *RandomBeacon
	n            *Node
	txnPool      TxnPool
	updater      Updater

	mu sync.RWMutex
	// reorg will never happen to the finalized block
	finalized             []Hash
	lastFinalizedState    State
	lastFinalizedSysState *SysState
	fork                  []*blockNode
	bpNotOnFork           []*bpNode
	unFinalizedState      map[Hash]State
	hashToBlock           map[Hash]*Block
	hashToBP              map[Hash]*BlockProposal
	hashToNtShare         map[Hash]*NtShare
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
	if genesisState.Hash() != genesis.StateRoot {
		panic(fmt.Errorf("genesis state hash and block state root does not match, state hash: %x, blocks state root: %x", genesisState.Hash(), genesis.StateRoot))
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
	return &Chain{
		cfg:                   cfg,
		updater:               u,
		txnPool:               txnPool,
		randomBeacon:          NewRandomBeacon(seed, sysState.groups, cfg),
		finalized:             []Hash{gh},
		lastFinalizedState:    genesisState,
		lastFinalizedSysState: sysState,
		unFinalizedState:      make(map[Hash]State),
		hashToBlock:           map[Hash]*Block{gh: genesis},
		hashToBP:              make(map[Hash]*BlockProposal),
		hashToNtShare:         make(map[Hash]*NtShare),
		bpToNtShares:          make(map[Hash][]*NtShare),
		bpNeedNotarize:        make(map[Hash]bool),
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
	return s
}

// ProposeBlock proposes a new block.
func (c *Chain) ProposeBlock(sk SK) *BlockProposal {
	txns := c.txnPool.Txns()
	block, state, _ := c.Leader()
	round := block.Round + 1

	trans := state.Transition(round)
	for _, txn := range txns {
		valid, _ := trans.Record(txn)
		if !valid {
			// TODO: handle "lost" txn due to reorg.
			c.txnPool.Remove(SHA3(txn))
		}
	}

	txns = trans.Txns()
	b, err := rlp.EncodeToBytes(txns)
	if err != nil {
		panic(err)
	}

	var bp BlockProposal
	bp.PrevBlock = SHA3(block.Encode(true))
	bp.Round = round
	pk, err := sk.PK()
	if err != nil {
		panic(err)
	}

	bp.Owner = pk.Addr()
	bp.Data = b
	bp.OwnerSig = sk.Sign(bp.Encode(false))
	return &bp
}

// Block returns the block of the given hash.
func (c *Chain) Block(h Hash) *Block {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.hashToBlock[h]
}

// BlockProposal returns the block proposal of the given hash.
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

func (c *Chain) heaviestFork() *blockNode {
	// TODO: implement correctly
	n := c.fork[0]
	for len(n.blockChildren) > 0 {
		n = n.blockChildren[0]
	}

	return n
}

func (c *Chain) leader() (*Block, State, *SysState) {
	if len(c.fork) == 0 {
		return c.hashToBlock[c.finalized[len(c.finalized)-1]], c.lastFinalizedState, c.lastFinalizedSysState
	}

	n := c.heaviestFork()
	return c.hashToBlock[n.Block], c.unFinalizedState[n.Block], c.lastFinalizedSysState

}

// Leader returns the block of the current round whose chain is the
// heaviest.
func (c *Chain) Leader() (*Block, State, *SysState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.leader()
}

func findPrevBlockNode(prevBlock Hash, bs []*blockNode) (*blockNode, int) {
	for i, block := range bs {
		if block.Block == prevBlock {
			return block, i
		}

		n, idx := findPrevBlockNode(prevBlock, block.blockChildren)
		if n != nil {
			return n, idx
		}
	}

	return nil, 0
}

func (c *Chain) addBP(bp *BlockProposal, weight float64) (bool, error) {
	log.Debug("add block proposal to chain", "hash", bp.Hash(), "weight", weight)
	c.mu.Lock()
	defer c.mu.Unlock()
	h := bp.Hash()

	if _, ok := c.hashToBP[h]; ok {
		return false, nil
	}

	prevBlockNode, _ := findPrevBlockNode(bp.PrevBlock, c.fork)
	if prevBlockNode == nil {
		if c.finalized[len(c.finalized)-1] != bp.PrevBlock {
			return false, fmt.Errorf("block proposal's parent not found: %x, round: %d", bp.PrevBlock, bp.Round)
		}
	}

	c.hashToBP[h] = bp
	u := &bpNode{Weight: weight, BP: h}

	if prevBlockNode != nil {
		prevBlockNode.bpChildren = append(prevBlockNode.bpChildren, u)
	} else {
		c.bpNotOnFork = append(c.bpNotOnFork, u)
	}
	c.bpNeedNotarize[h] = true
	go c.n.recvBPForNotary(bp)
	return true, nil
}

func (c *Chain) addNtShare(n *NtShare, groupID int) (b *Block, added, success bool) {
	log.Debug("add notarization share to chain", "hash", n.Hash(), "group", groupID)
	c.mu.Lock()
	defer c.mu.Unlock()

	bp, ok := c.hashToBP[n.BP]
	if !ok {
		log.Warn("add nt share but block proposal not found")
		success = false
		return
	}

	if !c.bpNeedNotarize[n.BP] {
		success = true
		return
	}

	for _, s := range c.bpToNtShares[n.BP] {
		if s.Owner == n.Owner {
			log.Warn("notarization share from the owner already received")
			success = true
			return
		}
	}

	c.bpToNtShares[n.BP] = append(c.bpToNtShares[n.BP], n)
	added = true
	success = true

	if len(c.bpToNtShares[n.BP]) >= c.cfg.GroupThreshold {
		sig, err := recoverNtSig(c.bpToNtShares[n.BP])
		if err != nil {
			// should not happen
			panic(err)
		}

		state := c.blockState(bp.PrevBlock)
		if state == nil {
			panic("should never happen: can not find prev block, it should be already synced")
		}

		trans, err := recordTxns(state, bp.Data, bp.Round)
		if err != nil {
			panic("should never happen: notarized block's txns should be all valid")
		}

		// TODO: make sure the fields (except signature and
		// owner) of all nt shares are same
		b = &Block{
			Owner:         bp.Owner,
			Round:         bp.Round,
			BlockProposal: bp.Hash(),
			PrevBlock:     bp.PrevBlock,
			SysTxns:       bp.SysTxns,
			StateRoot:     trans.StateHash(),
		}

		msg := b.Encode(false)
		if !sig.Verify(c.randomBeacon.groups[groupID].PK, msg) {
			panic("should never happen: group sig not valid")
		}

		b.NotarizationSig = sig

		delete(c.bpNeedNotarize, n.BP)
		for _, share := range c.bpToNtShares[n.BP] {
			delete(c.hashToNtShare, share.Hash())
		}
		delete(c.bpToNtShares, n.BP)
		return
	}

	c.hashToNtShare[n.Hash()] = n
	return
}

func recordTxns(state State, txnData []byte, round uint64) (trans Transition, err error) {
	trans = state.Transition(round)

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

func (c *Chain) addBlock(b *Block, bp *BlockProposal, s State, weight float64) (bool, error) {
	log.Debug("add block to chain", "hash", b.Hash(), "weight", weight)
	c.mu.Lock()
	defer c.mu.Unlock()
	beginRound := c.round()

	h := b.Hash()
	if _, ok := c.hashToBlock[h]; ok {
		return false, nil
	}

	nt := &blockNode{Block: h, Weight: weight, BP: b.BlockProposal}
	c.unFinalizedState[nt.Block] = s

	prevFinalized := bp.PrevBlock == c.finalized[len(c.finalized)-1]
	if prevFinalized {
		c.fork = append(c.fork, nt)
		removeIdx := -1
		for i, e := range c.bpNotOnFork {
			if e.BP == nt.BP {
				removeIdx = i
			}
		}

		if removeIdx < 0 {
			log.Info("block's proposal not found on chain", "bp", b.BlockProposal, "b", b.Hash())
		} else {
			c.bpNotOnFork = append(c.bpNotOnFork[:removeIdx], c.bpNotOnFork[removeIdx+1:]...)
		}
	} else {
		prev, removeIdx := findPrevBlockNode(bp.PrevBlock, c.fork)
		if prev == nil {
			panic("should never happen: can not find prev block, it should be already synced")
		}
		prev.blockChildren = append(prev.blockChildren, nt)
		prev.bpChildren = append(prev.bpChildren[:removeIdx], prev.bpChildren[removeIdx+1:]...)
	}

	c.hashToBlock[h] = b
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

	if len(bp.Data) > 0 {
		var txns [][]byte
		err := rlp.DecodeBytes(bp.Data, &txns)
		if err != nil {
			return false, fmt.Errorf("should never happen: notarized block contains invalid txn data: %v", err)
		}

		for _, txn := range txns {
			c.txnPool.Remove(SHA3(txn))
		}
	}

	_, leaderState, _ := c.leader()
	go c.updater.Update(leaderState)

	if beginRound == b.Round && beginRound+1 == round {
		go c.n.EndRound(beginRound)
	}
	return true, nil
}

// must be called with mutex held
func (c *Chain) finalize(round uint64) {
	depth := round
	var count uint64
	count += uint64(len(c.finalized))
	if depth < count {
		return
	}

	depth -= count

	// TODO: release finalized state/bp/block from memory, since
	// its persisted on disk, peers can still ask for them.

	if depth == 0 {
		if len(c.fork) > 1 {
			// more than one block in the finalized round,
			// wait for next time to determin which fork
			// is finalized.
			return
		}

		f := c.fork[0]
		c.finalized = append(c.finalized, f.Block)
		// TODO: compact not used state
		c.lastFinalizedState = c.unFinalizedState[f.Block]
		delete(c.unFinalizedState, f.Block)
		c.fork = f.blockChildren
		c.bpNotOnFork = f.bpChildren
		return
	}

	// TODO: add to history if condition met

	// TODO: delete removed states from map

	// TODO: handle condition of not normal operation. E.g, remove
	// the peer of the finalized parents

	fmt.Println(c.graphviz(10))
	panic("not under normal operation, not implemented")
}

// Graphviz returns the Graphviz dot formate encoded chain
// visualization.
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
		bpNode           = `node [shape = octagon, style=filled, color = aliceblue];`
	)

	finalized := finalizedNode
	notFinalized := notFinalizedNode
	bps := bpNode

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

	graph, bps = graphUpdateBP(c.bpNotOnFork, start, graph, bps)
	graph, notFinalized, bps = graphUpdateBlock(c.fork, start, graph, notFinalized, bps)
	return strings.Join([]string{begin, finalized, notFinalized, bps, graph, end}, "\n")
}

func graphUpdateBP(ns []*bpNode, start, graph, bp string) (string, string) {
	for _, u := range ns {
		str := fmt.Sprintf("proposal_%x", u.BP[:2])
		bp += " " + str
		graph += start + " -> " + str + "\n"
	}
	return graph, bp
}

func graphUpdateBlock(ns []*blockNode, start, graph, block, bp string) (string, string, string) {
	for _, u := range ns {
		str := fmt.Sprintf("block_%x", u.Block[:2])
		block += " " + str
		graph += start + " -> " + str + "\n"

		if len(u.blockChildren) > 0 {
			graph, block, bp = graphUpdateBlock(u.blockChildren, str, graph, block, bp)
		}

		if len(u.bpChildren) > 0 {
			graph, bp = graphUpdateBP(u.bpChildren, str, graph, bp)
		}
	}
	return graph, block, bp
}
