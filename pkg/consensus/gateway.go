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
	case txnItem, sysTxnItem, blockItem, blockProposalItem, ntShareItem:
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
	node               *Node

	mu             sync.Mutex
	rbSigWaiters   map[uint64][]chan *RandBeaconSig
	blockWaiters   map[Hash][]chan *Block
	bpWaiters      map[Hash][]chan *BlockProposal
	requestingItem map[Item]bool
}

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
		requestingItem:     make(map[Item]bool),
	}

	n.syncer = newSyncer(chain, n)
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

func (n *gateway) RequestBlockProposal(ctx context.Context, addr unicastAddr, hash Hash) (*BlockProposal, error) {
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
			log.Debug("recvTxn", "hash", SHA3(v))
			go n.recvTxn(v)
		case *RandBeaconSig:
			log.Debug("recvRandBeaconSig", "round", v.Round)
			go n.recvRandBeaconSig(addr, v)
		case *RandBeaconSigShare:
			log.Debug("recvRandBeaconSigShare", "round", v.Round)
			go n.recvRandBeaconSigShare(addr, v)
		case *Block:
			log.Debug("recvBlock", "round", v.Round, "hash", v.Hash(), "state root", v.StateRoot)
			go n.recvBlock(addr, v)
			go n.node.BlockForRoundProduced(v.Round)
		case *BlockProposal:
			log.Debug("recvBlockProposal", "round", v.Round, "hash", v.Hash(), "block", v.PrevBlock)
			go n.recvBlockProposal(addr, v)
		case *NtShare:
			log.Debug("recvNtShare", "round", v.Round, "hash", v.Hash(), "bp", v.BP)
			go n.recvNtShare(addr, v)
		case Item:
			go n.recvInventory(addr, v)
		case itemRequest:
			go n.serveData(addr, Item(v))
		default:
			panic(fmt.Errorf("received unsupported data type: %T", pac.Data))
		}
	}
}

func (n *gateway) broadcast(item Item) {
	n.net.Send(broadcast{}, packet{Data: item})
}

func (n *gateway) recvTxn(t []byte) {
	txn, broadcast := n.chain.txnPool.Add(t)
	if broadcast {
		go n.broadcast(Item{T: txnItem, Hash: SHA3(txn.Raw)})
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

	// TODO: validate by checking group signature

	if broadcast {
		go n.broadcast(Item{T: randBeaconSigItem, Round: r.Round})
	}
}

func (n *gateway) recvRandBeaconSigShare(addr unicastAddr, r *RandBeaconSigShare) {
	if r.Round == 0 {
		log.Error("received RandBeaconSigShare of 0 round, should not happen")
		return
	}

	n.chain.randomBeacon.WaitUntil(r.Round - 1)

	groupID, valid := n.v.ValidateRandBeaconSigShare(r)

	if !valid {
		return
	}

	sig, broadcast, success := n.chain.randomBeacon.AddRandBeaconSigShare(r, groupID)
	if !success {
		return
	}

	if sig != nil {
		go n.recvRandBeaconSig(addr, sig)
		return
	}

	if broadcast {
		go n.broadcast(Item{T: randBeaconSigShareItem, Round: r.Round, Hash: r.Hash()})
	}
}

func (n *gateway) recvBlock(addr unicastAddr, b *Block) {
	h := b.Hash()
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

func (n *gateway) recvBlockProposal(addr unicastAddr, bp *BlockProposal) {
	h := bp.Hash()
	n.bpCache.Add(h, bp)

	n.mu.Lock()
	for _, c := range n.bpWaiters[h] {
		c <- bp
	}
	n.bpWaiters[h] = nil
	n.mu.Unlock()

	_, broadcast, err := n.syncer.SyncBlockProposal(addr, h)
	if err != nil {
		log.Warn("sync block proposal error", "err", err)
		return
	}

	if broadcast {
		go n.broadcast(Item{T: blockProposalItem, Hash: bp.Hash()})
	}
}

func (n *gateway) recvNtShare(addr unicastAddr, s *NtShare) {
	round := n.chain.Round()
	if s.Round < round {
		return
	}

	if nts := n.chain.NtShare(s.Hash()); nts != nil {
		return
	}

	bp, _, err := n.syncer.SyncBlockProposal(addr, s.BP)
	if err != nil {
		log.Error("error sync block proposal for nt share", "err", err)
		return
	}

	if bp.Round != s.Round {
		log.Error("notarization share round does not match block proposal round", "nt round", s.Round, "bp round", bp.Round)
		return
	}

	groupID, valid := n.v.ValidateNtShare(s)
	if !valid {
		return
	}

	b, broadcast, success := n.chain.addNtShare(s, groupID)
	if !success {
		return
	}

	if b != nil {
		go n.recvBlock(addr, b)
		return
	}

	if broadcast {
		go n.broadcast(Item{T: ntShareItem, Hash: s.Hash()})
	}
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
		if b := n.chain.Block(item.Hash); b != nil {
			return
		}

		n.requestItem(addr, item, false)
	case blockProposalItem:
		if bp := n.chain.BlockProposal(item.Hash); bp != nil {
			return
		}

		n.requestItem(addr, item, false)
	case ntShareItem:
		if nt := n.chain.NtShare(item.Hash); nt != nil {
			return
		}

		n.requestItem(addr, item, false)
	case randBeaconSigShareItem:
		if n.chain.randomBeacon.Round()+1 != item.Round {
			return
		}

		share := n.chain.randomBeacon.GetShare(item.Hash)
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
		b := n.chain.Block(item.Hash)
		if b == nil {
			return
		}
		go n.net.Send(addr, packet{Data: b})
	case blockProposalItem:
		bp := n.chain.BlockProposal(item.Hash)
		if bp == nil {
			return
		}
		go n.net.Send(addr, packet{Data: bp})
	case ntShareItem:
		nts := n.chain.NtShare(item.Hash)
		if nts == nil {
			return
		}
		go n.net.Send(addr, packet{Data: nts})
	case randBeaconSigShareItem:
		share := n.chain.randomBeacon.GetShare(item.Hash)
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
	}

	log.Debug("serving item", "item", item, "addr", addr.Addr)
}
