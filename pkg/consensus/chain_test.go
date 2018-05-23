package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGraphviz(t *testing.T) {
	chain := NewChain(&Block{}, nil, Rand{}, Config{})
	chain.History = append(chain.History, Hash{1})
	chain.History = append(chain.History, Hash{2})
	chain.Finalized = append(chain.Finalized, &finalized{Block: Hash{3}})
	chain.Finalized = append(chain.Finalized, &finalized{Block: Hash{4}})
	chain.UnNotarizedNotOnFork = append(chain.UnNotarizedNotOnFork, &unNotarized{BP: Hash{5}})
	chain.UnNotarizedNotOnFork = append(chain.UnNotarizedNotOnFork, &unNotarized{BP: Hash{6}})
	fork0 := &notarized{Block: Hash{7}}
	fork01 := &notarized{Block: Hash{8}}
	fork02 := &notarized{Block: Hash{12}}
	fork02.NonNtChildren = []*unNotarized{&unNotarized{BP: Hash{13}}, &unNotarized{BP: Hash{14}}}
	fork0.NtChildren = []*notarized{fork01, fork02}
	fork1 := &notarized{Block: Hash{9}}
	fork11 := &notarized{Block: Hash{10}}
	fork1.NtChildren = []*notarized{fork11}

	chain.Fork = append(chain.Fork, fork0)
	chain.Fork = append(chain.Fork, fork1)
	assert.Equal(t, `digraph chain {
rankdir=LR;
size="8,5"
node [shape = rect, style=filled, color = forestgreen]; block_53b9 block_0100 block_0200
node [shape = rect, style=filled, color = chartreuse2]; block_0300 block_0400
node [shape = rect, style=filled, color = aquamarine]; block_0700 block_0800 block_0c00 block_0900 block_0a00
node [shape = octagon, style=filled, color = aliceblue]; proposal_0500 proposal_0600 proposal_0d00 proposal_0e00
block_53b9 -> block_0100 -> block_0200 -> block_0300 -> block_0400
block_0400 -> proposal_0500
block_0400 -> proposal_0600
block_0400 -> block_0700
block_0700 -> block_0800
block_0700 -> block_0c00
block_0c00 -> proposal_0d00
block_0c00 -> proposal_0e00
block_0400 -> block_0900
block_0900 -> block_0a00

}
`, chain.Graphviz())
}
