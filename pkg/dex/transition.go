package dex

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
	log "github.com/helinwang/log15"
)

var flatFee = uint64(0.0001 * math.Pow10(int(BNBInfo.Decimals)))

type Transition struct {
	round uint64
	fee   uint64
	// don't collect fee if proposer is nil, this happens when:
	// a. replaying a block rather than proposing a block
	// b. in unit test
	proposer        PK
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

func newTransition(s *State, round uint64, proposer PK) *Transition {
	return &Transition{
		state:           s,
		round:           round,
		proposer:        proposer,
		expirations:     make(map[uint64][]orderExpiration),
		orderBooks:      make(map[MarketSymbol]*orderBook),
		dirtyOrderBooks: make(map[MarketSymbol]bool),
		tokenCache:      newTokenCache(s),
		filledOrders:    make([]PendingOrder, 0, 1000), // optimization: preallocate buffer
	}
}

func (t *Transition) RecordSerialized(blob []byte, pool consensus.TxnPool) (int, error) {
	var txns [][]byte
	err := rlp.DecodeBytes(blob, &txns)
	if err != nil {
		return 0, err
	}

	for _, b := range txns {
		hash := consensus.SHA3(b)
		txn := pool.Get(hash)
		if txn == nil {
			txn, _ = pool.Add(b)
		}

		if txn.MinerFeeTxn {
			t.giveMinerFee(*txn.Decoded.(*MinerFeeTxn))
			continue
		}

		err = t.RecordImpl(txn, true)
		if err != nil {
			return 0, err
		}
		pool.Remove(hash)
	}

	return len(txns), nil
}

// Record records a transition to the state transition.
func (t *Transition) Record(txn *consensus.Txn) (err error) {
	return t.RecordImpl(txn, false)
}

func (t *Transition) RecordImpl(txn *consensus.Txn, forceFee bool) (err error) {
	if t.finalized {
		panic("record should never be called after finalized")
	}
	acc := t.state.Account(txn.Owner)
	if acc == nil {
		return errors.New("txn owner not found")
	}

	if !txn.MinerFeeTxn {
		if nonce := acc.Nonce(); txn.Nonce < nonce {
			return errors.New("nonce not valid")
		} else if txn.Nonce > nonce {
			return consensus.ErrTxnNonceTooBig
		}
	}

	payFee := forceFee || t.proposer != nil

	if payFee {
		nativeCoin := acc.Balance(0)
		if nativeCoin.Available < flatFee {
			return errors.New("account don't have sufficient balance to pay fee")
		}

		nativeCoin.Available -= flatFee
		acc.UpdateBalance(0, nativeCoin)
		t.fee += flatFee
	}
	defer func() {
		if payFee && err != nil {
			nativeCoin := acc.Balance(0)
			nativeCoin.Available += flatFee
			acc.UpdateBalance(0, nativeCoin)
			t.fee -= flatFee
		}

		if !txn.MinerFeeTxn && err == nil {
			acc.IncrementNonce()
		}
	}()

	switch tx := txn.Decoded.(type) {
	case *PlaceOrderTxn:
		if err := t.placeOrder(acc, tx, t.round); err != nil {
			return err
		}
	case *CancelOrderTxn:
		if err := t.cancelOrder(acc, tx); err != nil {
			return err
		}
	case *IssueTokenTxn:
		if err := t.issueToken(acc, tx); err != nil {
			return err
		}
	case *SendTokenTxn:
		if err := t.sendToken(acc, tx); err != nil {
			return err
		}
	case *FreezeTokenTxn:
		if err := t.freezeToken(acc, tx); err != nil {
			return err
		}
	case *BurnTokenTxn:
		if err := t.burnToken(acc, tx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown txn type: %T", txn.Decoded)
	}

	t.txns = append(t.txns, txn.Raw)
	return nil
}

func (t *Transition) burnToken(acc *Account, txn *BurnTokenTxn) error {
	if txn.Quant == 0 {
		return errors.New("burn token quantity should not be 0")
	}

	info := t.tokenCache.Info(txn.ID)
	if info == zeroInfo {
		return fmt.Errorf("trying to burn non-existent token: %d", txn.ID)
	}

	balance := acc.Balance(txn.ID)
	if balance.Available < txn.Quant {
		return fmt.Errorf("not enough token to burn, want: %d, have: %d", txn.Quant, balance.Available)
	}

	if info.TotalUnits < txn.Quant {
		return fmt.Errorf("not enough total supply to burn, want: %d, have: %d", txn.Quant, info.TotalUnits)
	}

	balance.Available -= txn.Quant
	info.TotalUnits -= txn.Quant
	acc.UpdateBalance(txn.ID, balance)
	t.state.UpdateToken(Token{ID: txn.ID, TokenInfo: info})
	return nil
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

func calcQuoteQuant(baseQuantUnit uint64, quoteDecimals uint8, priceQuantUnit uint64, priceDecimals, baseDecimals uint8) uint64 {
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

func (t *Transition) cancelOrder(owner *Account, txn *CancelOrderTxn) error {
	cancel, ok := owner.PendingOrder(txn.ID)
	if !ok {
		return fmt.Errorf("can not find the order to cancel: %v", txn.ID)
	}

	book := t.getOrderBook(txn.ID.Market)
	book.Cancel(txn.ID.ID)
	t.dirtyOrderBooks[txn.ID.Market] = true
	owner.RemovePendingOrder(txn.ID)
	t.refundAfterCancel(owner, cancel, txn.ID.Market)
	return nil
}

func (t *Transition) refundAfterCancel(owner *Account, cancel PendingOrder, market MarketSymbol) {
	if cancel.Quant <= cancel.Executed {
		panic(fmt.Errorf("pending order remain amount should be greater than 0, total: %d, executed: %d", cancel.Quant, cancel.Executed))
	}

	refund := cancel.Quant - cancel.Executed
	if cancel.SellSide {
		baseBalance := owner.Balance(market.Base)

		if baseBalance.Pending < refund {
			panic(fmt.Errorf("pending balance smaller than refund, pending: %d, refund: %d", baseBalance.Pending, refund))
		}

		baseBalance.Pending -= refund
		baseBalance.Available += refund
		owner.UpdateBalance(market.Base, baseBalance)
	} else {
		quoteBalance := owner.Balance(market.Quote)
		quoteInfo := t.tokenCache.idToInfo[market.Quote]
		baseInfo := t.tokenCache.idToInfo[market.Base]
		pendingQuant := calcQuoteQuant(refund, quoteInfo.Decimals, cancel.Price, OrderPriceDecimals, baseInfo.Decimals)

		if quoteBalance.Pending < pendingQuant {
			panic(fmt.Errorf("pending balance smaller than refund, pending: %d, refund: %d", quoteBalance.Pending, pendingQuant))
		}

		quoteBalance.Pending -= pendingQuant
		quoteBalance.Available += pendingQuant
		owner.UpdateBalance(market.Quote, quoteBalance)
	}
}

type ExecutionReport struct {
	Round      uint64
	ID         OrderID
	SellSide   bool
	TradePrice uint64
	Quant      uint64
	Fee        uint64
}

func (t *Transition) placeOrder(owner *Account, txn *PlaceOrderTxn, round uint64) error {
	if !txn.Market.Valid() {
		return fmt.Errorf("order's market is invalid: %v", txn.Market)
	}
	if txn.ExpireRound > 0 && round >= txn.ExpireRound {
		return fmt.Errorf("order already expired, order expire round: %d, cur round: %d", txn.ExpireRound, round)
	}

	baseInfo := t.tokenCache.Info(txn.Market.Base)
	if baseInfo == zeroInfo {
		return fmt.Errorf("trying to place order on nonexistent token: %d", txn.Market.Base)
	}

	quoteInfo := t.tokenCache.Info(txn.Market.Quote)
	if quoteInfo == zeroInfo {
		return fmt.Errorf("trying to place order on nonexistent token: %d", txn.Market.Quote)
	}

	if txn.SellSide {
		if txn.Quant == 0 {
			return errors.New("sell: can not sell 0 quantity")
		}

		baseBalance := owner.Balance(txn.Market.Base)
		if baseBalance.Available < txn.Quant {
			return fmt.Errorf("sell failed: insufficient balance, quant: %d, available: %d", txn.Quant, baseBalance.Available)
		}

		baseBalance.Available -= txn.Quant
		baseBalance.Pending += txn.Quant
		owner.UpdateBalance(txn.Market.Base, baseBalance)
	} else {
		if txn.Quant == 0 {
			return errors.New("buy failed: can not buy 0 quantity")
		}

		pendingQuant := calcQuoteQuant(txn.Quant, quoteInfo.Decimals, txn.Price, OrderPriceDecimals, baseInfo.Decimals)
		if pendingQuant == 0 {
			return errors.New("buy failed: converted quote quant is 0")
		}

		quoteBalance := owner.Balance(txn.Market.Quote)
		if quoteBalance.Available < pendingQuant {
			return fmt.Errorf("buy failed, insufficient balance, required: %d, available %d", pendingQuant, quoteBalance.Available)
		}

		quoteBalance.Available -= pendingQuant
		quoteBalance.Pending += pendingQuant
		owner.UpdateBalance(txn.Market.Quote, quoteBalance)
	}

	order := Order{
		Owner:       owner.PK().Addr(),
		SellSide:    txn.SellSide,
		Quant:       txn.Quant,
		Price:       txn.Price,
		ExpireRound: txn.ExpireRound,
	}

	book := t.getOrderBook(txn.Market)
	orderID, executions := book.Limit(order)
	t.dirtyOrderBooks[txn.Market] = true
	id := OrderID{ID: orderID, Market: txn.Market}
	pendingOrder := PendingOrder{
		ID:    id,
		Order: order,
	}
	owner.UpdatePendingOrder(pendingOrder)
	if order.ExpireRound > 0 {
		t.expirations[order.ExpireRound] = append(t.expirations[order.ExpireRound], orderExpiration{ID: id, Owner: owner.PK().Addr()})
	}

	if len(executions) > 0 {
		for _, exec := range executions {
			acc := t.state.Account(exec.Owner)
			orderID := OrderID{ID: exec.ID, Market: txn.Market}
			report := ExecutionReport{
				Round:      round,
				ID:         orderID,
				SellSide:   exec.SellSide,
				TradePrice: exec.Price,
				Quant:      exec.Quant,
			}
			acc.AddExecutionReport(report)
			executedOrder, ok := acc.PendingOrder(orderID)
			if !ok {
				panic(fmt.Errorf("impossible: can not find matched order %d, market: %v, executed order: %v", exec.ID, txn.Market, exec))
			}

			executedOrder.Executed += exec.Quant
			if executedOrder.Executed == executedOrder.Quant {
				acc.RemovePendingOrder(orderID)
				t.filledOrders = append(t.filledOrders, executedOrder)
			} else {
				acc.UpdatePendingOrder(executedOrder)
			}

			baseBalance := acc.Balance(txn.Market.Base)
			quoteBalance := acc.Balance(txn.Market.Quote)
			if exec.SellSide {
				if baseBalance.Pending < exec.Quant {
					panic(fmt.Errorf("insufficient pending balance, owner: %v, pending %d, executed: %d, sell side, taker: %t", exec.Owner, baseBalance.Pending, exec.Quant, exec.Taker))
				}

				baseBalance.Pending -= exec.Quant
				recvQuant := calcQuoteQuant(exec.Quant, quoteInfo.Decimals, exec.Price, OrderPriceDecimals, baseInfo.Decimals)
				quoteBalance.Available += recvQuant
				acc.UpdateBalance(txn.Market.Base, baseBalance)
				acc.UpdateBalance(txn.Market.Quote, quoteBalance)
			} else {
				recvQuant := exec.Quant
				pendingQuant := calcQuoteQuant(exec.Quant, quoteInfo.Decimals, executedOrder.Price, OrderPriceDecimals, baseInfo.Decimals)
				givenQuant := calcQuoteQuant(exec.Quant, quoteInfo.Decimals, exec.Price, OrderPriceDecimals, baseInfo.Decimals)

				if quoteBalance.Pending < pendingQuant {
					panic(fmt.Errorf("insufficient pending balance, owner: %v, pending %d, executed: %d, buy side, taker: %t", exec.Owner, quoteBalance.Pending, exec.Quant, exec.Taker))
				}

				quoteBalance.Pending -= pendingQuant
				quoteBalance.Available += pendingQuant
				quoteBalance.Available -= givenQuant
				baseBalance.Available += recvQuant
				acc.UpdateBalance(txn.Market.Base, baseBalance)
				acc.UpdateBalance(txn.Market.Quote, quoteBalance)
			}
		}
	}
	return nil
}

func (t *Transition) issueToken(owner *Account, txn *IssueTokenTxn) error {
	if t.tokenCache.Exists(txn.Info.Symbol) {
		return fmt.Errorf("token symbol %v already exists", txn.Info.Symbol)
	}

	for _, v := range t.tokenCreations {
		if strings.ToUpper(string(txn.Info.Symbol)) == strings.ToUpper(string(v.Symbol)) {
			return fmt.Errorf("token symbol %v already exists in the current transition", txn.Info.Symbol)
		}
	}

	id := TokenID(t.tokenCache.Size() + len(t.tokenCreations))
	token := Token{ID: id, TokenInfo: txn.Info}
	t.tokenCreations = append(t.tokenCreations, token)
	t.state.UpdateToken(token)
	owner.UpdateBalance(id, Balance{Available: txn.Info.TotalUnits})
	return nil
}

func (t *Transition) sendToken(owner *Account, txn *SendTokenTxn) error {
	if txn.Quant == 0 {
		return errors.New("send token quantity is 0")
	}

	b := owner.Balance(txn.TokenID)
	if b.Available < txn.Quant {
		return fmt.Errorf("insufficient available token balance, tokenID: %v, quant: %d, available: %d", txn.TokenID, txn.Quant, b.Available)
	}

	toAddr := txn.To.Addr()
	toAcc := t.state.Account(toAddr)
	if toAcc == nil {
		toAcc = t.state.NewAccount(txn.To)
	}

	b.Available -= txn.Quant
	owner.UpdateBalance(txn.TokenID, b)
	toAccBalance := toAcc.Balance(txn.TokenID)
	toAccBalance.Available += txn.Quant
	toAcc.UpdateBalance(txn.TokenID, toAccBalance)
	return nil
}

func (t *Transition) Txns() []byte {
	t.finalizeState()

	if len(t.txns) == 0 {
		return nil
	}

	b, err := rlp.EncodeToBytes(t.txns)
	if err != nil {
		panic(err)
	}

	return b
}

func (t *Transition) giveMinerFee(txn MinerFeeTxn) {
	pk := txn.Miner
	addr := pk.Addr()
	acc := t.state.Account(addr)
	if acc == nil {
		acc = t.state.NewAccount(pk)
	}
	nativeCoin := acc.Balance(0)
	nativeCoin.Available += txn.Fee
	acc.UpdateBalance(0, nativeCoin)
}

func (t *Transition) appendFeeTxn() {
	if t.proposer != nil {
		if t.fee == 0 {
			return
		}

		feeTxn := MinerFeeTxn{
			Miner: t.proposer,
			Fee:   t.fee,
		}
		txn := Txn{
			T:    MinerFee,
			Data: gobEncode(feeTxn),
		}

		b, err := rlp.EncodeToBytes(txn)
		if err != nil {
			panic(err)
		}

		t.fee = 0
		t.txns = append(t.txns, b)
		t.giveMinerFee(feeTxn)
	}
}

func (t *Transition) finalizeState() {
	if !t.finalized {
		t.appendFeeTxn()
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
		t.state.CommitCache()
		t.finalized = true
	}
}

func (t *Transition) recordOrderExpirations() {
	for expireRound, ids := range t.expirations {
		t.state.AddOrderExpirations(expireRound, ids)
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
	rounds := make(map[uint64]int)
	filled := make(map[OrderID]bool)
	for _, o := range t.filledOrders {
		if o.ExpireRound == 0 {
			continue
		}

		filled[o.ID] = true
		rounds[o.ExpireRound]++
	}

	for round, toRemove := range rounds {
		// remove filled order's expiration from the
		// to-be-added expirations of this round.
		expirations := t.expirations[round]
		newExpirations := make([]orderExpiration, 0, len(expirations))
		for _, exp := range expirations {
			if !filled[exp.ID] {
				newExpirations = append(newExpirations, exp)
			}
		}
		t.expirations[round] = newExpirations
		removed := len(newExpirations) - len(expirations)
		if removed == toRemove {
			continue
		}

		// remove filled order's expiration from the saved
		// expiration from disk.
		t.state.RemoveOrderExpirations(round, filled)
	}
}

func (t *Transition) releaseTokens() {
	// release the tokens that will be released next round
	tokens := t.state.GetFreezeTokens(t.round + 1)
	addrToAcc := make(map[consensus.Addr]*Account)
	for _, token := range tokens {
		acc, ok := addrToAcc[token.Addr]
		if !ok {
			acc = t.state.Account(token.Addr)
			addrToAcc[token.Addr] = acc
		}

		b := acc.Balance(token.TokenID)
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
		acc.UpdateBalance(token.TokenID, b)
	}
}

func (t *Transition) expireOrders() {
	// expire orders whose expiration is the next round
	orders := t.state.GetOrderExpirations(t.round + 1)
	addrToAcc := make(map[consensus.Addr]*Account)
	for _, o := range orders {
		t.getOrderBook(o.ID.Market).Cancel(o.ID.ID)
		t.dirtyOrderBooks[o.ID.Market] = true

		acc, ok := addrToAcc[o.Owner]
		if !ok {
			acc = t.state.Account(o.Owner)
			addrToAcc[o.Owner] = acc
		}

		order, ok := acc.PendingOrder(o.ID)
		if !ok {
			log.Error("can not find expiring order", "order", o.ID)
			continue
		}

		acc.RemovePendingOrder(o.ID)
		t.refundAfterCancel(acc, order, o.ID.Market)
	}
}

func (t *Transition) freezeToken(acc *Account, txn *FreezeTokenTxn) error {
	if txn.Quant == 0 {
		return errors.New("freeze token quantity is 0")
	}

	if txn.AvailableRound <= t.round {
		return fmt.Errorf("trying to freeze token to too early round, available round: %d, cur round: %d", txn.AvailableRound, t.round)
	}

	b := acc.Balance(txn.TokenID)

	if b.Available < txn.Quant {
		return fmt.Errorf("insufficient available token balance, token id: %v, quantity: %d, available: %d", txn.TokenID, txn.Quant, b.Available)
	}

	frozen := Frozen{
		AvailableRound: txn.AvailableRound,
		Quant:          txn.Quant,
	}
	b.Available -= txn.Quant
	b.Frozen = append(b.Frozen, frozen)
	acc.UpdateBalance(txn.TokenID, b)
	t.state.FreezeToken(txn.AvailableRound, freezeToken{Addr: acc.PK().Addr(), TokenID: txn.TokenID, Quant: txn.Quant})
	return nil
}

func (t *Transition) StateHash() consensus.Hash {
	t.finalizeState()
	return t.state.Hash()
}

func (t *Transition) Commit() consensus.State {
	t.finalizeState()
	for _, v := range t.tokenCreations {
		t.tokenCache.Update(v.ID, v.TokenInfo)
	}

	return t.state
}
