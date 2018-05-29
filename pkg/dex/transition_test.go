package dex

import (
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/stretchr/testify/assert"
)

func init() {
	bls.Init(int(bls.CurveFp254BNb))
}

func TestSendToken(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	var acc Account
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	acc.PK = sk.GetPublicKey().Serialize()
	addr := consensus.SHA3(acc.PK).Addr()
	acc.Balances = make(map[TokenID]*Balance)
	acc.Balances[0] = &Balance{Available: 100}
	s.accounts.Update(addr[:], gobEncode(acc))

	newAcc := s.Account(addr)
	assert.Equal(t, 100, newAcc.Balances[0].Available)

	var skRecv bls.SecretKey
	skRecv.SetByCSPRNG()
	pk := consensus.PK(skRecv.GetPublicKey().Serialize())
	send := SendTokenTxn{
		TokenID: 0,
		To:      pk,
		Quant:   20,
	}
	txn := Txn{
		T:     SendToken,
		Owner: acc.PK.Addr(),
		Data:  gobEncode(send),
	}
	txn.Sig = sk.Sign(string(txn.Encode(false))).Serialize()

	trans := s.Transition()
	trans.Record(gobEncode(txn))

	newAcc = s.Account(addr)
	assert.Equal(t, 100, newAcc.Balances[0].Available)

	trans.Commit()
	toAcc := s.Account(pk.Addr())
	assert.Equal(t, 20, toAcc.Balances[0].Available)
	newAcc = s.Account(addr)
	assert.Equal(t, 80, newAcc.Balances[0].Available)
}
