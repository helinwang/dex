package dex

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/helinwang/dex/pkg/consensus"
)

type SK []byte
type PK []byte
type Sig []byte

type Credential struct {
	PK PK
	SK SK
}

func RandKeyPair() (PK, SK) {
	key, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	pubkey := elliptic.Marshal(secp256k1.S256(), key.X, key.Y)
	return PK(pubkey), SK(math.PaddedBigBytes(key.D, 32))
}

func (p PK) Addr() consensus.Addr {
	return consensus.SHA3(p).Addr()
}

func (s SK) Sign(msg []byte) Sig {
	in := consensus.SHA3(msg)
	sig, err := secp256k1.Sign(in[:], s)
	if err != nil {
		panic(err)
	}

	return Sig(sig)
}

func (s Sig) Verify(msg []byte, pk PK) bool {
	in := consensus.SHA3(msg)
	return secp256k1.VerifySignature(pk, in[:], s[:64])
}
