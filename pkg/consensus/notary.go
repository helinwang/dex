package consensus

import (
	"context"
	"log"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Notary notarizes blocks.
type Notary struct {
	owner Addr
	sk    bls.SecretKey
	share bls.SecretKey
	rb    *RandomBeacon
}

// NewNotary creates a new notary.
func NewNotary(owner Addr, sk, share bls.SecretKey, rb *RandomBeacon) *Notary {
	return &Notary{owner: owner, share: share, rb: rb}
}

// Notarize returns the notarized blocks of the current round,
// produced by the highest rank block proposer until ctx is cancelled.
//
// ctx will be cancelled when reaching the next round: when a
// notarized block of the current round is received.
func (n *Notary) Notarize(ctx, cancel context.Context, bCh chan *BlockProposal) []*NtShare {
	// TODO: validate BlockProposal, perhaps should be done by the
	// data layer.
	var bestRankBPs []*BlockProposal
	var bestRank int
	for {
		select {
		case <-ctx.Done():
			if len(bestRankBPs) == 0 {
				select {
				case b := <-bCh:
					bestRankBPs = append(bestRankBPs, b)
				case <-cancel.Done():
					return nil
				}
			}

			var bps []*NtShare
			for _, bp := range bestRankBPs {
				b := n.notarize(bp)
				bps = append(bps, b)
			}

			// TODO: continue to notarize even ctx is Done.
			return bps
		case bp := <-bCh:
			rank, err := n.rb.Rank(bp.Owner)
			if err != nil {
				log.Println(err)
				continue
			}

			if len(bestRankBPs) == 0 {
				bestRankBPs = []*BlockProposal{bp}
				bestRank = rank
				continue
			}

			if rank < bestRank {
				bestRankBPs = []*BlockProposal{bp}
				bestRank = rank
			} else if rank == bestRank {
				bestRankBPs = append(bestRankBPs, bp)
			}
		}
	}
}

func (n *Notary) notarize(bp *BlockProposal) *NtShare {
	// TODO: calculate state root
	b := &NtShare{
		Round: bp.Round,
		BP:    bp.Hash(),
		Owner: n.owner,
	}
	b.SigShare = n.share.Sign(string(bp.Encode(true))).Serialize()
	b.OwnerSig = n.sk.Sign(string(b.Encode(false))).Serialize()
	return b
}
