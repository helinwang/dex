package consensus

import (
	"testing"
	"time"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/stretchr/testify/assert"
)

func makeNetwork() *network {
	var sk bls.SecretKey
	sk.SetByCSPRNG()
	return newNetwork(SK(sk.GetLittleEndian()))
}

func TestNetworkConnectSeed(t *testing.T) {
	n0 := makeNetwork()
	n1 := makeNetwork()
	addr0, err := n0.Start("127.0.0.1", 11001)
	if err != nil {
		panic(err)
	}

	addr1, err := n1.Start("127.0.0.1", 11000)
	if err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Millisecond)
	err = n1.ConnectSeed(addr0.Addr)
	if err != nil {
		panic(err)
	}

	assert.Equal(t, []unicastAddr{addr0}, n1.publicNodes)
	assert.Equal(t, []unicastAddr{addr1}, n0.publicNodes)
}
