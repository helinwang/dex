package consensus

import (
	"context"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	log "github.com/helinwang/log15"
)

const (
	connectPeerCount = 8
)

// Item is the identification of an item that the current node owns.
type Item struct {
	T     itemType
	Round uint64
	Hash  Hash
}

func (i Item) String() string {
	switch i.T {
	case txnItem, sysTxnItem, blockItem, shardBlockProposalItem, shardBlockItem, ntShareItem:
		return fmt.Sprintf("%v_hash_%v", i.T, i.Hash)
	case randBeaconSigShareItem, randBeaconSigItem:
		return fmt.Sprintf("%v_round_%v", i.T, i.Round)
	default:
		panic(i.T)
	}
}

type itemRequest Item

// itemType is the different type of items.
type itemType int

// different types of items
const (
	txnItem itemType = iota
	sysTxnItem
	blockItem
	shardBlockItem
	shardBlockProposalItem
	ntShareItem
	randBeaconSigShareItem
	randBeaconSigItem
)

func (i itemType) String() string {
	switch i {
	case txnItem:
		return "TxnItem"
	case sysTxnItem:
		return "SysTxnItem"
	case blockItem:
		return "BlockItem"
	case shardBlockItem:
		return "ShardBlockItem"
	case shardBlockProposalItem:
		return "ShardBlockProposalItem"
	case ntShareItem:
		return "NtShareItem"
	case randBeaconSigShareItem:
		return "RandBeaconSigShareItem"
	case randBeaconSigItem:
		return "RandBeaconSigItem"
	default:
		panic("unknown item")
	}
}

// gateway is the gateway through which the node talks with its peers
// in the network.
type gateway struct {
	addr                     unicastAddr
	net                      *network
	v                        *validator
	chain                    *Chain
	syncer                   *syncer
	shardBlockCache          *lru.Cache
	blockCache               *lru.Cache
	bpCache                  *lru.Cache
	randBeaconSigCache       *lru.Cache
	node                     *Node
	store                    *storage
	shardNtShareCollector    *collector
	randBeaconShareCollector *collector

	mu                sync.Mutex
	rbSigWaiters      map[uint64][]chan *RandBeaconSig
	blockWaiters      map[Hash][]chan *Block
	shardBlockWaiters map[Hash][]chan *ShardBlock
	bpWaiters         map[Hash][]chan *ShardBlockProposal
	requestingItem    map[Item]bool
}

func newGateway(net *network, chain *Chain, store *storage, groupThreshold int) *gateway {
	bCache, err := lru.New(1024)
	if err != nil {
		panic(err)
	}

	sbCache, err := lru.New(1024)
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

	n := &gateway{
		net:                      net,
		store:                    store,
		shardBlockCache:          sbCache,
		blockCache:               bCache,
		bpCache:                  bpCache,
		randBeaconSigCache:       randBeaconSigCache,
		v:                        newValidator(chain, store),
		chain:                    chain,
		rbSigWaiters:             make(map[uint64][]chan *RandBeaconSig),
		blockWaiters:             make(map[Hash][]chan *Block),
		shardBlockWaiters:        make(map[Hash][]chan *ShardBlock),
		bpWaiters:                make(map[Hash][]chan *ShardBlockProposal),
		requestingItem:           make(map[Item]bool),
		shardNtShareCollector:    newCollector(groupThreshold),
		randBeaconShareCollector: newCollector(groupThreshold),
	}

	n.syncer = newSyncer(chain, n, store)
	return n
}

func (n *gateway) requestItem(addr unicastAddr, item Item, forceRequest bool) error {
	if !forceRequest && n.requestingItem[item] {
		return nil
	}

	log.Debug("requesting item", "item", item)
	n.requestingItem[item] = true
	time.AfterFunc(2*time.Second, func() {
		n.mu.Lock()
		delete(n.requestingItem, item)
		n.mu.Unlock()
	})
	return n.net.Send(addr, packet{Data: itemRequest(item)})
}

func (n *gateway) RequestRandBeaconSig(ctx context.Context, addr unicastAddr, round uint64) (*RandBeaconSig, error) {
	cached, ok := n.randBeaconSigCache.Get(round)
	if ok {
		return cached.(*RandBeaconSig), nil
	}

	v := n.store.RandBeaconSig(round)
	if v != nil {
		return v, nil
	}

	c := make(chan *RandBeaconSig, 1)
	n.mu.Lock()
	n.rbSigWaiters[round] = append(n.rbSigWaiters[round], c)
	if len(n.rbSigWaiters[round]) == 1 {
		err := n.requestItem(addr, Item{
			T:     randBeaconSigItem,
			Round: round,
		}, true)
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

func (n *gateway) RequestBlock(ctx context.Context, addr unicastAddr, hash Hash) (*Block, error) {
	v, ok := n.blockCache.Get(hash)
	if ok {
		return v.(*Block), nil
	}

	if b := n.store.Block(hash); b != nil {
		return b, nil
	}

	c := make(chan *Block, 1)
	n.mu.Lock()
	n.blockWaiters[hash] = append(n.blockWaiters[hash], c)
	if len(n.blockWaiters[hash]) == 1 {
		err := n.requestItem(addr, Item{
			T:    blockItem,
			Hash: hash,
		}, true)
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

func (n *gateway) RequestShardBlock(ctx context.Context, addr unicastAddr, hash Hash) (*ShardBlock, error) {
	v, ok := n.shardBlockCache.Get(hash)
	if ok {
		return v.(*ShardBlock), nil
	}

	if b := n.store.ShardBlock(hash); b != nil {
		return b, nil
	}

	c := make(chan *ShardBlock, 1)
	n.mu.Lock()
	n.shardBlockWaiters[hash] = append(n.shardBlockWaiters[hash], c)
	if len(n.blockWaiters[hash]) == 1 {
		err := n.requestItem(addr, Item{
			T:    shardBlockItem,
			Hash: hash,
		}, true)
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

func (n *gateway) RequestShardBlockProposal(ctx context.Context, addr unicastAddr, hash Hash) (*ShardBlockProposal, error) {
	v, ok := n.bpCache.Get(hash)
	if ok {
		return v.(*ShardBlockProposal), nil
	}

	if bp := n.store.ShardBlockProposal(hash); bp != nil {
		return bp, nil
	}

	c := make(chan *ShardBlockProposal, 1)
	n.mu.Lock()
	n.bpWaiters[hash] = append(n.bpWaiters[hash], c)
	if len(n.bpWaiters[hash]) == 1 {
		err := n.requestItem(addr, Item{
			T:    shardBlockProposalItem,
			Hash: hash,
		}, true)
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
func (n *gateway) Start(host string, port int, seedAddr string) error {
	myAddr, err := n.net.Start(host, port)
	if err != nil {
		return err
	}
	n.addr = myAddr

	go n.recvData()
	if seedAddr == "" {
		return nil
	}

	return n.net.ConnectSeed(seedAddr)
}

func (n *gateway) recvData() {
	for {
		addr, pac := n.net.Recv()
		// see conn.go:init() for the list of possible data
		// types
		switch v := pac.Data.(type) {
		case []byte:
			log.Debug("recvTxn")
			go n.recvTxn(v)
		case *RandBeaconSig:
			log.Debug("recvRandBeaconSig", "round", v.Round)
			go n.recvRandBeaconSig(addr, v)
		case *RandBeaconSigShare:
			log.Debug("recvRandBeaconSigShare", "round", v.Round)
			go n.recvRandBeaconSigShare(addr, v)
		case *Block:
			h := v.Hash()
			log.Debug("recvBlock", "round", v.Round, "hash", h, "state root", v.StateRoot)
			go n.recvBlock(addr, v, h)
			go n.node.BlockForRoundProduced(v.Round)
		case *ShardBlock:
			h := v.Hash()
			log.Debug("recvShardBlockProposal", "round", v.Round, "hash", h, "block", v.PrevBlock)
			go n.recvShardBlock(addr, v, h)
		case *ShardBlockProposal:
			h := v.Hash()
			log.Debug("recvShardBlockProposal", "round", v.Round, "hash", h, "block", v.PrevBlock)
			go n.recvShardBlockProposal(addr, v, h)
		case *ShardNtShare:
			h := v.Hash()
			log.Debug("recvNtShare", "round", v.Round, "hash", h, "bp", v.BP)
			go n.recvShardNtShare(addr, v, h)
		case Item:
			go n.recvInventory(addr, v)
		case itemRequest:
			go n.serveData(addr, Item(v))
		default:
			panic(fmt.Errorf("received unsupported data type: %T", pac.Data))
		}
	}
}

// shardBroadcast broadcasts the item to peers in the same shard.
func (n *gateway) shardBroadcast(item Item) {
	n.net.Send(shardBroadcast{}, packet{Data: item})
}

func (n *gateway) broadcast(item Item) {
	n.net.Send(broadcast{}, packet{Data: item})
}

func (n *gateway) recvTxn(t []byte) {
	txn, broadcast := n.chain.txnPool.Add(t)
	if broadcast {
		go n.shardBroadcast(Item{T: txnItem, Hash: SHA3(txn.Raw)})
	}
}

func (n *gateway) recvSysTxn(t *SysTxn) {
	panic(sysTxnNotImplemented)
}

func (n *gateway) recvRandBeaconSig(addr unicastAddr, r *RandBeaconSig) {
	if r.Round == 0 {
		log.Error("received RandBeaconSig of 0 round, should not happen")
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
		go n.broadcast(Item{T: randBeaconSigItem, Round: r.Round})
	}
}

func (n *gateway) recvRandBeaconSigShare(addr unicastAddr, r *RandBeaconSigShare) {
	if r.Round == 0 {
		log.Error("received RandBeaconSigShare of 0 round, should not happen")
		return
	}

	h := r.Hash()
	n.chain.randomBeacon.WaitUntil(r.Round - 1)
	groupID, valid := n.v.ValidateRandBeaconSigShare(r)

	if !valid {
		return
	}

	shares, broadcast := n.randBeaconShareCollector.Add(r.LastSigHash, h, r)
	if shares != nil {
		n.randBeaconShareCollector.Remove(r.LastSigHash)
		s := make([]*RandBeaconSigShare, len(shares))
		for i := range s {
			s[i] = shares[i].(*RandBeaconSigShare)
		}
		sig := n.chain.randomBeacon.AddRandBeaconSigShares(s, groupID)
		if sig != nil {
			go n.recvRandBeaconSig(addr, sig)
			// will broadcast rand beacon sig instead of
			// rand beacon sig share.
			return
		}
	}

	if broadcast {
		go n.broadcast(Item{T: randBeaconSigShareItem, Round: r.Round, Hash: h})
	}
}

func (n *gateway) recvShardBlock(addr unicastAddr, b *ShardBlock, h Hash) {
	n.shardBlockCache.Add(h, b)

	n.mu.Lock()
	for _, c := range n.shardBlockWaiters[h] {
		c <- b
	}
	n.blockWaiters[h] = nil
	n.mu.Unlock()

	_, broadcast, err := n.syncer.SyncShardBlock(addr, h, b.Round)
	if err != nil {
		log.Warn("sync shard block error", "err", err)
		return
	}

	if broadcast {
		go n.broadcast(Item{T: shardBlockItem, Hash: h})
	}
}

func (n *gateway) recvBlock(addr unicastAddr, b *Block, h Hash) {
	n.blockCache.Add(h, b)

	n.mu.Lock()
	for _, c := range n.blockWaiters[h] {
		c <- b
	}
	n.blockWaiters[h] = nil
	n.mu.Unlock()

	_, broadcast, err := n.syncer.SyncBlock(addr, h, b.Round)
	if err != nil {
		log.Warn("sync block error", "err", err)
		return
	}

	if broadcast {
		go n.broadcast(Item{T: blockItem, Hash: b.Hash()})
	}
}

func (n *gateway) recvShardBlockProposal(addr unicastAddr, bp *ShardBlockProposal, h Hash) {
	n.bpCache.Add(h, bp)

	n.mu.Lock()
	for _, c := range n.bpWaiters[h] {
		c <- bp
	}
	n.bpWaiters[h] = nil
	n.mu.Unlock()

	_, broadcast, err := n.syncer.SyncShardBlockProposal(addr, h)
	if err != nil {
		log.Warn("sync block proposal error", "err", err)
		return
	}

	if broadcast {
		go n.shardBroadcast(Item{T: shardBlockProposalItem, Hash: h})
	}
}

func (n *gateway) recvShardNtShare(addr unicastAddr, s *ShardNtShare, h Hash) {
	round := n.chain.Round()
	if round > s.Round {
		return
	}

	shares, broadcast := n.shardNtShareCollector.Add(s.BP, h, s)
	if shares != nil {
		ss := make([]*ShardNtShare, len(shares))
		for i := range ss {
			ss[i] = shares[i].(*ShardNtShare)
		}

		bp, _, err := n.syncer.SyncShardBlockProposal(addr, s.BP)
		if err != nil {
			log.Error("error recover shard nt share, can not sync block proposal", "err", err)
			return
		}
		n.shardNtShareCollector.Remove(s.BP)

		shardBlock := recoverShardBlock(ss, bp, n.chain.randomBeacon, bp.ShardIdx)
		go n.recvShardBlock(addr, shardBlock, shardBlock.Hash())
		// will broadcast shard block instead of shard block
		// nt share.
		return
	}

	if broadcast {
		go n.shardBroadcast(Item{T: ntShareItem, Hash: h, Round: s.Round})
	}
}

func recoverShardBlock(shares []*ShardNtShare, bp *ShardBlockProposal, rb *RandomBeacon, shardIdx uint16) *ShardBlock {
	log.Debug("generating shard block from proposal and notarization", "bp", shares[0].BP)
	sig, err := recoverNtSig(shares)
	if err != nil {
		// should not happen
		panic(err)
	}

	_, _, ntGroup, _ := rb.Committees(bp.Round)

	b := &ShardBlock{
		ShardIdx:  shardIdx,
		Owner:     bp.Owner,
		Round:     bp.Round,
		PrevBlock: bp.PrevBlock,
		Txns:      bp.Txns,
	}

	msg := b.Encode(false)
	if !sig.Verify(rb.groups[ntGroup].PK, msg) {
		panic(fmt.Errorf("should never happen: group %d sig not valid", ntGroup))
	}

	b.Notarization = sig
	return b
}

// TODO: fix lint
// nolint: gocyclo
func (n *gateway) recvInventory(addr unicastAddr, item Item) {
	n.mu.Lock()
	defer n.mu.Unlock()

	switch item.T {
	case txnItem:
		if n.chain.txnPool.NotSeen(item.Hash) {
			n.requestItem(addr, item, false)
		}
	case sysTxnItem:
		panic(sysTxnNotImplemented)
	case blockItem:
		if n.blockCache.Contains(item.Hash) {
			return
		}

		if b := n.store.Block(item.Hash); b != nil {
			return
		}

		n.requestItem(addr, item, false)
	case shardBlockItem:
		if n.shardBlockCache.Contains(item.Hash) {
			return
		}

		if b := n.store.ShardBlock(item.Hash); b != nil {
			return
		}

		n.requestItem(addr, item, false)
	case shardBlockProposalItem:
		if n.bpCache.Contains(item.Hash) {
			return
		}

		if bp := n.store.ShardBlockProposal(item.Hash); bp != nil {
			return
		}

		n.requestItem(addr, item, false)
	case ntShareItem:
		if n.chain.Round() > item.Round {
			return
		}

		if nt := n.shardNtShareCollector.Get(item.Hash); nt != nil {
			return
		}

		n.requestItem(addr, item, false)
	case randBeaconSigShareItem:
		if n.chain.randomBeacon.Round()+1 != item.Round {
			return
		}

		share := n.randBeaconShareCollector.Get(item.Hash)
		if share != nil {
			return
		}

		n.requestItem(addr, item, false)
	case randBeaconSigItem:
		if n.chain.randomBeacon.Round() >= item.Round {
			return
		}
		n.requestItem(addr, item, false)
	}
}

// TODO: fix lint
// nolint: gocyclo
func (n *gateway) serveData(addr unicastAddr, item Item) {
	switch item.T {
	case txnItem:
		b := n.chain.txnPool.Get(item.Hash)
		if b == nil {
			return
		}
		go n.net.Send(addr, packet{Data: b.Raw})
	case sysTxnItem:
		panic(sysTxnNotImplemented)
	case blockItem:
		b := n.store.Block(item.Hash)
		if b == nil {
			return
		}
		go n.net.Send(addr, packet{Data: b})
	case shardBlockProposalItem:
		bp := n.store.ShardBlockProposal(item.Hash)
		if bp == nil {
			return
		}
		go n.net.Send(addr, packet{Data: bp})
	case shardBlockItem:
		b := n.store.ShardBlock(item.Hash)
		if b == nil {
			return
		}
		go n.net.Send(addr, packet{Data: b})
	case ntShareItem:
		nts := n.shardNtShareCollector.Get(item.Hash)
		if nts == nil {
			return
		}
		go n.net.Send(addr, packet{Data: nts})
	case randBeaconSigShareItem:
		share := n.randBeaconShareCollector.Get(item.Hash)
		if share == nil {
			return
		}
		go n.net.Send(addr, packet{Data: share})
	case randBeaconSigItem:
		history := n.chain.randomBeacon.History()
		if item.Round >= uint64(len(history)) {
			return
		}

		r := history[item.Round]
		go n.net.Send(addr, packet{Data: r})
	default:
		panic(fmt.Errorf("unknow requested item type: %v", item.T))
	}

	log.Debug("serving item", "item", item, "addr", addr.Addr)
}
