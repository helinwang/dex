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
	addr  Addr
	cfg   Config
	sk    SK
	net   *gateway
	chain *Chain

	mu sync.Mutex
	// the memberships of different groups
	memberships    []membership
	notarizeChs    []chan *BlockProposal
	bpForNotary    map[uint64][]*BlockProposal
	round          uint64
	roundEnd       bool
	cancelNotarize func()
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
func NewNode(chain *Chain, sk SK, net *gateway, cfg Config) *Node {
	pk, err := sk.PK()
	if err != nil {
		panic(err)
	}

	addr := pk.Addr()
	n := &Node{
		addr:        addr,
		cfg:         cfg,
		sk:          sk,
		chain:       chain,
		net:         net,
		bpForNotary: make(map[uint64][]*BlockProposal),
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
	return n.net.Start(host, port, seedAddr)
}

// StartRound marks the start of the given round. It happens when the
// random beacon signature for the given round is received.
func (n *Node) StartRound(round uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.round = round
	n.roundEnd = false
	var ntCancelCtx context.Context
	rb, bp, nt := n.chain.randomBeacon.Committees(round)
	log.Info("start round", "round", round, "rb group", rb, "bp group", bp, "nt group", nt, "rand beacon", SHA3(n.chain.randomBeacon.History()[round].Sig))

	// at most spend blockTime / 2 for proposing block, to avoid
	// the case that there are too many transactions to be
	// included in the block proposal
	proposeBlockCtx, cancelProposeBlock := context.WithTimeout(context.Background(), n.cfg.BlockTime/2)
	defer cancelProposeBlock()
	for _, m := range n.memberships {
		if m.groupID == bp {
			bp := n.chain.ProposeBlock(proposeBlockCtx, n.sk)
			go func() {
				log.Debug("proposing block", "owner", n.addr, "round", bp.Round, "hash", bp.Hash())
				n.net.recvBlockProposal(n.net.addr, bp)
			}()
		}

		if m.groupID == nt {
			if ntCancelCtx == nil {
				ntCancelCtx, n.cancelNotarize = context.WithCancel(context.Background())
			}

			notary := NewNotary(n.addr, n.sk, m.skShare, n.chain)
			inCh := make(chan *BlockProposal, 20)
			n.notarizeChs = append(n.notarizeChs, inCh)
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(n.cfg.BlockTime))
			go func() {
				onNotarize := func(s *NtShare) {
					go n.net.recvNtShare(n.net.addr, s)
				}

				notary.Notarize(ctx, ntCancelCtx, inCh, onNotarize)
				cancel()
			}()
		}
	}

	if bps := n.bpForNotary[round]; len(bps) > 0 {
		if len(n.notarizeChs) > 0 {
			for _, ch := range n.notarizeChs {
				for _, bp := range bps {
					ch <- bp
				}
			}
		}
		delete(n.bpForNotary, round)
	}
}

// EndRound marks the end of the given round. It happens when the
// block for the given round is received.
func (n *Node) EndRound(round uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	log.Info("end round", "round", round)
	n.roundEnd = true
	n.notarizeChs = nil
	if n.cancelNotarize != nil {
		n.cancelNotarize()
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
			n.net.recvRandBeaconSigShare(n.net.addr, s)
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
	} else if bp.Round > n.round || (n.round == bp.Round && len(n.notarizeChs) == 0 && !n.roundEnd) {
		n.bpForNotary[n.round] = append(n.bpForNotary[n.round], bp)
		return
	}

	for _, ch := range n.notarizeChs {
		ch <- bp
	}
}

func (n *Node) SendTxn(t []byte) {
	n.net.recvTxn(t)
}

// MakeNode makes a new node with the given configurations.
func MakeNode(credentials NodeCredentials, cfg Config, genesis Genesis, state State, txnPool TxnPool, u Updater) *Node {
	randSeed := Rand(SHA3([]byte("dex")))
	err := state.Deserialize(genesis.State)
	if err != nil {
		panic(err)
	}

	chain := NewChain(&genesis.Block, state, randSeed, cfg, txnPool, u)
	net := newNetwork(credentials.SK)
	networking := newGateway(net, chain)
	node := NewNode(chain, credentials.SK, networking, cfg)
	for j := range credentials.Groups {
		share := credentials.GroupShares[j]
		m := membership{groupID: credentials.Groups[j], skShare: share}
		node.memberships = append(node.memberships, m)
	}
	node.chain.randomBeacon.n = node
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
