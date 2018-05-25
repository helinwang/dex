package consensus

import (
	"context"
	"sync"
	"time"

	log "github.com/helinwang/log15"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Node is a node in the consensus infrastructure.
//
// Nodes form a group randomly, the randomness comes from the random
// beacon.
type Node struct {
	addr  Addr
	cfg   Config
	sk    bls.SecretKey
	net   *Networking
	chain *Chain

	mu sync.Mutex
	// the memberships of different groups
	memberships    []membership
	notarizeChs    []chan *BlockProposal
	cancelNotarize func()
}

// NodeCredentials stores the credentials of the node.
type NodeCredentials struct {
	SK          []byte
	Groups      []int
	GroupShares [][]byte
}

type membership struct {
	skShare bls.SecretKey
	groupID int
}

// Config is the consensus layer configuration.
type Config struct {
	ProposalWaitDur time.Duration
	BlockTime       time.Duration
	GroupSize       int
	GroupThreshold  int
}

// NewNode creates a new node.
func NewNode(chain *Chain, sk bls.SecretKey, net *Networking, cfg Config) *Node {
	pk := sk.GetPublicKey()
	pkHash := SHA3(pk.Serialize())
	addr := pkHash.Addr()
	n := &Node{
		addr:  addr,
		cfg:   cfg,
		sk:    sk,
		chain: chain,
		net:   net,
	}
	chain.n = n
	return n
}

// StartRound tells the node that a new round has just started.
func (n *Node) StartRound(round int) {
	n.mu.Lock()
	defer n.mu.Unlock()

	log.Debug("start round", "round", round, "addr", n.addr)

	n.notarizeChs = nil
	if n.cancelNotarize != nil {
		n.cancelNotarize()
	}

	var ntCancelCtx context.Context
	rb, bp, nt := n.chain.RandomBeacon.Committees(round)
	for _, m := range n.memberships {
		if m.groupID == rb {
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
				history := n.chain.RandomBeacon.History()
				idx := round - 1
				if idx >= len(history) {
					// TODO: handle this case better, need to be retry
					log.Error("new round started, but have not received last round random beacon", "idx", idx, "len", len(history))
					return
				}

				lastSigHash := SHA3(history[idx].Sig)
				s := signRandBeaconShare(n.sk, keyShare, round, lastSigHash)
				n.net.recvRandBeaconSigShare(s)
			}()
		}

		if m.groupID == bp {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(n.cfg.ProposalWaitDur))
			go func() {
				block, state, sysState := n.chain.Leader()
				b := NewBlockProposer(n.sk, block, state, sysState)
				// TODO: handle txn
				proposal := b.CollectTxn(ctx, nil, nil, make(chan []byte, 100))
				cancel()
				log.Debug("proposing block", "addr", n.addr, "round", proposal.Round)
				n.net.recvBlockProposal(proposal)
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
				shares := notary.Notarize(ctx, ntCancelCtx, inCh)
				cancel()

				for _, b := range shares {
					go n.net.recvNtShare(b)
				}
			}()
		}
	}
}

// RecvBlockProposal tells the node that a valid block proposal of the
// current round is received.
func (n *Node) RecvBlockProposal(bp *BlockProposal) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, ch := range n.notarizeChs {
		ch <- bp
	}
}
