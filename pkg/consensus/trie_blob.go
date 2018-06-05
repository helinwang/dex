package consensus

type putter interface {
	Put(key, val []byte) error
}

// TrieBlob is a serialized trie.
type TrieBlob struct {
	Root Hash
	Data map[Hash][]byte
}

// Fill fills the data blob into the putter.
func (b *TrieBlob) Fill(p putter) error {
	for k, v := range b.Data {
		err := p.Put(k[:], v)
		if err != nil {
			return err
		}
	}
	return nil
}
