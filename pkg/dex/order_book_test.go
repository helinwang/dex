package dex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderBook(t *testing.T) {
	orders := []Order{
		{
			Quant: 10,
			Price: 1,
		},
		{
			Quant: 1,
			Price: 3,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    2,
		},
	}
	book := &orderBook{}
	for _, o := range orders {
		book.Limit(o)
	}
	assert.Equal(t, 2, int(book.askMin.Price))
	assert.Equal(t, 1, int(book.askMin.ListHead.Quant))
	assert.Equal(t, 1, int(book.bidMax.Price))
	assert.Equal(t, 10, int(book.bidMax.ListHead.Quant))

	book.Limit(Order{
		Quant:    100,
		Price:    0,
		SellSide: true,
	})
	assert.Nil(t, book.bidMax)
	assert.Equal(t, 0, int(book.askMin.Price))
	assert.Equal(t, 90, int(book.askMin.ListHead.Quant))
}
