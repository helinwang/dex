package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type myUpdater struct {
}

func (m *myUpdater) Update(State) {
}

func TestGraphviz(t *testing.T) {
	chain := NewChain(&Block{}, nil, Rand{}, Config{}, nil, &myUpdater{})
	chain.Finalized = append(chain.Finalized, Hash{1})
	chain.Finalized = append(chain.Finalized, Hash{2})
	chain.Finalized = append(chain.Finalized, Hash{3})
	chain.Finalized = append(chain.Finalized, Hash{4})
	chain.UnNotarizedNotOnFork = append(chain.UnNotarizedNotOnFork, &unNotarized{BP: Hash{5}})
	chain.UnNotarizedNotOnFork = append(chain.UnNotarizedNotOnFork, &unNotarized{BP: Hash{6}})
	fork0 := &notarized{Block: Hash{7}}
	fork01 := &notarized{Block: Hash{8}}
	fork02 := &notarized{Block: Hash{9}}
	fork02.NonNtChildren = []*unNotarized{&unNotarized{BP: Hash{10}}, &unNotarized{BP: Hash{11}}}
	fork0.NtChildren = []*notarized{fork01, fork02}
	fork1 := &notarized{Block: Hash{12}}
	fork11 := &notarized{Block: Hash{13}}
	fork1.NtChildren = []*notarized{fork11}

	chain.Fork = append(chain.Fork, fork0)
	chain.Fork = append(chain.Fork, fork1)
	assert.Equal(t, `digraph chain {
rankdir=LR;
size="12,8"
node [shape = rect, style=filled, color = chartreuse2]; block_3ba7 block_0100 block_0200 block_0300 block_0400
node [shape = rect, style=filled, color = aquamarine]; block_0700 block_0800 block_0900 block_0c00 block_0d00
node [shape = octagon, style=filled, color = aliceblue]; proposal_0500 proposal_0600 proposal_0a00 proposal_0b00
block_3ba7 -> block_0100 -> block_0200 -> block_0300 -> block_0400
block_0400 -> proposal_0500
block_0400 -> proposal_0600
block_0400 -> block_0700
block_0700 -> block_0800
block_0700 -> block_0900
block_0900 -> proposal_0a00
block_0900 -> proposal_0b00
block_0400 -> block_0c00
block_0c00 -> block_0d00

}
`, chain.Graphviz(0))
}
