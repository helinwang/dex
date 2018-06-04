package dex

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
)

type getter interface {
	Get(key []byte) ([]byte, error)
}

type putter interface {
	Put(key, val []byte) error
}

// trieBlob is a serizlied trie.
type trieBlob struct {
	Root common.Hash
	Data map[common.Hash][]byte
}

func (b *trieBlob) Fill(p putter) error {
	for k, v := range b.Data {
		err := p.Put(k[:], v)
		if err != nil {
			return err
		}
	}
	return nil
}

func serializeTrie(t *trie.Trie, db *trie.Database, getter getter) (blob *trieBlob, err error) {
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

	blob = &trieBlob{Data: make(map[common.Hash][]byte)}
	hasNext := true
	for ; hasNext; hasNext = iter.Next(true) {
		if iter.Error() != nil {
			err = iter.Error()
			return
		}

		h := iter.Hash()
		var d []byte
		d, err := getter.Get(h[:])
		if err != nil {
			continue
		}

		blob.Data[h] = d
	}

	blob.Root = root
	return
}
