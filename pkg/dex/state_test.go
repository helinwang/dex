package dex

import (
	"testing"
	"unsafe"

	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func TestMarketSymbolBytes(t *testing.T) {
	m0 := MarketSymbol{Quote: (1 << 64) - 2, Base: (1 << 64) - 1}
	m1 := MarketSymbol{Quote: (1 << 64) - 2, Base: (1 << 64) - 3}

	assert.Equal(t, 64, int(unsafe.Sizeof(m0.Quote))*8, "PathPrefix assumes PathPrefix.Quote being 64 bits.")
	assert.Equal(t, 64, int(unsafe.Sizeof(m0.Base))*8, "PathPrefix assumes PathPrefix.Base being 64 bits.")

	p0 := m0.Encode()
	p1 := m1.Encode()
	assert.NotEqual(t, p0, p1)
}

func TestMarketEncodeDecode(t *testing.T) {
	m := MarketSymbol{Base: 1<<64 - 1, Quote: 1}
	var m1 MarketSymbol
	err := m1.Decode(m.Encode())
	if err != nil {
		panic(err)
	}

	assert.Equal(t, m, m1)
}

func TestStateTokens(t *testing.T) {
	memDB := ethdb.NewMemDatabase()
	s := NewState(memDB)
	token0 := Token{ID: 1, TokenInfo: TokenInfo{Symbol: "BNB", Decimals: 8, TotalUnits: 10000000000}}
	token1 := Token{ID: 2, TokenInfo: TokenInfo{Symbol: "BTC", Decimals: 8, TotalUnits: 10000000000}}
	s.UpdateToken(token0)
	s.UpdateToken(token1)
	assert.Equal(t, []Token{token0, token1}, s.Tokens())
}

func TestStateSerialize(t *testing.T) {
	owner := consensus.RandSK().MustPK()
	token0 := Token{ID: 1, TokenInfo: TokenInfo{Symbol: "BTC", Decimals: 8, TotalUnits: 10000000000}}
	token1 := Token{ID: 2, TokenInfo: TokenInfo{Symbol: "ETH", Decimals: 8, TotalUnits: 1000000000}}
	s := CreateGenesisState([]consensus.PK{owner}, []TokenInfo{token0.TokenInfo, token1.TokenInfo})
	nativeToken := Token{ID: 0, TokenInfo: BNBInfo}
	s.UpdateToken(token0)
	s.UpdateToken(token1)
	assert.Equal(t, []Token{nativeToken, token0, token1}, s.Tokens())
	acc := s.Account(owner.Addr())
	assert.NotNil(t, acc)
	b0, _ := acc.Balance(token0.ID)
	assert.Equal(t, token0.TotalUnits, b0.Available)
	b1, _ := acc.Balance(token1.ID)
	assert.Equal(t, token1.TotalUnits, b1.Available)

	b, err := s.Serialize()
	if err != nil {
		panic(err)
	}

	s0 := NewState(ethdb.NewMemDatabase())
	err = s0.Deserialize(b)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, []Token{nativeToken, token0, token1}, s0.Tokens())
	acc = s0.Account(owner.Addr())
	assert.NotNil(t, acc)
	b0, _ = acc.Balance(token0.ID)
	b1, _ = acc.Balance(token1.ID)
	assert.Equal(t, token0.TotalUnits, b0.Available)
	assert.Equal(t, token1.TotalUnits, b1.Available)
}
