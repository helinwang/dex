package dex

import (
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/helinwang/dex/pkg/consensus"
)

const (
	maxNonceIdx = 100
)

type Balance struct {
	Available uint64
	Pending   uint64
}

// TODO: record account has pending orders on which markets.
type Account struct {
	PK consensus.PK
	// a vector of nonce that enables concurrent transactions.
	NonceVec []uint64
	Balances map[TokenID]*Balance
}

func (a *Account) EncodeRLP(w io.Writer) error {
	err := rlp.Encode(w, a.PK)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, a.NonceVec)
	if err != nil {
		return err
	}

	keys := make([]TokenID, len(a.Balances))
	values := make([]*Balance, len(a.Balances))
	i := 0
	for k, v := range a.Balances {
		keys[i] = k
		values[i] = v
		i++
	}

	err = rlp.Encode(w, keys)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, values)
	if err != nil {
		return err
	}

	return nil
}

func (a *Account) DecodeRLP(s *rlp.Stream) error {
	var b Account
	v, err := s.Bytes()
	if err != nil {
		return err
	}
	b.PK = consensus.PK(v)

	l, err := s.List()
	if err != nil {
		return err
	}

	vec := make([]uint64, int(l))
	for i := range vec {
		v, err := s.Uint()
		if err != nil {
			return err
		}

		vec[i] = v
	}
	b.NonceVec = vec

	err = s.ListEnd()
	if err != nil {
		return err
	}

	l, err = s.List()
	if err != nil {
		return err
	}

	keys := make([]uint8, int(l))
	for i := range keys {
		v, err := s.Uint()
		if err != nil {
			return err
		}

		keys[i] = uint8(v)
	}

	err = s.ListEnd()
	if err != nil {
		return err
	}

	b.Balances = make(map[TokenID]*Balance)

	_, err = s.List()
	if err != nil {
		return err
	}

	for i := range keys {
		var balance Balance
		d, err := s.Raw()
		if err != nil {
			return err
		}

		err = rlp.DecodeBytes(d, &balance)
		if err != nil {
			return err
		}

		b.Balances[TokenID(keys[i])] = &balance
	}

	err = s.ListEnd()
	if err != nil {
		return err
	}

	*a = b
	return nil
}
