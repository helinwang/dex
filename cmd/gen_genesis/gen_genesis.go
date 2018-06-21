package main

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/dfinity/go-dfinity-crypto/bls"
	"github.com/helinwang/dex/pkg/consensus"
)

func getMasterSecretKey(sk bls.SecretKey, k int, rand consensus.Rand) ([]bls.SecretKey, consensus.Rand) {
	msk := make([]bls.SecretKey, k)
	msk[0] = sk
	for i := 1; i < k; i++ {
		msk[i] = rand.SK()
		rand = rand.Derive(rand[:])
	}
	return msk, rand
}

func makeShares(t int, idVec []bls.ID, rand consensus.Rand) (bls.PublicKey, []bls.SecretKey, consensus.Rand) {
	sk := rand.SK()
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

func main() {
	var numNode int
	var numGroup int
	var groupSize int
	var threshold int
	var outDir string
	var seed string

	flag.IntVar(&numNode, "N", 30, "number of nodes registered in the genesis block")
	flag.IntVar(&numGroup, "g", 20, "number of groups registered in the genesis block")
	flag.IntVar(&groupSize, "n", 5, "group size")
	flag.IntVar(&threshold, "t", 3, "group threshold size")
	flag.StringVar(&outDir, "d", "./credentials", "output directoy name")
	flag.StringVar(&seed, "seed", "dex-genesis-group", "random seed")
	flag.Parse()

	rand := consensus.Rand(consensus.SHA3([]byte(seed)))
	nodeDir := path.Join(outDir, "nodes")

	err = os.MkdirAll(nodeDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	var sysTxns []consensus.SysTxn
	nodePKs := make([]bls.PublicKey, numNode)
	nodes := make([]consensus.NodeCredentials, numNode)
	for i := 0; i < numNode; i++ {
		sk := rand.SK()
		nodePKs[i] = *sk.GetPublicKey()
		nodes[i].SK = sk.GetLittleEndian()
		rand = rand.Derive(rand[:])

		txn := consensus.ReadyJoinGroupTxn{
			ID: i,
			PK: nodePKs[i].Serialize(),
		}
		sysTxns = append(sysTxns, consensus.SysTxn{
			Type: consensus.ReadyJoinGroup,
			Data: gobEncode(txn),
		})
	}

	groupIDs := make([]int, numGroup)
	for i := range groupIDs {
		perm := rand.Perm(groupSize, numNode)
		rand = rand.Derive(rand[:])
		idVec := make([]bls.ID, groupSize)
		for i := range idVec {
			pk := nodePKs[perm[i]]
			idVec[i] = consensus.SHA3(pk.Serialize()).Addr().ID()
		}
		var groupPK bls.PublicKey
		var shares []bls.SecretKey
		groupPK, shares, rand = makeShares(threshold, idVec, rand)
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
			PK:         groupPK.Serialize(),
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

	genesis := &consensus.Block{
		SysTxns: sysTxns,
	}
	f, err := os.Create(path.Join(outDir, "genesis.gob"))
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

	nativeCoinOwnerSK, err := nodes[0].SK.Get()
	if err != nil {
		panic(err)
	}

	fmt.Println(base64.StdEncoding.EncodeToString(nativeCoinOwnerSK.GetPublicKey().Serialize()))
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
