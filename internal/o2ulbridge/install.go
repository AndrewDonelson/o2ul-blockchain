package o2ulbridge

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/nft"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/proofs"
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
}

func DefaultRuntimeBackendConfig() RuntimeBackendConfig {
	return RuntimeBackendConfig{
		Proofs:    BackendModeDeterministic,
		Shielded:  BackendModeDeterministic,
		NFT:       BackendModeDeterministic,
		Threshold: BackendModeDeterministic,
		ViewKeys:  BackendModeDeterministic,
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
	return cfg, nil
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

	return pblockchain.NewRuntimeBridge(pblockchain.RuntimeBridgeDeps{
		Proofs:       proofSys,
		Shielded:     shieldedPool,
		NFT:          nftRegistry,
		NFTOwnership: nftOwnership,
		Threshold:    thresholdSigner,
		ViewKeys:     viewKeyManager,
	})
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
