package dex

import (
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
)

// ExecutionEvent is either a trade event, an order cancellation event
// or an order expiry event.
//
// - trade event: both buy and sell is not nil.
// - order expiry event: only one of the buy and sell is not nil,
// order.ExpireHeight is same as the round which the event is
// recorded.
// - order cancellation event: only one of the buy and sell is not nil,
// and not a cancellation event.
type ExecutionEvent struct {
	Sell *Order
	Buy  *Order
}

type pricePoint struct {
	Price     uint64
	ListHead  *orderBookEntry
	ListTail  *orderBookEntry
	NextPoint *pricePoint
}

// TODO: does the user care about order txn id when submitting it?
// probably not. We can generate an order txn id after it being
// matched, by SHA3(market||orderCounter). The user can cancel the
// order using it then.

type orderBookEntryData struct {
	Owner        consensus.Addr
	Quant        uint64
	ExpireHeight uint64
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
	bidMax *pricePoint
	askMin *pricePoint
}

// Limit processes a incoming limit order.
func (o *orderBook) Limit(order Order) {
	if !order.SellSide {
		// match the incoming buy order
		for o.askMin != nil && order.Price >= o.askMin.Price {
			entry := o.askMin.ListHead
			for entry != nil {
				if entry.Quant < order.Quant {
					// TODO: trade event
					order.Quant -= entry.Quant
				} else {
					// order is filled
					if entry.Quant > order.Quant {
						entry.Quant -= order.Quant
					} else {
						entry = entry.Next
					}

					// TODO: trade event
					o.askMin.ListHead = entry
					return
				}
				entry = entry.Next
			}

			// all the orders in the current price point
			// is filled, move to next price point.
			o.askMin = o.askMin.NextPoint
		}

		// TODO: handle order expire
		// TODO: if a IOC order, do not need to insert
		// no more matching orders, add to the order book
		entry := &orderBookEntry{
			orderBookEntryData: orderBookEntryData{
				Owner:        order.Owner,
				Quant:        order.Quant,
				ExpireHeight: order.ExpireHeight,
			},
		}

		if o.bidMax == nil || order.Price > o.bidMax.Price {
			o.bidMax = &pricePoint{
				Price:     order.Price,
				NextPoint: o.bidMax,
				ListHead:  entry,
				ListTail:  entry,
			}
		} else if order.Price == o.bidMax.Price {
			o.bidMax.ListTail.Next = entry
			o.bidMax.ListHead = entry
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
				if entry.Quant < order.Quant {
					// TODO: trade event
					order.Quant -= entry.Quant
				} else {
					// order is filled
					if entry.Quant > order.Quant {
						entry.Quant -= order.Quant
					} else {
						entry = entry.Next
					}

					// TODO: trade event
					o.bidMax.ListHead = entry
					return
				}
				entry = entry.Next
			}

			o.bidMax = o.bidMax.NextPoint
		}

		// TODO: if a IOC order, do not need to insert
		entry := &orderBookEntry{
			orderBookEntryData: orderBookEntryData{
				Owner:        order.Owner,
				Quant:        order.Quant,
				ExpireHeight: order.ExpireHeight,
			},
		}

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

func unflattenPoint(point orderBookPointToMarshal) *pricePoint {
	if len(point.Entries) == 0 {
		return nil
	}

	p := &pricePoint{
		Price: point.Price,
	}

	entries := make([]*orderBookEntry, len(point.Entries))
	var last *orderBookEntry
	for i := len(entries) - 1; i >= 0; i-- {
		entries[i] = &orderBookEntry{
			orderBookEntryData: point.Entries[i],
			Next:               last,
		}
		last = entries[i]
	}

	p.ListHead = entries[0]
	p.ListTail = entries[len(entries)-1]
	return p
}

func unflatten(points []orderBookPointToMarshal) *pricePoint {
	var root *pricePoint
	var prev *pricePoint
	for _, p := range points {
		cur := unflattenPoint(p)
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

	return nil
}

func (o *orderBook) DecodeRLP(s *rlp.Stream) error {
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

	o.askMin = unflatten(askPoints)
	o.bidMax = unflatten(bidPoints)
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
