package dex

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/helinwang/dex/pkg/consensus"
)

func generateKeyPair() (pubkey, privkey []byte) {
	key, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	pubkey = elliptic.Marshal(secp256k1.S256(), key.X, key.Y)
	return pubkey, math.PaddedBigBytes(key.D, 32)
}

func BenchmarkECDSAVerify(b *testing.B) {
	pk, sk := generateKeyPair()
	msg := consensus.SHA3([]byte("hello"))
	sig, err := secp256k1.Sign(msg[:], sk)
	if err != nil {
		panic(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !secp256k1.VerifySignature(pk, msg[:], sig[:64]) {
			panic("verify sig failed")
		}
	}
}

func BenchmarkBLSVerify(b *testing.B) {
	sk := consensus.RandSK()
	pk := sk.MustPK()
	msg := consensus.SHA3([]byte("hello"))
	sig := sk.Sign(msg[:])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !sig.Verify(pk, msg[:]) {
			panic("verify sig failed")
		}
	}
}
