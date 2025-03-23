// file: /core/genesis_ultrastable.go
// description: Genesis integration for UltraStable token system
// module: Blockchain Core
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson

package core

import (
	"math/big"
	"time"

	proprietary "github.com/AndrewDonelson/o2ul-proprietary"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// SetupUltraStableToken initializes the UltraStable token system in genesis
func SetupUltraStableToken(statedb *state.StateDB, treasuryAddr common.Address) {
	// Get configuration from proprietary module
	propManager := proprietary.NewManager()
	config := propManager.GetStableConfig()

	log.Info("Initializing UltraStable token system in genesis",
		"initialSupply", config.InitialSupply,
		"updateFrequency", config.UpdateFrequency)

	// Convert big.Int to uint256.Int for AddBalance
	initialSupply, overflow := uint256.FromBig(config.InitialSupply)
	if overflow {
		log.Error("UltraStable initial supply overflow",
			"supply", config.InitialSupply)
		return
	}

	// Define a genesis initialization reason constant
	const genesisInitReason = 0 // Use 0 as a special reason for genesis initialization

	// Allocate initial supply to treasury
	statedb.AddBalance(treasuryAddr, initialSupply, genesisInitReason)

	// Set token parameters
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_token_name"),
		common.BytesToHash([]byte("UltraStable")))

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_token_symbol"),
		common.BytesToHash([]byte("USUL")))

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_token_decimals"),
		common.BytesToHash(big.NewInt(18).Bytes()))

	// Set initial supply and parameters
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_initial_supply"),
		common.BytesToHash(config.InitialSupply.Bytes()))

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_current_supply"),
		common.BytesToHash(config.InitialSupply.Bytes()))

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_minimum_supply"),
		common.BytesToHash(big.NewInt(1e18).Bytes())) // Minimum 1.0 token

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_update_frequency"),
		common.BytesToHash(big.NewInt(int64(config.UpdateFrequency)).Bytes()))

	// Set initial values
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_initial_value"),
		common.BytesToHash(big.NewInt(1e18).Bytes())) // Initial 1.0

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_current_value"),
		common.BytesToHash(big.NewInt(1e18).Bytes()))

	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_target_value"),
		common.BytesToHash(big.NewInt(1e18).Bytes()))

	// Initialize update time
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_last_update_time"),
		common.BytesToHash(big.NewInt(time.Now().Unix()).Bytes()))

	// Initialize market volatility (0-100)
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("market_volatility"),
		common.BytesToHash(big.NewInt(25).Bytes())) // Initial 25% volatility

	// Initialize adjustment history
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("adjustment_history_count"),
		common.BytesToHash(big.NewInt(0).Bytes()))

	// Store continental weights from config
	for continent, weight := range config.ContinentalWeights {
		statedb.SetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("continental_weight_"+continent),
			common.BytesToHash(big.NewInt(int64(weight)).Bytes()))
	}

	// Store timeframe weights from config
	for timeframe, weight := range config.TimeframeWeights {
		statedb.SetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("timeframe_weight_"+timeframe),
			common.BytesToHash(big.NewInt(int64(weight)).Bytes()))
	}

	// Store smoothing windows from config
	for timeframe, window := range config.SmoothingWindows {
		statedb.SetState(
			params.UltraStableTokenSystemAddress,
			common.HexToHash("smoothing_window_"+timeframe),
			common.BytesToHash(big.NewInt(int64(window)).Bytes()))
	}

	// Set treasury address
	statedb.SetState(
		params.UltraStableTokenSystemAddress,
		common.HexToHash("treasury_address"),
		common.BytesToHash(treasuryAddr.Bytes()))

	log.Info("Completed UltraStable token initialization in genesis")
}
