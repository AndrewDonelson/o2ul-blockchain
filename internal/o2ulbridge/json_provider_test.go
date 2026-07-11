package o2ulbridge

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/nft"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/proofs"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/shielded"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/threshold"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/viewkeys"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/metrics"
)

type captureExternalProviderObserver struct {
	events []proofs.ExternalProviderCallEvent
}

func (c *captureExternalProviderObserver) ObserveExternalProviderCall(event proofs.ExternalProviderCallEvent) {
	c.events = append(c.events, event)
}

func externalProviderMetricCount(name string) int64 {
	m := metrics.Get(name)
	if m == nil {
		return 0
	}
	c, ok := m.(*metrics.Counter)
	if !ok {
		return 0
	}
	return c.Snapshot().Count()
}

func newTestRuntimeBridge(t *testing.T) *pblockchain.RuntimeBridge {
	t.Helper()
	bridge, err := pblockchain.NewRuntimeBridge(pblockchain.RuntimeBridgeDeps{
		Proofs:       proofs.NewHashProofSystem(0),
		Shielded:     shielded.NewInMemoryPool(),
		NFT:          nft.NewInMemoryRegistry(),
		NFTOwnership: nft.NewHashOwnershipVerifier(),
		Threshold:    threshold.NewSimpleSigner(),
		ViewKeys:     viewkeys.NewSimpleManager(),
	})
	if err != nil {
		t.Fatalf("new runtime bridge: %v", err)
	}
	return bridge
}

func TestJSONRuntimeProviderRequiresBridge(t *testing.T) {
	p := NewJSONRuntimeHookProvider(nil)
	_, err := p.VerifyProofHook([]byte(`{}`))
	if !errors.Is(err, ErrRuntimeBridgeNotSet) {
		t.Fatalf("expected ErrRuntimeBridgeNotSet, got %v", err)
	}
}

func TestJSONRuntimeProviderRoundTripAndVMPrecompileRouting(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newTestRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	proof, err := proofs.NewHashProofSystem(0).Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	reqBytes, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	out, err := pc.Run(reqBytes)
	if err != nil {
		t.Fatalf("run precompile: %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification response")
	}
}

func TestJSONRuntimeProviderSupportsShieldedCreateAndReplayCheck(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newTestRuntimeBridge(t))

	ownerKey := protocol.PrivateKey("owner-secret")
	owner := shielded.OwnerFromSpendKey(ownerKey)
	createReq, err := json.Marshal(pblockchain.ShieldedCreateRequest{
		Owner:     owner,
		Value:     protocol.Amount{Value: big.NewInt(10)},
		AssetType: protocol.AssetTypeFungible,
	})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}
	out, err := provider.CreateShieldedNoteHook(createReq)
	if err != nil {
		t.Fatalf("create shielded note hook: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected create shielded response payload")
	}

	vkReq, err := json.Marshal(pblockchain.ViewKeyReplayCheckRequest{
		Disclosure: protocol.EncryptedDisclosure("d"),
		ViewKey:    protocol.ViewKey("vk"),
		Recipient:  protocol.PublicKey("recipient"),
		Nonce:      []byte("nonce"),
	})
	if err != nil {
		t.Fatalf("marshal replay request: %v", err)
	}
	out, err = provider.IsDisclosureReplayHook(vkReq)
	if err != nil {
		t.Fatalf("is disclosure replay hook: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected replay check response payload")
	}
}

func TestInstallDefaultRuntimeBridgeSetsVMProvider(t *testing.T) {
	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallDefaultRuntimeBridge(); err != nil {
		t.Fatalf("install default runtime bridge: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	reqBytes, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        protocol.Proof("p"),
		PublicInputs: protocol.PublicInputs("i"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	_, err = pc.Run(reqBytes)
	if errors.Is(err, vm.ErrO2ULRuntimeProviderNotSet) {
		t.Fatal("expected runtime provider to be installed")
	}
}

func TestRuntimeBackendConfigFromEnvAndInstall(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")

	cfg, err := RuntimeBackendConfigFromEnv()
	if err != nil {
		t.Fatalf("runtime backend config from env: %v", err)
	}
	if cfg.Proofs != BackendModeProduction || cfg.Threshold != BackendModeProduction {
		t.Fatalf("unexpected parsed modes: %+v", cfg)
	}

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	production := proofs.NewHashProductionBackend(0)
	proof, err := production.Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("production prove: %v", err)
	}
	req, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	out, err := pc.Run(req)
	if errors.Is(err, vm.ErrO2ULRuntimeProviderNotSet) {
		t.Fatal("expected runtime provider to be installed from env")
	}
	if err != nil {
		t.Fatalf("run proof precompile: %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification with production mode")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsProofsProductionRegistryPath(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	backend, err := proofs.NewRegistryProductionBackend(map[protocol.CircuitID][]byte{protocol.CircuitID("proof"): []byte("key-1")}, 0)
	if err != nil {
		t.Fatalf("new registry backend: %v", err)
	}
	proof, err := backend.Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	req, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	out, err := pc.Run(req)
	if err != nil {
		t.Fatalf("run proof verify precompile: %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification with registry production backend")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsProofsProductionExternalFlavor(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	provider := filepath.Join(t.TempDir(), "provider.sh")
	providerScript := "#!/bin/sh\n" +
		"input=$(cat)\n" +
		"case \"$input\" in\n" +
		"  *\\\"action\\\":\\\"prove\\\"*) echo '{\"ok\":true,\"proofHex\":\"70726f6f66\",\"publicInputsHex\":\"7769746e657373\"}' ;;\n" +
		"  *\\\"action\\\":\\\"verify\\\"*) echo '{\"ok\":true}' ;;\n" +
		"  *) echo '{\"ok\":false,\"error\":\"unknown action\"}' ;;\n" +
		"esac\n"
	if err := os.WriteFile(provider, []byte(providerScript), 0o700); err != nil {
		t.Fatalf("write provider script: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_CMD", provider)

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	records, err := proofs.LoadCircuitKeyRecordsFromJSON(path)
	if err != nil {
		t.Fatalf("load circuit key records: %v", err)
	}
	engine, err := proofs.NewProcessExternalZKEngine(provider)
	if err != nil {
		t.Fatalf("new process external zk engine: %v", err)
	}
	backend, err := proofs.NewExternalZKRegistryBackendWithRecords(records, 0, engine)
	if err != nil {
		t.Fatalf("new external zk backend: %v", err)
	}
	proof, err := backend.Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	req, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	out, err := pc.Run(req)
	if err != nil {
		t.Fatalf("run proof verify precompile: %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification with external production backend")
	}
}

func TestInstallRuntimeBridgeFromEnvWiresExternalProviderObserver(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	provider := filepath.Join(t.TempDir(), "provider.sh")
	providerScript := "#!/bin/sh\n" +
		"input=$(cat)\n" +
		"case \"$input\" in\n" +
		"  *\\\"action\\\":\\\"prove\\\"*) echo '{\"ok\":true,\"proofHex\":\"70726f6f66\",\"publicInputsHex\":\"7769746e657373\"}' ;;\n" +
		"  *\\\"action\\\":\\\"verify\\\"*) echo '{\"ok\":true}' ;;\n" +
		"  *) echo '{\"ok\":false,\"error\":\"unknown action\"}' ;;\n" +
		"esac\n"
	if err := os.WriteFile(provider, []byte(providerScript), 0o700); err != nil {
		t.Fatalf("write provider script: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_CMD", provider)

	capture := &captureExternalProviderObserver{}
	prevFactory := newExternalProviderObserver
	newExternalProviderObserver = func() proofs.ExternalProviderObserver { return capture }
	t.Cleanup(func() { newExternalProviderObserver = prevFactory })

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	records, err := proofs.LoadCircuitKeyRecordsFromJSON(path)
	if err != nil {
		t.Fatalf("load circuit key records: %v", err)
	}
	engine, err := proofs.NewProcessExternalZKEngine(provider)
	if err != nil {
		t.Fatalf("new process external zk engine: %v", err)
	}
	backend, err := proofs.NewExternalZKRegistryBackendWithRecords(records, 0, engine)
	if err != nil {
		t.Fatalf("new external zk backend: %v", err)
	}
	proof, err := backend.Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	req, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	out, err := pc.Run(req)
	if err != nil {
		t.Fatalf("run proof verify precompile: %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification with external production backend")
	}
	if len(capture.events) == 0 {
		t.Fatal("expected observer events from installed external provider")
	}
	last := capture.events[len(capture.events)-1]
	if last.Transport != "process" {
		t.Fatalf("expected process transport event, got %q", last.Transport)
	}
	if last.Action != "verify" {
		t.Fatalf("expected verify action event from precompile path, got %q", last.Action)
	}
	if !last.Success {
		t.Fatal("expected successful verify observer event")
	}
}

func TestInstallRuntimeBridgeFromEnvExportsExternalProviderMetrics(t *testing.T) {
	metrics.Enable()
	metricNames := []string{
		externalProviderMetricsPrefix + "/calls/total",
		externalProviderMetricsPrefix + "/calls/success",
		externalProviderMetricsPrefix + "/calls/process/verify/total",
		externalProviderMetricsPrefix + "/calls/process/verify/success",
		externalProviderMetricsPrefix + "/latency",
	}
	for _, name := range metricNames {
		metrics.Unregister(name)
		t.Cleanup(func() { metrics.Unregister(name) })
	}

	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	provider := filepath.Join(t.TempDir(), "provider.sh")
	providerScript := "#!/bin/sh\n" +
		"input=$(cat)\n" +
		"case \"$input\" in\n" +
		"  *\\\"action\\\":\\\"prove\\\"*) echo '{\"ok\":true,\"proofHex\":\"70726f6f66\",\"publicInputsHex\":\"7769746e657373\"}' ;;\n" +
		"  *\\\"action\\\":\\\"verify\\\"*) echo '{\"ok\":true}' ;;\n" +
		"  *) echo '{\"ok\":false,\"error\":\"unknown action\"}' ;;\n" +
		"esac\n"
	if err := os.WriteFile(provider, []byte(providerScript), 0o700); err != nil {
		t.Fatalf("write provider script: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_CMD", provider)

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	records, err := proofs.LoadCircuitKeyRecordsFromJSON(path)
	if err != nil {
		t.Fatalf("load circuit key records: %v", err)
	}
	engine, err := proofs.NewProcessExternalZKEngine(provider)
	if err != nil {
		t.Fatalf("new process external zk engine: %v", err)
	}
	backend, err := proofs.NewExternalZKRegistryBackendWithRecords(records, 0, engine)
	if err != nil {
		t.Fatalf("new external zk backend: %v", err)
	}
	proof, err := backend.Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	req, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	if _, err := pc.Run(req); err != nil {
		t.Fatalf("run proof verify precompile: %v", err)
	}

	if got := externalProviderMetricCount(externalProviderMetricsPrefix + "/calls/total"); got < 1 {
		t.Fatalf("expected total provider call metric >=1, got %d", got)
	}
	if got := externalProviderMetricCount(externalProviderMetricsPrefix + "/calls/success"); got < 1 {
		t.Fatalf("expected success provider call metric >=1, got %d", got)
	}
	if got := externalProviderMetricCount(externalProviderMetricsPrefix + "/calls/process/verify/total"); got < 1 {
		t.Fatalf("expected process verify total metric >=1, got %d", got)
	}
	if got := externalProviderMetricCount(externalProviderMetricsPrefix + "/calls/process/verify/success"); got < 1 {
		t.Fatalf("expected process verify success metric >=1, got %d", got)
	}
}

func TestInstallRuntimeBridgeFromEnvRejectsExternalFlavorWithoutProviderCommand(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected external provider configuration required error")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsProofsProductionExternalFlavorHTTPProvider(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req["action"] == "prove" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":              true,
				"proofHex":        "70726f6f66",
				"publicInputsHex": "7769746e657373",
			})
			return
		}
		if req["action"] == "verify" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "unknown action"})
	}))
	defer ts.Close()
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_URL", ts.URL)
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_AUTH_BEARER", "token-123")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	records, err := proofs.LoadCircuitKeyRecordsFromJSON(path)
	if err != nil {
		t.Fatalf("load circuit key records: %v", err)
	}
	engine, err := proofs.NewHTTPExternalZKEngineWithConfig(proofs.HTTPExternalZKEngineConfig{
		URL:             ts.URL,
		AuthBearerToken: "token-123",
	})
	if err != nil {
		t.Fatalf("new http external zk engine: %v", err)
	}
	backend, err := proofs.NewExternalZKRegistryBackendWithRecords(records, 0, engine)
	if err != nil {
		t.Fatalf("new external zk backend: %v", err)
	}
	proof, err := backend.Prove(protocol.CircuitID("proof"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	req, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("proof"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof verify precompile")
	}
	out, err := pc.Run(req)
	if err != nil {
		t.Fatalf("run proof verify precompile: %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification with external http provider")
	}
}

func TestInstallRuntimeBridgeFromEnvRejectsInvalidExternalHTTPNumericSettings(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_URL", "http://example.invalid")
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_TIMEOUT_MS", "bad")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected invalid numeric env error")
	}
}

func TestInstallRuntimeBridgeFromEnvRejectsProofsAllRevokedKeys(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")

	path := filepath.Join(t.TempDir(), "proof-circuits-revoked.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":{"key":"6b65792d31","version":1,"revoked":true}}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected revoked proof keys configuration error")
	}
}

func TestRuntimeBackendConfigFromEnvRejectsInvalidMode(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "bad-mode")
	_, err := RuntimeBackendConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsViewKeysProductionPath(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "production")

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env with viewkeys production: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	generateReq, err := json.Marshal(pblockchain.ViewKeyGenerateRequest{
		Note:  protocol.Note{AssetType: protocol.AssetTypeFungible, Payload: []byte("payload")},
		Owner: protocol.PrivateKey("owner-secret"),
	})
	if err != nil {
		t.Fatalf("marshal viewkey generate request: %v", err)
	}
	generatePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileViewKeyGenerate]
	if generatePC == nil {
		t.Fatal("expected viewkey generate precompile")
	}
	generateOut, err := generatePC.Run(generateReq)
	if err != nil {
		t.Fatalf("run viewkey generate precompile: %v", err)
	}
	var generateResp struct {
		ViewKey protocol.ViewKey `json:"viewKey"`
	}
	if err := json.Unmarshal(generateOut, &generateResp); err != nil {
		t.Fatalf("unmarshal viewkey generate response: %v", err)
	}
	if len(generateResp.ViewKey) == 0 {
		t.Fatal("expected non-empty viewkey")
	}

	nonce := []byte("nonce-1")
	recipient := protocol.PublicKey("recipient")
	discloseReq, err := json.Marshal(pblockchain.ViewKeyDiscloseRequest{
		ViewKey:   generateResp.ViewKey,
		Recipient: recipient,
		Nonce:     nonce,
	})
	if err != nil {
		t.Fatalf("marshal viewkey disclose request: %v", err)
	}
	disclosePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileViewKeyDisclose]
	if disclosePC == nil {
		t.Fatal("expected viewkey disclose precompile")
	}
	discloseOut, err := disclosePC.Run(discloseReq)
	if err != nil {
		t.Fatalf("run viewkey disclose precompile: %v", err)
	}
	var discloseResp struct {
		Disclosure protocol.EncryptedDisclosure `json:"disclosure"`
	}
	if err := json.Unmarshal(discloseOut, &discloseResp); err != nil {
		t.Fatalf("unmarshal viewkey disclose response: %v", err)
	}
	if len(discloseResp.Disclosure) == 0 {
		t.Fatal("expected non-empty disclosure")
	}

	replayReq, err := json.Marshal(pblockchain.ViewKeyReplayCheckRequest{
		Disclosure: discloseResp.Disclosure,
		ViewKey:    generateResp.ViewKey,
		Recipient:  recipient,
		Nonce:      nonce,
	})
	if err != nil {
		t.Fatalf("marshal viewkey replay request: %v", err)
	}
	replayPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileViewKeyReplayCheck]
	if replayPC == nil {
		t.Fatal("expected viewkey replay precompile")
	}
	replayOut, err := replayPC.Run(replayReq)
	if err != nil {
		t.Fatalf("run viewkey replay precompile: %v", err)
	}
	var replayResp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(replayOut, &replayResp); err != nil {
		t.Fatalf("unmarshal viewkey replay response: %v", err)
	}
	if !replayResp.OK {
		t.Fatal("expected replay detection success in production mode")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsShieldedProductionPath(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "production")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_SHIELDED_NULLIFIER_DB", filepath.Join(t.TempDir(), "shielded", "nullifiers.json"))

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env with shielded production: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	owner := shielded.OwnerFromSpendKey(protocol.PrivateKey("owner-secret"))
	req, err := json.Marshal(pblockchain.ShieldedCreateRequest{
		Owner:     owner,
		Value:     protocol.Amount{Value: big.NewInt(10)},
		AssetType: protocol.AssetTypeFungible,
	})
	if err != nil {
		t.Fatalf("marshal shielded create request: %v", err)
	}
	pc := vm.PrecompiledContractsPrague[vm.O2ULPrecompileShieldedCreate]
	if pc == nil {
		t.Fatal("expected shielded create precompile")
	}
	out, err := pc.Run(req)
	if err != nil {
		t.Fatalf("run shielded create precompile: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected shielded create response payload")
	}
}

func TestInstallRuntimeBridgeFromEnvUsesNodeDataDirForShieldedDefaultPath(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "production")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_SHIELDED_NULLIFIER_DB", "")

	nodeDataDir := t.TempDir()
	defaultDir := filepath.Join(nodeDataDir, "o2ul", "shielded")

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnvWithNodeDataDir(nodeDataDir); err != nil {
		t.Fatalf("install runtime bridge from env with node data dir: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	if _, err := os.Stat(defaultDir); err != nil {
		t.Fatalf("expected shielded default directory at %s: %v", defaultDir, err)
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsNFTProductionPath(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "production")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env with nft production: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	creator := nft.OwnerFromSpendKey(protocol.PrivateKey("nft-owner"))
	mintReq, err := json.Marshal(pblockchain.NFTMintRequest{
		Creator:      creator,
		MetadataHash: protocol.Hash("meta-a"),
		Salt:         []byte("salt-a"),
	})
	if err != nil {
		t.Fatalf("marshal nft mint request: %v", err)
	}
	mintPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileNFTMint]
	if mintPC == nil {
		t.Fatal("expected nft mint precompile")
	}
	mintOut, err := mintPC.Run(mintReq)
	if err != nil {
		t.Fatalf("run nft mint precompile: %v", err)
	}
	var mintResp struct {
		Note protocol.Note `json:"note"`
	}
	if err := json.Unmarshal(mintOut, &mintResp); err != nil {
		t.Fatalf("unmarshal nft mint response: %v", err)
	}
	if len(mintResp.Note.Payload) == 0 {
		t.Fatal("expected minted note payload")
	}

	proof, err := nft.NewHashProductionOwnershipVerifier().CreateProof(mintResp.Note)
	if err != nil {
		t.Fatalf("create ownership proof: %v", err)
	}
	verifyReq, err := json.Marshal(pblockchain.NFTOwnershipVerifyRequest{
		Note:  mintResp.Note,
		Proof: proof,
	})
	if err != nil {
		t.Fatalf("marshal nft verify request: %v", err)
	}
	verifyPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileNFTOwnershipVerify]
	if verifyPC == nil {
		t.Fatal("expected nft ownership verify precompile")
	}
	verifyOut, err := verifyPC.Run(verifyReq)
	if err != nil {
		t.Fatalf("run nft ownership verify precompile: %v", err)
	}
	var verifyResp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(verifyOut, &verifyResp); err != nil {
		t.Fatalf("unmarshal nft verify response: %v", err)
	}
	if !verifyResp.OK {
		t.Fatal("expected successful nft ownership verification with production mode")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsThresholdProductionPath(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "production")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")

	vm.SetO2ULRuntimeHookProvider(nil)
	if err := InstallRuntimeBridgeFromEnv(); err != nil {
		t.Fatalf("install runtime bridge from env with threshold production: %v", err)
	}
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	gkReq, err := json.Marshal(pblockchain.ThresholdGenerateGroupKeyRequest{
		Participants: []protocol.PublicKey{protocol.PublicKey("p1"), protocol.PublicKey("p2")},
		Threshold:    2,
	})
	if err != nil {
		t.Fatalf("marshal threshold group key request: %v", err)
	}
	gkPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileThresholdGenerate]
	if gkPC == nil {
		t.Fatal("expected threshold group key precompile")
	}
	gkOut, err := gkPC.Run(gkReq)
	if err != nil {
		t.Fatalf("run threshold group key precompile: %v", err)
	}
	var gkResp struct {
		GroupKey protocol.GroupKey `json:"groupKey"`
	}
	if err := json.Unmarshal(gkOut, &gkResp); err != nil {
		t.Fatalf("unmarshal threshold group key response: %v", err)
	}
	if len(gkResp.GroupKey) == 0 {
		t.Fatal("expected non-empty threshold group key response")
	}

	partialReq, err := json.Marshal(pblockchain.ThresholdSignPartialRequest{
		Share:   protocol.KeyShare("share-1"),
		Message: []byte("msg"),
	})
	if err != nil {
		t.Fatalf("marshal threshold partial request: %v", err)
	}
	partialPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileThresholdSign]
	if partialPC == nil {
		t.Fatal("expected threshold sign partial precompile")
	}
	partialOut, err := partialPC.Run(partialReq)
	if err != nil {
		t.Fatalf("run threshold sign partial precompile: %v", err)
	}
	var partialResp struct {
		Partial protocol.PartialSig `json:"partial"`
	}
	if err := json.Unmarshal(partialOut, &partialResp); err != nil {
		t.Fatalf("unmarshal threshold partial response: %v", err)
	}
	if len(partialResp.Partial) == 0 {
		t.Fatal("expected non-empty threshold partial response")
	}

	aggReq, err := json.Marshal(pblockchain.ThresholdAggregateRequest{Partials: []protocol.PartialSig{partialResp.Partial}})
	if err != nil {
		t.Fatalf("marshal threshold aggregate request: %v", err)
	}
	aggPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileThresholdAggregate]
	if aggPC == nil {
		t.Fatal("expected threshold aggregate precompile")
	}
	aggOut, err := aggPC.Run(aggReq)
	if err != nil {
		t.Fatalf("run threshold aggregate precompile: %v", err)
	}
	var aggResp struct {
		Signature protocol.Signature `json:"signature"`
	}
	if err := json.Unmarshal(aggOut, &aggResp); err != nil {
		t.Fatalf("unmarshal threshold aggregate response: %v", err)
	}
	if len(aggResp.Signature) == 0 {
		t.Fatal("expected non-empty threshold aggregate response")
	}
}
