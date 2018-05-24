package consensus

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveRand(t *testing.T) {
	msg := []byte("hello")
	r := Rand(SHA3(msg))
	assert.Equal(t, "3338be694f50c5f338814986cdf0686453a888b84f424d792af4b9202398f392", fmt.Sprintf("%x", r[:]))
	r = r.Derive(msg)
	assert.Equal(t, "4bb222e4ebf5d6d91c8e2029c32d67fe9e9cb3e9cd796b9177517907f6ae4b83", fmt.Sprintf("%x", r[:]))
	r = r.Derive(msg)
	assert.Equal(t, "41b1a842aa9110326ac8ef1d0b3269d23e195abf4c2492cc2d96cdedd14787b5", fmt.Sprintf("%x", r[:]))
}

func TestMod(t *testing.T) {
	r := Rand(SHA3([]byte{1}))
	assert.Equal(t, r.Mod(7), r.Mod(7))
	assert.Equal(t, 4, r.Mod(7))
	assert.Equal(t, 0, r.Mod(11))
}

func TestPerm(t *testing.T) {
	r := Rand(SHA3([]byte{1}))
	assert.Equal(t, r.Perm(7, 7), r.Perm(7, 7))
	assert.Equal(t, []int{6, 4, 2, 0, 5, 1, 3}, r.Perm(7, 7))
	assert.Equal(t, []int{31, 16, 8, 50, 2, 17, 43}, r.Perm(7, 51))
}

func TestSK(t *testing.T) {
	r := Rand(SHA3([]byte{1}))
	assert.Equal(t, r.SK(), r.SK())
	assert.NotEqual(t, r.SK(), r.Derive([]byte{1}).SK())
}
