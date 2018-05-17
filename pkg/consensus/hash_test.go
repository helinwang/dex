package consensus

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash(t *testing.T) {
	h := hash([]byte("hello"))
	assert.Equal(t, fmt.Sprintf("%x", h[:]), "3338be694f50c5f338814986cdf0686453a888b84f424d792af4b9202398f392")
}

func TestAddr(t *testing.T) {
	h := hash([]byte("hello"))
	addr := h.Addr()
	assert.Equal(t, fmt.Sprintf("%x", addr[:]), "cdf0686453a888b84f424d792af4b9202398f392")
}
