package consensus

import (
	"context"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Notary notarizes blocks.
type Notary struct {
	sk bls.SecretKey
}

// NewNotary creates a new notary.
func NewNotary(sk bls.SecretKey) *Notary {
	return &Notary{sk: sk}
}

// Notarize returns the notarized blocks of the current round,
// produced by the highest rank block proposer until ctx is cancelled.
//
// ctx will be cancelled when reaching the next round: when a
// notarized block of the current round is received.
func (n *Notary) Notarize(ctx, cancel context.Context, bCh chan *BlockProposal) []*Block {
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

			var blocks []*Block
			for _, bp := range bestRankBPs {
				b := notarize(n.sk, bp)
				blocks = append(blocks, b)
			}
			return blocks
		case bp := <-bCh:
			var rank int
			// TODO: calculate rank
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

func notarize(sk bls.SecretKey, bp *BlockProposal) *Block {
	// TODO: calculate state root
	b := &Block{
		Round:         bp.Round,
		PrevBlock:     bp.PrevBlock,
		BlockProposal: bp.Hash(),
		SysTxns:       bp.SysTxns,
	}

	b.NotarizationSig = sk.Sign(string(b.Encode(false))).Serialize()
	return b
}
