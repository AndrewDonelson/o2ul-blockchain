// file: /params/addresses.go
// description: System addresses for O2UL blockchain
// module: Blockchain Core Parameters
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson

package params

import (
	"github.com/ethereum/go-ethereum/common"
)

var (
	// O2ULTokenSystemAddress is the official system address for O2UL token operations
	O2ULTokenSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001001")

	// UltraStableTokenSystemAddress is the official system address for UltraStable token operations
	UltraStableTokenSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001002")

	// StakingSystemAddress is the official system address for staking operations
	StakingSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001003")

	// OracleSystemAddress is the official system address for AI oracle operations
	OracleSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001004")

	// SeigniorageSystemAddress is the official system address for seigniorage operations
	SeigniorageSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001005")

	// GovernanceSystemAddress is the official system address for governance operations
	GovernanceSystemAddress = common.HexToAddress("0x0000000000000000000000000000000000001006")
)
