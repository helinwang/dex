package dex

import "sort"

func orderLess(i, j Order) bool {
	if i.SellSide != j.SellSide {
		// buy always smaller than sell
		if !i.SellSide {
			return true
		}

		return false
	}

	if i.PriceUnit < j.PriceUnit {
		return true
	} else if i.PriceUnit > j.PriceUnit {
		return false
	}

	if i.SellSide {
		// i sell, j sell. Treat ealier order lower
		// in the sell side of the order book (higher
		// priority)
		return i.PlacedHeight < j.PlacedHeight
	}

	// i buy, j buy. Treat earlier order higher in the buy
	// side of the order book (higher priority).
	return i.PlacedHeight > j.PlacedHeight
}

func sortOrders(orders []Order) {
	sort.Slice(orders, func(i, j int) bool {
		return orderLess(orders[i], orders[j])
	})
}

func merge(sortedShards [numShardPerMarket][]Order) []Order {
	// TODO: implement k-way merge
	total := 0
	for i := range sortedShards {
		total += len(sortedShards[i])
	}

	r := make([]Order, 0, total)
	for i := range sortedShards {
		r = append(r, sortedShards[i]...)
	}

	sortOrders(r)
	return r
}
