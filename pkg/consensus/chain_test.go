package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type myUpdater struct {
}

func (m *myUpdater) Update(State) {
}

type myState struct {
}

func (s *myState) Hash() Hash {
	return Hash{}
}

func (s *myState) CommitCache() {
}

func (s *myState) Transition(round uint64) Transition {
	return nil
}

func (s *myState) Serialize() (TrieBlob, error) {
	return TrieBlob{}, nil
}

func (s *myState) Deserialize(TrieBlob) error {
	return nil
}

func TestGraphviz(t *testing.T) {
	chain := NewChain(&Block{}, &myState{}, Rand{}, Config{}, nil, &myUpdater{}, newStorage())
	chain.finalized = append(chain.finalized, Hash{1})
	chain.finalized = append(chain.finalized, Hash{2})
	chain.finalized = append(chain.finalized, Hash{3})
	chain.finalized = append(chain.finalized, Hash{4})
	fork0 := &blockNode{Block: Hash{7}}
	fork01 := &blockNode{Block: Hash{8}}
	fork02 := &blockNode{Block: Hash{9}}
	fork0.blockChildren = []*blockNode{fork01, fork02}
	fork1 := &blockNode{Block: Hash{12}}
	fork11 := &blockNode{Block: Hash{13}}
	fork1.blockChildren = []*blockNode{fork11}

	chain.fork = append(chain.fork, fork0)
	chain.fork = append(chain.fork, fork1)
	assert.Equal(t, `digraph chain {
rankdir=LR;
size="12,8"
node [shape = rect, style=filled, color = chartreuse2]; block_26df block_0100 block_0200 block_0300 block_0400
node [shape = rect, style=filled, color = aquamarine]; block_0700 block_0800 block_0900 block_0c00 block_0d00
block_26df -> block_0100 -> block_0200 -> block_0300 -> block_0400
block_0400 -> block_0700
block_0700 -> block_0800
block_0700 -> block_0900
block_0400 -> block_0c00
block_0c00 -> block_0d00

}
`, chain.Graphviz(0))
}

func TestForkTraversal(t *testing.T) {
	fork := make([]*blockNode, 2)
	fork[0] = &blockNode{}
	fork[1] = &blockNode{}
	assert.Equal(t, 2, forkWidth(fork, 0))
	fork[0].blockChildren = make([]*blockNode, 1)
	fork[0].blockChildren[0] = &blockNode{}
	fork[1].blockChildren = make([]*blockNode, 2)
	fork[1].blockChildren[0] = &blockNode{}
	fork[1].blockChildren[1] = &blockNode{}
	assert.Equal(t, 3, forkWidth(fork, 1))
	fork[0].blockChildren[0].blockChildren = make([]*blockNode, 2)
	fork[0].blockChildren[0].blockChildren[0] = &blockNode{}
	fork[0].blockChildren[0].blockChildren[1] = &blockNode{}
	assert.Equal(t, 2, forkWidth(fork, 2))

	n := &blockNode{Weight: 1}
	fork[0].blockChildren[0].blockChildren[0].blockChildren = []*blockNode{n}
	assert.Equal(t, n, nodeAtDepthInFork(fork, 3))
}

func TestHeaviestFork(t *testing.T) {
	fork := make([]*blockNode, 2)
	fork[0] = &blockNode{Weight: 1}
	fork[1] = &blockNode{Weight: 2}
	fork[0].blockChildren = make([]*blockNode, 1)
	fork[0].blockChildren[0] = &blockNode{Weight: 3}
	fork[1].blockChildren = make([]*blockNode, 2)
	fork[1].blockChildren[0] = &blockNode{Weight: 4}
	fork[1].blockChildren[1] = &blockNode{Weight: 5}
	fork[0].blockChildren[0].blockChildren = make([]*blockNode, 2)
	fork[0].blockChildren[0].blockChildren[0] = &blockNode{Weight: 6}
	fork[0].blockChildren[0].blockChildren[1] = &blockNode{Weight: 7}
	n0 := &blockNode{Weight: 8}
	n1 := &blockNode{Weight: 9}
	fork[0].blockChildren[0].blockChildren[0].blockChildren = []*blockNode{n0, n1}
	assert.Equal(t, 2, forkWidth(fork, 3))
	r := heaviestFork(fork, 0)
	assert.Equal(t, float64(2), r.Weight)
	r = heaviestFork(fork, 1)
	assert.Equal(t, float64(5), r.Weight)
	r = heaviestFork(fork, 2)
	assert.Equal(t, float64(7), r.Weight)
	r = heaviestFork(fork, 3)
	assert.Equal(t, float64(9), r.Weight)
	assert.Equal(t, n1, r)
	assert.Equal(t, 4, maxHeight(fork))
}
