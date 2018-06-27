package dex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerify(t *testing.T) {
	pk, sk := RandKeyPair()
	msg := []byte("hello world")
	sig := sk.Sign(msg)
	assert.True(t, sig.Verify(msg[:], pk))
}
