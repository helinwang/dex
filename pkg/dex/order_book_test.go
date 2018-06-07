package dex

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestOrderBookBid(t *testing.T) {
	book := &orderBook{}
	book.Limit(Order{
		Quant: 10,
		Price: 1,
	})
	e := &orderBookEntry{
		orderBookEntryData: orderBookEntryData{
			Quant: 10,
		},
	}
	assert.Equal(t, book.bidMax, &pricePoint{
		Price:    1,
		ListHead: e,
		ListTail: e,
	})

	book.Limit(Order{
		Quant: 12,
		Price: 1,
	})
	e1 := &orderBookEntry{
		orderBookEntryData: orderBookEntryData{
			Quant: 12,
		},
	}
	e.Next = e1
	assert.Equal(t, book.bidMax, &pricePoint{
		Price:    1,
		ListHead: e,
		ListTail: e1,
	})
}

func TestOrderBookSell(t *testing.T) {
	book := &orderBook{}
	book.Limit(Order{
		Quant:    10,
		Price:    1,
		SellSide: true,
	})
	e := &orderBookEntry{
		orderBookEntryData: orderBookEntryData{
			Quant: 10,
		},
	}
	assert.Equal(t, book.askMin, &pricePoint{
		Price:    1,
		ListHead: e,
		ListTail: e,
	})

	book.Limit(Order{
		Quant:    12,
		Price:    1,
		SellSide: true,
	})
	e1 := &orderBookEntry{
		orderBookEntryData: orderBookEntryData{
			Quant: 12,
		},
	}
	e.Next = e1
	assert.Equal(t, book.askMin, &pricePoint{
		Price:    1,
		ListHead: e,
		ListTail: e1,
	})
}

func TestOrderBookMatching(t *testing.T) {
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
		Price:    1,
		SellSide: true,
	})
	assert.Nil(t, book.bidMax)
	assert.Equal(t, 1, int(book.askMin.Price))
	assert.Equal(t, 90, int(book.askMin.ListHead.Quant))
}

func TestOrderBookEncodeDecode(t *testing.T) {
	orders := []Order{
		{
			Quant: 10,
			Price: 1,
		},
		{
			Quant: 101,
			Price: 0,
		},
		{
			Quant: 12,
			Price: 1,
		},
		{
			Quant: 100,
			Price: 0,
		},
		{
			Quant: 1,
			Price: 3,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    21,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    31,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    31,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    51,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    60,
		},
		{
			SellSide: true,
			Quant:    2,
			Price:    60,
		},
	}

	book := &orderBook{}
	for _, o := range orders {
		book.Limit(o)
	}

	b, err := rlp.EncodeToBytes(book)
	if err != nil {
		panic(err)
	}

	var book1 orderBook
	err = rlp.DecodeBytes(b, &book1)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, book.bidMax, book1.bidMax)
}
