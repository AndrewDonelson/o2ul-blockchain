package o2ulbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/consensus"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/fees"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/nft"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/proofs"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/shielded"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/threshold"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/viewkeys"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

const externalProviderMetricsPrefix = "o2ul/proofs/external_provider"

var newExternalProviderObserver = func() proofs.ExternalProviderObserver {
	return externalProviderCompositeObserver{observers: []proofs.ExternalProviderObserver{
		externalProviderLogObserver{},
		externalProviderMetricsObserver{prefix: externalProviderMetricsPrefix},
	}}
}

type externalProviderCompositeObserver struct {
	observers []proofs.ExternalProviderObserver
}

func (o externalProviderCompositeObserver) ObserveExternalProviderCall(event proofs.ExternalProviderCallEvent) {
	for _, obs := range o.observers {
		if obs == nil {
			continue
		}
		obs.ObserveExternalProviderCall(event)
	}
}

type externalProviderLogObserver struct{}

func (externalProviderLogObserver) ObserveExternalProviderCall(event proofs.ExternalProviderCallEvent) {
	if event.Success {
		log.Debug("o2ul external proofs provider call",
			"engine", event.Engine,
			"transport", event.Transport,
			"action", event.Action,
			"attempt", event.Attempt,
			"httpStatus", event.HTTPStatus,
			"durationMs", event.Duration.Milliseconds(),
		)
		return
	}
	log.Warn("o2ul external proofs provider call failed",
		"engine", event.Engine,
		"transport", event.Transport,
		"action", event.Action,
		"attempt", event.Attempt,
		"httpStatus", event.HTTPStatus,
		"durationMs", event.Duration.Milliseconds(),
		"error", event.ErrorMessage,
	)
}

type externalProviderMetricsObserver struct {
	prefix string
}

func (o externalProviderMetricsObserver) ObserveExternalProviderCall(event proofs.ExternalProviderCallEvent) {
	if !metrics.Enabled() {
		return
	}
	metrics.GetOrRegisterCounter(o.prefix+"/calls/total", nil).Inc(1)
	metrics.GetOrRegisterTimer(o.prefix+"/latency", nil).Update(event.Duration)

	transport := metricKeyPart(event.Transport, "unknown")
	action := metricKeyPart(event.Action, "unknown")
	metrics.GetOrRegisterCounter(fmt.Sprintf("%s/calls/%s/%s/total", o.prefix, transport, action), nil).Inc(1)

	status := "failure"
	if event.Success {
		status = "success"
	}
	metrics.GetOrRegisterCounter(o.prefix+"/calls/"+status, nil).Inc(1)
	metrics.GetOrRegisterCounter(fmt.Sprintf("%s/calls/%s/%s/%s", o.prefix, transport, action, status), nil).Inc(1)

	if event.HTTPStatus > 0 {
		class := fmt.Sprintf("%dxx", event.HTTPStatus/100)
		metrics.GetOrRegisterCounter(o.prefix+"/http_status/"+class, nil).Inc(1)
	}
}

func metricKeyPart(value string, fallback string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return fallback
	}
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "-", "_")
	return v
}

type BackendMode string

const (
	BackendModeDeterministic BackendMode = "deterministic"
	BackendModeProduction    BackendMode = "production"
)

type RuntimeBackendConfig struct {
	Proofs    BackendMode
	Shielded  BackendMode
	NFT       BackendMode
	Threshold BackendMode
	ViewKeys  BackendMode
	Consensus ConsensusRuntimeConfig
}

type ConsensusRuntimeConfig struct {
	NetworkType       string
	RequiredCircuitID protocol.CircuitID
	RegisteredNodes   []protocol.NodeID
	GenesisHash       protocol.Hash
}

func DefaultRuntimeBackendConfig() RuntimeBackendConfig {
	return RuntimeBackendConfig{
		Proofs:    BackendModeDeterministic,
		Shielded:  BackendModeDeterministic,
		NFT:       BackendModeDeterministic,
		Threshold: BackendModeDeterministic,
		ViewKeys:  BackendModeDeterministic,
		Consensus: ConsensusRuntimeConfig{},
	}
}

func RuntimeBackendConfigFromEnv() (RuntimeBackendConfig, error) {
	cfg := DefaultRuntimeBackendConfig()
	var err error
	if cfg.Proofs, err = parseBackendModeWithDefault("O2UL_BACKEND_PROOFS", cfg.Proofs); err != nil {
		return RuntimeBackendConfig{}, err
	}
	if cfg.Shielded, err = parseBackendModeWithDefault("O2UL_BACKEND_SHIELDED", cfg.Shielded); err != nil {
		return RuntimeBackendConfig{}, err
	}
	if cfg.NFT, err = parseBackendModeWithDefault("O2UL_BACKEND_NFT", cfg.NFT); err != nil {
		return RuntimeBackendConfig{}, err
	}
	if cfg.Threshold, err = parseBackendModeWithDefault("O2UL_BACKEND_THRESHOLD", cfg.Threshold); err != nil {
		return RuntimeBackendConfig{}, err
	}
	if cfg.ViewKeys, err = parseBackendModeWithDefault("O2UL_BACKEND_VIEWKEYS", cfg.ViewKeys); err != nil {
		return RuntimeBackendConfig{}, err
	}
	if cfg.Consensus, err = parseConsensusRuntimeConfigFromEnv(cfg.Consensus); err != nil {
		return RuntimeBackendConfig{}, err
	}
	return cfg, nil
}

func parseConsensusRuntimeConfigFromEnv(def ConsensusRuntimeConfig) (ConsensusRuntimeConfig, error) {
	networkType := strings.TrimSpace(strings.ToLower(os.Getenv("O2UL_CONSENSUS_NETWORK_TYPE")))
	if networkType == "" {
		if strings.TrimSpace(def.NetworkType) == "" {
			return ConsensusRuntimeConfig{}, nil
		}
		networkType = def.NetworkType
	}
	profile, err := consensus.ResolveNetworkProfile(networkType)
	if err != nil {
		return ConsensusRuntimeConfig{}, fmt.Errorf("consensus runtime config: %w", err)
	}

	registeredRaw := strings.TrimSpace(os.Getenv("O2UL_CONSENSUS_REGISTERED_NODES"))
	registeredNodes := make([]protocol.NodeID, 0)
	if registeredRaw != "" {
		for _, part := range strings.Split(registeredRaw, ",") {
			node := protocol.NodeID(strings.TrimSpace(part))
			if node == "" {
				continue
			}
			registeredNodes = append(registeredNodes, node)
		}
	}

	genesisRaw := strings.TrimSpace(os.Getenv("O2UL_CONSENSUS_GENESIS_HASH"))
	var genesisHash protocol.Hash
	if genesisRaw != "" {
		genesisHash = protocol.Hash(genesisRaw)
	}

	return ConsensusRuntimeConfig{
		NetworkType:       networkType,
		RequiredCircuitID: profile.CircuitID,
		RegisteredNodes:   registeredNodes,
		GenesisHash:       genesisHash,
	}, nil
}

type consensusCircuitEnforcingProofSystem struct {
	inner           proofs.ProofSystem
	requiredCircuit protocol.CircuitID
}

func (p consensusCircuitEnforcingProofSystem) Prove(circuit protocol.CircuitID, witness protocol.Witness) (protocol.Proof, error) {
	return p.inner.Prove(circuit, witness)
}

func (p consensusCircuitEnforcingProofSystem) Verify(circuit protocol.CircuitID, proof protocol.Proof, publicInputs protocol.PublicInputs) (bool, error) {
	if p.requiredCircuit != "" && circuit != p.requiredCircuit {
		return false, fmt.Errorf("consensus circuit policy mismatch: expected %s got %s", p.requiredCircuit, circuit)
	}
	return p.inner.Verify(circuit, proof, publicInputs)
}

func parseBackendModeWithDefault(env string, def BackendMode) (BackendMode, error) {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(env)))
	if raw == "" {
		return def, nil
	}
	mode := BackendMode(raw)
	switch mode {
	case BackendModeDeterministic, BackendModeProduction:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid %s=%q, expected deterministic|production", env, raw)
	}
}

func NewRuntimeBridgeWithConfig(cfg RuntimeBackendConfig) (*pblockchain.RuntimeBridge, error) {
	return newRuntimeBridgeWithConfig(cfg, "")
}

func newRuntimeBridgeWithConfig(cfg RuntimeBackendConfig, nodeDataDir string) (*pblockchain.RuntimeBridge, error) {
	proofCfg := proofs.BackendConfig{Kind: proofs.BackendKind(cfg.Proofs)}
	if cfg.Proofs == BackendModeProduction {
		proofBackend, err := buildProofProductionBackend()
		if err != nil {
			return nil, err
		}
		proofCfg.Production = proofBackend
	}
	proofSys, err := proofs.NewProofSystemFromConfig(proofCfg)
	if err != nil {
		return nil, err
	}
	if cfg.Consensus.RequiredCircuitID != "" {
		proofSys = consensusCircuitEnforcingProofSystem{
			inner:           proofSys,
			requiredCircuit: cfg.Consensus.RequiredCircuitID,
		}
	}

	shieldedPool, err := buildShieldedPool(cfg, nodeDataDir)
	if err != nil {
		return nil, err
	}
	nftRegistry, nftOwnership, err := buildNFTAdapters(cfg)
	if err != nil {
		return nil, err
	}
	viewKeyManager, err := buildViewKeyManager(cfg)
	if err != nil {
		return nil, err
	}

	thresholdSigner, err := buildThresholdSigner(cfg)
	if err != nil {
		return nil, err
	}

	var consensusAdapter pblockchain.ConsensusAdapter
	if cfg.Consensus.NetworkType != "" {
		consensusAdapter, err = consensus.NewBasicEngineForNetwork(
			proofSys,
			cfg.Consensus.NetworkType,
			cfg.Consensus.GenesisHash,
			cfg.Consensus.RegisteredNodes,
		)
		if err != nil {
			return nil, fmt.Errorf("consensus adapter init: %w", err)
		}
	}

	feeLedger := fees.NewInMemoryDistributionLedger()
	governanceAuthorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, nodeDataDir)
	if err != nil {
		return nil, err
	}

	return pblockchain.NewRuntimeBridge(pblockchain.RuntimeBridgeDeps{
		Proofs:       proofSys,
		Shielded:     shieldedPool,
		NFT:          nftRegistry,
		NFTOwnership: nftOwnership,
		Threshold:    thresholdSigner,
		ViewKeys:     viewKeyManager,
		Consensus:    consensusAdapter,
		Fees:         feeLedger,
		Governance:   governanceAuthorizer,
	})
}

func buildFeeSplitGovernanceAuthorizerFromEnv(cfg RuntimeBackendConfig, nodeDataDir string) (pblockchain.FeeSplitGovernanceAuthorizer, error) {
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED")
	}
	if err := validateGovernanceBreakglassJustification(cfg); err != nil {
		return nil, fmt.Errorf("fee split governance init: %w", err)
	}
	if shouldRequireGovernanceAlwaysEnabled(cfg) && !required {
		return nil, fmt.Errorf("fee split governance init: O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED=false is not allowed when governance-always-enabled enforcement is enabled")
	}
	if !required {
		return nil, nil
	}

	source := strings.TrimSpace(strings.ToLower(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE")))
	if source == "" {
		source = "contract_abi"
	}
	if shouldRequireContractABIGovernanceSource(cfg) && source != "contract_abi" {
		return nil, fmt.Errorf("fee split governance init: O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE=%q is not allowed when contract_abi source enforcement is enabled", source)
	}
	if source == "contract_abi" {
		if shouldRequireGovernanceArtifactProfile(cfg, source) {
			if err := validateGovernanceArtifactProfilePath(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if shouldRequireGovernanceArtifactProfileChecksum(cfg, source) {
			if err := validateGovernanceArtifactProfileChecksumEnv(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if shouldRequireCanonicalGovernanceArtifactProfileChecksum(cfg, source) {
			if err := validateCanonicalGovernanceArtifactProfileChecksumEnv(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if shouldRequireGovernanceArtifactProfileFields(cfg, source) {
			if err := validateGovernanceArtifactProfileFieldEnv(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if shouldRequireGovernanceArtifactProfileNoEnvOverrides(cfg, source) {
			if err := validateGovernanceArtifactProfileNoEnvOverrides(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if err := applyGovernanceArtifactProfileDefaults(); err != nil {
			return nil, fmt.Errorf("fee split governance init: %w", err)
		}
		if shouldRequireCanonicalGovernanceArtifactSemantics(cfg, source) {
			if err := validateCanonicalGovernanceArtifactSemanticsEnv(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
	}

	switch source {
	case "contract_abi":
		if shouldRequireGovernanceArtifactABIs(cfg, source) {
			if err := validateGovernanceArtifactABIPaths(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if shouldRequireCanonicalGovernanceArtifactPayloads(cfg, source) {
			if err := validateCanonicalGovernanceArtifactPayloads(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		if shouldRequireExplicitGovernanceArtifactSemantics(cfg, source) {
			if err := validateGovernanceArtifactSemanticsEnv(); err != nil {
				return nil, fmt.Errorf("fee split governance init: %w", err)
			}
		}
		reader, err := newContractABIGovernanceReader(nodeDataDir)
		if err != nil {
			return nil, fmt.Errorf("fee split governance init: %w", err)
		}
		authorizer, err := pblockchain.NewTimelockGovernorFeeSplitAuthorizer(reader, reader)
		if err != nil {
			return nil, fmt.Errorf("fee split governance init: %w", err)
		}
		return authorizer, nil
	case "contract_storage":
		reader, err := newContractStorageGovernanceReader(nodeDataDir)
		if err != nil {
			return nil, fmt.Errorf("fee split governance init: %w", err)
		}
		authorizer, err := pblockchain.NewTimelockGovernorFeeSplitAuthorizer(reader, reader)
		if err != nil {
			return nil, fmt.Errorf("fee split governance init: %w", err)
		}
		return authorizer, nil
	case "static":
		callers := parseAddressListEnv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS")
		if len(callers) == 0 {
			return nil, fmt.Errorf("fee split governance init: O2UL_FEE_SPLIT_GOVERNANCE_CALLERS is required when static policy source is enabled")
		}
		proposalIDs := parseProposalIDListEnv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS")
		if len(proposalIDs) == 0 {
			return nil, fmt.Errorf("fee split governance init: O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS is required when static policy source is enabled")
		}

		timelock := pblockchain.NewStaticExecutableProposalSet(proposalIDs)
		governor := pblockchain.NewStaticGovernorCallerAllowlist(callers)
		authorizer, err := pblockchain.NewTimelockGovernorFeeSplitAuthorizer(timelock, governor)
		if err != nil {
			return nil, fmt.Errorf("fee split governance init: %w", err)
		}
		return authorizer, nil
	default:
		return nil, fmt.Errorf("fee split governance init: invalid O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE=%q, expected contract_abi|contract_storage|static", source)
	}
}

type governanceArtifactProfile struct {
	GovernorABIPath string `json:"governorAbiPath"`
	TimelockABIPath string `json:"timelockAbiPath"`
	GovernorMethod  string `json:"governorMethod"`
	TimelockMethod  string `json:"timelockMethod"`
	OperationIDMode string `json:"operationIdMode"`
	GovernorAddress string `json:"governorAddress"`
	TimelockAddress string `json:"timelockAddress"`
	ExecutorRole    string `json:"executorRole"`
}

const canonicalGovernorArtifactSHA256 = "28943ea452c41ae8fc89dae684f0bc9f718e634fd5c1cac5ce35a6f52923b840"
const canonicalTimelockArtifactSHA256 = "962ef78bbf8c662a7b2ab1aab77abd6692032c969d0da0e242463a2c12dc1fa0"
const canonicalGovernanceArtifactProfileSHA256 = "d75cab45af7d439ad86fa92bdb01fc4662143139e0dd91dc6895a1973924cc38"

func applyGovernanceArtifactProfileDefaults() error {
	profilePath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH"))
	if profilePath == "" {
		return nil
	}
	content, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH=%q: %w", profilePath, err)
	}
	if err := validateGovernanceArtifactProfileChecksum(content); err != nil {
		return err
	}
	var profile governanceArtifactProfile
	if err := json.Unmarshal(content, &profile); err != nil {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH=%q: %w", profilePath, err)
	}
	profileDir := filepath.Dir(profilePath)
	setEnvIfUnset("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", resolveProfilePath(profileDir, profile.GovernorABIPath))
	setEnvIfUnset("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", resolveProfilePath(profileDir, profile.TimelockABIPath))
	setEnvIfUnset("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", strings.TrimSpace(profile.GovernorMethod))
	setEnvIfUnset("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", strings.TrimSpace(profile.TimelockMethod))
	setEnvIfUnset("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", strings.TrimSpace(profile.OperationIDMode))
	setEnvIfUnset("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", strings.TrimSpace(profile.GovernorAddress))
	setEnvIfUnset("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", strings.TrimSpace(profile.TimelockAddress))
	setEnvIfUnset("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", strings.TrimSpace(profile.ExecutorRole))
	return nil
}

func resolveProfilePath(baseDir string, pathValue string) string {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return trimmed
	}
	return filepath.Join(baseDir, trimmed)
}

func setEnvIfUnset(name string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if strings.TrimSpace(os.Getenv(name)) != "" {
		return
	}
	_ = os.Setenv(name, value)
}

func shouldRequireGovernanceArtifactABIs(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS")
	}
	return required
}

func shouldRequireContractABIGovernanceSource(cfg RuntimeBackendConfig) bool {
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CONTRACT_ABI_SOURCE")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CONTRACT_ABI_SOURCE")
	}
	return required
}

func shouldRequireGovernanceAlwaysEnabled(cfg RuntimeBackendConfig) bool {
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ALWAYS_ENABLED")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ALWAYS_ENABLED")
	}
	return required
}

func validateGovernanceBreakglassJustification(cfg RuntimeBackendConfig) error {
	if !runtimeBridgeUsesProductionBackends(cfg) {
		return nil
	}
	required := true
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_BREAKGLASS_JUSTIFICATION")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_BREAKGLASS_JUSTIFICATION")
	}
	if !required {
		return nil
	}
	overrideToggles := []string{
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ALWAYS_ENABLED",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CONTRACT_ABI_SOURCE",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PROFILE_SHA256",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_FIELDS",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_PROFILE_NO_ENV_OVERRIDES",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PAYLOADS",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS",
		"O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_SEMANTICS",
	}
	disabled := make([]string, 0, len(overrideToggles))
	for _, name := range overrideToggles {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			continue
		}
		if !parseBoolEnv(name) {
			disabled = append(disabled, name)
		}
	}
	if len(disabled) == 0 {
		return nil
	}
	justification := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION"))
	if justification == "" {
		return fmt.Errorf("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION is required when disabling governance lock-in overrides: %s", strings.Join(disabled, ","))
	}
	return nil
}

func shouldRequireGovernanceArtifactProfile(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE")
	}
	return required
}

func shouldRequireGovernanceArtifactProfileChecksum(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256")
	}
	return required
}

func shouldRequireCanonicalGovernanceArtifactProfileChecksum(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PROFILE_SHA256")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PROFILE_SHA256")
	}
	return required
}

func shouldRequireGovernanceArtifactProfileFields(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_FIELDS")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_FIELDS")
	}
	return required
}

func shouldRequireGovernanceArtifactProfileNoEnvOverrides(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_PROFILE_NO_ENV_OVERRIDES")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_PROFILE_NO_ENV_OVERRIDES")
	}
	return required
}

func shouldRequireExplicitGovernanceArtifactSemantics(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS")
	}
	return required
}

func shouldRequireCanonicalGovernanceArtifactSemantics(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_SEMANTICS")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_SEMANTICS")
	}
	return required
}

func shouldRequireCanonicalGovernanceArtifactPayloads(cfg RuntimeBackendConfig, source string) bool {
	if source != "contract_abi" {
		return false
	}
	required := runtimeBridgeUsesProductionBackends(cfg)
	if raw := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PAYLOADS")); raw != "" {
		required = parseBoolEnv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PAYLOADS")
	}
	return required
}

func validateCanonicalGovernanceArtifactSemanticsEnv() error {
	type expectedEnv struct {
		name  string
		value string
	}
	required := []expectedEnv{
		{name: "O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", value: "isAuthorizedCaller"},
		{name: "O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", value: "isReadyOperation"},
		{name: "O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", value: "keccak_utf8"},
		{name: "O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", value: "0x0000000000000000000000000000000000001007"},
		{name: "O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", value: "0x0000000000000000000000000000000000001008"},
		{name: "O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", value: "EXECUTOR_ROLE"},
	}
	mismatches := make([]string, 0, len(required))
	for _, item := range required {
		actual := strings.TrimSpace(os.Getenv(item.name))
		if actual == item.value {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("%s=%q (expected %q)", item.name, actual, item.value))
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("canonical artifact semantics enforcement is enabled and requires deployed governance semantics: %s", strings.Join(mismatches, "; "))
	}
	return nil
}

func validateCanonicalGovernanceArtifactPayloads() error {
	type artifactCheck struct {
		envName        string
		expectedSHA256 string
	}
	checks := []artifactCheck{
		{envName: "O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", expectedSHA256: canonicalGovernorArtifactSHA256},
		{envName: "O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", expectedSHA256: canonicalTimelockArtifactSHA256},
	}
	mismatches := make([]string, 0, len(checks))
	for _, check := range checks {
		pathValue := strings.TrimSpace(os.Getenv(check.envName))
		if pathValue == "" {
			mismatches = append(mismatches, fmt.Sprintf("%s is empty", check.envName))
			continue
		}
		content, err := os.ReadFile(pathValue)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s read error: %v", check.envName, err))
			continue
		}
		sum := sha256.Sum256(content)
		actual := hex.EncodeToString(sum[:])
		if actual != check.expectedSHA256 {
			mismatches = append(mismatches, fmt.Sprintf("%s hash mismatch (actual=%s expected=%s)", check.envName, actual, check.expectedSHA256))
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("canonical artifact payload enforcement is enabled and requires pinned deployed artifacts: %s", strings.Join(mismatches, "; "))
	}
	return nil
}

func validateGovernanceArtifactABIPaths() error {
	governorPath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH"))
	timelockPath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH"))
	if governorPath == "" || timelockPath == "" {
		return fmt.Errorf("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH and O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH are required when artifact ABI enforcement is enabled")
	}
	return nil
}

func validateGovernanceArtifactProfilePath() error {
	profilePath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH"))
	if profilePath == "" {
		return fmt.Errorf("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH is required when artifact profile enforcement is enabled")
	}
	return nil
}

func validateGovernanceArtifactProfileChecksumEnv() error {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256")))
	if value == "" {
		return fmt.Errorf("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256 is required when artifact profile checksum enforcement is enabled")
	}
	if len(value) != 64 {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256=%q: expected 64-char hex SHA-256", value)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256=%q: %w", value, err)
	}
	return nil
}

func validateCanonicalGovernanceArtifactProfileChecksumEnv() error {
	profilePath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH"))
	if profilePath == "" {
		return nil
	}
	value := strings.TrimSpace(strings.ToLower(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256")))
	if value == "" {
		return fmt.Errorf("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256 must equal canonical deployed profile hash %q when canonical profile checksum enforcement is enabled", canonicalGovernanceArtifactProfileSHA256)
	}
	if value != canonicalGovernanceArtifactProfileSHA256 {
		return fmt.Errorf("canonical profile checksum enforcement is enabled and requires O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256=%q (got %q)", canonicalGovernanceArtifactProfileSHA256, value)
	}
	return nil
}

func validateGovernanceArtifactProfileFieldEnv() error {
	profilePath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH"))
	if profilePath == "" {
		return nil
	}
	content, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH=%q: %w", profilePath, err)
	}
	var profile governanceArtifactProfile
	if err := json.Unmarshal(content, &profile); err != nil {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH=%q: %w", profilePath, err)
	}
	missing := make([]string, 0, 8)
	if strings.TrimSpace(profile.GovernorABIPath) == "" {
		missing = append(missing, "governorAbiPath")
	}
	if strings.TrimSpace(profile.TimelockABIPath) == "" {
		missing = append(missing, "timelockAbiPath")
	}
	if strings.TrimSpace(profile.GovernorMethod) == "" {
		missing = append(missing, "governorMethod")
	}
	if strings.TrimSpace(profile.TimelockMethod) == "" {
		missing = append(missing, "timelockMethod")
	}
	if strings.TrimSpace(profile.OperationIDMode) == "" {
		missing = append(missing, "operationIdMode")
	}
	if strings.TrimSpace(profile.GovernorAddress) == "" {
		missing = append(missing, "governorAddress")
	}
	if strings.TrimSpace(profile.TimelockAddress) == "" {
		missing = append(missing, "timelockAddress")
	}
	if strings.TrimSpace(profile.ExecutorRole) == "" {
		missing = append(missing, "executorRole")
	}
	if len(missing) > 0 {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH=%q: missing required profile fields: %s", profilePath, strings.Join(missing, ","))
	}
	return nil
}

func validateGovernanceArtifactProfileNoEnvOverrides() error {
	profilePath := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH"))
	if profilePath == "" {
		return nil
	}
	overrides := []string{
		"O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH",
		"O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH",
		"O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD",
		"O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD",
		"O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE",
		"O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS",
		"O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS",
		"O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE",
	}
	configured := make([]string, 0, len(overrides))
	for _, name := range overrides {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			continue
		}
		configured = append(configured, name)
	}
	if len(configured) > 0 {
		return fmt.Errorf("artifact profile lock-in is enabled and does not allow env override vars: %s", strings.Join(configured, ","))
	}
	return nil
}

func validateGovernanceArtifactProfileChecksum(content []byte) error {
	expected := strings.TrimSpace(strings.ToLower(os.Getenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256")))
	if expected == "" {
		return nil
	}
	sum := sha256.Sum256(content)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return fmt.Errorf("invalid O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256=%q: profile content SHA-256 mismatch (actual=%q)", expected, actual)
	}
	return nil
}

func validateGovernanceArtifactSemanticsEnv() error {
	governorMethod := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD"))
	timelockMethod := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD"))
	operationMode := strings.TrimSpace(os.Getenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE"))
	if governorMethod == "" || timelockMethod == "" || operationMode == "" {
		return fmt.Errorf("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD, O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD, and O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE are required when explicit artifact semantics enforcement is enabled")
	}
	return nil
}

func runtimeBridgeUsesProductionBackends(cfg RuntimeBackendConfig) bool {
	return cfg.Proofs == BackendModeProduction &&
		cfg.Shielded == BackendModeProduction &&
		cfg.NFT == BackendModeProduction &&
		cfg.Threshold == BackendModeProduction &&
		cfg.ViewKeys == BackendModeProduction
}

func parseAddressListEnv(name string) []protocol.Address {
	parts := strings.Split(strings.TrimSpace(os.Getenv(name)), ",")
	out := make([]protocol.Address, 0, len(parts))
	for _, part := range parts {
		value := protocol.Address(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func parseProposalIDListEnv(name string) []protocol.ProposalID {
	parts := strings.Split(strings.TrimSpace(os.Getenv(name)), ",")
	out := make([]protocol.ProposalID, 0, len(parts))
	for _, part := range parts {
		value := protocol.ProposalID(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func buildProofProductionBackend() (proofs.ProductionBackend, error) {
	flavor := strings.TrimSpace(strings.ToLower(os.Getenv("O2UL_PROOFS_PRODUCTION_FLAVOR")))
	if flavor == "" {
		flavor = "registry"
	}
	path := strings.TrimSpace(os.Getenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON"))
	if path == "" {
		if flavor == "external" {
			return nil, fmt.Errorf("proofs production backend init: O2UL_PROOFS_CIRCUIT_KEYS_JSON is required for external flavor")
		}
		return proofs.NewHashProductionBackend(0), nil
	}
	records, err := proofs.LoadCircuitKeyRecordsFromJSON(path)
	if err != nil {
		return nil, fmt.Errorf("proofs production backend init: %w", err)
	}
	if err := proofs.ValidateCircuitKeyRecords(records); err != nil {
		return nil, fmt.Errorf("proofs production backend init: %w", err)
	}
	if flavor == "external" {
		providerURL := strings.TrimSpace(os.Getenv("O2UL_PROOFS_EXTERNAL_PROVIDER_URL"))
		providerCmd := strings.TrimSpace(os.Getenv("O2UL_PROOFS_EXTERNAL_PROVIDER_CMD"))
		if providerURL != "" && providerCmd != "" {
			return nil, fmt.Errorf("proofs production backend init: only one of O2UL_PROOFS_EXTERNAL_PROVIDER_URL or O2UL_PROOFS_EXTERNAL_PROVIDER_CMD may be set")
		}
		observer := newExternalProviderObserver()
		timeoutMS, err := parseOptionalIntEnv("O2UL_PROOFS_EXTERNAL_PROVIDER_TIMEOUT_MS", 5000)
		if err != nil {
			return nil, fmt.Errorf("proofs production backend init: %w", err)
		}
		var engine proofs.ExternalZKEngine
		if providerURL != "" {
			maxRetries, err := parseOptionalIntEnv("O2UL_PROOFS_EXTERNAL_PROVIDER_MAX_RETRIES", 0)
			if err != nil {
				return nil, fmt.Errorf("proofs production backend init: %w", err)
			}
			retryDelayMS, err := parseOptionalIntEnv("O2UL_PROOFS_EXTERNAL_PROVIDER_RETRY_DELAY_MS", 100)
			if err != nil {
				return nil, fmt.Errorf("proofs production backend init: %w", err)
			}
			insecureSkipVerify := parseBoolEnv("O2UL_PROOFS_EXTERNAL_PROVIDER_TLS_INSECURE_SKIP_VERIFY")
			httpEngine, err := proofs.NewHTTPExternalZKEngineWithConfig(proofs.HTTPExternalZKEngineConfig{
				URL:                providerURL,
				AuthBearerToken:    strings.TrimSpace(os.Getenv("O2UL_PROOFS_EXTERNAL_PROVIDER_AUTH_BEARER")),
				Timeout:            time.Duration(timeoutMS) * time.Millisecond,
				MaxRetries:         maxRetries,
				RetryDelay:         time.Duration(retryDelayMS) * time.Millisecond,
				InsecureSkipVerify: insecureSkipVerify,
				Observer:           observer,
			})
			if err != nil {
				return nil, fmt.Errorf("proofs production backend init: %w", err)
			}
			engine = httpEngine
		} else if providerCmd != "" {
			procEngine, err := proofs.NewProcessExternalZKEngineWithConfig(proofs.ProcessExternalZKEngineConfig{
				CommandLine: providerCmd,
				Timeout:     time.Duration(timeoutMS) * time.Millisecond,
				Observer:    observer,
			})
			if err != nil {
				return nil, fmt.Errorf("proofs production backend init: %w", err)
			}
			engine = procEngine
		} else {
			return nil, fmt.Errorf("proofs production backend init: one of O2UL_PROOFS_EXTERNAL_PROVIDER_URL or O2UL_PROOFS_EXTERNAL_PROVIDER_CMD is required for external flavor")
		}

		backend, err := proofs.NewExternalZKRegistryBackendWithRecords(records, 0, engine)
		if err != nil {
			return nil, fmt.Errorf("proofs production backend init: %w", err)
		}
		return backend, nil
	}
	if flavor != "registry" {
		return nil, fmt.Errorf("proofs production backend init: invalid O2UL_PROOFS_PRODUCTION_FLAVOR=%q, expected registry|external", flavor)
	}
	backend, err := proofs.NewRegistryProductionBackendWithRecords(records, 0)
	if err != nil {
		return nil, fmt.Errorf("proofs production backend init: %w", err)
	}
	return backend, nil
}

func parseOptionalIntEnv(name string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q", name, raw)
	}
	return v, nil
}

func parseBoolEnv(name string) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func buildViewKeyManager(cfg RuntimeBackendConfig) (*viewkeys.SimpleManager, error) {
	if cfg.ViewKeys == BackendModeProduction {
		keyHex := strings.TrimSpace(os.Getenv("O2UL_VIEWKEYS_DISCLOSURE_KEY_HEX"))
		if keyHex == "" {
			return viewkeys.NewSimpleManagerWithCipher(viewkeys.NewHashProductionDisclosureCipher()), nil
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("viewkeys production backend init: invalid O2UL_VIEWKEYS_DISCLOSURE_KEY_HEX=%q", keyHex)
		}
		cipher, err := viewkeys.NewHashProductionDisclosureCipherWithKey(key)
		if err != nil {
			return nil, fmt.Errorf("viewkeys production backend init: %w", err)
		}
		return viewkeys.NewSimpleManagerWithCipher(cipher), nil
	}
	return viewkeys.NewSimpleManager(), nil
}

func buildThresholdSigner(cfg RuntimeBackendConfig) (threshold.ThresholdSigner, error) {
	if cfg.Threshold == BackendModeProduction {
		keyHex := strings.TrimSpace(os.Getenv("O2UL_THRESHOLD_PRODUCTION_KEY_HEX"))
		if keyHex == "" {
			return threshold.NewProductionSigner(threshold.NewHashProductionBackend()), nil
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("threshold production backend init: invalid O2UL_THRESHOLD_PRODUCTION_KEY_HEX=%q", keyHex)
		}
		backend, err := threshold.NewHashProductionBackendWithKey(key)
		if err != nil {
			return nil, fmt.Errorf("threshold production backend init: %w", err)
		}
		return threshold.NewProductionSigner(backend), nil
	}
	return threshold.NewSimpleSigner(), nil
}

func buildNFTAdapters(cfg RuntimeBackendConfig) (*nft.InMemoryRegistry, nft.OwnershipVerifier, error) {
	if cfg.NFT == BackendModeProduction {
		keyHex := strings.TrimSpace(os.Getenv("O2UL_NFT_PROVENANCE_KEY_HEX"))
		if keyHex == "" {
			ownership := nft.NewHashProductionOwnershipVerifier()
			return nft.NewInMemoryRegistryWithVerifier(ownership), ownership, nil
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, nil, fmt.Errorf("nft production backend init: invalid O2UL_NFT_PROVENANCE_KEY_HEX=%q", keyHex)
		}
		ownership, err := nft.NewProvenanceProductionOwnershipVerifierWithKey(key)
		if err != nil {
			return nil, nil, fmt.Errorf("nft production backend init: %w", err)
		}
		return nft.NewInMemoryRegistryWithVerifier(ownership), ownership, nil
	}
	ownership := nft.NewHashOwnershipVerifier()
	return nft.NewInMemoryRegistryWithVerifier(ownership), ownership, nil
}

func buildShieldedPool(cfg RuntimeBackendConfig, nodeDataDir string) (*shielded.InMemoryPool, error) {
	if cfg.Shielded != BackendModeProduction {
		return shielded.NewInMemoryPool(), nil
	}
	path := strings.TrimSpace(os.Getenv("O2UL_SHIELDED_NULLIFIER_DB"))
	if path == "" {
		resolvedDataDir := strings.TrimSpace(nodeDataDir)
		if resolvedDataDir == "" {
			resolvedDataDir = strings.TrimSpace(os.Getenv("O2UL_NODE_DATA_DIR"))
		}
		if resolvedDataDir != "" {
			path = filepath.Join(resolvedDataDir, "o2ul", "shielded", "nullifiers.json")
		} else {
			path = filepath.Join(os.TempDir(), "o2ul", "shielded", "nullifiers.json")
		}
	}
	p, err := shielded.NewFileNullifierPersistence(path)
	if err != nil {
		return nil, fmt.Errorf("shielded production backend init: %w", err)
	}
	return shielded.NewInMemoryPoolWithAdapters(p, shielded.NewJSONTxPublicInputsCodec()), nil
}

func NewDefaultRuntimeBridge() (*pblockchain.RuntimeBridge, error) {
	return NewRuntimeBridgeWithConfig(DefaultRuntimeBackendConfig())
}

func InstallRuntimeBridgeWithConfig(cfg RuntimeBackendConfig) error {
	return installRuntimeBridgeWithConfig(cfg, "")
}

func installRuntimeBridgeWithConfig(cfg RuntimeBackendConfig, nodeDataDir string) error {
	bridge, err := newRuntimeBridgeWithConfig(cfg, nodeDataDir)
	if err != nil {
		return err
	}
	InstallRuntimeBridge(bridge)
	return nil
}

func InstallRuntimeBridgeWithConfigAndNodeDataDir(cfg RuntimeBackendConfig, nodeDataDir string) error {
	return installRuntimeBridgeWithConfig(cfg, nodeDataDir)
}

func InstallRuntimeBridgeFromEnvWithNodeDataDir(nodeDataDir string) error {
	cfg, err := RuntimeBackendConfigFromEnv()
	if err != nil {
		return err
	}
	return installRuntimeBridgeWithConfig(cfg, nodeDataDir)
}

func InstallRuntimeBridgeFromEnv() error {
	return InstallRuntimeBridgeFromEnvWithNodeDataDir("")
}

func InstallDefaultRuntimeBridge() error {
	return installRuntimeBridgeWithConfig(DefaultRuntimeBackendConfig(), "")
}
