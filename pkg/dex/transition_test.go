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

func sendTokenTxn(sk bls.SecretKey, to consensus.PK) []byte {
	send := SendTokenTxn{
		TokenID: 0,
		To:      to,
		Quant:   20,
	}
	txn := Txn{
		T:     SendToken,
		Owner: consensus.PK(sk.GetPublicKey().Serialize()).Addr(),
		Data:  gobEncode(send),
	}
	txn.Sig = sk.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func createAccount(s *State) (bls.SecretKey, consensus.Addr) {
	var acc Account
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	acc.PK = sk.GetPublicKey().Serialize()
	addr := consensus.SHA3(acc.PK).Addr()
	acc.Balances = make(map[TokenID]*Balance)
	acc.Balances[0] = &Balance{Available: 100}
	s.accounts.Update(addr[:], gobEncode(acc))
	return sk, addr
}

func TestSendToken(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	sk, addr := createAccount(s)

	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))

	var skRecv bls.SecretKey
	skRecv.SetByCSPRNG()

	to := consensus.PK(skRecv.GetPublicKey().Serialize())
	txn := sendTokenTxn(sk, to)
	trans := s.Transition()
	trans.Record(txn)

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	trans.Commit()

	toAcc := s.Account(to.Addr())
	assert.Equal(t, 20, int(toAcc.Balances[0].Available))
	newAcc = s.Account(addr)
	assert.Equal(t, 80, int(newAcc.Balances[0].Available))
}
