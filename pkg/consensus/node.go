package consensus

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	log "github.com/helinwang/log15"
)

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr    Addr
	cfg     Config
	sk      SK
	gateway *gateway
	chain   *Chain
	store   *storage

	mu sync.Mutex
	// the memberships of different groups
	memberships    []membership
	notarizeChs    map[uint64][]chan *BlockProposal
	bpForNotary    map[uint64][]*BlockProposal
	round          uint64
	recvBlockTime  map[uint64]time.Time
	cancelNotarize map[uint64]func()
}

// NodeCredentials stores the credentials of the node.
type NodeCredentials struct {
	SK          SK
	Groups      []int
	GroupShares []SK
}

type membership struct {
	skShare SK
	groupID int
}

// Config is the consensus layer configuration.
type Config struct {
	BlockTime      time.Duration
	GroupSize      int
	GroupThreshold int
}

// NewNode creates a new node.
func NewNode(chain *Chain, sk SK, net *gateway, cfg Config, store *storage) *Node {
	pk, err := sk.PK()
	if err != nil {
		panic(err)
	}

	addr := pk.Addr()
	n := &Node{
		addr:           addr,
		store:          store,
		cfg:            cfg,
		sk:             sk,
		chain:          chain,
		gateway:        net,
		bpForNotary:    make(map[uint64][]*BlockProposal),
		notarizeChs:    make(map[uint64][]chan *BlockProposal),
		cancelNotarize: make(map[uint64]func()),
		recvBlockTime:  make(map[uint64]time.Time),
	}
	chain.n = n
	return n
}

// Chain returns node's block chain.
func (n *Node) Chain() *Chain {
	return n.chain
}

// Start starts the p2p network service.
func (n *Node) Start(host string, port int, seedAddr string) error {
	return n.gateway.Start(host, port, seedAddr)
}

func (n *Node) proposeBlock(round uint64, group int, lastRoundEndTime time.Time) {
	n.chain.WaitUntil(round)
	n.mu.Lock()
	nodeRound := n.round
	n.mu.Unlock()

	if nodeRound > round {
		// missed the chance to propose block
		return
	}

	// at most spend blockTime/3 for proposing block, to avoid the
	// case that there are too many transactions to be included in
	// the block proposal
	ctx, cancel := context.WithTimeout(context.Background(), n.cfg.BlockTime/3)
	defer cancel()

	start := time.Now()
	log.Debug("start propose block", "owner", n.addr, "round", round, "group", group, "since last round end", time.Now().Sub(lastRoundEndTime))
	bp := n.chain.ProposeBlock(ctx, n.sk, round)
	h := bp.Hash()
	if bp != nil {
		log.Info("propose block done", "owner", n.addr, "round", round, "hash", h, "group", group, "since last round end", time.Now().Sub(lastRoundEndTime), "dur", time.Now().Sub(start))
		n.gateway.recvBlockProposal(n.gateway.addr, bp, h)
	}

}

func (n *Node) notarizeBlock(notary *Notary, inCh chan *BlockProposal, cancelCtx context.Context, lastRoundEndTime time.Time, round uint64, group int) {
	log.Debug("begin notarize", "group", group, "round", round)
	onNotarize := func(s *NtShare, spentTime time.Duration) {
		h := s.Hash()
		sinceLastRoundEnd := time.Now().Sub(lastRoundEndTime)
		remainTime := n.cfg.BlockTime - spentTime - sinceLastRoundEnd
		log.Info("produced one notarization share", "group", group, "round", round, "notarized proposal", s.BP, "hash", h, "since last round end", sinceLastRoundEnd, "remain time", remainTime)
		if remainTime <= 0 {
			go n.gateway.recvNtShare(n.gateway.addr, s, h)
		} else {
			time.AfterFunc(remainTime, func() {
				n.gateway.recvNtShare(n.gateway.addr, s, h)
			})
		}
	}

	ctx, cancel := context.WithDeadline(context.Background(), lastRoundEndTime.Add(n.cfg.BlockTime))
	defer cancel()
	notary.Notarize(ctx, cancelCtx, inCh, onNotarize)
}

// StartRound marks the start of the given round. It happens when the
// random beacon signature for the given round is received.
func (n *Node) StartRound(round uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.round >= round {
		return
	}

	recvLastRoundBlock, ok := n.recvBlockTime[round-1]
	if !ok {
		recvLastRoundBlock = time.Now()
	}

	n.round = round
	var ntCancelCtx context.Context
	rbGroup, bpGroup, ntGroup := n.chain.randomBeacon.Committees(round)
	log.Info("start round", "round", round, "rand beacon", SHA3(n.chain.randomBeacon.History()[round].Sig), "rb group", rbGroup, "bp group", bpGroup, "nt group", ntGroup)

	for _, m := range n.memberships {
		if m.groupID == bpGroup {
			go n.proposeBlock(round, bpGroup, recvLastRoundBlock)
		}

		if m.groupID == ntGroup {
			if ntCancelCtx == nil {
				ntCancelCtx, n.cancelNotarize[round] = context.WithCancel(context.Background())
			}

			notary := NewNotary(n.addr, n.sk, m.skShare, n.chain, n.store)
			inCh := make(chan *BlockProposal, 20)
			n.notarizeChs[round] = append(n.notarizeChs[round], inCh)
			go n.notarizeBlock(notary, inCh, ntCancelCtx, recvLastRoundBlock, round, ntGroup)
		}
	}

	if bps := n.bpForNotary[round]; len(bps) > 0 {
		if len(n.notarizeChs[round]) > 0 {
			for _, ch := range n.notarizeChs[round] {
				for _, bp := range bps {
					ch <- bp
				}
			}
		}
		delete(n.bpForNotary, round)
	}
}

func (n *Node) BlockForRoundProduced(round uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.recvBlockTime[round]; !ok {
		n.recvBlockTime[round] = time.Now()
	}
}

// EndRound marks the end of the given round. It happens when the
// block for the given round is received.
func (n *Node) EndRound(round uint64) {
	log.Info("end round", "round", round)
	delete(n.notarizeChs, round)
	if c := n.cancelNotarize[round]; c != nil {
		c()
		delete(n.cancelNotarize, round)
	}

	rb, _, _ := n.chain.randomBeacon.Committees(round)
	for _, m := range n.memberships {
		if m.groupID != rb {
			continue
		}
		// Current node is a member of the random
		// beacon committee, members collatively
		// produce the random beacon signature using
		// BLS threshold signature scheme. There are
		// multiple committees, which committee will
		// produce the next random beacon signature is
		// derived from the current random beacon
		// signature.
		keyShare := m.skShare
		go func() {
			history := n.chain.randomBeacon.History()
			lastSigHash := SHA3(history[round].Sig)
			s := signRandBeaconSigShare(n.sk, keyShare, round+1, lastSigHash)
			n.gateway.recvRandBeaconSigShare(n.gateway.addr, s)
		}()
	}
}

// RecvBlockProposal tells the node that a valid block proposal of the
// current round is received.
func (n *Node) recvBPForNotary(bp *BlockProposal) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if bp.Round < n.round {
		return
	} else if bp.Round > n.round {
		n.bpForNotary[bp.Round] = append(n.bpForNotary[bp.Round], bp)
		return
	}

	for _, ch := range n.notarizeChs[bp.Round] {
		ch <- bp
	}
}

func (n *Node) SendTxn(t []byte) {
	n.gateway.recvTxn(t)
}

// MakeNode makes a new node with the given configurations.
func MakeNode(credentials NodeCredentials, cfg Config, genesis Genesis, state State, txnPool TxnPool, u Updater, proposerPK []byte) *Node {
	randSeed := Rand(SHA3([]byte("dex")))
	err := state.Deserialize(genesis.State)
	if err != nil {
		panic(err)
	}

	store := newStorage()
	chain := NewChain(&genesis.Block, state, randSeed, cfg, txnPool, u, store, proposerPK)
	net := newNetwork(credentials.SK)
	gateway := newGateway(net, chain, store, cfg.GroupThreshold)
	net.onPeerConnect = gateway.onPeerConnect
	node := NewNode(chain, credentials.SK, gateway, cfg, store)
	for j := range credentials.Groups {
		share := credentials.GroupShares[j]
		m := membership{groupID: credentials.Groups[j], skShare: share}
		node.memberships = append(node.memberships, m)
	}
	node.chain.randomBeacon.n = node
	gateway.node = node
	gateway.syncer.node = node
	return node
}

// LoadCredential loads node credential from disk.
func LoadCredential(path string) (NodeCredentials, error) {
	var c NodeCredentials
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("open credential file failed: %v", err)
	}

	dec := gob.NewDecoder(bytes.NewReader(b))
	err = dec.Decode(&c)
	if err != nil {
		return c, fmt.Errorf("decode credential file failed: %v", err)
	}

	return c, nil
}
