package dex

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

// matchOrders returns matched trades, in the form of execution event.
func matchOrders(prev []Order) []ExecutionEvent {
	return nil
}

// TODO: clean up and move the following notes to wiki.

/*

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
