package dex

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func serializeAndDeserialize(t *trie.Trie, db *trie.Database, getter getter) *trie.Trie {
	b, err := serializeTrie(t, db, getter)
	if err != nil {
		panic(err)
	}

	memDB := ethdb.NewMemDatabase()
	err = b.Fill(memDB)
	if err != nil {
		panic(err)
	}

	trie1, err := trie.New(common.Hash(b.Root), trie.NewDatabase(memDB))
	if err != nil {
		panic(err)
	}

	return trie1
}

func TestSerializeAndDeserialize(t *testing.T) {
	getter := ethdb.NewMemDatabase()
	db := trie.NewDatabase(getter)
	trie0, err := trie.New(common.Hash{}, db)
	if err != nil {
		panic(err)
	}

	for i := 0; i < 1000; i++ {
		key := consensus.SHA3([]byte{byte(i >> 8), byte(i)})
		val := consensus.SHA3(key[:])
		trie0.Update(key[:], val[:])
	}

	trie1 := serializeAndDeserialize(trie0, db, getter)
	assert.Equal(t, trie0.Hash(), trie1.Hash())
}
