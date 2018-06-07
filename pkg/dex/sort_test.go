package dex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortOrders(t *testing.T) {
	orders := []Order{
		{Price: 300, PlacedHeight: 1},
		{Price: 200},
		{Price: 300},
		{Price: 100},
		{Price: 600},
		{Price: 200, SellSide: true},
		{Price: 400, SellSide: true},
		{Price: 300, SellSide: true},
		{Price: 500, SellSide: true},
		{Price: 300, PlacedHeight: 1, SellSide: true},
	}

	sortOrders(orders)

	assert.Equal(t, []Order{
		{Price: 100},
		{Price: 200},
		{Price: 300, PlacedHeight: 1},
		{Price: 300},
		{Price: 600},
		{Price: 200, SellSide: true},
		{Price: 300, SellSide: true},
		{Price: 300, PlacedHeight: 1, SellSide: true},
		{Price: 400, SellSide: true},
		{Price: 500, SellSide: true},
	}, orders)
}
