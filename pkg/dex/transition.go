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
	tokenCreations  []Token
	txns            [][]byte
	state           *State
	orderBooks      map[MarketSymbol]*orderBook
	dirtyOrderBooks map[MarketSymbol]bool
}

func newTransition(s *State) *Transition {
	return &Transition{
		state:           s,
		orderBooks:      make(map[MarketSymbol]*orderBook),
		dirtyOrderBooks: make(map[MarketSymbol]bool),
	}
}

// Record records a transition to the state transition.
func (t *Transition) Record(b []byte, round uint64) (valid, success bool) {
	txn, acc, ready, valid := validateSigAndNonce(t.state, b)
	if !valid {
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
		if !t.placeOrder(acc, txn, round) {
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
	default:
		panic("unknown txn type")
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

	order := owner.PendingOrders[idx]

	var pendingQuant uint64
	var token TokenID
	if order.SellSide {
		token = txn.ID.Market.Base
		pendingQuant = order.Quant
	} else {
		token = txn.ID.Market.Quote
		quoteInfo := t.state.tokenCache.idToInfo[txn.ID.Market.Quote]
		baseInfo := t.state.tokenCache.idToInfo[txn.ID.Market.Base]
		pendingQuant = calcBaseSellQuant(order.Quant, quoteInfo.Decimals, order.Price, OrderPriceDecimals, baseInfo.Decimals)
	}

	owner.Balances[token].Pending -= pendingQuant
	owner.Balances[token].Available += pendingQuant
	owner.PendingOrders = append(owner.PendingOrders[:idx], owner.PendingOrders[idx+1:]...)
	t.state.UpdateAccount(owner)
	return true
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
	if round >= txn.ExpireHeight {
		log.Warn("order already expired", "expire round", txn.ExpireHeight, "cur round", round)
		return false
	}

	baseInfo := t.state.tokenCache.Info(txn.Market.Base)
	if baseInfo == nil {
		log.Error("trying to place order on nonexistent token", "token", txn.Market.Base)
		return false
	}

	quoteInfo := t.state.tokenCache.Info(txn.Market.Quote)
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

	// TODO: handle order expire height
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
			_ = removeIdx

			if removeIdx >= 0 {
				acc.PendingOrders = append(acc.PendingOrders[:removeIdx], acc.PendingOrders[removeIdx+1:]...)
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
	if t.state.tokenCache.Exists(txn.Info.Symbol) {
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
	id := TokenID(t.state.tokenCache.Size() + len(t.tokenCreations))
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
		log.Warn("in sufficient available token balance", "tokenID", txn.TokenID, "quant", txn.Quant, "available", b.Available)
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

func (t *Transition) saveOrderBookIfDirty() {
	for m, b := range t.orderBooks {
		if t.dirtyOrderBooks[m] {
			t.state.saveOrderBook(m, b)
			t.dirtyOrderBooks[m] = false
		}
	}
}

func (t *Transition) StateHash() consensus.Hash {
	t.saveOrderBookIfDirty()
	return t.state.Hash()
}

// Commit commits the transition to the state root.
func (t *Transition) Commit() consensus.State {
	t.saveOrderBookIfDirty()
	t.state.Commit()
	for _, v := range t.tokenCreations {
		t.state.tokenCache.Update(v.ID, &v.TokenInfo)
	}

	return t.state
}
