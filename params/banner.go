// file: /github.com/AndrewDonelson/o2ul-blockchain/params/banner.go
// description: Custom banner and chain description for the O²UL blockchain
// module: Client
// License: MIT
// Author: Andrew Donelson
// Copyright 2025 Andrew Donelson

package params

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/log"
)

// O2ULBannerColor represents different ANSI colors for terminal output
type O2ULBannerColor string

const (
	Reset     O2ULBannerColor = "\033[0m"
	Bold      O2ULBannerColor = "\033[1m"
	Dim       O2ULBannerColor = "\033[2m"
	Underline O2ULBannerColor = "\033[4m"
	Blink     O2ULBannerColor = "\033[5m"
	Black     O2ULBannerColor = "\033[30m"
	Red       O2ULBannerColor = "\033[31m"
	Green     O2ULBannerColor = "\033[32m"
	Yellow    O2ULBannerColor = "\033[33m"
	Blue      O2ULBannerColor = "\033[34m"
	Magenta   O2ULBannerColor = "\033[35m"
	Cyan      O2ULBannerColor = "\033[36m"
	White     O2ULBannerColor = "\033[37m"
	BgBlack   O2ULBannerColor = "\033[40m"
	BgRed     O2ULBannerColor = "\033[41m"
	BgGreen   O2ULBannerColor = "\033[42m"
	BgYellow  O2ULBannerColor = "\033[43m"
	BgBlue    O2ULBannerColor = "\033[44m"
	BgMagenta O2ULBannerColor = "\033[45m"
	BgCyan    O2ULBannerColor = "\033[46m"
	BgWhite   O2ULBannerColor = "\033[47m"
)

func LogO2ULBanner() {
	log.Info(" ╔═════════════════════════════════════════════════════╗")
	log.Info(" ║                                                     ║")
	log.Info(" ║          █████╗  ██████╗  ██╗   ██╗██╗              ║")
	log.Info(" ║         ██╔═══██╗╚════██╗ ██║   ██║██║              ║")
	log.Info(" ║         ██║   ██║ █████╔╝ ██║   ██║██║              ║")
	log.Info(" ║         ██║   ██║██╔═══╝  ██║   ██║██║              ║")
	log.Info(" ║         ╚██████╔╝███████╗ ╚██████╔╝███████╗         ║")
	log.Info(" ║          ╚═════╝ ╚══════╝  ╚═════╝ ╚══════╝         ║")
	log.Info(" ║                                                     ║")
	log.Info(" ╚═════════════════════════════════════════════════════╝")
	log.Info("")
	log.Info("               ORBIS OMNIRA UNITAS LEX")
	log.Info("               THE UNIVERSAL CURRENCY")
}

func LogO2ULDescription(c *ChainConfig) {
	var networkType string

	switch c.ChainID.String() {
	case "20213":
		networkType = "mainnet"
	case "20214":
		networkType = "testnet"
	case "20215":
		networkType = "devnet"
	case "20216":
		networkType = "stagenet"
	default:
		networkType = "unknown"
	}

	log.Info(strings.Repeat("─", 73))
	log.Info("╔═════════════════════════════════════════════════════════════════════════╗")
	log.Info("║ Blockchain Information                                                  ║")
	log.Info("╠═════════════════════════════════════════════════════════════════════════╣")
	log.Info(fmt.Sprintf("║ Chain ID:     %-56d ║", c.ChainID.Int64()))
	log.Info(fmt.Sprintf("║ Network:      %-56s ║", networkType))
	log.Info(fmt.Sprintf("║ Consensus:    %-56s ║", "Proof-of-Stake with Continental AI Oracle"))
	log.Info(fmt.Sprintf("║ Version:      %-56s ║", "v1.0.0"))
	log.Info("╠═════════════════════════════════════════════════════════════════════════╣")
	log.Info("║ Dual-Token System                                                      ║")
	log.Info("║   • Value Token         - Max Supply: 21 million                       ║")
	log.Info("║   • Ultra-Stable Token  - AI-powered continental fiat assessment       ║")
	log.Info("║                         - 5.58x volatility reduction                   ║")
	log.Info("╠═════════════════════════════════════════════════════════════════════════╣")
	log.Info("║ Stability Mechanism                                                    ║")
	log.Info("║   • Continental data analysis from AI oracles                          ║")
	log.Info("║   • 6-hour update frequency with smoothing algorithm                   ║")
	log.Info("║   • Time-weighted averaging across multiple regions                    ║")
	log.Info("╠═════════════════════════════════════════════════════════════════════════╣")
	log.Info("║ Fee Structure                                                          ║")
	log.Info("║   • 0.5% flat transaction fee                                          ║")
	log.Info("║   • 50% to staked Value Token holders, 50% to protocol treasury        ║")
	log.Info("║   • Minimum fee: $0.01 (minimum transaction size: $2.00)               ║")
	log.Info("╚═════════════════════════════════════════════════════════════════════════╝")
	log.Info(strings.Repeat("─", 73))
}

// O2ULDescription returns a detailed description of the blockchain
func O2ULDescription(c *ChainConfig) string {
	var networkType string

	switch c.ChainID.String() {
	case "20213":
		networkType = "mainnet"
	case "20214":
		networkType = "testnet"
	case "20215":
		networkType = "devnet"
	case "20216":
		networkType = "stagenet"
	default:
		networkType = "unknown"
	}

	description := fmt.Sprintf(`%s
╔═════════════════════════════════════════════════════════════════════════╗
║ %sBlockchain Information%s                                                ║
╠═════════════════════════════════════════════════════════════════════════╣
║ %sChain ID:%s     %-56d ║
║ %sNetwork:%s      %-56s ║
║ %sConsensus:%s    %-56s ║
║ %sVersion:%s      %-56s ║
╠═════════════════════════════════════════════════════════════════════════╣
║ %sDual-Token System%s                                                    ║
║   • Value Token         - Max Supply: 21 million                       ║
║   • Ultra-Stable Token  - AI-powered continental fiat assessment       ║
║                         - 5.58x volatility reduction                   ║
╠═════════════════════════════════════════════════════════════════════════╣
║ %sStability Mechanism%s                                                  ║
║   • Continental data analysis from AI oracles                          ║
║   • 6-hour update frequency with smoothing algorithm                   ║
║   • Time-weighted averaging across multiple regions                    ║
╠═════════════════════════════════════════════════════════════════════════╣
║ %sFee Structure%s                                                        ║
║   • 0.5%% flat transaction fee                                          ║
║   • 50%% to staked Value Token holders, 50%% to protocol treasury       ║
║   • Minimum fee: $0.01 (minimum transaction size: $2.00)               ║
╚═════════════════════════════════════════════════════════════════════════╝
%s`,
		strings.Repeat("─", 73),
		Bold, Reset,
		Bold, Reset, c.ChainID.Int64(),
		Bold, Reset, networkType,
		Bold, Reset, "Proof-of-Stake with Continental AI Oracle",
		Bold, Reset, "v1.0.0",
		Bold, Reset,
		Bold, Reset,
		Bold, Reset,
		strings.Repeat("─", 73))

	return description
}

// OverrideChainConfigDescription overrides the original Description method
// func OverrideChainConfigDescription(c *ChainConfig) string {
// 	return O2ULBanner() + O2ULDescription(c)
// }
