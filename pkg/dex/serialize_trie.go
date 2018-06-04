package dex

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie"
)

type Getter interface {
	Get(key []byte) ([]byte, error)
}

type Putter interface {
	Put(key, val []byte) error
}

// Blob is a serizlied trie.
type Blob struct {
	Root common.Hash
	Data map[common.Hash][]byte
}

func (b *Blob) Fill(p Putter) error {
	for k, v := range b.Data {
		err := p.Put(k[:], v)
		if err != nil {
			return err
		}
	}
	return nil
}

func SerializeTrie(t *trie.Trie, db *trie.Database, getter Getter) (blob *Blob, err error) {
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

	blob = &Blob{Data: make(map[common.Hash][]byte)}
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
