package dex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortOrders(t *testing.T) {
	orders := []Order{
		{PriceUnit: 300, PlacedHeight: 1},
		{PriceUnit: 200},
		{PriceUnit: 300},
		{PriceUnit: 100},
		{PriceUnit: 600},
		{PriceUnit: 200, SellSide: true},
		{PriceUnit: 400, SellSide: true},
		{PriceUnit: 300, SellSide: true},
		{PriceUnit: 500, SellSide: true},
		{PriceUnit: 300, PlacedHeight: 1, SellSide: true},
	}

	sortOrders(orders)

	assert.Equal(t, []Order{
		{PriceUnit: 100},
		{PriceUnit: 200},
		{PriceUnit: 300, PlacedHeight: 1},
		{PriceUnit: 300},
		{PriceUnit: 600},
		{PriceUnit: 200, SellSide: true},
		{PriceUnit: 300, SellSide: true},
		{PriceUnit: 300, PlacedHeight: 1, SellSide: true},
		{PriceUnit: 400, SellSide: true},
		{PriceUnit: 500, SellSide: true},
	}, orders)
}
