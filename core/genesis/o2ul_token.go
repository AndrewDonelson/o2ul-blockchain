// file: /core/genesis/o2ul_token.go
// description: Native implementation of the O2UL value token
// module: Blockchain Core
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson

package genesis

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

var (
	// MaxSupply represents the maximum supply of O2UL tokens (21 million)
	MaxSupply = new(big.Int).Mul(big.NewInt(21000000), big.NewInt(1e18))

	// FounderAllocation represents 60% of the total supply
	FounderAllocation = new(big.Int).Mul(big.NewInt(12600000), big.NewInt(1e18))

	// ReserveAllocation represents 40% of the total supply
	ReserveAllocation = new(big.Int).Mul(big.NewInt(8400000), big.NewInt(1e18))
)

// SetupO2ULToken initializes the O2UL token allocation in the genesis state
func SetupO2ULToken(statedb *state.StateDB, founder common.Address, reserve common.Address) {
	log.Info("Initializing O2UL token supply", "maxSupply", MaxSupply,
		"founderAllocation", FounderAllocation, "reserveAllocation", ReserveAllocation)

	// Convert big.Int to uint256.Int for AddBalance
	founderAmount, _ := uint256.FromBig(FounderAllocation)
	reserveAmount, _ := uint256.FromBig(ReserveAllocation)

	// Define a genesis initialization reason constant directly here as a workaround
	const genesisInitReason = 0 // Use 0 as a special reason for genesis initialization

	// Allocate tokens to founder (60%)
	statedb.AddBalance(founder, founderAmount, genesisInitReason)

	// Allocate tokens to reserve (40%)
	statedb.AddBalance(reserve, reserveAmount, genesisInitReason)

	// Set up special O2UL token parameters in state
	statedb.SetState(params.O2ULTokenSystemAddress,
		common.HexToHash("o2ul_max_supply"),
		common.BytesToHash(MaxSupply.Bytes()))

	// Additional O2UL token parameters
	statedb.SetState(params.O2ULTokenSystemAddress,
		common.HexToHash("o2ul_token_name"),
		common.BytesToHash([]byte("Orbis Omnira Unitas Lex")))

	statedb.SetState(params.O2ULTokenSystemAddress,
		common.HexToHash("o2ul_token_symbol"),
		common.BytesToHash([]byte("O2UL")))

	statedb.SetState(params.O2ULTokenSystemAddress,
		common.HexToHash("o2ul_token_decimals"),
		common.BytesToHash(big.NewInt(18).Bytes()))

	// Get balance from state
	founderBalance := statedb.GetBalance(founder).ToBig()
	reserveBalance := statedb.GetBalance(reserve).ToBig()

	// Log balances
	log.Info("Completed O2UL token allocation",
		"founderAddress", founder,
		"founderBalance", founderBalance,
		"reserveAddress", reserve,
		"reserveBalance", reserveBalance)
}
