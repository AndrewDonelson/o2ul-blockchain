package o2ulbridge

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/AndrewDonelson/o2ul-proprietary/pkg/fees"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
)

const testCanonicalGovernanceArtifactProfileSHA256 = "d75cab45af7d439ad86fa92bdb01fc4662143139e0dd91dc6895a1973924cc38"
const testCanonicalGovernanceArtifactProfileMissingFieldsSHA256 = "2267460088eff0bdf9cce3ac15033a55bdc849cdcb9d9243a53bc4f750d83384"

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

	t.Run("production disallows governance disabled by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "false")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production explicitly disables governance")
		}
	})

	t.Run("governance always-enabled requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ALWAYS_ENABLED", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit always-enabled disable to allow governance disabled mode, got %v", err)
		}
		if authorizer != nil {
			t.Fatal("expected nil authorizer when governance disabled explicitly")
		}
	})

	t.Run("production requires contract_abi governance policy source by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "static")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "governor-1")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "proposal-1")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production uses non-contract_abi governance source")
		}
	})

	t.Run("contract_abi governance source requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CONTRACT_ABI_SOURCE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "static")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "governor-1")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "proposal-1")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit contract_abi source requirement disable to allow static source, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("governor-1"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with static source when contract_abi source requirement is disabled, got %v", err)
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", testCanonicalGovernanceArtifactProfileSHA256)
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", testCanonicalGovernanceArtifactProfileSHA256)
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
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

	t.Run("production contract_abi requires canonical deployed artifact semantics by default", func(t *testing.T) {
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "hasRole")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isOperationReady")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "keccak_utf8")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001007")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001008")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "EXECUTOR_ROLE")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi uses non-canonical artifact semantics")
		}
	})

	t.Run("production contract_abi requires canonical deployed artifact payloads by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

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

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_abi.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_abi.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "keccak_utf8")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001007")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001008")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "EXECUTOR_ROLE")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi uses non-canonical artifact payloads")
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PAYLOADS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_SEMANTICS", "false")
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

	t.Run("canonical artifact payload requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

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

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PAYLOADS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_abi.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_abi.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "keccak_utf8")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001007")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001008")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "EXECUTOR_ROLE")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit canonical-payload disable to allow non-canonical ABI artifacts, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}
	})

	t.Run("canonical artifact semantics requirement can be disabled explicitly", func(t *testing.T) {
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_SEMANTICS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "hex_bytes32")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001007")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001008")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "EXECUTOR_ROLE")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit canonical-semantics disable to allow non-canonical method semantics, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		_ = split
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
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

	t.Run("production contract_abi requires artifact profile checksum by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", "")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi has no artifact profile checksum")
		}
	})

	t.Run("production contract_abi requires canonical artifact profile checksum by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		tmpProfilePath := t.TempDir() + "/profile.json"
		canonicalContent, readErr := os.ReadFile(governanceABIFixturePath("canonical_contract_abi_profile.json"))
		if readErr != nil {
			t.Fatalf("read canonical profile fixture: %v", readErr)
		}
		modified := append(canonicalContent, '\n')
		if writeErr := os.WriteFile(tmpProfilePath, modified, 0o644); writeErr != nil {
			t.Fatalf("write temp profile fixture: %v", writeErr)
		}
		sum := sha256.Sum256(modified)
		nonCanonical := hex.EncodeToString(sum[:])

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", tmpProfilePath)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", nonCanonical)

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when profile checksum is non-canonical in production")
		}
	})

	t.Run("artifact profile checksum mismatch is rejected", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected checksum mismatch error")
		}
	})

	t.Run("artifact profile checksum requirement can be disabled explicitly", func(t *testing.T) {
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit checksum requirement disable to allow profile defaults, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with explicit checksum requirement disable, got %v", err)
		}
	})

	t.Run("canonical artifact profile checksum requirement can be disabled explicitly", func(t *testing.T) {
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

		tmpProfilePath := t.TempDir() + "/profile.json"
		canonicalContent, readErr := os.ReadFile(governanceABIFixturePath("canonical_contract_abi_profile.json"))
		if readErr != nil {
			t.Fatalf("read canonical profile fixture: %v", readErr)
		}
		modified := append(canonicalContent, '\n')
		if writeErr := os.WriteFile(tmpProfilePath, modified, 0o644); writeErr != nil {
			t.Fatalf("write temp profile fixture: %v", writeErr)
		}
		sum := sha256.Sum256(modified)
		nonCanonical := hex.EncodeToString(sum[:])

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", server.URL)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", tmpProfilePath)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", nonCanonical)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CANONICAL_ARTIFACT_PROFILE_SHA256", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_PROFILE_NO_ENV_OVERRIDES", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_ABI_PATH", governanceABIFixturePath("governor_access_control_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_ABI_PATH", governanceABIFixturePath("timelock_controller_artifact.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_IS_OPERATION_READY_METHOD", "isReadyOperation")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_OPERATION_ID_MODE", "keccak_utf8")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001007")
		t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001008")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_EXECUTOR_ROLE", "EXECUTOR_ROLE")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit canonical profile checksum disable to allow non-canonical checksum, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with canonical profile checksum requirement disabled, got %v", err)
		}
	})

	t.Run("production contract_abi requires artifact profile required fields by default", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile_missing_fields.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", testCanonicalGovernanceArtifactProfileMissingFieldsSHA256)

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when production contract_abi profile is missing required fields")
		}
	})

	t.Run("artifact profile required fields requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile_missing_fields.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", testCanonicalGovernanceArtifactProfileMissingFieldsSHA256)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_FIELDS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_ABIS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_EXPLICIT_ARTIFACT_SEMANTICS", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_ARTIFACT_PROFILE_SHA256", "false")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected profile with missing fields to still fail downstream ABI/method setup validation")
		}
	})

	t.Run("production contract_abi disallows env overrides when profile lock-in is enabled", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "contract_abi")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_PATH", governanceABIFixturePath("canonical_contract_abi_profile.json"))
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", testCanonicalGovernanceArtifactProfileSHA256)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when profile lock-in disallows env overrides")
		}
	})

	t.Run("profile no-env-override requirement can be disabled explicitly", func(t *testing.T) {
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
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_ARTIFACT_PROFILE_SHA256", testCanonicalGovernanceArtifactProfileSHA256)
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_PROFILE_NO_ENV_OVERRIDES", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "staging migration test")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_HAS_ROLE_METHOD", "isAuthorizedCaller")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected explicit no-env-override disable to allow matching env override, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}

		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("0x000000000000000000000000000000000000beef"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success with no-env-override disable, got %v", err)
		}
	})

	t.Run("production requires breakglass justification for lock-in override disables", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CONTRACT_ABI_SOURCE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "static")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "governor-1")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "proposal-1")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_BREAKGLASS_JUSTIFICATION", "")

		if _, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, ""); err == nil {
			t.Fatal("expected error when override disable is set without breakglass justification")
		}
	})

	t.Run("breakglass justification requirement can be disabled explicitly", func(t *testing.T) {
		cfg := DefaultRuntimeBackendConfig()
		cfg.Proofs = BackendModeProduction
		cfg.Shielded = BackendModeProduction
		cfg.NFT = BackendModeProduction
		cfg.Threshold = BackendModeProduction
		cfg.ViewKeys = BackendModeProduction

		split := fees.DistributionSplit{ProversValidatorsBps: 4000, ArbitratorPoolBps: 2500, DevTreasuryBps: 3000, BurnBps: 500}
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRED", "true")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_CONTRACT_ABI_SOURCE", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_REQUIRE_BREAKGLASS_JUSTIFICATION", "false")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_POLICY_SOURCE", "static")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_CALLERS", "governor-1")
		t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_EXECUTABLE_PROPOSALS", "proposal-1")

		authorizer, err := buildFeeSplitGovernanceAuthorizerFromEnv(cfg, "")
		if err != nil {
			t.Fatalf("expected breakglass justification requirement disable to allow override without justification, got %v", err)
		}
		if authorizer == nil {
			t.Fatal("expected non-nil authorizer")
		}
		err = authorizer.AuthorizeFeeSplitUpdate(protocol.Address("governor-1"), protocol.ProposalID("proposal-1"), split)
		if err != nil {
			t.Fatalf("expected authorize success when breakglass requirement disabled, got %v", err)
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
