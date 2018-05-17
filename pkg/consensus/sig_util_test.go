package consensus

import (
	"fmt"
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/stretchr/testify/assert"
)

func init() {
	bls.Init(int(bls.CurveFp254BNb))
}

func TestVerifySig(t *testing.T) {
	const msg = "hello"
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	sign := sk.Sign(msg)
	assert.True(t, verifySig(*sk.GetPublicKey(), sign.Serialize(), []byte(msg)))
}

func TestVerifyBPSig(t *testing.T) {
	addr := Addr([addrBytes]byte{1})
	var sk bls.SecretKey
	sk.SetByCSPRNG()

	pk := *sk.GetPublicKey()
	bp := &BlockProposal{Owner: addr}
	d := bp.Encode(false)
	bp.OwnerSig = sk.Sign(string(d)).Serialize()

	data := []struct {
		b *BlockProposal
		r bool
	}{
		{
			&BlockProposal{},
			false,
		},
		{
			&BlockProposal{Owner: addr},
			false,
		},
		{
			bp,
			true,
		},
	}

	for i, d := range data {
		assert.Equal(t, d.r, verifySig(pk, d.b.OwnerSig, d.b.Encode(false)), fmt.Sprintf("row: %d", i))
	}
}
