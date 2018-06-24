package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"io/ioutil"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/ethdb"

	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/dex"
	"github.com/helinwang/log15"
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

func createNode(c consensus.NodeCredentials, genesis consensus.Genesis, u consensus.Updater) *consensus.Node {
	cfg := consensus.Config{
		BlockTime:      time.Second,
		GroupSize:      3,
		GroupThreshold: 2,
	}

	state := dex.NewState(ethdb.NewMemDatabase())
	return consensus.MakeNode(c, cfg, genesis, state, dex.NewTxnPool(), u)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	lvl := flag.String("lvl", "info", "log level, possible values: debug, info, warn, error, crit")
	c := flag.String("c", "./genesis", "path to the node credential file")
	host := flag.String("host", "127.0.0.1", "node address to listen connection on")
	port := flag.Int("port", 11001, "node address to listen connection on")
	seedNode := flag.String("seed", "", "seed node address")
	g := flag.String("genesis", "", "path to the genesis block file")
	rpcAddr := flag.String("rpc-addr", ":12001", "rpc address used to serve wallet RPC calls")
	flag.Parse()

	l, err := log15.LvlFromString(*lvl)
	if err != nil {
		panic(err)
	}

	log15.Root().SetHandler(log15.LvlFilterHandler(l, log15.StdoutHandler))
	var genesis consensus.Genesis
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

	server := dex.NewRPCServer()
	n := createNode(credentials, genesis, server)
	server.SetSender(n)
	server.SetStater(n.Chain())
	err = server.Start(*rpcAddr)
	if err != nil {
		log15.Warn("can not start wallet service", "err", err)
	}

	err = n.Start(*host, *port, *seedNode)
	if err != nil {
		log15.Error("can not connect to seed node", "seed", *seedNode, "err", err)
		return
	}

	n.EndRound(0)

	select {}
}
