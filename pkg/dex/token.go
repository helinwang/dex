package dex

type TokenSymbol string

type TokenInfo struct {
	Symbol      TokenSymbol
	TotalSupply uint64
	Decimals    int
}

type TokenID uint32

type Token struct {
	ID TokenID
	TokenInfo
}
