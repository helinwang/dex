package consensus

import (
	"context"

	log "github.com/helinwang/log15"
)

// Notary notarizes blocks.
type Notary struct {
	owner Addr
	sk    SK
	share SK
	chain *Chain
}

// NewNotary creates a new notary.
func NewNotary(owner Addr, sk, share SK, chain *Chain) *Notary {
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
					rank, err := n.chain.RandomBeacon.Rank(bp.Owner, n.chain.Height())
					if err != nil {
						log.Error("get rank error", "err", err, "bp round", bp.Round, "chain round", n.chain.Height())
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
			rank, err := n.chain.RandomBeacon.Rank(bp.Owner, n.chain.Height())
			if err != nil {
				log.Error("get rank error", "err", err, "bp round", bp.Round, "chain round", n.chain.Height())
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
	nts := &NtShare{
		Round: bp.Round,
		BP:    bp.Hash(),
	}

	prevBlock := n.chain.Block(bp.PrevBlock)
	if prevBlock == nil {
		panic("TODO")
	}

	state := n.chain.BlockToState(bp.PrevBlock)
	if state == nil {
		panic("TODO")
	}

	trans, err := getTransition(state, bp.Data, bp.Round)
	if err != nil {
		panic("TODO: " + err.Error())
	}

	blk := &Block{
		Owner:         bp.Owner,
		Round:         bp.Round,
		BlockProposal: bp.Hash(),
		PrevBlock:     bp.PrevBlock,
		SysTxns:       bp.SysTxns,
		StateRoot:     trans.StateHash(),
	}

	nts.SigShare = n.share.Sign(blk.Encode(false))
	nts.Owner = n.owner
	nts.Sig = n.sk.Sign(nts.Encode(false))
	return nts
}
