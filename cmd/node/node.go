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

func createNode(c consensus.NodeCredentials, addr, seed string, genesis *consensus.Block) *consensus.Node {
	// TODO: simplify
	randSeed := consensus.Rand(consensus.SHA3([]byte("dex")))
	cfg := consensus.Config{
		ProposalWaitDur: 150 * time.Millisecond,
		BlockTime:       200 * time.Millisecond,
		GroupSize:       5,
		GroupThreshold:  3,
	}

	sk, err := c.SK.Get()
	if err != nil {
		panic(err)
	}

	db := trie.NewDatabase(ethdb.NewMemDatabase())
	chain := consensus.NewChain(genesis, dex.NewState(db), randSeed, cfg)
	networking := consensus.NewNetworking(&network.Network{}, addr, chain)
	err = networking.Start(seed)
	if err != nil {
		panic(err)
	}

	node := consensus.NewNode(chain, sk, networking, cfg)
	return node
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

	n := createNode(credentials, *addr, *seedNode, &genesis)
	n.StartRound(1)

	select {}
}
