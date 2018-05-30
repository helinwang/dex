package main

import (
	"bytes"
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

func createNode(c consensus.NodeCredentials, genesis *consensus.Block) *consensus.Node {
	cfg := consensus.Config{
		ProposalWaitDur: 150 * time.Millisecond,
		BlockTime:       200 * time.Millisecond,
		GroupSize:       3,
		GroupThreshold:  2,
	}

	db := trie.NewDatabase(ethdb.NewMemDatabase())
	return consensus.MakeNode(c, &network.Network{}, cfg, genesis, dex.NewState(db))
}

func main() {
	err := bls.Init(int(bls.CurveFp254BNb))
	if err != nil {
		panic(err)
	}

	c := flag.String("c", "", "path to the node credential file")
	addr := flag.String("addr", ":8008", "node address to listen connection on")
	seedNode := flag.String("seed", "", "seed node address")
	g := flag.String("genesis", "", "path to the genesis block file")
	flag.Parse()

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

	n := createNode(credentials, &genesis)
	n.Start(*addr, *seedNode)
	n.StartRound(1)

	select {}
}
