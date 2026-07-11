package o2ulbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/nft"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/proofs"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/shielded"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/threshold"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/viewkeys"
)

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

	shieldedPool, err := buildShieldedPool(cfg)
	if err != nil {
		return nil, err
	}
	nftRegistry, nftOwnership := buildNFTAdapters(cfg)
	viewKeyManager := buildViewKeyManager(cfg)

	thresholdSigner := buildThresholdSigner(cfg)

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
		backend, err := proofs.NewExternalZKRegistryBackendWithRecords(records, 0, proofs.NewHashBackedExternalZKEngine("sim-external-zk-v1"))
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

func buildViewKeyManager(cfg RuntimeBackendConfig) *viewkeys.SimpleManager {
	if cfg.ViewKeys == BackendModeProduction {
		return viewkeys.NewSimpleManagerWithCipher(viewkeys.NewHashProductionDisclosureCipher())
	}
	return viewkeys.NewSimpleManager()
}

func buildThresholdSigner(cfg RuntimeBackendConfig) threshold.ThresholdSigner {
	if cfg.Threshold == BackendModeProduction {
		return threshold.NewProductionSigner(threshold.NewHashProductionBackend())
	}
	return threshold.NewSimpleSigner()
}

func buildNFTAdapters(cfg RuntimeBackendConfig) (*nft.InMemoryRegistry, nft.OwnershipVerifier) {
	if cfg.NFT == BackendModeProduction {
		ownership := nft.NewHashProductionOwnershipVerifier()
		return nft.NewInMemoryRegistryWithVerifier(ownership), ownership
	}
	ownership := nft.NewHashOwnershipVerifier()
	return nft.NewInMemoryRegistryWithVerifier(ownership), ownership
}

func buildShieldedPool(cfg RuntimeBackendConfig) (*shielded.InMemoryPool, error) {
	if cfg.Shielded != BackendModeProduction {
		return shielded.NewInMemoryPool(), nil
	}
	path := strings.TrimSpace(os.Getenv("O2UL_SHIELDED_NULLIFIER_DB"))
	if path == "" {
		path = filepath.Join(os.TempDir(), "o2ul", "shielded", "nullifiers.json")
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
	bridge, err := NewRuntimeBridgeWithConfig(cfg)
	if err != nil {
		return err
	}
	InstallRuntimeBridge(bridge)
	return nil
}

func InstallRuntimeBridgeFromEnv() error {
	cfg, err := RuntimeBackendConfigFromEnv()
	if err != nil {
		return err
	}
	return InstallRuntimeBridgeWithConfig(cfg)
}

func InstallDefaultRuntimeBridge() error {
	return InstallRuntimeBridgeWithConfig(DefaultRuntimeBackendConfig())
}
