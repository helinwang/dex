package consensus

import (
	"golang.org/x/crypto/sha3"
)

const (
	hashBytes = 32
)

// Hash is the hash of a piece of data.
type Hash [hashBytes]byte

// SHA3 hashs the given slices with SHA3.
func SHA3(b ...[]byte) Hash {
	d := sha3.New256()
	for _, e := range b {
		_, err := d.Write(e)
		if err != nil {
			// should not happen
			panic(err)
		}
	}
	h := d.Sum(nil)
	var hash Hash
	copy(hash[:], h)
	return hash
}

// Addr returns the address associated to the hash.
func (h Hash) Addr() Addr {
	var addr Addr
	copy(addr[:], h[hashBytes-addrBytes:])
	return addr
}
