package dex

import (
	"io"
	"sort"

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

type Account struct {
	PK consensus.PK
	// a vector of nonce that enables concurrent transactions.
	NonceVec            []uint64
	Balances            map[TokenID]*Balance
	PendingOrderMarkets []MarketSymbol
}

func (a *Account) EncodeRLP(w io.Writer) error {
	err := rlp.Encode(w, a.PK)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, a.PendingOrderMarkets)
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
	for k := range a.Balances {
		keys[i] = k
		i++
	}

	// sort keys so the encoded bytes are deterministic
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for i, k := range keys {
		values[i] = a.Balances[k]
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

	// the rlp package is not returning the correct list size,
	// devide by 3 will result a correct size. 3 seems to come
	// from the fact that type `MarketSymbol` has 2 elements.
	l = l / 3

	ps := make([]MarketSymbol, int(l))
	for i := range ps {
		d, err := s.Raw()
		if err != nil {
			return err
		}

		var p MarketSymbol
		err = rlp.DecodeBytes(d, &p)
		if err != nil {
			return err
		}

		ps[i] = p
	}
	b.PendingOrderMarkets = ps

	err = s.ListEnd()
	if err != nil {
		return err
	}

	l, err = s.List()
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
