package consensus

import (
	"context"

	log "github.com/helinwang/log15"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Notary notarizes blocks.
type Notary struct {
	owner Addr
	sk    bls.SecretKey
	share bls.SecretKey
	chain *Chain
}

// NewNotary creates a new notary.
func NewNotary(owner Addr, sk, share bls.SecretKey, chain *Chain) *Notary {
	return &Notary{owner: owner, sk: sk, share: share, chain: chain}
}

// Notarize returns the notarized blocks of the current round,
// produced by the highest rank block proposer until ctx is cancelled.
//
// ctx will be cancelled when reaching the next round: when a
// notarized block of the current round is received.
// TODO: fix lint
// nolint: gocyclo
func (n *Notary) Notarize(ctx, cancel context.Context, bCh chan *BlockProposal, onNotarize func(*NtShare)) {
	var bestRankBPs []*BlockProposal
	var bestRank int
	for {
		select {
		case <-ctx.Done():

			for _, bp := range bestRankBPs {
				s := n.notarize(bp)
				if s != nil {
					onNotarize(s)
				}
			}

			for {
				select {
				case <-cancel.Done():
					return
				case bp := <-bCh:
					rank, err := n.chain.RandomBeacon.Rank(bp.Owner, n.chain.Round())
					if err != nil {
						log.Error("get rank error", "err", err, "bp round", bp.Round, "chain round", n.chain.Round())
						continue
					}

					if rank <= bestRank {
						bestRank = rank
						s := n.notarize(bp)
						if s != nil {
							onNotarize(s)
						}
					}
				}
			}
		case bp := <-bCh:
			rank, err := n.chain.RandomBeacon.Rank(bp.Owner, n.chain.Round())
			if err != nil {
				log.Error("get rank error", "err", err, "bp round", bp.Round, "chain round", n.chain.Round())
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
		case <-cancel.Done():
			return
		}
	}
}

func (n *Notary) notarize(bp *BlockProposal) *NtShare {
	// TODO: calculate state root
	b := &NtShare{
		Round: bp.Round,
		BP:    bp.Hash(),
	}

	prevBlock := n.chain.Block(bp.PrevBlock)
	if prevBlock == nil {
		panic("TODO")
	}

	blk, err := n.chain.BPToBlock(bp)
	if err != nil {
		log.Warn("failed to calculate state root during notarization", "err", err)
		return nil
	}

	b.SigShare = n.share.Sign(string(blk.Encode(false))).Serialize()
	b.Owner = n.owner
	b.OwnerSig = n.sk.Sign(string(b.Encode(false))).Serialize()
	return b
}
