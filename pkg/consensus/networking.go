package consensus

import (
	"bytes"
	"context"
	"encoding/gob"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	log "github.com/helinwang/log15"
)

const (
	connectPeerCount = 8
)

// TODO: networking should ensure that adding things to the chain is
// in order: from lower round to higher round.

// Item is the identification of an item that the current node owns.
type Item struct {
	T     ItemType
	Round uint64
	Hash  Hash
}

type ItemRequest Item

// ItemType is the different type of items.
type ItemType int

// different types of items
const (
	TxnItem ItemType = iota
	SysTxnItem
	BlockItem
	BlockProposalItem
	NtShareItem
	// TODO: rename to RandBeaconSigShareItem and RandBeaconSigItem
	RandBeaconShareItem
	RandBeaconItem
)

func (i ItemType) String() string {
	switch i {
	case TxnItem:
		return "TxnItem"
	case SysTxnItem:
		return "SysTxnItem"
	case BlockItem:
		return "BlockItem"
	case BlockProposalItem:
		return "BlockProposalItem"
	case NtShareItem:
		return "NtShareItem"
	case RandBeaconShareItem:
		return "RandBeaconShareItem"
	case RandBeaconItem:
		return "RandBeaconItem"
	default:
		panic("unknown item")
	}
}

// Network is used to connect to the peers.
// type Network interface {
// 	Start(addr string, onPeerConnect func(p Unicast)) error
// 	Connect(addr Unicast) error
// }

// Networking is the component that enables the node to talk to its
// peers over the network.
type Networking struct {
	addr               UnicastAddr
	net                *Network
	v                  *validator
	chain              *Chain
	blockCache         *lru.Cache
	bpCache            *lru.Cache
	randBeaconSigCache *lru.Cache
	syncer             *syncer

	mu           sync.Mutex
	rbSigWaiters map[uint64][]chan *RandBeaconSig
	blockWaiters map[Hash][]chan *Block
	bpWaiters    map[Hash][]chan *BlockProposal
}

// NewNetworking creates a new networking component.
func NewNetworking(net *Network, chain *Chain) *Networking {
	bCache, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	bpCache, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	randBeaconSigCache, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	n := &Networking{
		net:                net,
		v:                  newValidator(chain),
		chain:              chain,
		blockCache:         bCache,
		bpCache:            bpCache,
		randBeaconSigCache: randBeaconSigCache,
		rbSigWaiters:       make(map[uint64][]chan *RandBeaconSig),
		blockWaiters:       make(map[Hash][]chan *Block),
		bpWaiters:          make(map[Hash][]chan *BlockProposal),
	}

	n.syncer = newSyncer(n.v, chain, n)
	return n
}

func gobEncode(v interface{}) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		// should not happen
		panic(err)
	}
	return buf.Bytes()
}

func (n *Networking) requestItem(addr UnicastAddr, item Item) error {
	return n.net.Send(addr, Packet{Data: ItemRequest(item)})
}

func (n *Networking) RequestRandBeaconSig(ctx context.Context, addr UnicastAddr, round uint64) (*RandBeaconSig, error) {
	v, ok := n.randBeaconSigCache.Get(round)
	if ok {
		return v.(*RandBeaconSig), nil
	}

	c := make(chan *RandBeaconSig, 1)
	n.mu.Lock()
	n.rbSigWaiters[round] = append(n.rbSigWaiters[round], c)
	if len(n.rbSigWaiters[round]) == 1 {
		err := n.requestItem(addr, Item{
			T:     RandBeaconItem,
			Round: round,
		})
		if err != nil {
			n.mu.Unlock()
			return nil, err
		}
	}
	n.mu.Unlock()

	select {
	case r := <-c:
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *Networking) RequestBlock(ctx context.Context, addr UnicastAddr, hash Hash) (*Block, error) {
	v, ok := n.blockCache.Get(hash)
	if ok {
		return v.(*Block), nil
	}

	if b := n.chain.Block(hash); b != nil {
		return b, nil
	}

	c := make(chan *Block, 1)
	n.mu.Lock()
	n.blockWaiters[hash] = append(n.blockWaiters[hash], c)
	if len(n.blockWaiters[hash]) == 1 {
		err := n.requestItem(addr, Item{
			T:    BlockItem,
			Hash: hash,
		})
		if err != nil {
			n.mu.Unlock()
			return nil, err
		}
	}
	n.mu.Unlock()

	select {
	case b := <-c:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *Networking) RequestBlockProposal(ctx context.Context, addr UnicastAddr, hash Hash) (*BlockProposal, error) {
	v, ok := n.bpCache.Get(hash)
	if ok {
		return v.(*BlockProposal), nil
	}

	if bp := n.chain.BlockProposal(hash); bp != nil {
		return bp, nil
	}

	c := make(chan *BlockProposal, 1)
	n.mu.Lock()
	n.bpWaiters[hash] = append(n.bpWaiters[hash], c)
	if len(n.bpWaiters[hash]) == 1 {
		err := n.requestItem(addr, Item{
			T:    BlockProposalItem,
			Hash: hash,
		})
		if err != nil {
			n.mu.Unlock()
			return nil, err
		}
	}
	n.mu.Unlock()

	select {
	case bp := <-c:
		return bp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Start starts the networking component.
// TODO: fix lint
// nolint: gocyclo
func (n *Networking) Start(host string, port int, seedAddr string) error {
	myAddr, err := n.net.Start(host, port)
	if err != nil {
		return err
	}
	n.addr = myAddr

	return n.net.ConnectSeed(seedAddr)
}

// TODO: don't broadcast when syncing.

func (n *Networking) broadcast(item Item) {
	n.net.Send(Broadcast{}, Packet{Data: item})
}

func (n *Networking) RecvTxn(t []byte) {
	broadcast := n.chain.TxnPool.Add(t)
	if broadcast {
		go n.broadcast(Item{T: TxnItem, Hash: SHA3(t)})
	}
}

func (n *Networking) RecvSysTxn(t *SysTxn) {
	panic("not implemented")
}

func (n *Networking) recvRandBeaconSig(addr UnicastAddr, r *RandBeaconSig) {
	if !n.v.ValidateRandBeaconSig(r) {
		log.Warn("failed to validate rand beacon sig", "round", r.Round, "hash", r.Hash())
		return
	}

	n.randBeaconSigCache.Add(r.Round, r)
	n.mu.Lock()
	for _, ch := range n.rbSigWaiters[r.Round] {
		ch <- r
	}
	n.rbSigWaiters[r.Round] = nil
	n.mu.Unlock()

	broadcast, err := n.syncer.SyncRandBeaconSig(addr, r.Round)
	if err != nil {
		log.Warn("SyncRandBeaconSig failed", "err", err)
		return
	}

	if broadcast {
		go n.broadcast(Item{T: RandBeaconItem, Round: r.Round})
	}
}

func (n *Networking) recvRandBeaconSigShare(addr UnicastAddr, r *RandBeaconSigShare) {
	groupID, valid := n.v.ValidateRandBeaconSigShare(r)

	if !valid {
		return
	}

	sig, success := n.chain.RandomBeacon.AddRandBeaconSigShare(r, groupID)
	if !success {
		return
	}

	if sig != nil {
		go n.recvRandBeaconSig(addr, sig)
		return
	}

	go n.broadcast(Item{T: RandBeaconShareItem, Round: r.Round})
}

func (n *Networking) recvBlock(addr UnicastAddr, b *Block) {
	// TODO: if not able to validate block, wait until random
	// beacon syncer finished the corresponding round.
	_, valid := n.v.ValidateBlock(b)

	if !valid {
		return
	}

	h := b.Hash()
	n.blockCache.Add(h, b)

	n.mu.Lock()
	for _, c := range n.blockWaiters[h] {
		c <- b
	}
	n.blockWaiters[h] = nil
	n.mu.Unlock()

	err := n.syncer.SyncBlock(addr, h, b.Round)
	if err != nil {
		log.Warn("sync block error", "err", err)
		return
	}

	go n.broadcast(Item{T: BlockItem, Hash: b.Hash()})
}

func (n *Networking) recvBlockProposal(addr UnicastAddr, bp *BlockProposal) {
	weight, valid := n.v.ValidateBlockProposal(bp)
	if !valid {
		return
	}

	h := bp.Hash()
	n.bpCache.Add(h, bp)

	n.mu.Lock()
	for _, c := range n.bpWaiters[h] {
		c <- bp
	}
	n.bpWaiters[h] = nil
	n.mu.Unlock()

	if bp.Round > 0 {
		err := n.syncer.SyncBlock(addr, bp.PrevBlock, bp.Round-1)
		if err != nil {
			log.Warn("sync block error", "err", err)
			return
		}
	} else {
		log.Error("round 0 does should not have block proposal")
		return
	}

	err := n.chain.addBP(bp, weight)
	if err != nil {
		log.Warn("add block proposal failed", "err", err)
		return
	}

	go n.broadcast(Item{T: BlockProposalItem, Hash: bp.Hash()})
}

func (n *Networking) recvNtShare(addr UnicastAddr, s *NtShare) {
	log.Info("recv nt share", "hash", s.Hash())
	groupID, valid := n.v.ValidateNtShare(s)
	if !valid {
		return
	}

	b, success := n.chain.addNtShare(s, groupID)
	if !success {
		return
	}

	if b != nil {
		go n.recvBlock(addr, b)
		return
	}

	// TODO: use multicast rather than broadcast
	go n.broadcast(Item{T: NtShareItem, Hash: s.Hash()})
}

// TODO: fix lint
// nolint: gocyclo
func (n *Networking) recvInventory(addr UnicastAddr, item Item) {
	n.mu.Lock()
	defer n.mu.Unlock()

	log.Info("recv inventory", "inventory", item)

	switch item.T {
	case TxnItem:
		if n.chain.TxnPool.NotSeen(item.Hash) {
			log.Info("request TxnItem", "item", item)
			n.requestItem(addr, item)
		}
	case SysTxnItem:
		panic("not implemented")
	case BlockItem:
		if b := n.chain.Block(item.Hash); b != nil {
			return
		}

		log.Info("request BlockItem", "item", item)
		n.requestItem(addr, item)
	case BlockProposalItem:
		if bp := n.chain.BlockProposal(item.Hash); bp != nil {
			return
		}

		log.Info("request BlockProposalItem", "item", item)
		// TODO: replace n.requestItem with syncer.Sync
		n.requestItem(addr, item)
	case NtShareItem:
		// TODO: move all existance check to syncer.
		if nt := n.chain.NtShare(item.Hash); nt != nil {
			return
		}

		log.Info("request NtShareItem", "item", item)
		// TODO: rename blockSyncer to syncer
		go n.syncer.SyncNtShare(addr, item.Hash)
	case RandBeaconShareItem:
		share := n.chain.RandomBeacon.GetShare(item.Hash)
		if share != nil {
			return
		}

		log.Info("request RandBeaconShareItem", "item", item)
		go n.syncer.SyncRandBeaconSigShare(addr, item.Round)
	case RandBeaconItem:
		log.Info("request RandBeaconItem", "item", item)
		go n.syncer.SyncRandBeaconSig(addr, item.Round)
	}
}

// TODO: fix lint
// nolint: gocyclo
func (n *Networking) serveData(addr UnicastAddr, item Item) {
	switch item.T {
	case TxnItem:
		txn := n.chain.TxnPool.Get(item.Hash)
		if txn != nil {
			log.Info("serving TxnItem", "item", item)
			go n.net.Send(addr, Packet{Data: txn})
		}
	case SysTxnItem:
		panic("not implemented")
	case BlockItem:
		b := n.chain.Block(item.Hash)
		if b == nil {
			return
		}

		log.Info("serving BlockItem", "item", item)
		go n.net.Send(addr, Packet{Data: b})
	case BlockProposalItem:
		bp := n.chain.BlockProposal(item.Hash)
		if bp == nil {
			return
		}

		log.Info("serving BlockProposalItem", "item", item)
		go n.net.Send(addr, Packet{Data: bp})
	case NtShareItem:
		nts := n.chain.NtShare(item.Hash)
		if nts == nil {
			return
		}

		log.Info("serving NtShareItem", "item", item)
		go n.net.Send(addr, Packet{Data: nts})
	case RandBeaconShareItem:
		share := n.chain.RandomBeacon.GetShare(item.Hash)
		if share == nil {
			return
		}

		log.Info("serving RandBeaconShareItem", "item", item)
		go n.net.Send(addr, Packet{Data: share})
	case RandBeaconItem:
		history := n.chain.RandomBeacon.History()
		r := history[item.Round]
		log.Info("serving RandBeaconItem", "item", item)
		go n.net.Send(addr, Packet{Data: r})
	}
}
