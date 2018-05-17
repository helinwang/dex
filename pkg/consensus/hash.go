package consensus

import (
	"math/big"

	"golang.org/x/crypto/sha3"
)

const (
	hashBytes = 32
)

// Hash is the hash of a piece of data.
type Hash [hashBytes]byte

func hash(b ...[]byte) Hash {
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

func hashMod(h Hash, n int) int {
	var b big.Int
	b.SetBytes(h[:])
	b.Mod(&b, big.NewInt(int64(n)))
	return int(b.Int64())
}

// Addr returns the address associated to the hash.
func (h Hash) Addr() Addr {
	var addr Addr
	copy(addr[:], h[hashBytes-addrBytes:])
	return addr
}
