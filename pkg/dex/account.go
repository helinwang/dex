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

type PendingOrder struct {
	ID       uint64
	Market   MarketSymbol
	Executed uint64
	Order
}

type Account struct {
	PK consensus.PK
	// a vector of nonce that enables concurrent transactions.
	NonceVec      []uint64
	Balances      map[TokenID]*Balance
	PendingOrders []PendingOrder
}

func (a *Account) EncodeRLP(w io.Writer) error {
	err := rlp.Encode(w, a.PK)
	if err != nil {
		return err
	}

	err = rlp.Encode(w, a.PendingOrders)
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

	// sort keys, the encoded bytes is deterministic given that
	// the keys are sorted and unique.
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

	v, err = s.Raw()
	if err != nil {
		return err
	}
	var pendingOrders []PendingOrder
	err = rlp.DecodeBytes(v, &pendingOrders)
	if err != nil {
		return err
	}
	b.PendingOrders = pendingOrders

	v, err = s.Raw()
	if err != nil {
		return err
	}
	var nonceVec []uint64
	err = rlp.DecodeBytes(v, &nonceVec)
	if err != nil {
		return err
	}
	b.NonceVec = nonceVec

	v, err = s.Raw()
	if err != nil {
		return err
	}
	var keys []TokenID
	err = rlp.DecodeBytes(v, &keys)
	if err != nil {
		return err
	}

	v, err = s.Raw()
	if err != nil {
		return err
	}
	var values []*Balance
	err = rlp.DecodeBytes(v, &values)
	if err != nil {
		return err
	}

	b.Balances = make(map[TokenID]*Balance)

	for i := range keys {
		b.Balances[keys[i]] = values[i]
	}

	*a = b
	return nil
}
