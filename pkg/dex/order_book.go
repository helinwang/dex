package dex

import (
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
)

type pricePoint struct {
	Price     uint64
	ListHead  *orderBookEntry
	ListTail  *orderBookEntry
	NextPoint *pricePoint
}

type orderBookEntryData struct {
	ID    uint64
	Owner consensus.Addr
	Quant uint64
}

type orderBookEntry struct {
	orderBookEntryData
	Next *orderBookEntry
}

// orderBook is the order book which performs the order matching.
//
// Inspired by voyager who wrote "QuantCup 1: Price-Time Matching
// Engine":
// https://gist.github.com/helinwang/935ab9558195a6ea8c16567caef5911b
type orderBook struct {
	nextOrderID uint64
	bidMax      *pricePoint
	askMin      *pricePoint
	idToEntry   map[uint64]*orderBookEntry
}

type orderExecution struct {
	Owner    consensus.Addr
	ID       uint64
	SellSide bool
	Quant    uint64
	Price    uint64
	Taker    bool
}

type Order struct {
	Owner    consensus.Addr
	SellSide bool
	// quant step size is the decimals of the token, specific when
	// the token is issued, e.g., quant = Quant * 10^-(decimals)
	Quant uint64
	// price tick size is 10^-8, e.g,. price = Price * 10^-8
	Price uint64
	// the order is expired when ExpireRound >= block height
	ExpireRound uint64
}

func newOrderBook() *orderBook {
	return &orderBook{
		// don't need to actively remove the entries that are
		// cancelled or matched, they will be "garbage
		// collected" each block, during the order book
		// serialization.
		idToEntry: make(map[uint64]*orderBookEntry),
	}
}

func (o *orderBook) Cancel(id uint64) {
	entry := o.idToEntry[id]
	if entry != nil {
		entry.Quant = 0
	}
}

func (o *orderBook) getEntry(data orderBookEntryData) *orderBookEntry {
	e := &orderBookEntry{orderBookEntryData: data}
	o.idToEntry[data.ID] = e
	return e
}

// Limit processes a incoming limit order.
func (o *orderBook) Limit(order Order) (id uint64, executions []orderExecution) {
	id = o.nextOrderID
	o.nextOrderID++

	if !order.SellSide {
		// match the incoming buy order
		for o.askMin != nil && order.Price >= o.askMin.Price {
			entry := o.askMin.ListHead
			for entry != nil {
				if entry.Quant >= order.Quant {
					// order is filled
					execA := orderExecution{
						Owner:    order.Owner,
						ID:       id,
						SellSide: false,
						Quant:    order.Quant,
						Price:    o.askMin.Price,
						Taker:    true,
					}

					execB := orderExecution{
						Owner:    entry.Owner,
						ID:       entry.ID,
						SellSide: true,
						Quant:    order.Quant,
						Price:    o.askMin.Price,
						Taker:    false,
					}

					executions = append(executions, execA, execB)
					entry.Quant -= order.Quant
					if entry.Quant == 0 {
						if entry.Next != nil {
							o.askMin.ListHead = entry.Next
						} else {
							o.askMin = o.askMin.NextPoint
						}
					}
					return
				}

				if entry.Quant > 0 {
					order.Quant -= entry.Quant
					execA := orderExecution{
						Owner:    order.Owner,
						ID:       id,
						SellSide: false,
						Quant:    entry.Quant,
						Price:    o.askMin.Price,
						Taker:    true,
					}

					execB := orderExecution{
						Owner:    entry.Owner,
						ID:       entry.ID,
						SellSide: true,
						Quant:    entry.Quant,
						Price:    o.askMin.Price,
						Taker:    false,
					}
					executions = append(executions, execA, execB)
					entry.Quant = 0
				}
				entry = entry.Next
			}

			// all the orders in the current price point
			// is filled, move to next price point.
			o.askMin = o.askMin.NextPoint
		}

		// TODO: if a IOC order, do not need to insert
		// no more matching orders, add to the order book
		entry := o.getEntry(orderBookEntryData{
			ID:    id,
			Owner: order.Owner,
			Quant: order.Quant,
		})

		if o.bidMax == nil || order.Price > o.bidMax.Price {
			o.bidMax = &pricePoint{
				Price:     order.Price,
				NextPoint: o.bidMax,
				ListHead:  entry,
				ListTail:  entry,
			}
		} else if order.Price == o.bidMax.Price {
			o.bidMax.ListTail.Next = entry
			o.bidMax.ListTail = entry
		} else {
			prev := o.bidMax
			cur := o.bidMax.NextPoint
			for ; ; prev, cur = cur, cur.NextPoint {
				if cur == nil || cur.Price < order.Price {
					point := &pricePoint{
						Price:     order.Price,
						NextPoint: cur,
						ListHead:  entry,
						ListTail:  entry,
					}
					prev.NextPoint = point
					break
				} else if cur.Price == order.Price {
					cur.ListTail.Next = entry
					cur.ListTail = entry
					break
				}
			}
		}
	} else {
		// match the incoming sell order
		for o.bidMax != nil && order.Price <= o.bidMax.Price {
			entry := o.bidMax.ListHead
			for entry != nil {
				if entry.Quant >= order.Quant {
					// order is filled
					execA := orderExecution{
						Owner:    order.Owner,
						ID:       id,
						SellSide: true,
						Quant:    order.Quant,
						Price:    o.bidMax.Price,
						Taker:    true,
					}

					execB := orderExecution{
						Owner:    entry.Owner,
						ID:       entry.ID,
						SellSide: false,
						Quant:    order.Quant,
						Price:    o.bidMax.Price,
						Taker:    false,
					}

					executions = append(executions, execA, execB)
					entry.Quant -= order.Quant
					if entry.Quant == 0 {
						if entry.Next != nil {
							o.bidMax.ListHead = entry.Next
						} else {
							o.bidMax = o.bidMax.NextPoint
						}
					}
					return
				}

				if entry.Quant > 0 {
					order.Quant -= entry.Quant
					execA := orderExecution{
						Owner:    order.Owner,
						ID:       id,
						SellSide: true,
						Quant:    entry.Quant,
						Price:    o.bidMax.Price,
						Taker:    true,
					}

					execB := orderExecution{
						Owner:    entry.Owner,
						ID:       entry.ID,
						SellSide: false,
						Quant:    entry.Quant,
						Price:    o.bidMax.Price,
						Taker:    false,
					}
					executions = append(executions, execA, execB)
					entry.Quant = 0
				}
				entry = entry.Next
			}

			o.bidMax = o.bidMax.NextPoint
		}

		// TODO: if a IOC order, do not need to insert
		entry := o.getEntry(orderBookEntryData{
			ID:    id,
			Owner: order.Owner,
			Quant: order.Quant,
		})

		if o.askMin == nil || order.Price < o.askMin.Price {
			o.askMin = &pricePoint{
				Price:     order.Price,
				NextPoint: o.askMin,
				ListHead:  entry,
				ListTail:  entry,
			}
		} else if order.Price == o.askMin.Price {
			o.askMin.ListTail.Next = entry
			o.askMin.ListTail = entry
		} else {
			prev := o.askMin
			cur := o.askMin.NextPoint
			for ; ; prev, cur = cur, cur.NextPoint {
				if cur == nil || cur.Price > order.Price {
					point := &pricePoint{
						Price:     order.Price,
						NextPoint: cur,
						ListHead:  entry,
						ListTail:  entry,
					}
					prev.NextPoint = point
					break
				} else if cur.Price == order.Price {
					cur.ListTail.Next = entry
					cur.ListTail = entry
					break
				}
			}
		}
	}

	return
}

type orderBookPointToMarshal struct {
	Price   uint64
	Entries []orderBookEntryData
}

func flatten(p *pricePoint) []orderBookPointToMarshal {
	var r []orderBookPointToMarshal
	for ; p != nil; p = p.NextPoint {
		var entries []orderBookEntryData
		e := p.ListHead
		for ; e != nil; e = e.Next {
			if e.Quant == 0 {
				// 0 quant entries are cancelled
				// entries, skip.
				continue
			}

			entries = append(entries, e.orderBookEntryData)
		}
		r = append(r, orderBookPointToMarshal{
			Price:   p.Price,
			Entries: entries,
		})
	}

	return r
}

func (o *orderBook) unflattenPoint(point orderBookPointToMarshal) *pricePoint {
	if len(point.Entries) == 0 {
		return nil
	}

	p := &pricePoint{
		Price: point.Price,
	}

	entries := make([]*orderBookEntry, len(point.Entries))
	var last *orderBookEntry
	for i := len(entries) - 1; i >= 0; i-- {
		entries[i] = o.getEntry(point.Entries[i])
		entries[i].Next = last
		last = entries[i]
	}

	p.ListHead = entries[0]
	p.ListTail = entries[len(entries)-1]
	return p
}

func (o *orderBook) unflatten(points []orderBookPointToMarshal) *pricePoint {
	var root *pricePoint
	var prev *pricePoint
	for _, p := range points {
		cur := o.unflattenPoint(p)
		if cur == nil {
			continue
		}

		if root == nil {
			root = cur
		} else {
			prev.NextPoint = cur
		}
		prev = cur
	}
	return root
}

func (o *orderBook) EncodeRLP(w io.Writer) error {
	askPoints := flatten(o.askMin)
	bidPoints := flatten(o.bidMax)
	err := rlp.Encode(w, askPoints)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, bidPoints)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, o.nextOrderID)
	return err
}

func (o *orderBook) DecodeRLP(s *rlp.Stream) error {
	o.idToEntry = make(map[uint64]*orderBookEntry)
	b, err := s.Raw()
	if err != nil {
		return err
	}

	var askPoints []orderBookPointToMarshal
	err = rlp.DecodeBytes(b, &askPoints)
	if err != nil {
		return err
	}

	b, err = s.Raw()
	if err != nil {
		return err
	}

	var bidPoints []orderBookPointToMarshal
	err = rlp.DecodeBytes(b, &bidPoints)
	if err != nil {
		return err
	}

	nextOrderID, err := s.Uint()
	if err != nil {
		return err
	}

	o.nextOrderID = nextOrderID
	o.askMin = o.unflatten(askPoints)
	o.bidMax = o.unflatten(bidPoints)
	return nil
}

// TODO: clean up and move the following notes to wiki.

/*

order related events (saved in the receipt trie):

- order placed
- order (partially) matched
- order expired
- order cancelled

------

nodes should not need to do the CPU work to process every order of
every market during chain sync.

With routed networking,

- two kinds of nodes:
  a. group member, ones participating in randomly selected committees
  by the random beacon.
  b. client, ones generating transactions. A group member can be a
  client too.

- routing target ids:
  a. all group members: target is every groups member.
  b. a specific group.
  c. a specific node, could be a group member node or a client node.

- execution event: An execution event is either a trade event, an
  order cancellation event or an order expiry event. Order matchin
  committee (discussed below) producess a list of execution events.

- receipt: A receipt is either an execution event, a send/freeze/burn
  token event or a ICO purchase event. The information of interest is
  added to the bloom filter in the block header. The receipts are
  saved in a patricia merkle tree, whose root hash is stored in the
  block header. It can provide merkle proof for each individual
  receipt.

- different types of committees (a committee is a group):

  a. block proposer committee: each member propose block individually,
  each of them has a rank, the block proposal weight is based on the
  rank. Each round only a single block proposer committee is active.

  b. order matching committees: a single order matching committee
  matches one or multiple market, it generates execution events. Each
  round there are multiple active order matching committees, matching
  multiple markets in parallel.

  c. send token committee: handles

- transactions are routed to the all group members.

- block proposer proposes the block proposal, containing the
  aggregated transactions. The block proposal is routed to the current

- block proposer committee receives different matching committees are
  responsible for different markets, notary committee notarize the
  matching result - from different committees multicast signature
  shares locally,

- IMPORTANT: Node in the network should not need to see and process
  every single transaction.

  a. Trading shard: When a new group joins, it is automatically
  assigned to a shard. Each shard is in charges the order matching of
  several markets. The trade transaction of a given market is sent to
  its own shard only. The shard's groups are markets adjusted
  periodically.

  b. Local block proposal group: each trading shard has a local block
  proposal group selected by the random beacon each round. The block
  proposed only contains orders of the markets belongs to the shard.

  c. Local notarization group: each trading shard has a local
  notarizaion group selected by the random beacon each round. It
  matches the orders in the block proposal. Generate the execution
  events.

  b. Glocal notarization group: it is selected by the random beacon
  each round.

----------------------

- multiple sub chain -> multiple blocks being used to form the virtual
  global block -> how to reach agreement on which blocks forms the
  virtual global block to begin with?

  We need a single global block, thus a single chain. Notarization
  ensures timely publication, the finalization happens very fast over
  time.

- How about instead of sharding, do a very fast implementation of the
  matching engine. Sure, the scalability is bounded, but we could
  already have a very good TPS.

*/

/*

execution report, order.ID vs order.owner

execution report needs:

- price
- quant
- side
- owner

the user wants to know:

- order is in the block chain, can be checked from the order status
  below.
- order status: price, side, remaining quant. Which can be part of the
  account state.
- execution report, append only, should it be in the account state? It
  is actually very suitable for archiving services to archive. Maybe a
  seperate trie would be good.

the user wants to be able to:

- cancel a order that is sent, not necessarily recorded on the
  blockchain <- actually, we can't do that, otherwise this information
  has to stay on the chain forever. The user can simply send another
  txn with same nonce to override the order if it is not yet recorded.
- cancel a order that is pending on the blockchain. He has to cancel
  with the order ID. The order ID better be unique, so the execution
  report would make sense.

conclusion:

1. the order ID gets generated by the blockchain, upon receiving the
order. A increasing integer per order book would be fine.

2. the user cancels the order with the order ID.

*/
