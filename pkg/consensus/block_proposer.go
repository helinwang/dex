package consensus

import (
	"context"

	log "github.com/helinwang/log15"

	"github.com/dfinity/go-dfinity-crypto/bls"
)

// Validator validates the system transaction.
type Validator interface {
	Validate(SysTxn) bool
}

// BlockProposer produces one block proposal if it is in the block
// proposal committee in the current round.
type BlockProposer struct {
	sk       bls.SecretKey
	Block    *Block
	State    State
	SysState *SysState
}

// NewBlockProposer creates a new block proposer.
func NewBlockProposer(sk bls.SecretKey, b *Block, s State, sys *SysState) *BlockProposer {
	return &BlockProposer{sk: sk, Block: b, State: s, SysState: sys}
}

// CollectTxn collects transactions and returns a block proposal when
// the context is done.
func (b *BlockProposer) CollectTxn(ctx context.Context, txCh chan []byte, sysTxCh chan SysTxn, pendingTx chan []byte) *BlockProposal {
	var bp BlockProposal
	bp.PrevBlock = SHA3(b.Block.Encode(true))
	bp.Round = b.Block.Round + 1
	bp.Owner = SHA3(b.sk.GetPublicKey().Serialize()).Addr()
	transition := b.State.Transition()
	sysTransition := b.SysState.Transition()
	for {
		select {
		case <-ctx.Done():
			bp.SysTxns = sysTransition.Txns()
			data := transition.Clear()
			bp.Data = gobEncode(data)
			bp.OwnerSig = b.sk.Sign(string(bp.Encode(false))).Serialize()
			close(pendingTx)
			return &bp
		case tx := <-txCh:
			valid, success := transition.Record(tx)
			if !valid {
				log.Warn("received invalid txn", "len", len(tx))
				continue
			}

			if !success {
				// try in the future
				pendingTx <- tx
			}
		case sysTx := <-sysTxCh:
			if !sysTransition.Record(sysTx) {
				log.Warn("received invalid sys txn")
				continue
			}
		}
	}
}
