package main

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"flag"
	"io/ioutil"
	"time"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/dex"
	"github.com/helinwang/dex/pkg/network"
)

func decodeFromFile(path string, v interface{}) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	dec := gob.NewDecoder(bytes.NewReader(b))
	err = dec.Decode(v)
	if err != nil {
		panic(err)
	}
}

func createNode(c consensus.NodeCredentials, genesis *consensus.Block, nativeCoinOwnerPK consensus.PK, u consensus.Updater) *consensus.Node {
	cfg := consensus.Config{
		BlockTime:      200 * time.Millisecond,
		GroupSize:      3,
		GroupThreshold: 2,
	}

	db := trie.NewDatabase(ethdb.NewMemDatabase())
	state := dex.NewState(db)
	state = state.GenesisDistribution(&nativeCoinOwnerPK, dex.BNBInfo).(*dex.State)
	return consensus.MakeNode(c, &network.Network{}, cfg, genesis, state, dex.NewTxnPool(state), u)
}

func main() {
	err := bls.Init(int(bls.CurveFp254BNb))
	if err != nil {
		panic(err)
	}

	nativeCoinOwnerPK := flag.String("genesis-coin-owner-pk", "", "base64 encoded pre-mined native coin owner PK at the genesis")
	c := flag.String("c", "", "path to the node credential file")
	addr := flag.String("addr", ":8008", "node address to listen connection on")
	seedNode := flag.String("seed", "", "seed node address")
	g := flag.String("genesis", "", "path to the genesis block file")
	rpcAddr := flag.String("rpc-addr", ":12001", "rpc address used to serve wallet RPC calls")
	flag.Parse()

	if *nativeCoinOwnerPK == "" {
		panic("please specify argument -genesis-coin-owner-pk")
	}

	var genesis consensus.Block
	decodeFromFile(*g, &genesis)

	cb, err := ioutil.ReadFile(*c)
	if err != nil {
		panic(err)
	}

	var credentials consensus.NodeCredentials
	dec := gob.NewDecoder(bytes.NewReader(cb))
	err = dec.Decode(&credentials)
	if err != nil {
		panic(err)
	}

	pk, err := base64.StdEncoding.DecodeString(*nativeCoinOwnerPK)
	if err != nil {
		panic(err)
	}

	server := dex.NewRPCServer()
	n := createNode(credentials, &genesis, consensus.PK(pk), server)
	server.SetSender(n)
	server.Start(*rpcAddr)
	n.Start(*addr, *seedNode)
	n.StartRound(1)

	select {}
}
