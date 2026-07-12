package o2ulbridge

import (
	"testing"

	"github.com/AndrewDonelson/o2ul-proprietary/pkg/fees"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
)

func TestBuildFeeSplitGovernanceAuthorizerFromEnv(t *testing.T) {
	t.Run("deterministic defaults to disabled", func(t *testing.T) {
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(DefaultRuntimeBackendConfig(), "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if authorizer != nil {
			t.Fatal("expected nil authorizer in deterministic defaults")
		}
	})

	t.Run("production requires governance env", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CALLER_MAPPING_SLOT", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_EXECUTABLE_MAPPING_SLOT", "")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production runtime has no governance env")
		}
	})

	t.Run("production contract_abi requires artifact ABI paths by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}
		_ = split

		server := newGovernanceABIRPCFixtureServer(t, true, true)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", "")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi has no artifact ABI paths")
		}
	})

	t.Run("production contract_abi requires artifact profile path by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi has no artifact profile path")
		}
	})

	t.Run("artifact profile supplies production contract_abi defaults", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}

		server := newGovernanceABIRPCFixtureServerWithMethods(
			t,
			true,
			true,
			mustReadABIFixture(t, "governor_access_control_artifact.json"),
			"isAuthorizedCaller",
			mustReadABIFixture(t, "timelock_controller_artifact.json"),
			"isReadyOperation",
		)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected profile-driven contract_abi init success, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with profile defaults, got %v", err)
		}
	})

	t.Run("explicit env overrides artifact profile defaults", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		server := newGovernanceABIRPCFixtureServer(t, true, true)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "hasRole")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected failure from explicit env override incompatible with profile ABI")
		}
	})

	t.Run("invalid artifact profile path is rejected", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", "http://127.0.0.1:8545")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "testdata/governance/missing_profile.json")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected invalid profile path error")
		}
	})

	t.Run("production contract_abi requires explicit artifact semantics by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		server := newGovernanceABIRPCFixtureServer(t, true, true)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi has no explicit artifact semantics env")
		}
	})

	t.Run("artifact ABI requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}

		server := newGovernanceABIRPCFixtureServer(t, true, true)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit artifact requirement disable to allow default ABI path, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with explicit artifact requirement disable, got %v", err)
		}
	})

	t.Run("artifact profile requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}

		server := newGovernanceABIRPCFixtureServerWithMethods(
			t,
			true,
			true,
			mustReadABIFixture(t, "governor_access_control_artifact.json"),
			"isAuthorizedCaller",
			mustReadABIFixture(t, "timelock_controller_artifact.json"),
			"isReadyOperation",
		)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "keccak_utf8")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit artifact profile requirement disable to allow env-configured ABI semantics, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with explicit artifact profile requirement disable, got %v", err)
		}
	})

	t.Run("enabled governance authorizes configured caller and proposal", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "static")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "governor-1, governor-2")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "prop-1, prop-2")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("build governance authorizer: %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil governance authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate("governor-1", "prop-1", split)
		if err != nil {
			t.Fatalf("expected configured caller/proposal to authorize, got %v", err)
		}

		err = authorizer.AuthorizeFeeSplitUpdate("intruder", "prop-1", split)
		if err == nil {
			t.Fatal("expected unauthorized caller rejection")
		}
	})

	t.Run("contract_abi source wires reader and authorizes from rpc fixture", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}
		governorABI := mustReadABIFixture(t, "governor_access_control_artifact.json")
		timelockABI := mustReadABIFixture(t, "timelock_controller_artifact.json")

		server := newGovernanceABIRPCFixtureServerWithMethods(t, true, true, governorABI, "isAuthorizedCaller", timelockABI, "isReadyOperation")
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "keccak_utf8")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("build governance authorizer (contract_abi): %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil governance authorizer for contract_abi source")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected contract_abi authorizer to approve fixture-backed call, got %v", err)
		}
	})

	t.Run("contract_abi source rejects invalid operation id mode", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		server := newGovernanceABIRPCFixtureServer(t, true, true)
		defer server.Close()

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "invalid-mode")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error for invalid operation id mode")
		}
	})
}
