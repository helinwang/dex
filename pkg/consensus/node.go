package consensus

import (
	"bytes"
	"context"
	"encoding/gob"
	"io/ioutil"
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
	SK          SK
	Groups      []int
	GroupShares []SK
}

type membership struct {
	skShare bls.SecretKey
	groupID int
}

// Config is the consensus layer configuration.
type Config struct {
	BlockTime      time.Duration
	GroupSize      int
	GroupThreshold int
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

func (n *Node) Start(myAddr, seedAddr string) {
	n.net.Start(myAddr, seedAddr)
}

// StartRound tells the node that a new round has just started.
func (n *Node) StartRound(round uint64) {
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
				if round < 1 {
					log.Error("round should not < 1")
					return
				}

				idx := round - 1
				if idx >= uint64(len(history)) {
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
			txns := n.chain.TxnPool.Txns()
			block, _, _ := n.chain.Leader()
			var bp BlockProposal
			bp.PrevBlock = SHA3(block.Encode(true))
			bp.Round = block.Round + 1
			bp.Owner = SHA3(n.sk.GetPublicKey().Serialize()).Addr()
			// TODO: support SysTxn when needed (e.g., open participation)
			bp.Data = txns
			bp.OwnerSig = n.sk.Sign(string(bp.Encode(false))).Serialize()

			go func() {
				log.Debug("proposing block", "addr", n.addr, "round", bp.Round, "hash", bp.Hash())
				n.net.recvBlockProposal(&bp)
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
					go n.net.recvNtShare(s)
				}

				notary.Notarize(ctx, ntCancelCtx, inCh, onNotarize)
				cancel()
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

func (n *Node) SendTxn(t []byte) {
	n.net.RecvTxn(t)
}

// MakeNode makes a new node with the given configurations.
func MakeNode(credentials NodeCredentials, net Network, cfg Config, genesis *Block, state State, txnPool TxnPool, u Updater) *Node {
	randSeed := Rand(SHA3([]byte("dex")))

	sk, err := credentials.SK.Get()
	if err != nil {
		panic(err)
	}

	chain := NewChain(genesis, state, randSeed, cfg, txnPool, u)
	networking := NewNetworking(net, chain)
	node := NewNode(chain, sk, networking, cfg)
	for j := range credentials.Groups {
		share, err := credentials.GroupShares[j].Get()
		if err != nil {
			panic(err)
		}

		m := membership{groupID: credentials.Groups[j], skShare: share}
		node.memberships = append(node.memberships, m)
	}

	return node
}

// LoadCredential loads node credential from disk.
func LoadCredential(path string) (NodeCredentials, error) {
	var c NodeCredentials
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return c, err
	}

	dec := gob.NewDecoder(bytes.NewReader(b))
	err = dec.Decode(&c)
	if err != nil {
		return c, err
	}

	return c, nil
}
