// file: /core/genesis/ultrastable_token.go
// description: Native implementation of the UltraStable token
// module: Blockchain Core
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson

package genesis

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	// TODO: These values should be moved to a separate proprietary repo module https://github.com/AndrewDonelson/o2ul-proprietary/genesis

	// Initial supply of UltraStable tokens
	InitialUltraStableSupply = new(big.Int).Mul(big.NewInt(1000000), big.NewInt(1e18))

	// The update frequency in seconds (6 hours)
	UpdateFrequency = uint64(6 * 60 * 60)

	// Continental weights for value calculation
	ContinentalWeights = map[string]uint8{
		"NorthAmerica": 32, // 20,
		"Europe":       16, // 20,
		"Asia":         8,  // 25,
		"Africa":       4,  // 10,
		"SouthAmerica": 2,  // 15,
		"Oceania":      1,  // 10,
	}

	// Timeframe weights for smoothing algorithm
	TimeframeWeights = map[string]uint8{
		"Current": 1,  // 15
		"3Day":    2,  // 15
		"1Week":   4,  // 15
		"1Month":  8,  // 15
		"3Month":  16, // 15
		"6Month":  32, // 15
		"1Year":   64, // 10
	}
)

// SetupUltraStableToken initializes the UltraStable token in the genesis state
func SetupUltraStableToken(statedb *state.StateDB, treasury common.Address) {
	log.Info("Initializing UltraStable token",
		"initialSupply", InitialUltraStableSupply,
		"updateFrequency", UpdateFrequency)

	// Set the system parameters for the UltraStable token
	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_token_name"),
		common.BytesToHash([]byte("UltraStable")))

	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_token_symbol"),
		common.BytesToHash([]byte("USUL")))

	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_token_decimals"),
		common.BytesToHash(big.NewInt(18).Bytes()))

	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_initial_supply"),
		common.BytesToHash(InitialUltraStableSupply.Bytes()))

	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_update_frequency"),
		common.BytesToHash(big.NewInt(int64(UpdateFrequency)).Bytes()))

	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_last_update_timestamp"),
		common.BytesToHash(big.NewInt(time.Now().Unix()).Bytes()))

	// Initialize continental weights
	for continent, weight := range ContinentalWeights {
		statedb.SetState(params.UltraStableTokenSystemAddress,
			common.HexToHash("continental_weight_"+continent),
			common.BytesToHash(big.NewInt(int64(weight)).Bytes()))
	}

	// Initialize timeframe weights
	for timeframe, weight := range TimeframeWeights {
		statedb.SetState(params.UltraStableTokenSystemAddress,
			common.HexToHash("timeframe_weight_"+timeframe),
			common.BytesToHash(big.NewInt(int64(weight)).Bytes()))
	}

	// Set initial exchange rate to 1:1 with a weighted average of continental currencies
	// This is a placeholder, will be updated by oracle data
	statedb.SetState(params.UltraStableTokenSystemAddress,
		common.HexToHash("ultrastable_initial_rate"),
		common.BytesToHash(big.NewInt(1e18).Bytes()))
}
