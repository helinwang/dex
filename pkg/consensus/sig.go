package consensus

import "github.com/dfinity/go-dfinity-crypto/bls"

// PK is a serialized public key.
type PK []byte

func (p PK) MustGet() bls.PublicKey {
	var pk bls.PublicKey
	err := pk.Deserialize(p)
	if err != nil {
		panic(err)
	}

	return pk
}

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

// SK is a serialized secret key
type SK []byte

func (s SK) Get() (bls.SecretKey, error) {
	var sk bls.SecretKey
	err := sk.SetLittleEndian(s)
	if err != nil {
		return bls.SecretKey{}, err
	}

	return sk, nil
}

func (s SK) MustGet() bls.SecretKey {
	var sk bls.SecretKey
	err := sk.SetLittleEndian(s)
	if err != nil {
		panic(err)
	}

	return sk
}

func (s SK) PK() (PK, error) {
	var sk bls.SecretKey
	err := sk.SetLittleEndian(s)
	if err != nil {
		return nil, err
	}

	return PK(sk.GetPublicKey().Serialize()), nil
}

func (s SK) MustPK() PK {
	var sk bls.SecretKey
	err := sk.SetLittleEndian(s)
	if err != nil {
		panic(err)
	}

	return PK(sk.GetPublicKey().Serialize())
}

func (s SK) Sign(b []byte) Sig {
	key := s.MustGet()
	return Sig(key.Sign(string(b)).Serialize())
}

// Sig is a serialized signature
type Sig []byte

func (s Sig) Verify(pk PK, msg []byte) bool {
	if len(s) == 0 || len(pk) == 0 {
		return false
	}

	var sign bls.Sign
	err := sign.Deserialize(s)
	if err != nil {
		return false
	}

	key := pk.MustGet()
	return sign.Verify(&key, string(msg))
}
