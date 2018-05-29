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

func sendTokenTxn(sk bls.SecretKey, to consensus.PK, quant uint64) []byte {
	send := SendTokenTxn{
		TokenID: 0,
		To:      to,
		Quant:   quant,
	}
	txn := Txn{
		T:     SendToken,
		Owner: consensus.PK(sk.GetPublicKey().Serialize()).Addr(),
		Data:  gobEncode(send),
	}
	txn.Sig = sk.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func createAccount(s *State, quant uint64) (bls.SecretKey, consensus.Addr) {
	var acc Account
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	acc.PK = sk.GetPublicKey().Serialize()
	addr := consensus.SHA3(acc.PK).Addr()
	acc.Balances = make(map[TokenID]*Balance)
	acc.Balances[0] = &Balance{Available: quant}
	s.accounts.Update(addr[:], gobEncode(acc))
	return sk, addr
}

func TestSendToken(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	sk, addr := createAccount(s, 100)

	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))

	var skRecv bls.SecretKey
	skRecv.SetByCSPRNG()

	to := consensus.PK(skRecv.GetPublicKey().Serialize())
	txn := sendTokenTxn(sk, to, 20)
	trans := s.Transition()
	valid, success := trans.Record(txn)
	assert.True(t, valid)
	assert.True(t, success)

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	trans.Commit()

	toAcc := s.Account(to.Addr())
	assert.Equal(t, 20, int(toAcc.Balances[0].Available))
	newAcc = s.Account(addr)
	assert.Equal(t, 80, int(newAcc.Balances[0].Available))
}

func TestTransitionNotCommitToDB(t *testing.T) {
	memDB := ethdb.NewMemDatabase()
	db := trie.NewDatabase(memDB)
	s := NewState(db)
	sk, addr := createAccount(s, 100)
	h, err := s.accounts.Commit(nil)
	if err != nil {
		panic(err)
	}

	err = db.Commit(h, false)
	if err != nil {
		panic(err)
	}

	dbLen := memDB.Len()
	assert.Equal(t, 1, dbLen)

	newAcc := s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	trans := s.Transition()

	for i := 0; i < 99; i++ {
		var skRecv bls.SecretKey
		skRecv.SetByCSPRNG()

		to := consensus.PK(skRecv.GetPublicKey().Serialize())
		txn := sendTokenTxn(sk, to, 1)
		valid, success := trans.Record(txn)
		assert.True(t, valid)
		assert.True(t, success)
	}

	newAcc = s.Account(addr)
	assert.Equal(t, 100, int(newAcc.Balances[0].Available))
	// test len does not change, transition not committed to DB
	assert.Equal(t, 1, memDB.Len())

	trans.Commit()
	newAcc = s.Account(addr)
	assert.Equal(t, 1, int(newAcc.Balances[0].Available))
}

func placeOrderTxn(sk bls.SecretKey, addr consensus.Addr, t PlaceOrderTxn) []byte {
	txn := Txn{
		T:     PlaceOrder,
		Owner: addr,
		Data:  gobEncode(t),
	}
	txn.Sig = sk.Sign(string(txn.Encode(false))).Serialize()
	return txn.Encode(true)
}

func TestPlaceOrder(t *testing.T) {
	s := NewState(trie.NewDatabase(ethdb.NewMemDatabase()))
	sk, addr := createAccount(s, 100)
	order := PlaceOrderTxn{
		Sell:         0,
		SellQuant:    40,
		Buy:          1,
		BuyQuant:     20,
		ExpireHeight: 0,
	}
	trans := s.Transition()
	trans.Record(placeOrderTxn(sk, addr, order))
	trans.Commit()

	acc := s.Account(addr)
	assert.Equal(t, 40, int(acc.Balances[0].Pending))

}

func TestPlaceOrderAlreadyExpire(t *testing.T) {
	// TODO
}

func TestPlaceOrderExpireLater(t *testing.T) {
	// TODO
	// TODO: also handle height reduced due to reorg.
}
