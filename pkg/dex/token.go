package dex

import "strings"

type TokenSymbol string

type TokenInfo struct {
	Symbol      TokenSymbol
	Decimals    uint8
	TotalSupply uint64
}

type TokenID uint64

type Token struct {
	ID TokenID
	TokenInfo
}

type TokenCache struct {
	idToInfo map[TokenID]*TokenInfo
	exists   map[TokenSymbol]bool
}

func newTokenCache() *TokenCache {
	return &TokenCache{
		idToInfo: make(map[TokenID]*TokenInfo),
		exists:   make(map[TokenSymbol]bool),
	}
}

func (t *TokenCache) Exists(s TokenSymbol) bool {
	return t.exists[s]
}

func (t *TokenCache) Info(id TokenID) *TokenInfo {
	return t.idToInfo[id]
}

func (t *TokenCache) Update(id TokenID, info *TokenInfo) {
	t.idToInfo[id] = info
	t.exists[TokenSymbol(strings.ToUpper(string(info.Symbol)))] = true
}

func (t *TokenCache) Size() int {
	return len(t.idToInfo)
}
