package matching

import "github.com/helinwang/dex/pkg/consensus"

type Order struct {
	Owner    consensus.Addr
	SellSide bool
	// quant step size is the decimals of the token, specific when
	// the token is issued, e.g., quant = QuantUnit * 10^-(decimals)
	QuantUnit uint64
	// price tick size is 10^-8, e.g,. price = PriceUnit * 10^-8
	PriceUnit uint64
	// the height that the order is placed
	PlacedHeight uint64
	// the order is expired when ExpireHeight >= block height
	ExpireHeight uint64
}
