package dex

import (
	"bytes"

	"github.com/dave/stablegob"
)

func stableGobEncode(v interface{}) []byte {
	var buf bytes.Buffer
	enc := stablegob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}
