package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/helinwang/dex/pkg/consensus"
	"github.com/helinwang/dex/pkg/dex"
)

func getMasterSecretKey(sk bls.SecretKey, k int, rand consensus.Rand) ([]bls.SecretKey, consensus.Rand) {
	msk := make([]bls.SecretKey, k)
	msk[0] = sk
	for i := 1; i < k; i++ {
		msk[i] = rand.SK().MustGet()
		rand = rand.Derive(rand[:])
	}
	return msk, rand
}

func makeShares(t int, idVec []bls.ID, rand consensus.Rand) (bls.PublicKey, []bls.SecretKey, consensus.Rand) {
	sk := rand.SK().MustGet()
	rand = rand.Derive(rand[:])

	msk, rand := getMasterSecretKey(sk, t, rand)
	skShares := make([]bls.SecretKey, len(idVec))

	for i := range skShares {
		err := skShares[i].Set(msk, &idVec[i])
		if err != nil {
			panic(err)
		}
	}

	return *sk.GetPublicKey(), skShares, rand
}

func loadCredentials(dir string) ([]consensus.PK, error) {
	var r []consensus.PK
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		if !strings.HasPrefix(f.Name(), "node-") {
			continue
		}

		path := path.Join(dir, f.Name())
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}

		dec := gob.NewDecoder(bytes.NewReader(b))
		var c consensus.NodeCredentials
		err = dec.Decode(&c)
		if err != nil {
			fmt.Printf("error decode credential from file: %s, err: %v, skip\n", path, err)
			continue
		}

		r = append(r, c.SK.MustPK())
	}

	return r, nil
}

func main() {
	numNode := flag.Int("N", 30, "number of nodes registered in the genesis block")
	numGroup := flag.Int("g", 20, "number of groups registered in the genesis block")
	groupSize := flag.Int("n", 3, "group size")
	threshold := flag.Int("t", 2, "group threshold size")
	outDir := flag.String("dir", "./genesis", "output directoy name")
	distributeTo := flag.String("distribute-to", "./credentials", "the native token (and the optionally created tokens) will be evenly distributed to all credentials in this folder")
	seed := flag.String("seed", "dex-genesis-group", "random seed")
	additionalTokenPath := flag.String("tokens", "", "path to the file which contains additional tokens to evenly distribute, each row is in format SYMBOL,QUANTITY,DECIMALS. BNB does not have to be in this file, it's distributed by default")
	flag.Parse()

	var additionalTokens []dex.TokenInfo
	if *additionalTokenPath != "" {
		b, err := ioutil.ReadFile(*additionalTokenPath)
		if err != nil {
			fmt.Printf("error loading additional token file: %v\n", err)
			return
		}

		s := bufio.NewScanner(bytes.NewReader(b))
		for s.Scan() {
			ss := strings.Split(s.Text(), ",")
			if len(ss) != 3 {
				fmt.Println("additional token file's format is not correct, each row should be SYMBOL,QUANTITY,DECIMALS")
				return
			}

			symbol := ss[0]
			quant, err := strconv.ParseFloat(ss[1], 64)
			if err != nil {
				fmt.Printf("error parses quantity in additional token file: %v\n", err)
				return
			}

			decimals, err := strconv.ParseUint(ss[2], 10, 8)
			if err != nil {
				fmt.Printf("error parses decimals in additional token file: %v\n", err)
				return
			}

			quantUnits := uint64(quant * math.Pow10(int(decimals)))
			additionalTokens = append(additionalTokens, dex.TokenInfo{Symbol: dex.TokenSymbol(symbol), Decimals: uint8(decimals), TotalUnits: quantUnits})
		}

		if s.Err() != nil {
			fmt.Printf("error reading additional token file: %v\n", err)
			return
		}
	}

	owners, err := loadCredentials(*distributeTo)
	if err != nil {
		fmt.Printf("error loading credentials to which the tokens will be distributed to, err: %v\n", err)
		return
	}

	if len(owners) == 0 {
		fmt.Println("no credential loaded, please specify the credentials to which the tokens will be distributed to")
		return
	}

	rand := consensus.Rand(consensus.SHA3([]byte(*seed)))
	nodeDir := path.Join(*outDir, "nodes")

	err = os.MkdirAll(nodeDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	var sysTxns []consensus.SysTxn
	nodePKs := make([]consensus.PK, *numNode)
	nodes := make([]consensus.NodeCredentials, *numNode)
	for i := 0; i < *numNode; i++ {
		sk := rand.SK()
		nodePKs[i] = sk.MustPK()
		nodes[i].SK = sk
		rand = rand.Derive(rand[:])

		txn := consensus.ReadyJoinGroupTxn{
			ID: i,
			PK: nodePKs[i],
		}
		sysTxns = append(sysTxns, consensus.SysTxn{
			Type: consensus.ReadyJoinGroup,
			Data: gobEncode(txn),
		})
	}

	groupIDs := make([]int, *numGroup)
	for i := range groupIDs {
		perm := rand.Perm(*groupSize, *numNode)
		rand = rand.Derive(rand[:])
		idVec := make([]bls.ID, *groupSize)
		for i := range idVec {
			pk := nodePKs[perm[i]]
			idVec[i] = pk.Addr().ID()
		}
		var groupPK bls.PublicKey
		var shares []bls.SecretKey
		groupPK, shares, rand = makeShares(*threshold, idVec, rand)
		groupIDs[i] = i

		memberVVec := make([]consensus.PK, len(shares))
		for i := range memberVVec {
			memberVVec[i] = shares[i].GetPublicKey().Serialize()
		}

		for j := range shares {
			nodes[perm[j]].GroupShares = append(nodes[perm[j]].GroupShares, shares[j].GetLittleEndian())
			nodes[perm[j]].Groups = append(nodes[perm[j]].Groups, i)
		}

		txn := consensus.RegGroupTxn{
			ID:         i,
			PK:         consensus.PK(groupPK.Serialize()),
			MemberIDs:  perm,
			MemberVVec: memberVVec,
		}
		sysTxns = append(sysTxns, consensus.SysTxn{
			Type: consensus.RegGroup,
			Data: gobEncode(txn),
		})
	}

	l := consensus.ListGroupsTxn{
		GroupIDs: groupIDs,
	}

	sysTxns = append(sysTxns, consensus.SysTxn{
		Type: consensus.ListGroups,
		Data: gobEncode(l),
	})

	state := dex.CreateGenesisState(owners, additionalTokens)
	stateBlob, err := state.Serialize()
	if err != nil {
		panic(err)
	}

	genesisBlock := consensus.Block{
		StateRoot: state.Hash(),
		SysTxns:   sysTxns,
	}
	genesis := consensus.Genesis{
		Block: genesisBlock,
		State: stateBlob,
	}
	f, err := os.Create(path.Join(*outDir, "genesis.gob"))
	if err != nil {
		panic(err)
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	err = enc.Encode(genesis)
	if err != nil {
		panic(err)
	}

	for i, n := range nodes {
		f, err := os.Create(fmt.Sprintf("%s/node-%d", nodeDir, i))
		if err != nil {
			panic(err)
		}

		enc := gob.NewEncoder(f)
		err = enc.Encode(n)
		if err != nil {
			panic(err)
		}

		err = f.Close()
		if err != nil {
			panic(err)
		}
	}
}

func gobEncode(v interface{}) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}
