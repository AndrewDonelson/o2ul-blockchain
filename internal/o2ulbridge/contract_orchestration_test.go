package o2ulbridge

import (
	"errors"
	"math/big"
	"reflect"
	"sort"
	"testing"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

type fixedTestPrecompile struct {
	output []byte
	err    error
}

type nonFlowWrapperCase struct {
	name                  string
	invoke                func() (OrchestrationDiagnostics, error)
	expectedErr           error
	expectedNormalized    string
	expectedStep          string
	expectedPrecompile    string
	expectedSuccess       bool
	expectedErrorCategory string
}

type nonFlowLegacyWrapperParityCase struct {
	name                  string
	legacy                func() (any, error)
	wrapper               func() (any, OrchestrationDiagnostics, error)
	expectedErr           error
	expectedNormalized    string
	expectedStep          string
	expectedPrecompile    string
	expectedSuccess       bool
	expectedErrorCategory string
}

type flowTraceExpectation struct {
	step          string
	precompile    string
	success       bool
	errorCategory string
}

type flowDiagnosticsCase struct {
	name               string
	invoke             func() (EscrowArbitrationOrchestrationResult, error)
	expectError        bool
	expectedErr        error
	expectedNormalized string
	expectedTraces     []flowTraceExpectation
	validateOutput     func(t *testing.T, out EscrowArbitrationOrchestrationResult)
}

func newFlowFixtureRequest() EscrowArbitrationOrchestrationRequest {
	return EscrowArbitrationOrchestrationRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		EvidenceRef:      protocol.Hash("ev-flow-fixture"),
		ViewKey:          protocol.EncryptedViewKey("vk-flow-fixture"),
		DisputeFee:       protocol.Amount{Value: big.NewInt(0)},
		Seed:             protocol.VRFSeed("seed-flow-fixture"),
		ArbitratorCount:  1,
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-flow-fixture"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	}
}

func newFlowSuccessFixturePrecompiles() vm.PrecompiledContracts {
	return vm.PrecompiledContracts{
		vm.O2ULPrecompileEscrowDispute:     vm.PrecompiledContractsPrague[vm.O2ULPrecompileEscrowDispute],
		vm.O2ULPrecompileArbitrationSelect: vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationSelect],
		vm.O2ULPrecompileArbitrationRule:   vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationRule],
		vm.O2ULPrecompileEscrowSettle:      vm.PrecompiledContractsPrague[vm.O2ULPrecompileEscrowSettle],
	}
}

func newFlowDecodeFixturePrecompiles() vm.PrecompiledContracts {
	return vm.PrecompiledContracts{
		vm.O2ULPrecompileEscrowDispute: fixedTestPrecompile{output: []byte("{")},
	}
}

func newFlowShapeFixturePrecompiles() vm.PrecompiledContracts {
	return vm.PrecompiledContracts{
		vm.O2ULPrecompileEscrowDispute:     fixedTestPrecompile{output: []byte(`{"disputeId":"fixture-dispute"}`)},
		vm.O2ULPrecompileArbitrationSelect: fixedTestPrecompile{output: []byte(`{"selected":[]}`)},
	}
}

func newFlowMissingFixturePrecompiles() vm.PrecompiledContracts {
	return vm.PrecompiledContracts{
		vm.O2ULPrecompileEscrowDispute: fixedTestPrecompile{output: []byte(`{"disputeId":"fixture-dispute"}`)},
	}
}

func TestPrecompileContractOrchestratorFlowFixtureFactoriesExpectedAddresses(t *testing.T) {
	tests := []struct {
		name     string
		factory  func() vm.PrecompiledContracts
		expected []common.Address
	}{
		{
			name:    "success",
			factory: newFlowSuccessFixturePrecompiles,
			expected: []common.Address{
				vm.O2ULPrecompileEscrowDispute,
				vm.O2ULPrecompileArbitrationSelect,
				vm.O2ULPrecompileArbitrationRule,
				vm.O2ULPrecompileEscrowSettle,
			},
		},
		{
			name:    "decode",
			factory: newFlowDecodeFixturePrecompiles,
			expected: []common.Address{
				vm.O2ULPrecompileEscrowDispute,
			},
		},
		{
			name:    "shape",
			factory: newFlowShapeFixturePrecompiles,
			expected: []common.Address{
				vm.O2ULPrecompileEscrowDispute,
				vm.O2ULPrecompileArbitrationSelect,
			},
		},
		{
			name:    "missing",
			factory: newFlowMissingFixturePrecompiles,
			expected: []common.Address{
				vm.O2ULPrecompileEscrowDispute,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			precompiles := tc.factory()
			if len(precompiles) != len(tc.expected) {
				t.Fatalf("expected %d precompile addresses, got %d", len(tc.expected), len(precompiles))
			}

			expectedSet := make(map[common.Address]struct{}, len(tc.expected))
			for _, address := range tc.expected {
				expectedSet[address] = struct{}{}
				if precompiles[address] == nil {
					t.Fatalf("missing expected precompile %s", address.Hex())
				}
			}

			for address := range precompiles {
				if _, ok := expectedSet[address]; !ok {
					t.Fatalf("unexpected precompile %s in fixture", address.Hex())
				}
			}
		})
	}
}

func assertNonFlowWrapperCases(t *testing.T, cases []nonFlowWrapperCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diag, err := tc.invoke()
			if tc.expectedErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}

			if diag.NormalizedErrorCategory != tc.expectedNormalized {
				t.Fatalf("expected normalized category %q, got %q", tc.expectedNormalized, diag.NormalizedErrorCategory)
			}
			if len(diag.StepTraces) != 1 {
				t.Fatalf("expected single trace, got %d", len(diag.StepTraces))
			}
			trace := diag.StepTraces[0]
			if trace.Step != tc.expectedStep || trace.Precompile != tc.expectedPrecompile || trace.Success != tc.expectedSuccess || trace.ErrorCategory != tc.expectedErrorCategory {
				t.Fatalf("unexpected trace: %+v", trace)
			}
		})
	}
}

func assertNonFlowLegacyWrapperParityCases(t *testing.T, cases []nonFlowLegacyWrapperParityCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			legacyValue, legacyErr := tc.legacy()
			wrapperValue, diag, wrapperErr := tc.wrapper()

			expectError := tc.expectedNormalized != OrchestrationErrorCategoryNone
			if expectError {
				if legacyErr == nil || wrapperErr == nil {
					t.Fatalf("expected errors for both routes, legacy=%v wrapper=%v", legacyErr, wrapperErr)
				}
				if tc.expectedErr != nil {
					if !errors.Is(legacyErr, tc.expectedErr) || !errors.Is(wrapperErr, tc.expectedErr) {
						t.Fatalf("expected errors.Is(..., %v), legacy=%v wrapper=%v", tc.expectedErr, legacyErr, wrapperErr)
					}
				}
				if normalizeOrchestrationError(legacyErr) != tc.expectedNormalized || normalizeOrchestrationError(wrapperErr) != tc.expectedNormalized {
					t.Fatalf("expected normalized category %q, legacy=%q wrapper=%q", tc.expectedNormalized, normalizeOrchestrationError(legacyErr), normalizeOrchestrationError(wrapperErr))
				}
			} else {
				if legacyErr != nil || wrapperErr != nil {
					t.Fatalf("unexpected errors for success parity case, legacy=%v wrapper=%v", legacyErr, wrapperErr)
				}
				if !reflect.DeepEqual(legacyValue, wrapperValue) {
					t.Fatalf("legacy/wrapper payload mismatch, legacy=%+v wrapper=%+v", legacyValue, wrapperValue)
				}
			}

			if diag.NormalizedErrorCategory != tc.expectedNormalized {
				t.Fatalf("expected diagnostics category %q, got %q", tc.expectedNormalized, diag.NormalizedErrorCategory)
			}
			if len(diag.StepTraces) != 1 {
				t.Fatalf("expected single diagnostics trace, got %d", len(diag.StepTraces))
			}
			trace := diag.StepTraces[0]
			if trace.Step != tc.expectedStep || trace.Precompile != tc.expectedPrecompile || trace.Success != tc.expectedSuccess || trace.ErrorCategory != tc.expectedErrorCategory {
				t.Fatalf("unexpected diagnostics trace: %+v", trace)
			}
		})
	}
}

func assertFlowDiagnosticsCases(t *testing.T, cases []flowDiagnosticsCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.invoke()
			if tc.expectError {
				if err == nil {
					t.Fatal("expected orchestration flow error")
				}
				if tc.expectedErr != nil && !errors.Is(err, tc.expectedErr) {
					t.Fatalf("expected errors.Is(err, %v), got %v", tc.expectedErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected orchestration flow error: %v", err)
			}

			if out.Diagnostics.NormalizedErrorCategory != tc.expectedNormalized {
				t.Fatalf("expected diagnostics category %q, got %q", tc.expectedNormalized, out.Diagnostics.NormalizedErrorCategory)
			}
			if len(out.Diagnostics.StepTraces) != len(tc.expectedTraces) {
				t.Fatalf("expected %d diagnostics traces, got %d", len(tc.expectedTraces), len(out.Diagnostics.StepTraces))
			}
			for i, expected := range tc.expectedTraces {
				trace := out.Diagnostics.StepTraces[i]
				if trace.Step != expected.step || trace.Precompile != expected.precompile || trace.Success != expected.success || trace.ErrorCategory != expected.errorCategory {
					t.Fatalf("unexpected flow trace[%d]: %+v", i, trace)
				}
			}

			if tc.validateOutput != nil {
				tc.validateOutput(t, out)
			}
		})
	}
}

func (p fixedTestPrecompile) RequiredGas(input []byte) uint64 { return 0 }

func (p fixedTestPrecompile) Run(input []byte) ([]byte, error) {
	if p.err != nil {
		return nil, p.err
	}
	return append([]byte(nil), p.output...), nil
}

func TestPrecompileContractOrchestratorExecuteEscrowArbitrationFlow(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(newFlowSuccessFixturePrecompiles())
	assertFlowDiagnosticsCases(t, []flowDiagnosticsCase{
		{
			name: "success",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				req := newFlowFixtureRequest()
				req.EvidenceRef = protocol.Hash("ev-orchestrate")
				req.ViewKey = protocol.EncryptedViewKey("vk-orchestrate")
				req.Seed = protocol.VRFSeed("seed-orchestrate")
				req.ReleaseSignature = protocol.Signature("sig-orchestrate")
				return orch.ExecuteEscrowArbitrationFlow(req)
			},
			expectedNormalized: OrchestrationErrorCategoryNone,
			expectedTraces: []flowTraceExpectation{
				{step: "trigger_dispute", precompile: vm.O2ULPrecompileEscrowDispute.Hex(), success: true},
				{step: "select_arbitrators", precompile: common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(), success: true},
				{step: "rule_arbitration", precompile: vm.O2ULPrecompileArbitrationRule.Hex(), success: true},
				{step: "settle_escrow", precompile: vm.O2ULPrecompileEscrowSettle.Hex(), success: true},
			},
			validateOutput: func(t *testing.T, out EscrowArbitrationOrchestrationResult) {
				if out.DisputeID == "" {
					t.Fatal("expected dispute id from orchestration flow")
				}
				if out.SettlementType != "release" {
					t.Fatalf("expected release settlement type, got %q", out.SettlementType)
				}
				if len(out.Selected) != 1 {
					t.Fatalf("expected one selected arbitrator, got %d", len(out.Selected))
				}
			},
		},
	})
}

func TestPrecompileContractOrchestratorGetBatchDisputeStatusesDeterministicOrder(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)
	flow, err := orch.ExecuteEscrowArbitrationFlow(EscrowArbitrationOrchestrationRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		EvidenceRef:      protocol.Hash("ev-orchestrate-status"),
		ViewKey:          protocol.EncryptedViewKey("vk-orchestrate-status"),
		DisputeFee:       protocol.Amount{Value: big.NewInt(0)},
		Seed:             protocol.VRFSeed("seed-orchestrate-status"),
		ArbitratorCount:  1,
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-orchestrate-status"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("execute orchestration flow for status test: %v", err)
	}

	batch, err := orch.GetBatchDisputeStatuses(pblockchain.DisputeLifecycleBatchStatusRequest{
		DisputeIDs: []protocol.DisputeID{"unknown-orchestrate", flow.DisputeID, flow.DisputeID},
	})
	if err != nil {
		t.Fatalf("get batch dispute statuses: %v", err)
	}
	if len(batch.Statuses) != 2 {
		t.Fatalf("expected 2 deduped statuses, got %d", len(batch.Statuses))
	}

	ids := make([]string, 0, len(batch.Statuses))
	statusByID := make(map[protocol.DisputeID]pblockchain.DisputeLifecycleStatusResult, len(batch.Statuses))
	for _, status := range batch.Statuses {
		ids = append(ids, string(status.DisputeID))
		statusByID[status.DisputeID] = status
	}
	expected := append([]string(nil), ids...)
	sort.Strings(expected)
	if !reflect.DeepEqual(ids, expected) {
		t.Fatalf("expected deterministic sorted status ids %v, got %v", expected, ids)
	}

	if st := statusByID[flow.DisputeID]; st.Stage != "settled" || !st.Settled {
		t.Fatalf("expected settled status for orchestrated dispute, got %+v", st)
	}
	if st := statusByID[protocol.DisputeID("unknown-orchestrate")]; st.Stage != "unknown" || st.Settled {
		t.Fatalf("expected unknown status for unknown dispute id, got %+v", st)
	}
}

func TestPrecompileContractOrchestratorValidationErrors(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)
	if out, err := orch.ExecuteEscrowArbitrationFlow(EscrowArbitrationOrchestrationRequest{ArbitratorCount: 0}); err == nil {
		t.Fatal("expected arbitration count validation error")
	} else {
		if !errors.Is(err, ErrOrchestrationValidation) {
			t.Fatalf("expected orchestration validation error, got %v", err)
		}
		if out.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryValidation {
			t.Fatalf("expected validation diagnostics category, got %q", out.Diagnostics.NormalizedErrorCategory)
		}
	}
	if _, err := orch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(0)}}); err == nil {
		t.Fatal("expected allocate fee validation error")
	} else if !errors.Is(err, ErrOrchestrationValidation) {
		t.Fatalf("expected orchestration validation error, got %v", err)
	}
	if err := orch.SubmitArbitrationEvidence(pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: "", EvidenceRef: protocol.Hash("ev"), ViewKey: protocol.EncryptedViewKey("vk")}); err == nil {
		t.Fatal("expected submit arbitration evidence validation error")
	} else if !errors.Is(err, ErrOrchestrationValidation) {
		t.Fatalf("expected orchestration validation error, got %v", err)
	}

	nilOrch := &PrecompileContractOrchestrator{}
	if _, err := nilOrch.GetBatchDisputeStatuses(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{"x"}}); err == nil {
		t.Fatal("expected precompile set not configured error")
	}
	if _, err := nilOrch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(1)}}); err == nil {
		t.Fatal("expected allocate fee precompile set error")
	}
	if err := nilOrch.SubmitArbitrationEvidence(pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: "x", EvidenceRef: protocol.Hash("ev"), ViewKey: protocol.EncryptedViewKey("vk")}); err == nil {
		t.Fatal("expected submit arbitration evidence precompile set error")
	}
}

func TestPrecompileContractOrchestratorDiagnosticsExecutionCategory(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(newFlowSuccessFixturePrecompiles())
	assertFlowDiagnosticsCases(t, []flowDiagnosticsCase{
		{
			name: "execution",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				req := newFlowFixtureRequest()
				req.EvidenceRef = protocol.Hash("ev-diagnostics-execution")
				req.ViewKey = protocol.EncryptedViewKey("vk-diagnostics-execution")
				req.Seed = protocol.VRFSeed("seed-diagnostics-execution")
				req.Decision = protocol.Decision("unsupported")
				req.ReleaseSignature = protocol.Signature("sig-diagnostics-execution")
				return orch.ExecuteEscrowArbitrationFlow(req)
			},
			expectError:        true,
			expectedNormalized: OrchestrationErrorCategoryExecution,
			expectedTraces: []flowTraceExpectation{
				{step: "trigger_dispute", precompile: vm.O2ULPrecompileEscrowDispute.Hex(), success: true},
				{step: "select_arbitrators", precompile: common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(), success: true},
				{step: "rule_arbitration", precompile: vm.O2ULPrecompileArbitrationRule.Hex(), success: true},
				{step: "settle_escrow", precompile: vm.O2ULPrecompileEscrowSettle.Hex(), success: false, errorCategory: OrchestrationErrorCategoryExecution},
			},
		},
	})
}

func TestPrecompileContractOrchestratorDiagnosticsResponseDecodeCategory(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(newFlowDecodeFixturePrecompiles())

	assertFlowDiagnosticsCases(t, []flowDiagnosticsCase{
		{
			name: "response_decode",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				req := newFlowFixtureRequest()
				req.EvidenceRef = protocol.Hash("ev-decode-fixture")
				req.ViewKey = protocol.EncryptedViewKey("vk-decode-fixture")
				req.Seed = protocol.VRFSeed("seed-decode-fixture")
				req.ReleaseSignature = protocol.Signature("sig-decode-fixture")
				return orch.ExecuteEscrowArbitrationFlow(req)
			},
			expectError:        true,
			expectedErr:        ErrOrchestrationResponseDecode,
			expectedNormalized: OrchestrationErrorCategoryResponseDecode,
			expectedTraces: []flowTraceExpectation{
				{step: "trigger_dispute", precompile: vm.O2ULPrecompileEscrowDispute.Hex(), success: false, errorCategory: OrchestrationErrorCategoryResponseDecode},
			},
		},
	})
}

func TestPrecompileContractOrchestratorDiagnosticsResponseShapeCategory(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(newFlowShapeFixturePrecompiles())

	assertFlowDiagnosticsCases(t, []flowDiagnosticsCase{
		{
			name: "response_shape",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				req := newFlowFixtureRequest()
				req.EvidenceRef = protocol.Hash("ev-shape-fixture")
				req.ViewKey = protocol.EncryptedViewKey("vk-shape-fixture")
				req.Seed = protocol.VRFSeed("seed-shape-fixture")
				req.ReleaseSignature = protocol.Signature("sig-shape-fixture")
				return orch.ExecuteEscrowArbitrationFlow(req)
			},
			expectError:        true,
			expectedErr:        ErrOrchestrationResponseShape,
			expectedNormalized: OrchestrationErrorCategoryResponseShape,
			expectedTraces: []flowTraceExpectation{
				{step: "trigger_dispute", precompile: vm.O2ULPrecompileEscrowDispute.Hex(), success: true},
				{step: "select_arbitrators", precompile: common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(), success: false, errorCategory: OrchestrationErrorCategoryResponseShape},
			},
		},
	})
}

func TestPrecompileContractOrchestratorDiagnosticsPrecompileMissingCategory(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(newFlowMissingFixturePrecompiles())

	assertFlowDiagnosticsCases(t, []flowDiagnosticsCase{
		{
			name: "precompile_missing",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				req := newFlowFixtureRequest()
				req.EvidenceRef = protocol.Hash("ev-missing-fixture")
				req.ViewKey = protocol.EncryptedViewKey("vk-missing-fixture")
				req.Seed = protocol.VRFSeed("seed-missing-fixture")
				req.ReleaseSignature = protocol.Signature("sig-missing-fixture")
				return orch.ExecuteEscrowArbitrationFlow(req)
			},
			expectError:        true,
			expectedErr:        ErrOrchestrationPrecompileMissing,
			expectedNormalized: OrchestrationErrorCategoryPrecompileMiss,
			expectedTraces: []flowTraceExpectation{
				{step: "trigger_dispute", precompile: vm.O2ULPrecompileEscrowDispute.Hex(), success: true},
				{step: "select_arbitrators", precompile: common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(), success: false, errorCategory: OrchestrationErrorCategoryPrecompileMiss},
			},
		},
	})
}

func TestPrecompileContractOrchestratorFlowDiagnosticsFixtureParityAndErrorIdentity(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	successPrecompiles := newFlowSuccessFixturePrecompiles()
	decodePrecompiles := newFlowDecodeFixturePrecompiles()
	shapePrecompiles := newFlowShapeFixturePrecompiles()
	missingPrecompiles := newFlowMissingFixturePrecompiles()

	baseReq := newFlowFixtureRequest()
	baseReq.EvidenceRef = protocol.Hash("ev-fixture-parity")
	baseReq.ViewKey = protocol.EncryptedViewKey("vk-fixture-parity")
	baseReq.Seed = protocol.VRFSeed("seed-fixture-parity")
	baseReq.ReleaseSignature = protocol.Signature("sig-fixture-parity")

	canonicalOrder := []string{"trigger_dispute", "select_arbitrators", "rule_arbitration", "settle_escrow"}
	canonicalPrecompileOrder := []string{vm.O2ULPrecompileEscrowDispute.Hex(), common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(), vm.O2ULPrecompileArbitrationRule.Hex(), vm.O2ULPrecompileEscrowSettle.Hex()}
	fixtures := []struct {
		name              string
		invoke            func() (EscrowArbitrationOrchestrationResult, error)
		expectedErr       error
		expectedCategory  string
		expectedFailIndex int
		expectedFailStep  string
		expectedFailPC    string
	}{
		{
			name: "execution",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				orch := NewPrecompileContractOrchestrator(successPrecompiles)
				req := baseReq
				req.Decision = protocol.Decision("unsupported")
				return orch.ExecuteEscrowArbitrationFlow(req)
			},
			expectedCategory:  OrchestrationErrorCategoryExecution,
			expectedFailIndex: 3,
			expectedFailStep:  "settle_escrow",
			expectedFailPC:    vm.O2ULPrecompileEscrowSettle.Hex(),
		},
		{
			name: "response_decode",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				orch := NewPrecompileContractOrchestrator(decodePrecompiles)
				return orch.ExecuteEscrowArbitrationFlow(baseReq)
			},
			expectedErr:       ErrOrchestrationResponseDecode,
			expectedCategory:  OrchestrationErrorCategoryResponseDecode,
			expectedFailIndex: 0,
			expectedFailStep:  "trigger_dispute",
			expectedFailPC:    vm.O2ULPrecompileEscrowDispute.Hex(),
		},
		{
			name: "response_shape",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				orch := NewPrecompileContractOrchestrator(shapePrecompiles)
				return orch.ExecuteEscrowArbitrationFlow(baseReq)
			},
			expectedErr:       ErrOrchestrationResponseShape,
			expectedCategory:  OrchestrationErrorCategoryResponseShape,
			expectedFailIndex: 1,
			expectedFailStep:  "select_arbitrators",
			expectedFailPC:    common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(),
		},
		{
			name: "precompile_missing",
			invoke: func() (EscrowArbitrationOrchestrationResult, error) {
				orch := NewPrecompileContractOrchestrator(missingPrecompiles)
				return orch.ExecuteEscrowArbitrationFlow(baseReq)
			},
			expectedErr:       ErrOrchestrationPrecompileMissing,
			expectedCategory:  OrchestrationErrorCategoryPrecompileMiss,
			expectedFailIndex: 1,
			expectedFailStep:  "select_arbitrators",
			expectedFailPC:    common.Address(vm.O2ULPrecompileArbitrationSelect).Hex(),
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			out, err := fixture.invoke()
			if err == nil {
				t.Fatal("expected flow fixture failure")
			}
			if fixture.expectedErr != nil && !errors.Is(err, fixture.expectedErr) {
				t.Fatalf("expected errors.Is(err, %v), got %v", fixture.expectedErr, err)
			}
			if out.Diagnostics.NormalizedErrorCategory != fixture.expectedCategory {
				t.Fatalf("expected category %q, got %q", fixture.expectedCategory, out.Diagnostics.NormalizedErrorCategory)
			}
			if normalizeOrchestrationError(err) != fixture.expectedCategory {
				t.Fatalf("expected normalized runtime error category %q, got %q", fixture.expectedCategory, normalizeOrchestrationError(err))
			}
			if len(out.Diagnostics.StepTraces) != fixture.expectedFailIndex+1 {
				t.Fatalf("expected %d traces, got %d", fixture.expectedFailIndex+1, len(out.Diagnostics.StepTraces))
			}

			for i, trace := range out.Diagnostics.StepTraces {
				if trace.Step != canonicalOrder[i] {
					t.Fatalf("expected step order %q at index %d, got %q", canonicalOrder[i], i, trace.Step)
				}
				if trace.Precompile != canonicalPrecompileOrder[i] {
					t.Fatalf("expected precompile %q at index %d, got %q", canonicalPrecompileOrder[i], i, trace.Precompile)
				}
				if i < fixture.expectedFailIndex {
					if !trace.Success || trace.ErrorCategory != "" {
						t.Fatalf("expected successful prefix trace at index %d, got %+v", i, trace)
					}
					continue
				}
				if trace.Step != fixture.expectedFailStep {
					t.Fatalf("expected failing step %q, got %q", fixture.expectedFailStep, trace.Step)
				}
				if trace.Precompile != fixture.expectedFailPC {
					t.Fatalf("expected failing step precompile %q, got %q", fixture.expectedFailPC, trace.Precompile)
				}
				if trace.Success {
					t.Fatalf("expected failing trace at index %d, got %+v", i, trace)
				}
				if trace.ErrorCategory != fixture.expectedCategory {
					t.Fatalf("expected failing trace category %q, got %q", fixture.expectedCategory, trace.ErrorCategory)
				}
			}
		})
	}
}

func TestPrecompileContractOrchestratorAllocateFeePositivePath(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)
	out, err := orch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
	if err != nil {
		t.Fatalf("allocate fee: %v", err)
	}
	if out.Total.Value == nil || out.Total.Value.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("expected total 100, got %+v", out.Total)
	}
	if out.ProversValidators.Value == nil || out.ArbitratorPool.Value == nil || out.DevTreasury.Value == nil || out.Burn.Value == nil {
		t.Fatalf("expected all fee buckets populated, got %+v", out)
	}
}

func TestPrecompileContractOrchestratorAllocateFeeWithDiagnostics(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)
	out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
	if err != nil {
		t.Fatalf("allocate fee with diagnostics: %v", err)
	}
	if out.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryNone {
		t.Fatalf("expected none diagnostics category, got %q", out.Diagnostics.NormalizedErrorCategory)
	}
	if len(out.Diagnostics.StepTraces) != 1 {
		t.Fatalf("expected single diagnostics trace for allocate fee, got %d", len(out.Diagnostics.StepTraces))
	}
	trace := out.Diagnostics.StepTraces[0]
	if trace.Step != "allocate_fee" || !trace.Success || trace.ErrorCategory != "" {
		t.Fatalf("unexpected allocate fee diagnostics trace: %+v", trace)
	}
}

func TestPrecompileContractOrchestratorNonFlowDiagnosticsWrappersCategories(t *testing.T) {
	nilOrch := &PrecompileContractOrchestrator{}

	allocOut, err := nilOrch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(1)}})
	if err == nil {
		t.Fatal("expected allocate fee config failure")
	}
	if allocOut.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryConfiguration {
		t.Fatalf("expected allocate fee configuration category, got %q", allocOut.Diagnostics.NormalizedErrorCategory)
	}

	evidenceOut, err := nilOrch.SubmitArbitrationEvidenceWithDiagnostics(pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: "d", EvidenceRef: protocol.Hash("ev"), ViewKey: protocol.EncryptedViewKey("vk")})
	if err == nil {
		t.Fatal("expected evidence config failure")
	}
	if evidenceOut.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryConfiguration {
		t.Fatalf("expected evidence configuration category, got %q", evidenceOut.Diagnostics.NormalizedErrorCategory)
	}

	statusOut, err := nilOrch.GetBatchDisputeStatusesWithDiagnostics(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{"x"}})
	if err == nil {
		t.Fatal("expected batch status config failure")
	}
	if statusOut.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryConfiguration {
		t.Fatalf("expected batch status configuration category, got %q", statusOut.Diagnostics.NormalizedErrorCategory)
	}
}

func TestPrecompileContractOrchestratorNonFlowLegacyWrapperParity(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)

	legacyAlloc, err := orch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
	if err != nil {
		t.Fatalf("legacy allocate fee: %v", err)
	}
	diagAlloc, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
	if err != nil {
		t.Fatalf("diagnostics allocate fee: %v", err)
	}
	if !reflect.DeepEqual(legacyAlloc, diagAlloc.Distribution) {
		t.Fatalf("allocate fee parity mismatch, legacy=%+v diagnostics=%+v", legacyAlloc, diagAlloc.Distribution)
	}
	if diagAlloc.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryNone {
		t.Fatalf("expected allocate fee diagnostics category none, got %q", diagAlloc.Diagnostics.NormalizedErrorCategory)
	}

	flow, err := orch.ExecuteEscrowArbitrationFlow(EscrowArbitrationOrchestrationRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		EvidenceRef:      protocol.Hash("ev-parity-replay"),
		ViewKey:          protocol.EncryptedViewKey("vk-parity-replay"),
		DisputeFee:       protocol.Amount{Value: big.NewInt(0)},
		Seed:             protocol.VRFSeed("seed-parity-replay"),
		ArbitratorCount:  1,
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-parity-replay"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("execute orchestration flow for parity replay: %v", err)
	}

	replayReq := pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: flow.DisputeID, EvidenceRef: protocol.Hash("ev-parity-replay"), ViewKey: protocol.EncryptedViewKey("vk-parity-replay")}
	batchReq := pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{flow.DisputeID, "unknown-parity", flow.DisputeID}}
	nilOrch := &PrecompileContractOrchestrator{}

	assertNonFlowLegacyWrapperParityCases(t, []nonFlowLegacyWrapperParityCase{
		{
			name: "allocate_fee_success",
			legacy: func() (any, error) {
				return orch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
			},
			wrapper: func() (any, OrchestrationDiagnostics, error) {
				out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
				return out.Distribution, out.Diagnostics, err
			},
			expectedNormalized: OrchestrationErrorCategoryNone,
			expectedStep:       "allocate_fee",
			expectedPrecompile: vm.O2ULPrecompileFeeAllocate.Hex(),
			expectedSuccess:    true,
		},
		{
			name: "batch_status_success",
			legacy: func() (any, error) {
				return orch.GetBatchDisputeStatuses(batchReq)
			},
			wrapper: func() (any, OrchestrationDiagnostics, error) {
				out, err := orch.GetBatchDisputeStatusesWithDiagnostics(batchReq)
				return out.Statuses, out.Diagnostics, err
			},
			expectedNormalized: OrchestrationErrorCategoryNone,
			expectedStep:       "get_batch_dispute_statuses",
			expectedPrecompile: vm.O2ULPrecompileDisputeStatusBatch.Hex(),
			expectedSuccess:    true,
		},
		{
			name: "submit_evidence_replay_execution_error",
			legacy: func() (any, error) {
				return nil, orch.SubmitArbitrationEvidence(replayReq)
			},
			wrapper: func() (any, OrchestrationDiagnostics, error) {
				out, err := orch.SubmitArbitrationEvidenceWithDiagnostics(replayReq)
				return nil, out.Diagnostics, err
			},
			expectedNormalized:    OrchestrationErrorCategoryExecution,
			expectedStep:          "submit_arbitration_evidence",
			expectedPrecompile:    vm.O2ULPrecompileArbitrationSubmit.Hex(),
			expectedSuccess:       false,
			expectedErrorCategory: OrchestrationErrorCategoryExecution,
		},
		{
			name: "allocate_fee_validation_error_identity",
			legacy: func() (any, error) {
				return nil, func() error {
					_, err := orch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(0)}})
					return err
				}()
			},
			wrapper: func() (any, OrchestrationDiagnostics, error) {
				out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(0)}})
				return nil, out.Diagnostics, err
			},
			expectedErr:           ErrOrchestrationValidation,
			expectedNormalized:    OrchestrationErrorCategoryValidation,
			expectedStep:          "allocate_fee",
			expectedPrecompile:    vm.O2ULPrecompileFeeAllocate.Hex(),
			expectedSuccess:       false,
			expectedErrorCategory: OrchestrationErrorCategoryValidation,
		},
		{
			name: "allocate_fee_configuration_error_identity",
			legacy: func() (any, error) {
				return nil, func() error {
					_, err := nilOrch.AllocateFee(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(1)}})
					return err
				}()
			},
			wrapper: func() (any, OrchestrationDiagnostics, error) {
				out, err := nilOrch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(1)}})
				return nil, out.Diagnostics, err
			},
			expectedErr:           ErrOrchestrationPrecompileSetNotConfigured,
			expectedNormalized:    OrchestrationErrorCategoryConfiguration,
			expectedStep:          "allocate_fee",
			expectedPrecompile:    vm.O2ULPrecompileFeeAllocate.Hex(),
			expectedSuccess:       false,
			expectedErrorCategory: OrchestrationErrorCategoryConfiguration,
		},
	})
}

func TestPrecompileContractOrchestratorNonFlowWrapperPrecompileMissingRoutes(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContracts{})

	assertNonFlowWrapperCases(t, []nonFlowWrapperCase{
		{
			name: "allocate_fee",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(10)}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationPrecompileMissing, expectedNormalized: OrchestrationErrorCategoryPrecompileMiss,
			expectedStep: "allocate_fee", expectedPrecompile: vm.O2ULPrecompileFeeAllocate.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryPrecompileMiss,
		},
		{
			name: "submit_arbitration_evidence",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.SubmitArbitrationEvidenceWithDiagnostics(pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: "d", EvidenceRef: protocol.Hash("ev"), ViewKey: protocol.EncryptedViewKey("vk")})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationPrecompileMissing, expectedNormalized: OrchestrationErrorCategoryPrecompileMiss,
			expectedStep: "submit_arbitration_evidence", expectedPrecompile: vm.O2ULPrecompileArbitrationSubmit.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryPrecompileMiss,
		},
		{
			name: "get_batch_dispute_statuses",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.GetBatchDisputeStatusesWithDiagnostics(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{"x"}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationPrecompileMissing, expectedNormalized: OrchestrationErrorCategoryPrecompileMiss,
			expectedStep: "get_batch_dispute_statuses", expectedPrecompile: vm.O2ULPrecompileDisputeStatusBatch.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryPrecompileMiss,
		},
	})
}

func TestPrecompileContractOrchestratorNonFlowWrapperResponseDecodeRoutes(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContracts{
		vm.O2ULPrecompileFeeAllocate:        fixedTestPrecompile{output: []byte("{")},
		vm.O2ULPrecompileDisputeStatusBatch: fixedTestPrecompile{output: []byte("{")},
	})

	assertNonFlowWrapperCases(t, []nonFlowWrapperCase{
		{
			name: "allocate_fee",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(10)}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationResponseDecode, expectedNormalized: OrchestrationErrorCategoryResponseDecode,
			expectedStep: "allocate_fee", expectedPrecompile: vm.O2ULPrecompileFeeAllocate.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryResponseDecode,
		},
		{
			name: "get_batch_dispute_statuses",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.GetBatchDisputeStatusesWithDiagnostics(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{"x"}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationResponseDecode, expectedNormalized: OrchestrationErrorCategoryResponseDecode,
			expectedStep: "get_batch_dispute_statuses", expectedPrecompile: vm.O2ULPrecompileDisputeStatusBatch.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryResponseDecode,
		},
	})
}

func TestPrecompileContractOrchestratorNonFlowWrapperValidationStepShape(t *testing.T) {
	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)

	assertNonFlowWrapperCases(t, []nonFlowWrapperCase{
		{
			name: "allocate_fee",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(0)}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationValidation, expectedNormalized: OrchestrationErrorCategoryValidation,
			expectedStep: "allocate_fee", expectedPrecompile: vm.O2ULPrecompileFeeAllocate.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryValidation,
		},
		{
			name: "submit_arbitration_evidence",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.SubmitArbitrationEvidenceWithDiagnostics(pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: "", EvidenceRef: protocol.Hash("ev"), ViewKey: protocol.EncryptedViewKey("vk")})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationValidation, expectedNormalized: OrchestrationErrorCategoryValidation,
			expectedStep: "submit_arbitration_evidence", expectedPrecompile: vm.O2ULPrecompileArbitrationSubmit.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryValidation,
		},
	})
}

func TestPrecompileContractOrchestratorNonFlowWrapperConfigurationStepShape(t *testing.T) {
	nilOrch := &PrecompileContractOrchestrator{}

	assertNonFlowWrapperCases(t, []nonFlowWrapperCase{
		{
			name: "allocate_fee",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := nilOrch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(1)}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationPrecompileSetNotConfigured, expectedNormalized: OrchestrationErrorCategoryConfiguration,
			expectedStep: "allocate_fee", expectedPrecompile: vm.O2ULPrecompileFeeAllocate.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryConfiguration,
		},
		{
			name: "submit_arbitration_evidence",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := nilOrch.SubmitArbitrationEvidenceWithDiagnostics(pblockchain.ArbitrationSubmitEvidenceRequest{DisputeID: "d", EvidenceRef: protocol.Hash("ev"), ViewKey: protocol.EncryptedViewKey("vk")})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationPrecompileSetNotConfigured, expectedNormalized: OrchestrationErrorCategoryConfiguration,
			expectedStep: "submit_arbitration_evidence", expectedPrecompile: vm.O2ULPrecompileArbitrationSubmit.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryConfiguration,
		},
		{
			name: "get_batch_dispute_statuses",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := nilOrch.GetBatchDisputeStatusesWithDiagnostics(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{"x"}})
				return out.Diagnostics, err
			},
			expectedErr: ErrOrchestrationPrecompileSetNotConfigured, expectedNormalized: OrchestrationErrorCategoryConfiguration,
			expectedStep: "get_batch_dispute_statuses", expectedPrecompile: vm.O2ULPrecompileDisputeStatusBatch.Hex(), expectedSuccess: false, expectedErrorCategory: OrchestrationErrorCategoryConfiguration,
		},
	})
}

func TestPrecompileContractOrchestratorNonFlowWrapperSuccessStepShape(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)

	allocOut, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
	if err != nil {
		t.Fatalf("allocate fee with diagnostics success path: %v", err)
	}
	if allocOut.Diagnostics.NormalizedErrorCategory != OrchestrationErrorCategoryNone {
		t.Fatalf("expected allocate fee success category none, got %q", allocOut.Diagnostics.NormalizedErrorCategory)
	}
	if len(allocOut.Diagnostics.StepTraces) != 1 {
		t.Fatalf("expected single allocate fee success trace, got %d", len(allocOut.Diagnostics.StepTraces))
	}
	allocTrace := allocOut.Diagnostics.StepTraces[0]
	if allocTrace.Step != "allocate_fee" || allocTrace.Precompile != vm.O2ULPrecompileFeeAllocate.Hex() || !allocTrace.Success || allocTrace.ErrorCategory != "" {
		t.Fatalf("unexpected allocate fee success trace: %+v", allocTrace)
	}

	evidenceOrch := NewPrecompileContractOrchestrator(vm.PrecompiledContracts{
		vm.O2ULPrecompileArbitrationSubmit: fixedTestPrecompile{output: []byte(`{"ok":true}`)},
	})

	assertNonFlowWrapperCases(t, []nonFlowWrapperCase{
		{
			name: "allocate_fee",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.AllocateFeeWithDiagnostics(pblockchain.AllocateFeeRequest{Total: protocol.Amount{Value: big.NewInt(100)}})
				return out.Diagnostics, err
			},
			expectedErr: nil, expectedNormalized: OrchestrationErrorCategoryNone,
			expectedStep: "allocate_fee", expectedPrecompile: vm.O2ULPrecompileFeeAllocate.Hex(), expectedSuccess: true, expectedErrorCategory: "",
		},
		{
			name: "submit_arbitration_evidence",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := evidenceOrch.SubmitArbitrationEvidenceWithDiagnostics(pblockchain.ArbitrationSubmitEvidenceRequest{
					DisputeID:   protocol.DisputeID("success-dispute"),
					EvidenceRef: protocol.Hash("ev-success-shape"),
					ViewKey:     protocol.EncryptedViewKey("vk-success-shape"),
				})
				return out.Diagnostics, err
			},
			expectedErr: nil, expectedNormalized: OrchestrationErrorCategoryNone,
			expectedStep: "submit_arbitration_evidence", expectedPrecompile: vm.O2ULPrecompileArbitrationSubmit.Hex(), expectedSuccess: true, expectedErrorCategory: "",
		},
		{
			name: "get_batch_dispute_statuses",
			invoke: func() (OrchestrationDiagnostics, error) {
				out, err := orch.GetBatchDisputeStatusesWithDiagnostics(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{"unknown-success-shape"}})
				return out.Diagnostics, err
			},
			expectedErr: nil, expectedNormalized: OrchestrationErrorCategoryNone,
			expectedStep: "get_batch_dispute_statuses", expectedPrecompile: vm.O2ULPrecompileDisputeStatusBatch.Hex(), expectedSuccess: true, expectedErrorCategory: "",
		},
	})
}

func TestPrecompileContractOrchestratorSubmitArbitrationEvidencePolicyPath(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	orch := NewPrecompileContractOrchestrator(vm.PrecompiledContractsPrague)
	flow, err := orch.ExecuteEscrowArbitrationFlow(EscrowArbitrationOrchestrationRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		EvidenceRef:      protocol.Hash("ev-evidence-policy"),
		ViewKey:          protocol.EncryptedViewKey("vk-evidence-policy"),
		DisputeFee:       protocol.Amount{Value: big.NewInt(0)},
		Seed:             protocol.VRFSeed("seed-evidence-policy"),
		ArbitratorCount:  1,
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-evidence-policy"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("execute orchestration flow for evidence policy test: %v", err)
	}

	if err := orch.SubmitArbitrationEvidence(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   flow.DisputeID,
		EvidenceRef: protocol.Hash("ev-evidence-policy"),
		ViewKey:     protocol.EncryptedViewKey("vk-evidence-policy"),
	}); err == nil {
		t.Fatal("expected settled dispute evidence replay rejection")
	}
}
