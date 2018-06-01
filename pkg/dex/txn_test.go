package dex

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestOrderEncodeDecode(t *testing.T) {
	o := Order{
		SellSide: true,
		Price:    2,
		Quant:    1000,
	}
	b, err := rlp.EncodeToBytes(&o)
	if err != nil {
		panic(err)
	}

	var o1 Order
	err = rlp.DecodeBytes(b, &o1)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, o, o1)
}
