package consensus

import (
	"context"
	"sync"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Notary notarizes blocks.
type Notary struct {
	sk   bls.SecretKey
	fork []*notarized
}

// NewNotary creates a new notary.
func NewNotary(sk bls.SecretKey, fork []*notarized) *Notary {
	return &Notary{sk: sk, fork: fork}
}

// Notarize returns the notarized blocks of the current round,
// produced by the highest rank block proposer until ctx is cancelled.
//
// ctx will be cancelled when reaching the next round: when a
// notarized block of the current round is received.
func (n *Notary) Notarize(ctx context.Context, bCh chan *BlockProposal, notarizedCh chan *Block) []*Block {
	// TODO: validate BlockProposal, perhaps should be done by the
	// data layer.
	var bestRankBPs []*BlockProposal
	var bestRank int
	for {
		select {
		case <-ctx.Done():
			var mu sync.Mutex
			var blocks []*Block
			var wg sync.WaitGroup
			for _, bp := range bestRankBPs {
				bp := bp
				wg.Add(1)
				go func() {
					b := notarize(n.sk, bp)
					mu.Lock()
					blocks = append(blocks, b)
					mu.Unlock()
				}()
			}
			wg.Wait()
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
	// TODO
	return nil
}
