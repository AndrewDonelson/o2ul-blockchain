// file: /core/genesis/staking.go
// description: Implementation of the O2UL token staking system
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
)

var (
	// StakingRewardPercentage is the percentage of transaction fees distributed to stakers (0.25%)
	StakingRewardPercentage = big.NewInt(25) // 0.25% = 25 basis points

	// MinimumStakingPeriod is the minimum number of blocks a token must be staked (1 week)
	MinimumStakingPeriod = big.NewInt(40320) // ~1 week with 15s blocks

	// StakingUnlockPeriod is the number of blocks required to unlock staked tokens (1 day)
	StakingUnlockPeriod = big.NewInt(5760) // ~1 day with 15s blocks
)

// SetupStakingSystem initializes the staking system in the genesis state
func SetupStakingSystem(statedb *state.StateDB) {
	log.Info("Initializing O2UL staking system",
		"rewardPercentage", StakingRewardPercentage,
		"minimumStakingPeriod", MinimumStakingPeriod,
		"unlockPeriod", StakingUnlockPeriod)

	// Initialize staking parameters
	statedb.SetState(params.StakingSystemAddress,
		common.HexToHash("staking_reward_percentage"),
		common.BytesToHash(StakingRewardPercentage.Bytes()))

	statedb.SetState(params.StakingSystemAddress,
		common.HexToHash("minimum_staking_period"),
		common.BytesToHash(MinimumStakingPeriod.Bytes()))

	statedb.SetState(params.StakingSystemAddress,
		common.HexToHash("staking_unlock_period"),
		common.BytesToHash(StakingUnlockPeriod.Bytes()))

	// Initialize total staked amount to zero
	statedb.SetState(params.StakingSystemAddress,
		common.HexToHash("total_staked_amount"),
		common.BytesToHash(big.NewInt(0).Bytes()))

	// Initialize last reward distribution block
	statedb.SetState(params.StakingSystemAddress,
		common.HexToHash("last_reward_block"),
		common.BytesToHash(big.NewInt(0).Bytes()))
}
