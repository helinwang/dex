package consensus

import "github.com/dfinity/go-dfinity-crypto/bls"

// PK is a serialized public key.
type PK []byte

func (p PK) Get() (bls.PublicKey, error) {
	var pk bls.PublicKey
	err := pk.Deserialize(p)
	if err != nil {
		return bls.PublicKey{}, err
	}

	return pk, nil
}

func (p PK) Addr() Addr {
	return SHA3(p).Addr()
}
