package consensus

import "github.com/dfinity/go-dfinity-crypto/bls"

const (
	groupSize      = 401
	groupThreshold = 200
)

// Group is a sample of all the nodes in the consensus infrastructure.
//
// Group can perform different roles:
// - random beacon committe
// - noterization committe
type Group struct {
	pks  []bls.PublicKey
	vVec []bls.SecretKey
}
