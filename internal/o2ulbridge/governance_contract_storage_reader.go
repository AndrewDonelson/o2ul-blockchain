package o2ulbridge

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	defaultGovernanceStorageReadTimeout = 2000 * time.Millisecond
)

type contractStorageGovernanceReader struct {
	rpcEndpoint           string
	timeout               time.Duration
	governorAddress       common.Address
	timelockAddress       common.Address
	governorCallerMapSlot uint64
	timelockExecMapSlot   uint64
}

func newContractStorageGovernanceReader(nodeDataDir string) (*contractStorageGovernanceReader, error) {
	rpcEndpoint := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL"))
	if rpcEndpoint == "" {
		rpcEndpoint = defaultGovernanceRPCEndpoint(nodeDataDir)
	}
	if rpcEndpoint == "" {
		return nil, fmt.Errorf("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL is required when contract_storage policy source is enabled")
	}

	governorAddress, err := parseAddressEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", params.GovernanceSystemAddress)
	if err != nil {
		return nil, err
	}
	timelockAddress, err := parseAddressEnvOrDefault("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", params.GovernanceSystemAddress)
	if err != nil {
		return nil, err
	}

	governorCallerSlot, err := parseStorageSlotEnv("O2UL_FEE_SPLIT_GOVERNOR_CALLER_MAPPING_SLOT")
	if err != nil {
		return nil, err
	}
	timelockExecSlot, err := parseStorageSlotEnv("O2UL_FEE_SPLIT_TIMELOCK_EXECUTABLE_MAPPING_SLOT")
	if err != nil {
		return nil, err
	}

	timeoutMS, err := parseOptionalIntEnv("O2UL_FEE_SPLIT_GOVERNANCE_STORAGE_TIMEOUT_MS", int(defaultGovernanceStorageReadTimeout/time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_STORAGE_TIMEOUT_MS: %w", err)
	}
	if timeoutMS <= 0 {
		return nil, fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_STORAGE_TIMEOUT_MS=%d", timeoutMS)
	}

	return &contractStorageGovernanceReader{
		rpcEndpoint:           rpcEndpoint,
		timeout:               time.Duration(timeoutMS) * time.Millisecond,
		governorAddress:       governorAddress,
		timelockAddress:       timelockAddress,
		governorCallerMapSlot: governorCallerSlot,
		timelockExecMapSlot:   timelockExecSlot,
	}, nil
}

func defaultGovernanceRPCEndpoint(nodeDataDir string) string {
	resolved := strings.TrimSpace(nodeDataDir)
	if resolved == "" {
		resolved = strings.TrimSpace(os.Getenv("O2UL_NODE_DATA_DIR"))
	}
	if resolved == "" {
		return ""
	}
	return filepath.Join(resolved, "geth.ipc")
}

func parseAddressEnvOrDefault(name string, def common.Address) (common.Address, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def, nil
	}
	if !common.IsHexAddress(raw) {
		return common.Address{}, fmt.Errorf("invalid %s=%q", name, raw)
	}
	return common.HexToAddress(raw), nil
}

func parseStorageSlotEnv(name string) (uint64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, fmt.Errorf("%s is required when contract_storage policy source is enabled", name)
	}
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		value, err := strconv.ParseUint(raw[2:], 16, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s=%q", name, raw)
		}
		return value, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q", name, raw)
	}
	return value, nil
}

func (r *contractStorageGovernanceReader) IsAuthorizedGovernorCaller(caller protocol.Address) bool {
	if r == nil || caller == "" {
		return false
	}
	callerAddress := common.HexToAddress(string(caller))
	location := mappingStorageSlotForAddress(callerAddress, r.governorCallerMapSlot)
	return r.readBooleanStorageSlot(r.governorAddress, location)
}

func (r *contractStorageGovernanceReader) IsProposalExecutable(proposalID protocol.ProposalID) bool {
	if r == nil || proposalID == "" {
		return false
	}
	proposalKey := crypto.Keccak256Hash([]byte(proposalID))
	location := mappingStorageSlotForHash(proposalKey, r.timelockExecMapSlot)
	return r.readBooleanStorageSlot(r.timelockAddress, location)
}

func (r *contractStorageGovernanceReader) readBooleanStorageSlot(contract common.Address, slot common.Hash) bool {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	client, err := rpc.DialContext(ctx, r.rpcEndpoint)
	if err != nil {
		log.Warn("o2ul governance storage reader dial failed", "endpoint", r.rpcEndpoint, "error", err)
		return false
	}
	defer client.Close()

	var out hexutil.Bytes
	if err := client.CallContext(ctx, &out, "eth_getStorageAt", contract.Hex(), slot.Hex(), "latest"); err != nil {
		log.Warn("o2ul governance storage reader call failed", "contract", contract.Hex(), "slot", slot.Hex(), "error", err)
		return false
	}

	for _, b := range out {
		if b != 0 {
			return true
		}
	}
	return false
}

func mappingStorageSlotForAddress(address common.Address, mappingSlot uint64) common.Hash {
	var key [32]byte
	copy(key[12:], address.Bytes())
	return mappingStorageSlot(key[:], mappingSlot)
}

func mappingStorageSlotForHash(hash common.Hash, mappingSlot uint64) common.Hash {
	return mappingStorageSlot(hash.Bytes(), mappingSlot)
}

func mappingStorageSlot(key []byte, mappingSlot uint64) common.Hash {
	var slot [32]byte
	new(big.Int).SetUint64(mappingSlot).FillBytes(slot[:])

	data := make([]byte, 0, 64)
	data = append(data, key...)
	data = append(data, slot[:]...)
	return crypto.Keccak256Hash(data)
}
