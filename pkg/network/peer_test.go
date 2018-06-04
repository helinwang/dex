package network

import (
	"bytes"
	"context"
	"encoding/gob"
	"testing"
	"time"

	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/network/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSequentialEncDec(t *testing.T) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	var pac packet
	pac.T = txnArg
	pac.Data = []byte{3}
	err := enc.Encode(pac)
	if err != nil {
		panic(err)
	}

	var pac1 packet
	pac1.T = sysTxnArg
	pac1.Data = []byte{4}
	err = enc.Encode(pac1)
	if err != nil {
		panic(err)
	}

	dec := gob.NewDecoder(bytes.NewReader(buf.Bytes()))
	var c packet
	err = dec.Decode(&c)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, pac, c)

	var d packet
	err = dec.Decode(&d)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, pac1, d)
}

func TestPeer(t *testing.T) {
	peerCh := make(chan consensus.Peer, 1)
	onPeerConnect := func(p consensus.Peer) {
		peerCh <- p
	}

	var net Network
	err := net.Start(":8081", onPeerConnect, &mocks.Peer{})
	if err != nil {
		panic(err)
	}

	dst := &mocks.Peer{}
	_, err = net.Connect(":8081", dst)
	if err != nil {
		panic(err)
	}

	p := <-peerCh
	r0 := []string{"peer0", "peer1"}
	r1 := []*consensus.RandBeaconSig{&consensus.RandBeaconSig{Round: 1}}
	r2 := []*consensus.Block{&consensus.Block{Round: 1}}
	dst.On("Txn", mock.Anything).Return(nil)
	dst.On("SysTxn", mock.Anything).Return(nil)
	dst.On("RandBeaconSigShare", mock.Anything).Return(nil)
	dst.On("RandBeaconSig", mock.Anything).Return(nil)
	dst.On("Block", mock.Anything, mock.Anything).Return(nil)
	dst.On("BlockProposal", mock.Anything, mock.Anything).Return(nil)
	dst.On("NotarizationShare", mock.Anything).Return(nil)
	dst.On("Inventory", mock.Anything, mock.Anything).Return(nil)
	dst.On("GetData", mock.Anything, mock.Anything).Return(nil)
	dst.On("Peers", mock.Anything).Return(r0, nil)
	dst.On("UpdatePeers", mock.Anything).Return(nil)
	dst.On("Ping", mock.Anything).Return(nil)
	dst.On("Sync", mock.Anything).Return(r1, r2, nil)

	for i := 0; i < 3; i++ {
		a0 := []byte{1}
		p.Txn(a0)
		a1 := &consensus.SysTxn{Data: []byte{2}}
		p.SysTxn(a1)
		a2 := &consensus.RandBeaconSigShare{Round: 1}
		p.RandBeaconSigShare(a2)
		a3 := &consensus.RandBeaconSig{Round: 1}
		p.RandBeaconSig(a3)
		a4 := &consensus.Block{Round: 1}
		p.Block(nil, a4)
		a5 := &consensus.BlockProposal{Round: 1}
		p.BlockProposal(nil, a5)
		a6 := &consensus.NtShare{Round: 1}
		p.NotarizationShare(a6)
		a71 := []consensus.ItemID{consensus.ItemID{ItemRound: 1}}
		p.Inventory(nil, a71)
		a81 := []consensus.ItemID{consensus.ItemID{ItemRound: 2}}
		p.GetData(nil, a81)
		ret0, _ := p.Peers()
		a9 := []string{"p0"}
		p.UpdatePeers(a9)
		p.Ping(context.Background())
		a10 := 1
		ret1, ret2, _ := p.Sync(a10)

		time.Sleep(20 * time.Millisecond)
		dst.AssertCalled(t, "Txn", a0)
		dst.AssertCalled(t, "SysTxn", a1)
		dst.AssertCalled(t, "RandBeaconSigShare", a2)
		dst.AssertCalled(t, "RandBeaconSig", a3)
		dst.AssertCalled(t, "Block", mock.Anything, a4)
		dst.AssertCalled(t, "BlockProposal", mock.Anything, a5)
		dst.AssertCalled(t, "NotarizationShare", a6)
		dst.AssertCalled(t, "Inventory", mock.Anything, a71)
		dst.AssertCalled(t, "GetData", mock.Anything, a81)
		dst.AssertCalled(t, "Peers")
		dst.AssertCalled(t, "UpdatePeers", a9)
		dst.AssertCalled(t, "Ping", mock.Anything)
		dst.AssertCalled(t, "Sync", a10)

		assert.Equal(t, r0, ret0)
		assert.Equal(t, r1, ret1)
		assert.Equal(t, r2, ret2)
	}
}
