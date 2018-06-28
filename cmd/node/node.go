package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"io/ioutil"
	"math/rand"
	"os"
	"runtime/pprof"
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

func createNode(c consensus.NodeCredentials, genesis consensus.Genesis, u consensus.Updater, t, g int) *consensus.Node {
	cfg := consensus.Config{
		BlockTime:      time.Second,
		GroupSize:      g,
		GroupThreshold: t,
	}

	state := dex.NewState(ethdb.NewMemDatabase())
	return consensus.MakeNode(c, cfg, genesis, state, dex.NewTxnPool(state), u)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	groupSize := flag.Int("g", 3, "group size")
	threshold := flag.Int("t", 2, "group signature threshold size")
	profileDur := flag.Duration("profile-dur", 0, "profile duration")
	lvl := flag.String("lvl", "info", "log level, possible values: debug, info, warn, error, crit")
	c := flag.String("c", "./genesis", "path to the node credential file")
	host := flag.String("host", "127.0.0.1", "node address to listen connection on")
	port := flag.Int("port", 11001, "node address to listen connection on")
	seedNode := flag.String("seed", "", "seed node address")
	g := flag.String("genesis", "", "path to the genesis block file")
	rpcAddr := flag.String("rpc-addr", ":12001", "rpc address used to serve wallet RPC calls")
	flag.Parse()

	if *profileDur > 0 {
		go func() {
			f, err := os.Create("profile.prof")
			if err != nil {
				panic(err)
			}

			err = pprof.StartCPUProfile(f)
			if err != nil {
				panic(err)
			}

			time.AfterFunc(*profileDur, pprof.StopCPUProfile)
		}()
	}

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

	var credential consensus.NodeCredentials
	dec := gob.NewDecoder(bytes.NewReader(cb))
	err = dec.Decode(&credential)
	if err != nil {
		panic(err)
	}

	server := dex.NewRPCServer()
	n := createNode(credential, genesis, server, *threshold, *groupSize)
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

	log15.Info("node info", "addr", credential.SK.MustPK().Addr(), "member of groups", credential.Groups)
	n.EndRound(0)

	select {}
}
