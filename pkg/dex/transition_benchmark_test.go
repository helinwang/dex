package dex

import (
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
)

type myPKer struct {
	m map[consensus.Addr]consensus.PK
}

func (m *myPKer) PK(addr consensus.Addr) consensus.PK {
	return m.m[addr]
}

func genTransTxns(p *myPKer) (consensus.Transition, []byte) {
	const (
		accountCount = 10000
		orderCount   = 10000
	)

	accountSKs := make([]consensus.SK, accountCount)
	accountPKs := make([]consensus.PK, accountCount)
	for i := range accountSKs {
		sk := consensus.RandSK()
		pk := sk.MustPK()
		accountSKs[i] = sk
		accountPKs[i] = pk
		p.m[pk.Addr()] = pk
	}

	var BTCInfo = TokenInfo{
		Symbol:     "BTC",
		Decimals:   8,
		TotalUnits: 200000000 * 100000000,
	}
	state := CreateGenesisState(accountPKs, []TokenInfo{BTCInfo})
	trans := state.Transition(1)
	var txns [][]byte
	for i := 0; i < orderCount; i++ {
		sk := accountSKs[rand.Intn(len(accountSKs))]
		t := PlaceOrderTxn{
			SellSide: rand.Intn(2) == 0,
			Quant:    uint64(rand.Intn(100) + 100000),
			Price:    uint64(rand.Intn(10) + 1000),
			Market:   MarketSymbol{Base: 0, Quote: 1},
		}
		txns = append(txns, MakePlaceOrderTxn(sk, t, 0, 0))
	}

	body, err := rlp.EncodeToBytes(txns)
	if err != nil {
		panic(err)
	}

	return trans, body
}

func BenchmarkPlaceOrder(b *testing.B) {
	p := &myPKer{m: make(map[consensus.Addr]consensus.PK)}
	trans, body := genTransTxns(p)
	pool := NewTxnPool(p)
	// make sure everything is in pool
	trans.RecordSerialized(body, pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trans.RecordSerialized(body, pool)
	}
}
