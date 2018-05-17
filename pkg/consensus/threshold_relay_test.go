package consensus

import (
	"testing"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

func makeShares(t int, nodes []*Node, rand Rand) (bls.PublicKey, []bls.SecretKey, Rand) {
	sk := rand.SK()
	rand = rand.Derive(rand[:])

	idVec := make([]bls.ID, len(nodes))
	for i := range idVec {
		idVec[i] = nodes[i].addr.ID()
	}

	msk := sk.GetMasterSecretKey(t)
	skShares := make([]bls.SecretKey, len(nodes))

	for i := range skShares {
		skShares[i].Set(msk, &idVec[i])
	}

	return *sk.GetPublicKey(), skShares, rand
}

func makeGroup(t, n int, nodes []*Node, rand Rand) (*Group, Rand) {
	members := make([]*Node, n)
	perm := rand.Perm(n, len(nodes))
	rand = rand.Derive(rand[:])

	for i := range members {
		members[i] = nodes[perm[i]]
	}

	pk, shares, rand := makeShares(t, members, rand)
	g := NewGroup(pk)
	for i := range members {
		members[i].memberships[g.PK] = membership{skShare: shares[i], g: g}
		g.MemberPK[members[i].addr] = *members[i].sk.GetPublicKey()
	}

	return g, rand
}

func TestThresholdRelay(t *testing.T) {
	const (
		numNode   = 10
		numGroup  = 20
		groupSize = 5
		threshold = 3
	)

	rand := Rand(hash([]byte("seed")))
	nodeSeed := rand.Derive([]byte("node"))

	nodes := make([]*Node, numNode)
	for i := range nodes {
		sk := rand.SK()
		rand = rand.Derive(rand[:])
		nodes[i] = NewNode(sk, nil, nodeSeed)
	}

	gs := make([]*Group, groupSize)
	for i := range gs {
		gs[i], rand = makeGroup(threshold, groupSize, nodes, rand)
	}

	for i := range nodes {
		nodes[i].roundInfo.groups = gs
	}
}
