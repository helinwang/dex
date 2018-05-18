package consensus

import (
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

func makeShares(t int, idVec []bls.ID, rand Rand) (bls.PublicKey, []bls.SecretKey, Rand) {
	sk := rand.SK()
	rand = rand.Derive(rand[:])

	msk := sk.GetMasterSecretKey(t)
	skShares := make([]bls.SecretKey, len(idVec))

	for i := range skShares {
		skShares[i].Set(msk, &idVec[i])
	}

	return *sk.GetPublicKey(), skShares, rand
}

func setupNodes() []*Node {
	const (
		numNode   = 10
		numGroup  = 20
		groupSize = 5
		threshold = 3
	)

	rand := Rand(hash([]byte("seed")))
	nodeSeed := rand.Derive([]byte("node"))

	genesis := &Block{}
	nodeSKs := make([]bls.SecretKey, numNode)
	for i := range nodeSKs {
		nodeSKs[i] = rand.SK()
		rand = rand.Derive(rand[:])
		txn := ReadyJoinGroupTxn{
			ID: i,
			PK: nodeSKs[i].GetPublicKey().Serialize(),
		}
		genesis.SysTxns = append(genesis.SysTxns, SysTxn{
			Type: ReadyJoinGroup,
			Data: gobEncode(txn),
		})
	}

	gs := make([]*Group, numGroup)
	groupIDs := make([]int, numGroup)
	sharesVec := make([][]bls.SecretKey, numGroup)
	perms := make([][]int, numGroup)
	for i := range groupIDs {
		perm := rand.Perm(groupSize, numNode)
		perms[i] = perm
		rand = rand.Derive(rand[:])
		idVec := make([]bls.ID, groupSize)
		for i := range idVec {
			pk := nodeSKs[perm[i]].GetPublicKey()
			idVec[i] = hash(pk.Serialize()).Addr().ID()
		}

		var groupPK bls.PublicKey
		groupPK, sharesVec[i], rand = makeShares(threshold, idVec, rand)
		gs[i] = NewGroup(groupPK)
		txn := RegGroupTxn{
			ID:        i,
			PK:        groupPK.Serialize(),
			MemberIDs: perm,
		}
		genesis.SysTxns = append(genesis.SysTxns, SysTxn{
			Type: RegGroup,
			Data: gobEncode(txn),
		})
		groupIDs[i] = i
	}

	l := ListGroupsTxn{
		GroupIDs: groupIDs,
	}

	genesis.SysTxns = append(genesis.SysTxns, SysTxn{
		Type: ListGroups,
		Data: gobEncode(l),
	})

	nodes := make([]*Node, numNode)
	for i := range nodes {
		nodes[i] = NewNode(genesis, nil, nodeSKs[i], nil, nodeSeed)
	}

	return nodes
}

func TestThresholdRelay(t *testing.T) {
	setupNodes()
}
