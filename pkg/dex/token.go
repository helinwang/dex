package dex

type TokenSymbol string

type TokenInfo struct {
	Symbol      TokenSymbol
	Decimals    int
	TotalSupply uint64
}

type TokenID uint64

type Token struct {
	ID TokenID
	TokenInfo
}

type TokenCache struct {
	m map[TokenID]*TokenInfo
}

func newTokenCache() *TokenCache {
	return &TokenCache{m: make(map[TokenID]*TokenInfo)}
}

func (t *TokenCache) Info(id TokenID) *TokenInfo {
	return t.m[id]
}

func (t *TokenCache) Update(id TokenID, info *TokenInfo) {
	t.m[id] = info
}
