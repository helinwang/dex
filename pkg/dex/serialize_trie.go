package dex

import (
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
)

type getter interface {
	Get(key []byte) ([]byte, error)
}

func serializeTrie(t *trie.Trie, db *trie.Database, getter getter) (blob *consensus.TrieBlob, err error) {
	root, err := t.Commit(nil)
	if err != nil {
		return
	}

	iter := t.NodeIterator([]byte{})
	if iter.Error() != nil {
		err = iter.Error()
		return
	}

	err = db.Commit(root, false)
	if err != nil {
		return
	}

	blob = &consensus.TrieBlob{Data: make(map[consensus.Hash][]byte)}
	hasNext := true
	for ; hasNext; hasNext = iter.Next(true) {
		if iter.Error() != nil {
			err = iter.Error()
			return
		}

		h := consensus.Hash(iter.Hash())
		var d []byte
		d, err := getter.Get(h[:])
		if err != nil {
			continue
		}

		blob.Data[h] = d
	}

	blob.Root = consensus.Hash(root)
	return
}
