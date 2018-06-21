package dex

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

type Transition struct {
	round           uint64
	finalized       bool
	tokenCreations  []Token
	txns            [][]byte
	expirations     map[uint64][]orderExpiration
	filledOrders    []PendingOrder
	state           *State
	orderBooks      map[MarketSymbol]*orderBook
	dirtyOrderBooks map[MarketSymbol]bool
	tokenCache      *TokenCache
}

func newTransition(s *State, round uint64) *Transition {
	return &Transition{
		state:           s,
		round:           round,
		expirations:     make(map[uint64][]orderExpiration),
		orderBooks:      make(map[MarketSymbol]*orderBook),
		dirtyOrderBooks: make(map[MarketSymbol]bool),
		tokenCache:      newTokenCache(s),
	}
}

// Record records a transition to the state transition.
func (t *Transition) Record(b []byte) (valid, success bool) {
	if t.finalized {
		panic("record should never be called after finalized")
	}

	txn, acc, ready, nonceValid := validateSigAndNonce(t.state, b)
	if !nonceValid {
		return
	}

	if !ready {
		return true, false
	}

	if len(acc.NonceVec) <= int(txn.NonceIdx) {
		acc.NonceVec = append(acc.NonceVec, make([]uint64, int(txn.NonceIdx)-len(acc.NonceVec)+1)...)
	}
	acc.NonceVec[txn.NonceIdx]++

	dec := gob.NewDecoder(bytes.NewBuffer(txn.Data))
	switch txn.T {
	case PlaceOrder:
		var txn PlaceOrderTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("PlaceOrderTxn decode failed", "err", err)
			return
		}
		if !t.placeOrder(acc, txn, t.round) {
			log.Warn("t.placeOrder failed")
			return
		}
	case CancelOrder:
		var txn CancelOrderTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("CancelOrderTxn decode failed", "err", err)
			return
		}
		if !t.cancelOrder(acc, txn) {
			log.Warn("t.cancelOrder failed")
			return
		}
	case IssueToken:
		var txn IssueTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("IssueTokenTxn decode failed", "err", err)
			return
		}
		if !t.issueToken(acc, txn) {
			log.Warn("CreateTokenTxn failed")
			return
		}
	case SendToken:
		var txn SendTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("SendTokenTxn decode failed", "err", err)
			return
		}
		if !t.sendToken(acc, txn) {
			log.Warn("SendTokenTxn failed")
			return
		}
	case FreezeToken:
		var txn FreezeTokenTxn
		err := dec.Decode(&txn)
		if err != nil {
			log.Warn("FreezeTokenTxn decode failed", "err", err)
			return
		}
		if !t.freezeToken(acc, txn) {
			log.Warn("FreezeTokenTxn failed")
			return
		}

	default:
		log.Warn("unknown txn type", "type", txn.T)
		return false, false
	}

	t.txns = append(t.txns, b)
	return true, true
}

func (t *Transition) getOrderBook(m MarketSymbol) *orderBook {
	book := t.orderBooks[m]
	if book == nil {
		book = t.state.loadOrderBook(m)
		if book == nil {
			book = newOrderBook()
		}
		t.orderBooks[m] = book
	}

	return book
}

func calcBaseSellQuant(baseQuantUnit uint64, quoteDecimals uint8, priceQuantUnit uint64, priceDecimals, baseDecimals uint8) uint64 {
	var quantUnit big.Int
	var quoteDenominator big.Int
	var priceU big.Int
	var priceDenominator big.Int
	var baseDenominator big.Int
	quantUnit.SetUint64(baseQuantUnit)
	quoteDenominator.SetUint64(uint64(math.Pow10(int(quoteDecimals))))
	priceU.SetUint64(priceQuantUnit)
	priceDenominator.SetUint64(uint64(math.Pow10(int(OrderPriceDecimals))))
	baseDenominator.SetUint64(uint64(math.Pow10(int(baseDecimals))))
	var result big.Int
	result.Mul(&quantUnit, &quoteDenominator)
	result.Mul(&result, &priceU)
	result.Div(&result, &baseDenominator)
	result.Div(&result, &priceDenominator)
	return result.Uint64()
}

func (t *Transition) cancelOrder(owner *Account, txn CancelOrderTxn) bool {
	idx := -1
	for i, o := range owner.PendingOrders {
		if o.ID == txn.ID {
			idx = i
			break
		}
	}

	if idx < 0 {
		log.Warn("can not find the order to cancel", "id", txn.ID)
		return false
	}

	book := t.getOrderBook(txn.ID.Market)
	book.Cancel(txn.ID.ID)
	t.dirtyOrderBooks[txn.ID.Market] = true
	cancel := owner.PendingOrders[idx]
	owner.PendingOrders = append(owner.PendingOrders[:idx], owner.PendingOrders[idx+1:]...)

	t.refundAfterCancel(owner, cancel, txn.ID.Market)
	t.state.UpdateAccount(owner)
	return true
}

func (t *Transition) refundAfterCancel(owner *Account, cancel PendingOrder, market MarketSymbol) {
	var pendingQuant uint64
	var token TokenID
	if cancel.SellSide {
		token = market.Base
		pendingQuant = cancel.Quant
	} else {
		token = market.Quote
		quoteInfo := t.tokenCache.idToInfo[market.Quote]
		baseInfo := t.tokenCache.idToInfo[market.Base]
		pendingQuant = calcBaseSellQuant(cancel.Quant, quoteInfo.Decimals, cancel.Price, OrderPriceDecimals, baseInfo.Decimals)
	}

	owner.Balances[token].Pending -= pendingQuant
	owner.Balances[token].Available += pendingQuant
}

type ExecutionReport struct {
	BlockHeight uint64
	ID          OrderID
	SellSide    bool
	TradePrice  uint64
	Quant       uint64
	Fee         uint64
}

func (t *Transition) placeOrder(owner *Account, txn PlaceOrderTxn, round uint64) bool {
	if txn.ExpireHeight > 0 && round >= txn.ExpireHeight {
		log.Warn("order already expired", "expire round", txn.ExpireHeight, "cur round", round)
		return false
	}

	baseInfo := t.tokenCache.Info(txn.Market.Base)
	if baseInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Base)
		return false
	}

	quoteInfo := t.tokenCache.Info(txn.Market.Quote)
	if quoteInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Quote)
		return false
	}

	var sellQuantUnit uint64
	var sell TokenID
	if txn.SellSide {
		sellQuantUnit = txn.Quant
		sell = txn.Market.Base
	} else {
		sellQuantUnit = calcBaseSellQuant(txn.Quant, quoteInfo.Decimals, txn.Price, OrderPriceDecimals, baseInfo.Decimals)
		sell = txn.Market.Quote
	}

	sb, ok := owner.Balances[sell]
	if !ok {
		log.Warn("does not have balance for the given token", "token", sell)
		return false
	}

	if sellQuantUnit == 0 {
		log.Warn("sell quant too small")
		return false
	}

	if sb.Available <= sellQuantUnit {
		log.Warn("insufficient quant to sell", "token", sell, "quant unit", sellQuantUnit)
		return false
	}

	owner.Balances[sell].Available -= sellQuantUnit
	owner.Balances[sell].Pending += sellQuantUnit
	order := Order{
		Owner:        owner.PK.Addr(),
		SellSide:     txn.SellSide,
		Quant:        txn.Quant,
		Price:        txn.Price,
		ExpireHeight: txn.ExpireHeight,
	}

	book := t.getOrderBook(txn.Market)
	orderID, executions := book.Limit(order)
	t.dirtyOrderBooks[txn.Market] = true
	pendingOrder := PendingOrder{
		ID:    OrderID{ID: orderID, Market: txn.Market},
		Order: order,
	}
	owner.PendingOrders = append(owner.PendingOrders, pendingOrder)
	id := OrderID{ID: orderID, Market: txn.Market}

	if order.ExpireHeight > 0 {
		t.expirations[order.ExpireHeight] = append(t.expirations[order.ExpireHeight], orderExpiration{ID: id, Owner: owner.PK.Addr()})
	}

	if len(executions) > 0 {
		addrToAcc := make(map[consensus.Addr]*Account)
		addrToAcc[owner.PK.Addr()] = owner

		for _, exec := range executions {
			acc := addrToAcc[exec.Owner]
			if acc == nil {
				acc = t.state.Account(exec.Owner)
				if acc == nil {
					panic(fmt.Errorf("owner %s not found on executed order, should never happen", exec.Owner.Hex()))
				}
				addrToAcc[exec.Owner] = acc
			}

			// TODO: report fee
			report := ExecutionReport{
				BlockHeight: round,
				ID:          OrderID{ID: exec.ID, Market: txn.Market},
				SellSide:    exec.SellSide,
				TradePrice:  exec.Price,
				Quant:       exec.Quant,
			}
			acc.ExecutionReports = append(acc.ExecutionReports, report)

			orderFound := false
			var orderPrice uint64
			removeIdx := -1
			for i := range acc.PendingOrders {
				if acc.PendingOrders[i].ID.Market == txn.Market && acc.PendingOrders[i].ID.ID == exec.ID {
					acc.PendingOrders[i].Executed += exec.Quant
					if acc.PendingOrders[i].Executed == acc.PendingOrders[i].Quant {
						removeIdx = i
					}

					orderPrice = acc.PendingOrders[i].Price
					orderFound = true
					break
				}
			}

			if !orderFound {
				panic(fmt.Errorf("impossible: can not find matched order %d, market: %v", exec.ID, txn.Market))
			}

			if removeIdx >= 0 {
				pendingOrder := acc.PendingOrders[removeIdx]
				acc.PendingOrders = append(acc.PendingOrders[:removeIdx], acc.PendingOrders[removeIdx+1:]...)
				t.filledOrders = append(t.filledOrders, pendingOrder)
			}

			var soldQuant, boughtQuant, refund uint64
			var sellSideBalance, buySideBalance *Balance
			if exec.SellSide {
				// sold, deduct base pending balance,
				// add quote available balance
				sellSideBalance = acc.Balances[txn.Market.Base]
				buySideBalance = acc.Balances[txn.Market.Quote]
				soldQuant = exec.Quant
				boughtQuant = calcBaseSellQuant(exec.Quant, quoteInfo.Decimals, exec.Price, OrderPriceDecimals, baseInfo.Decimals)
				if exec.Taker {
					refund = exec.Price - orderPrice
				}
			} else {
				// bought, deduct quote pending
				// balance, add base available balance
				sellSideBalance = acc.Balances[txn.Market.Quote]
				buySideBalance = acc.Balances[txn.Market.Base]
				boughtQuant = exec.Quant
				soldQuant = calcBaseSellQuant(exec.Quant, quoteInfo.Decimals, exec.Price, OrderPriceDecimals, baseInfo.Decimals)
				if exec.Taker {
					refund = calcBaseSellQuant(exec.Quant, quoteInfo.Decimals, orderPrice-exec.Price, OrderPriceDecimals, baseInfo.Decimals)
				}
			}

			if sellSideBalance.Pending < soldQuant {
				panic(fmt.Errorf("insufficient pending balance, owner: %s, pending %d, executed: %d", exec.Owner.Hex(), sellSideBalance.Pending, soldQuant))
			}

			sellSideBalance.Pending -= (soldQuant + refund)
			sellSideBalance.Available += refund

			if buySideBalance == nil {
				buySideBalance = &Balance{}
			}
			buySideBalance.Available += boughtQuant
		}

		for _, acc := range addrToAcc {
			t.state.UpdateAccount(acc)
		}
	} else {
		t.state.UpdateAccount(owner)
	}
	return true
}

func (t *Transition) issueToken(owner *Account, txn IssueTokenTxn) bool {
	if t.tokenCache.Exists(txn.Info.Symbol) {
		log.Warn("token symbol already exists", "symbol", txn.Info.Symbol)
		return false
	}

	for _, v := range t.tokenCreations {
		if strings.ToUpper(string(txn.Info.Symbol)) == strings.ToUpper(string(v.Symbol)) {
			log.Warn("token symbol already exists in the current transition", "symbol", txn.Info.Symbol)
			return false
		}
	}

	// TODO: fiture out how to pay fee.
	id := TokenID(t.tokenCache.Size() + len(t.tokenCreations))
	token := Token{ID: id, TokenInfo: txn.Info}
	t.tokenCreations = append(t.tokenCreations, token)
	t.state.UpdateToken(token)

	if owner.Balances == nil {
		owner.Balances = make(map[TokenID]*Balance)
	}
	owner.Balances[id] = &Balance{Available: txn.Info.TotalUnits}
	t.state.UpdateAccount(owner)
	return true
}

// TODO: all elements in trie should be serialized using rlp, not gob,
// since gob is not deterministic.
func (t *Transition) sendToken(owner *Account, txn SendTokenTxn) bool {
	if txn.Quant == 0 {
		return false
	}

	b, ok := owner.Balances[txn.TokenID]
	if !ok {
		log.Warn("trying to send token that the owner does not have", "tokenID", txn.TokenID)
		return false
	}

	if b.Available < txn.Quant {
		log.Warn("insufficient available token balance", "tokenID", txn.TokenID, "quant", txn.Quant, "available", b.Available)
		return false
	}

	toAddr := txn.To.Addr()
	toAcc := t.state.Account(toAddr)
	if toAcc == nil {
		toAcc = &Account{PK: txn.To, Balances: make(map[TokenID]*Balance)}
	}

	owner.Balances[txn.TokenID].Available -= txn.Quant
	if toAcc.Balances[txn.TokenID] == nil {
		toAcc.Balances[txn.TokenID] = &Balance{}
	}
	toAcc.Balances[txn.TokenID].Available += txn.Quant
	t.state.UpdateAccount(toAcc)
	t.state.UpdateAccount(owner)
	return true
}

func (t *Transition) Txns() [][]byte {
	return t.txns
}

func (t *Transition) finalizeState(round uint64) {
	if !t.finalized {
		t.removeFilledOrderFromExpiration()
		// must be called after
		// t.removeFilledOrderFromExpiration
		t.recordOrderExpirations()
		// must be called after t.recordOrderExpirations,
		// since current round may add expiring orders for the
		// next round.
		t.expireOrders()
		// must be called after t.expireOrders, since it could
		// make order book dirty.
		t.saveDirtyOrderBooks()
		t.releaseTokens()
		t.finalized = true
	}
}

func (t *Transition) recordOrderExpirations() {
	for expireHeight, ids := range t.expirations {
		t.state.addOrderExpirations(expireHeight, ids)
	}
}

func (t *Transition) saveDirtyOrderBooks() {
	for m, b := range t.orderBooks {
		if t.dirtyOrderBooks[m] {
			t.state.saveOrderBook(m, b)
		}
	}
}

func (t *Transition) removeFilledOrderFromExpiration() {
	heights := make(map[uint64]int)
	filled := make(map[OrderID]bool)
	for _, o := range t.filledOrders {
		if o.ExpireHeight == 0 {
			continue
		}

		filled[o.ID] = true
		heights[o.ExpireHeight]++
	}

	for height, toRemove := range heights {
		// remove filled order's expiration from the
		// to-be-added expirations of this round.
		expirations := t.expirations[height]
		newExpirations := make([]orderExpiration, 0, len(expirations))
		for _, exp := range expirations {
			if !filled[exp.ID] {
				newExpirations = append(newExpirations, exp)
			}
		}
		t.expirations[height] = newExpirations
		removed := len(newExpirations) - len(expirations)
		if removed == toRemove {
			continue
		}

		// remove filled order's expiration from the saved
		// expiration from disk.
		t.state.removeOrderExpirations(height, filled)
	}
}

// TODO: optimization: cache all changed accounts, and save them
// during the finalization.

func (t *Transition) releaseTokens() {

	// release the tokens that will be released next round
	tokens := t.state.getFreezeTokens(t.round + 1)
	addrToAcc := make(map[consensus.Addr]*Account)
	for _, token := range tokens {
		acc, ok := addrToAcc[token.Addr]
		if !ok {
			acc = t.state.Account(token.Addr)
			addrToAcc[token.Addr] = acc
		}

		b := acc.Balances[token.TokenID]
		removeIdx := -1
		for i, f := range b.Frozen {
			if f.Quant == token.Quant {
				removeIdx = i
				break
			}
		}
		f := b.Frozen[removeIdx]
		b.Frozen = append(b.Frozen[:removeIdx], b.Frozen[removeIdx+1:]...)
		b.Available += f.Quant
	}

	for _, acc := range addrToAcc {
		t.state.UpdateAccount(acc)
	}
}

func (t *Transition) expireOrders() {
	// expire orders whose expiration is the next round
	orders := t.state.getOrderExpirations(t.round + 1)
	addrToAcc := make(map[consensus.Addr]*Account)
	for _, o := range orders {
		t.getOrderBook(o.ID.Market).Cancel(o.ID.ID)
		t.dirtyOrderBooks[o.ID.Market] = true

		acc, ok := addrToAcc[o.Owner]
		if !ok {
			acc = t.state.Account(o.Owner)
			addrToAcc[o.Owner] = acc
		}

		idx := -1
		for i := range acc.PendingOrders {
			if acc.PendingOrders[i].ID == o.ID {
				idx = i
				break
			}
		}

		if idx < 0 {
			panic("can not find expiring order")
		}

		cancel := acc.PendingOrders[idx]
		acc.PendingOrders = append(acc.PendingOrders[:idx], acc.PendingOrders[idx+1:]...)
		t.refundAfterCancel(acc, cancel, o.ID.Market)
	}

	for _, acc := range addrToAcc {
		t.state.UpdateAccount(acc)
	}
}

func (t *Transition) freezeToken(acc *Account, txn FreezeTokenTxn) bool {
	if txn.Quant == 0 {
		return false
	}

	if txn.AvailableRound <= t.round {
		log.Warn("trying to freeze token to too early round", "available round", txn.AvailableRound, "cur round", t.round)
		return false
	}

	b, ok := acc.Balances[txn.TokenID]
	if !ok {
		log.Warn("trying to freeze token that the owner does not have", "tokenID", txn.TokenID)
		return false
	}

	if b.Available < txn.Quant {
		log.Warn("insufficient available token balance", "tokenID", txn.TokenID, "quant", txn.Quant, "available", b.Available)
		return false
	}

	frozen := Frozen{
		AvailableRound: txn.AvailableRound,
		Quant:          txn.Quant,
	}
	b.Available -= txn.Quant
	b.Frozen = append(b.Frozen, frozen)
	t.state.UpdateAccount(acc)
	t.state.freezeToken(txn.AvailableRound, freezeToken{Addr: acc.PK.Addr(), TokenID: txn.TokenID, Quant: txn.Quant})
	return true
}

func (t *Transition) StateHash() consensus.Hash {
	t.finalizeState(t.round)
	return t.state.Hash()
}

func (t *Transition) Commit() consensus.State {
	t.finalizeState(t.round)
	t.state.Commit()
	for _, v := range t.tokenCreations {
		t.tokenCache.Update(v.ID, &v.TokenInfo)
	}

	return t.state
}
