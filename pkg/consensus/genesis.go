package consensus

// Genesis is the genesis block.
var Genesis = Block{
	StateRoot: Hash{1, 2, 3},
}

// GenesisState is the state at genesis
var GenesisState State
