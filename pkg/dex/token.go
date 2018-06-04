package dex

import (
	"sort"
	"strings"
)

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

func (t *TokenCache) Clone() *TokenCache {
	idToInfo := make(map[TokenID]*TokenInfo)
	for k, v := range t.idToInfo {
		// v is a TokenInfo, which is immutable
		idToInfo[k] = v
	}
	exists := make(map[TokenSymbol]bool)
	for k, v := range t.exists {
		exists[k] = v
	}

	return &TokenCache{
		idToInfo: idToInfo,
		exists:   exists,
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

func (t *TokenCache) Tokens() []Token {
	keys := make([]TokenID, len(t.idToInfo))
	i := 0
	for k := range t.idToInfo {
		keys[i] = k
		i++
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	i = 0
	tokens := make([]Token, len(keys))
	for _, k := range keys {
		tokens[i] = Token{ID: k, TokenInfo: *t.idToInfo[k]}
		i++
	}

	return tokens
}
