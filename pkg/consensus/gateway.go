package consensus

import (
	"context"
	"fmt"
	"sync"

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

type itemRequest Item

// itemType is the different type of items.
type itemType int

// different types of items
const (
	txnItem itemType = iota
	sysTxnItem
	blockItem
	blockProposalItem
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
	case blockProposalItem:
		return "BlockProposalItem"
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
	addr               unicastAddr
	net                *network
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

// newGateway creates a new gateway
func newGateway(net *network, chain *Chain) *gateway {
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

	n := &gateway{
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

func (n *gateway) requestItem(addr unicastAddr, item Item) error {
	return n.net.Send(addr, packet{Data: itemRequest(item)})
}

func (n *gateway) requestRandBeaconSig(ctx context.Context, addr unicastAddr, round uint64) (*RandBeaconSig, error) {
	v, ok := n.randBeaconSigCache.Get(round)
	if ok {
		return v.(*RandBeaconSig), nil
	}

	c := make(chan *RandBeaconSig, 1)
	n.mu.Lock()
	n.rbSigWaiters[round] = append(n.rbSigWaiters[round], c)
	if len(n.rbSigWaiters[round]) == 1 {
		err := n.requestItem(addr, Item{
			T:     randBeaconSigItem,
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

func (n *gateway) requestBlock(ctx context.Context, addr unicastAddr, hash Hash) (*Block, error) {
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
			T:    blockItem,
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

func (n *gateway) requestBlockProposal(ctx context.Context, addr unicastAddr, hash Hash) (*BlockProposal, error) {
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
			T:    blockProposalItem,
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
		fmt.Printf("recv %v, %T\n", addr.Addr, pac.Data)
		// see conn.go:init() for the list of possible data
		// types
		switch v := pac.Data.(type) {
		case []byte:
			go n.recvTxn(v)
		case *RandBeaconSig:
			go n.recvRandBeaconSig(addr, v)
		case *RandBeaconSigShare:
			go n.recvRandBeaconSigShare(addr, v)
		case *Block:
			go n.recvBlock(addr, v)
		case *BlockProposal:
			go n.recvBlockProposal(addr, v)
		case *NtShare:
			go n.recvNtShare(addr, v)
		case Item:
			go n.recvInventory(addr, v)
		case itemRequest:
			fmt.Println("recv request", v)
			go n.serveData(addr, Item(v))
		default:
			panic(fmt.Errorf("received unsupported data type: %T", pac.Data))
		}
	}
}

// TODO: don't broadcast when syncing.

func (n *gateway) broadcast(item Item) {
	n.net.Send(broadcast{}, packet{Data: item})
}

func (n *gateway) recvTxn(t []byte) {
	broadcast := n.chain.TxnPool.Add(t)
	if broadcast {
		go n.broadcast(Item{T: txnItem, Hash: SHA3(t)})
	}
}

func (n *gateway) recvSysTxn(t *SysTxn) {
	panic("not implemented")
}

func (n *gateway) recvRandBeaconSig(addr unicastAddr, r *RandBeaconSig) {
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
		go n.broadcast(Item{T: randBeaconSigItem, Round: r.Round})
	}
}

func (n *gateway) recvRandBeaconSigShare(addr unicastAddr, r *RandBeaconSigShare) {
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

	fmt.Println("sig share round", r.Round)
	go n.broadcast(Item{T: randBeaconSigShareItem, Round: r.Round, Hash: r.Hash()})
}

func (n *gateway) recvBlock(addr unicastAddr, b *Block) {
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

	go n.broadcast(Item{T: blockItem, Hash: b.Hash()})
}

func (n *gateway) recvBlockProposal(addr unicastAddr, bp *BlockProposal) {
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

	go n.broadcast(Item{T: blockProposalItem, Hash: bp.Hash()})
}

func (n *gateway) recvNtShare(addr unicastAddr, s *NtShare) {
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
	go n.broadcast(Item{T: ntShareItem, Hash: s.Hash()})
}

// TODO: fix lint
// nolint: gocyclo
func (n *gateway) recvInventory(addr unicastAddr, item Item) {
	n.mu.Lock()
	defer n.mu.Unlock()

	log.Info("recv inventory", "inventory", item)

	switch item.T {
	case txnItem:
		if n.chain.TxnPool.NotSeen(item.Hash) {
			log.Info("request TxnItem", "item", item)
			n.requestItem(addr, item)
		}
	case sysTxnItem:
		panic("not implemented")
	case blockItem:
		if b := n.chain.Block(item.Hash); b != nil {
			return
		}

		log.Info("request BlockItem", "item", item)
		n.requestItem(addr, item)
	case blockProposalItem:
		if bp := n.chain.BlockProposal(item.Hash); bp != nil {
			return
		}

		log.Info("request BlockProposalItem", "item", item)
		// TODO: replace n.requestItem with syncer.Sync
		n.requestItem(addr, item)
	case ntShareItem:
		// TODO: move all existance check to syncer.
		if nt := n.chain.NtShare(item.Hash); nt != nil {
			return
		}

		log.Info("request NtShareItem", "item", item)
		n.requestItem(addr, item)
	case randBeaconSigShareItem:
		if n.chain.RandomBeacon.Round()+1 != item.Round {
			return
		}

		share := n.chain.RandomBeacon.GetShare(item.Hash)
		if share != nil {
			return
		}

		log.Info("request RandBeaconShareItem", "item", item)
		n.requestItem(addr, item)
	case randBeaconSigItem:
		go n.syncer.SyncRandBeaconSig(addr, item.Round)
	}
}

// TODO: fix lint
// nolint: gocyclo
func (n *gateway) serveData(addr unicastAddr, item Item) {
	switch item.T {
	case txnItem:
		txn := n.chain.TxnPool.Get(item.Hash)
		if txn != nil {
			log.Info("serving TxnItem", "item", item)
			go n.net.Send(addr, packet{Data: txn})
		}
	case sysTxnItem:
		panic("not implemented")
	case blockItem:
		b := n.chain.Block(item.Hash)
		if b == nil {
			return
		}

		log.Info("serving BlockItem", "item", item)
		go n.net.Send(addr, packet{Data: b})
	case blockProposalItem:
		bp := n.chain.BlockProposal(item.Hash)
		if bp == nil {
			return
		}

		log.Info("serving BlockProposalItem", "item", item)
		go n.net.Send(addr, packet{Data: bp})
	case ntShareItem:
		nts := n.chain.NtShare(item.Hash)
		if nts == nil {
			return
		}

		log.Info("serving NtShareItem", "item", item)
		go n.net.Send(addr, packet{Data: nts})
	case randBeaconSigShareItem:
		share := n.chain.RandomBeacon.GetShare(item.Hash)
		if share == nil {
			return
		}

		log.Info("serving RandBeaconShareItem", "item", item)
		go n.net.Send(addr, packet{Data: share})
	case randBeaconSigItem:
		history := n.chain.RandomBeacon.History()
		r := history[item.Round]
		log.Info("serving RandBeaconItem", "item", item)
		go n.net.Send(addr, packet{Data: r})
	}
}
