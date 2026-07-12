package o2ulbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	defaultGovernanceABICallTimeout = 2000 * time.Millisecond
)

var (
	governorAccessControlABI = mustParseABI(`[
  {
    "type": "function",
    "name": "hasRole",
    "stateMutability": "view",
    "inputs": [
      {"name": "role", "type": "bytes32"},
      {"name": "account", "type": "address"}
    ],
    "outputs": [{"name": "", "type": "bool"}]
  }
]`)

	timelockControllerABI = mustParseABI(`[
  {
    "type": "function",
    "name": "isOperationReady",
    "stateMutability": "view",
    "inputs": [{"name": "id", "type": "bytes32"}],
    "outputs": [{"name": "", "type": "bool"}]
  }
]`)
)

type contractABIGovernanceReader struct {
	rpcEndpoint     string
	timeout         time.Duration
	governorAddress common.Address
	timelockAddress common.Address
	executorRole    common.Hash
	governorABI     abi.ABI
	timelockABI     abi.ABI
	governorMethod  string
	timelockMethod  string
	operationIDMode string
}

func newContractABIGovernanceReader(nodeDataDir string) (*contractABIGovernanceReader, error) {
	rpcEndpoint := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL"))
	if rpcEndpoint == "" {
		rpcEndpoint = defaultGovernanceRPCEndpoint(nodeDataDir)
	}
	if rpcEndpoint == "" {
		return nil, fmt.Errorf("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL is required when contract_abi policy source is enabled")
	}

	governorAddress, err := parseAddressEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", params.GovernanceGovernorContractAddress)
	if err != nil {
		return nil, err
	}
	timelockAddress, err := parseAddressEnvOrDefault("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", params.GovernanceTimelockContractAddress)
	if err != nil {
		return nil, err
	}

	executorRole := parseRoleHashEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", crypto.Keccak256Hash([]byte("EXECUTOR_ROLE")))

	governorABI, err := parseABIFromEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governorAccessControlABI)
	if err != nil {
		return nil, err
	}
	timelockABI, err := parseABIFromEnvOrDefault("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", timelockControllerABI)
	if err != nil {
		return nil, err
	}

	governorMethod, err := parseABIMethodNameFromEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "hasRole", governorABI)
	if err != nil {
		return nil, err
	}
	if err := validateGovernorMethodSignature(governorMethod, governorABI.Methods[governorMethod]); err != nil {
		return nil, err
	}
	timelockMethod, err := parseABIMethodNameFromEnvOrDefault("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isOperationReady", timelockABI)
	if err != nil {
		return nil, err
	}
	if err := validateTimelockMethodSignature(timelockMethod, timelockABI.Methods[timelockMethod]); err != nil {
		return nil, err
	}

	operationIDMode, err := parseOperationIDModeFromEnv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE")
	if err != nil {
		return nil, err
	}

	timeoutMS, err := parseOptionalIntEnv("O2UL_FEE_SPLIT_GOVERNANCE_ABI_TIMEOUT_MS", int(defaultGovernanceABICallTimeout/time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ABI_TIMEOUT_MS: %w", err)
	}
	if timeoutMS <= 0 {
		return nil, fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ABI_TIMEOUT_MS=%d", timeoutMS)
	}

	return &contractABIGovernanceReader{
		rpcEndpoint:     rpcEndpoint,
		timeout:         time.Duration(timeoutMS) * time.Millisecond,
		governorAddress: governorAddress,
		timelockAddress: timelockAddress,
		executorRole:    executorRole,
		governorABI:     governorABI,
		timelockABI:     timelockABI,
		governorMethod:  governorMethod,
		timelockMethod:  timelockMethod,
		operationIDMode: operationIDMode,
	}, nil
}

func parseABIFromEnvOrDefault(name string, def abi.ABI) (abi.ABI, error) {
	path := strings.TrimSpace(os.Getenv(name))
	if path == "" {
		return def, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("invalid %s=%q: %w", name, path, err)
	}
	parsed, err := parseABIFromContent(content)
	if err != nil {
		return abi.ABI{}, fmt.Errorf("invalid %s=%q: %w", name, path, err)
	}
	return parsed, nil
}

func parseABIFromContent(content []byte) (abi.ABI, error) {
	parsed, err := abi.JSON(bytes.NewReader(content))
	if err == nil {
		return parsed, nil
	}

	var artifact struct {
		ABI json.RawMessage `json:"abi"`
	}
	if unmarshalErr := json.Unmarshal(content, &artifact); unmarshalErr != nil {
		return abi.ABI{}, err
	}
	if len(bytes.TrimSpace(artifact.ABI)) == 0 {
		return abi.ABI{}, err
	}

	parsedFromArtifact, parseErr := abi.JSON(bytes.NewReader(artifact.ABI))
	if parseErr != nil {
		return abi.ABI{}, parseErr
	}
	return parsedFromArtifact, nil
}

func parseABIMethodNameFromEnvOrDefault(name string, def string, contractABI abi.ABI) (string, error) {
	method := strings.TrimSpace(os.Getenv(name))
	if method == "" {
		method = def
	}
	if _, ok := contractABI.Methods[method]; !ok {
		return "", fmt.Errorf("invalid %s=%q: method not found in configured ABI", name, method)
	}
	return method, nil
}

func validateGovernorMethodSignature(methodName string, method abi.Method) error {
	if len(method.Inputs) != 2 || method.Inputs[0].Type.String() != "bytes32" || method.Inputs[1].Type.String() != "address" {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD=%q signature, expected (bytes32,address)->bool", methodName)
	}
	if len(method.Outputs) != 1 || method.Outputs[0].Type.String() != "bool" {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD=%q signature, expected (bytes32,address)->bool", methodName)
	}
	return nil
}

func validateTimelockMethodSignature(methodName string, method abi.Method) error {
	if len(method.Inputs) != 1 || method.Inputs[0].Type.String() != "bytes32" {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD=%q signature, expected (bytes32)->bool", methodName)
	}
	if len(method.Outputs) != 1 || method.Outputs[0].Type.String() != "bool" {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD=%q signature, expected (bytes32)->bool", methodName)
	}
	return nil
}

func parseOperationIDModeFromEnv(name string) (string, error) {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if mode == "" {
		return "auto", nil
	}
	switch mode {
	case "auto", "keccak_utf8", "hex_bytes32":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid %s=%q, expected auto|keccak_utf8|hex_bytes32", name, mode)
	}
}

func parseRoleHashEnvOrDefault(name string, def common.Hash) common.Hash {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	if isHexHash(raw) {
		return common.HexToHash(raw)
	}
	return crypto.Keccak256Hash([]byte(raw))
}

func isHexHash(value string) bool {
	if len(value) != 66 || !(strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X")) {
		return false
	}
	decoded, err := hexutil.Decode(value)
	if err != nil {
		return false
	}
	return len(decoded) == 32
}

func mustParseABI(spec string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(spec))
	if err != nil {
		panic(err)
	}
	return parsed
}

func (r *contractABIGovernanceReader) IsAuthorizedGovernorCaller(caller protocol.Address) bool {
	if r == nil || caller == "" {
		return false
	}
	if !common.IsHexAddress(string(caller)) {
		return false
	}

	ok, err := r.callBoolMethod(r.governorAddress, r.governorABI, r.governorMethod, r.executorRole, common.HexToAddress(string(caller)))
	if err != nil {
		log.Warn("o2ul governance ABI caller authorization failed", "caller", caller, "error", err)
		return false
	}
	return ok
}

func (r *contractABIGovernanceReader) IsProposalExecutable(proposalID protocol.ProposalID) bool {
	if r == nil || proposalID == "" {
		return false
	}
	opID, err := parseProposalOperationIDWithMode(proposalID, r.operationIDMode)
	if err != nil {
		log.Warn("o2ul governance ABI proposal id parse failed", "proposalID", proposalID, "error", err)
		return false
	}

	ok, err := r.callBoolMethod(r.timelockAddress, r.timelockABI, r.timelockMethod, opID)
	if err != nil {
		log.Warn("o2ul governance ABI proposal executability failed", "proposalID", proposalID, "error", err)
		return false
	}
	return ok
}

func parseProposalOperationID(proposalID protocol.ProposalID) (common.Hash, error) {
	return parseProposalOperationIDWithMode(proposalID, "auto")
}

func parseProposalOperationIDWithMode(proposalID protocol.ProposalID, mode string) (common.Hash, error) {
	raw := strings.TrimSpace(string(proposalID))
	if raw == "" {
		return common.Hash{}, fmt.Errorf("proposal id is required")
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		if isHexHash(raw) {
			return common.HexToHash(raw), nil
		}
		return crypto.Keccak256Hash([]byte(raw)), nil
	case "hex_bytes32":
		if !isHexHash(raw) {
			return common.Hash{}, fmt.Errorf("proposal id %q is not a bytes32 hex hash", raw)
		}
		return common.HexToHash(raw), nil
	case "keccak_utf8":
		return crypto.Keccak256Hash([]byte(raw)), nil
	default:
		return common.Hash{}, fmt.Errorf("unsupported operation id mode: %s", mode)
	}
}

func (r *contractABIGovernanceReader) callBoolMethod(contract common.Address, parsedABI abi.ABI, method string, args ...interface{}) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	client, err := rpc.DialContext(ctx, r.rpcEndpoint)
	if err != nil {
		return false, fmt.Errorf("dial governance rpc: %w", err)
	}
	defer client.Close()

	data, err := parsedABI.Pack(method, args...)
	if err != nil {
		return false, fmt.Errorf("pack %s call: %w", method, err)
	}

	call := map[string]string{
		"to":   contract.Hex(),
		"data": hexutil.Encode(data),
	}

	var out hexutil.Bytes
	if err := client.CallContext(ctx, &out, "eth_call", call, "latest"); err != nil {
		return false, fmt.Errorf("eth_call %s: %w", method, err)
	}

	values, err := parsedABI.Unpack(method, out)
	if err != nil {
		return false, fmt.Errorf("unpack %s response: %w", method, err)
	}
	if len(values) != 1 {
		return false, fmt.Errorf("unexpected %s response arity: %d", method, len(values))
	}
	result, ok := values[0].(bool)
	if !ok {
		return false, fmt.Errorf("unexpected %s response type %T", method, values[0])
	}
	return result, nil
}
