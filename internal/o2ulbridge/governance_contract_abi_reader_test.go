package o2ulbridge

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type rpcRequestEnvelope struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
	ID     interface{}       `json:"id"`
}

func governanceABIFixturePath(name string) string {
	return filepath.Join("testdata", "governance", name)
}

func mustReadABIFixture(t *testing.T, fixtureName string) abi.ABI {
	t.Helper()
	contentPath := governanceABIFixturePath(fixtureName)
	content, readErr := os.ReadFile(contentPath)
	if readErr != nil {
		t.Fatalf("read fixture %q: %v", contentPath, readErr)
	}
	parsedABI, parseErr := parseABIFromContent(content)
	if parseErr != nil {
		t.Fatalf("parse fixture %q: %v", contentPath, parseErr)
	}
	return parsedABI
}

func newGovernanceABIRPCFixtureServer(t *testing.T, hasRoleResult bool, operationReadyResult bool) *httptest.Server {
	t.Helper()
	return newGovernanceABIRPCFixtureServerWithMethods(
		t,
		hasRoleResult,
		operationReadyResult,
		governorAccessControlABI,
		"hasRole",
		timelockControllerABI,
		"isOperationReady",
	)
}

func newGovernanceABIRPCFixtureServerWithMethods(
	t *testing.T,
	hasRoleResult bool,
	operationReadyResult bool,
	governorABI abi.ABI,
	governorMethod string,
	timelockABI abi.ABI,
	timelockMethod string,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req rpcRequestEnvelope
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Method != "eth_call" || len(req.Params) < 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "error": map[string]interface{}{"code": -32601, "message": "unsupported"}})
			return
		}

		var callObj map[string]string
		if err := json.Unmarshal(req.Params[0], &callObj); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "error": map[string]interface{}{"code": -32602, "message": err.Error()}})
			return
		}
		data := strings.ToLower(callObj["data"])

		selectorHasRole := strings.ToLower(hexutil.Encode(governorABI.Methods[governorMethod].ID))
		selectorIsOperationReady := strings.ToLower(hexutil.Encode(timelockABI.Methods[timelockMethod].ID))

		resultHex := "0x"
		switch {
		case strings.HasPrefix(data, selectorHasRole):
			if hasRoleResult {
				resultHex = "0x0000000000000000000000000000000000000000000000000000000000000001"
			} else {
				resultHex = "0x0000000000000000000000000000000000000000000000000000000000000000"
			}
		case strings.HasPrefix(data, selectorIsOperationReady):
			if operationReadyResult {
				resultHex = "0x0000000000000000000000000000000000000000000000000000000000000001"
			} else {
				resultHex = "0x0000000000000000000000000000000000000000000000000000000000000000"
			}
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "error": map[string]interface{}{"code": -32000, "message": "unknown selector"}})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": resultHex})
	}))
}

func TestContractABIGovernanceReaderFixtureIntegration(t *testing.T) {
	serverAllow := newGovernanceABIRPCFixtureServer(t, true, true)
	defer serverAllow.Close()

	readerAllow := &contractABIGovernanceReader{
		rpcEndpoint:     serverAllow.URL,
		timeout:         defaultGovernanceABICallTimeout,
		governorAddress: common.HexToAddress("0x0000000000000000000000000000000000001007"),
		timelockAddress: common.HexToAddress("0x0000000000000000000000000000000000001008"),
		executorRole:    crypto.Keccak256Hash([]byte("EXECUTOR_ROLE")),
		governorABI:     governorAccessControlABI,
		timelockABI:     timelockControllerABI,
		governorMethod:  "hasRole",
		timelockMethod:  "isOperationReady",
		operationIDMode: "auto",
	}
	if !readerAllow.IsAuthorizedGovernorCaller(protocol.Address("0x000000000000000000000000000000000000beef")) {
		t.Fatal("expected caller to be authorized from fixture")
	}
	if !readerAllow.IsProposalExecutable(protocol.ProposalID("proposal-1")) {
		t.Fatal("expected proposal to be executable from fixture")
	}

	serverDeny := newGovernanceABIRPCFixtureServer(t, false, false)
	defer serverDeny.Close()
	readerDeny := &contractABIGovernanceReader{
		rpcEndpoint:     serverDeny.URL,
		timeout:         defaultGovernanceABICallTimeout,
		governorAddress: common.HexToAddress("0x0000000000000000000000000000000000001007"),
		timelockAddress: common.HexToAddress("0x0000000000000000000000000000000000001008"),
		executorRole:    crypto.Keccak256Hash([]byte("EXECUTOR_ROLE")),
		governorABI:     governorAccessControlABI,
		timelockABI:     timelockControllerABI,
		governorMethod:  "hasRole",
		timelockMethod:  "isOperationReady",
		operationIDMode: "auto",
	}
	if readerDeny.IsAuthorizedGovernorCaller(protocol.Address("0x000000000000000000000000000000000000beef")) {
		t.Fatal("expected caller to be denied from fixture")
	}
	if readerDeny.IsProposalExecutable(protocol.ProposalID("proposal-1")) {
		t.Fatal("expected proposal to be non-executable from fixture")
	}
}

func TestParseProposalOperationID(t *testing.T) {
	t.Run("hashes plain proposal id", func(t *testing.T) {
		opID, err := parseProposalOperationID(protocol.ProposalID("proposal-1"))
		if err != nil {
			t.Fatalf("parse proposal id: %v", err)
		}
		expected := crypto.Keccak256Hash([]byte("proposal-1"))
		if opID != expected {
			t.Fatalf("unexpected operation id: got %s want %s", opID.Hex(), expected.Hex())
		}
	})

	t.Run("accepts precomputed bytes32", func(t *testing.T) {
		raw := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		opID, err := parseProposalOperationID(protocol.ProposalID(raw))
		if err != nil {
			t.Fatalf("parse proposal id: %v", err)
		}
		if opID != common.HexToHash(raw) {
			t.Fatalf("unexpected bytes32 operation id: %s", opID.Hex())
		}
	})
}

func TestParseProposalOperationIDWithMode(t *testing.T) {
	rawHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	if _, err := parseProposalOperationIDWithMode(protocol.ProposalID(rawHash), "hex_bytes32"); err != nil {
		t.Fatalf("expected hex_bytes32 mode to accept bytes32 hash, got %v", err)
	}
	if _, err := parseProposalOperationIDWithMode(protocol.ProposalID("proposal-1"), "hex_bytes32"); err == nil {
		t.Fatal("expected hex_bytes32 mode to reject non-hash proposal id")
	}

	hashed, err := parseProposalOperationIDWithMode(protocol.ProposalID("proposal-1"), "keccak_utf8")
	if err != nil {
		t.Fatalf("keccak_utf8 mode: %v", err)
	}
	expected := crypto.Keccak256Hash([]byte("proposal-1"))
	if hashed != expected {
		t.Fatalf("unexpected keccak_utf8 hash: got %s want %s", hashed.Hex(), expected.Hex())
	}
}

func TestParseABIFromEnvOrDefaultAndMethodValidation(t *testing.T) {
	governorABIPath := governanceABIFixturePath("governor_access_control_artifact.json")
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governorABIPath)
	parsed, err := parseABIFromEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governorAccessControlABI)
	if err != nil {
		t.Fatalf("parse abi from env path: %v", err)
	}
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
	method, err := parseABIMethodNameFromEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "hasRole", parsed)
	if err != nil {
		t.Fatalf("parse method name from env: %v", err)
	}
	if method != "isAuthorizedCaller" {
		t.Fatalf("unexpected method selection: %s", method)
	}
}

func TestParseABIFromEnvOrDefaultRawABICompatibility(t *testing.T) {
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_abi.json"))
	parsed, err := parseABIFromEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governorAccessControlABI)
	if err != nil {
		t.Fatalf("parse raw abi fixture from env path: %v", err)
	}
	if _, ok := parsed.Methods["isAuthorizedCaller"]; !ok {
		t.Fatal("expected method isAuthorizedCaller from raw ABI fixture")
	}
}

func TestParseABIFromContentRejectsArtifactWithoutABI(t *testing.T) {
	if _, err := parseABIFromContent([]byte(`{"contractName":"NoABI"}`)); err == nil {
		t.Fatal("expected parse failure when artifact is missing abi field")
	}
}

func TestContractABIGovernanceReaderWithArtifactABIsAndMethods(t *testing.T) {
	governorABI := mustReadABIFixture(t, "governor_access_control_abi.json")
	timelockABI := mustReadABIFixture(t, "timelock_controller_abi.json")

	server := newGovernanceABIRPCFixtureServerWithMethods(
		t,
		true,
		true,
		governorABI,
		"isAuthorizedCaller",
		timelockABI,
		"isReadyOperation",
	)
	defer server.Close()

	reader := &contractABIGovernanceReader{
		rpcEndpoint:     server.URL,
		timeout:         defaultGovernanceABICallTimeout,
		governorAddress: common.HexToAddress("0x0000000000000000000000000000000000001007"),
		timelockAddress: common.HexToAddress("0x0000000000000000000000000000000000001008"),
		executorRole:    crypto.Keccak256Hash([]byte("EXECUTOR_ROLE")),
		governorABI:     governorABI,
		timelockABI:     timelockABI,
		governorMethod:  "isAuthorizedCaller",
		timelockMethod:  "isReadyOperation",
		operationIDMode: "auto",
	}

	if !reader.IsAuthorizedGovernorCaller(protocol.Address("0x000000000000000000000000000000000000beef")) {
		t.Fatal("expected caller to be authorized from artifact ABI fixture")
	}
	if !reader.IsProposalExecutable(protocol.ProposalID("proposal-1")) {
		t.Fatal("expected proposal to be executable from artifact ABI fixture")
	}
}

func TestParseRoleHashEnvOrDefault(t *testing.T) {
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "")
	def := crypto.Keccak256Hash([]byte("EXECUTOR_ROLE"))
	if got := parseRoleHashEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", def); got != def {
		t.Fatalf("expected default hash, got %s", got.Hex())
	}

	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if got := parseRoleHashEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", def); got != common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatalf("expected explicit hash, got %s", got.Hex())
	}

	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "ROLE_ALIAS")
	expected := crypto.Keccak256Hash([]byte("ROLE_ALIAS"))
	if got := parseRoleHashEnvOrDefault("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", def); got != expected {
		t.Fatalf("expected hashed alias role, got %s", got.Hex())
	}
}

func TestNewContractABIGovernanceReaderRejectsInvalidMethodSignatures(t *testing.T) {
	t.Run("governor bad signature", func(t *testing.T) {
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", "http://127.0.0.1:8545")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_bad_signature_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")

		if _, err := newContractABIGovernanceReader(""); err == nil {
			t.Fatal("expected invalid governor method signature error")
		}
	})

	t.Run("timelock bad signature", func(t *testing.T) {
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", "http://127.0.0.1:8545")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_bad_signature_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")

		if _, err := newContractABIGovernanceReader(""); err == nil {
			t.Fatal("expected invalid timelock method signature error")
		}
	})
}
