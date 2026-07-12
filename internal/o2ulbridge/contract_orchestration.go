package o2ulbridge

import (
	"encoding/json"
	"errors"
	"fmt"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/fees"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

var (
	ErrOrchestrationPrecompileSetNotConfigured = errors.New("orchestration precompile set is not configured")
	ErrOrchestrationPrecompileMissing          = errors.New("required orchestration precompile is missing")
	ErrOrchestrationValidation                 = errors.New("orchestration validation failed")
	ErrOrchestrationResponseDecode             = errors.New("orchestration response decode failed")
	ErrOrchestrationResponseShape              = errors.New("orchestration response shape is invalid")
)

const (
	OrchestrationErrorCategoryNone           = "none"
	OrchestrationErrorCategoryValidation     = "validation"
	OrchestrationErrorCategoryConfiguration  = "configuration"
	OrchestrationErrorCategoryPrecompileMiss = "precompile_missing"
	OrchestrationErrorCategoryResponseDecode = "response_decode"
	OrchestrationErrorCategoryResponseShape  = "response_shape"
	OrchestrationErrorCategoryExecution      = "execution"
)

type EscrowArbitrationOrchestrationRequest struct {
	Escrow           protocol.EscrowNote
	EvidenceRef      protocol.Hash
	ViewKey          protocol.EncryptedViewKey
	DisputeFee       protocol.Amount
	Seed             protocol.VRFSeed
	ArbitratorCount  int
	Decision         protocol.Decision
	ReleaseSignature protocol.Signature
	SettlementFee    protocol.Amount
}

type EscrowArbitrationOrchestrationResult struct {
	DisputeID      protocol.DisputeID
	Selected       []protocol.NodeID
	SettlementType string
	FeeAllocated   bool
	Diagnostics    OrchestrationDiagnostics
}

type OrchestrationStepTrace struct {
	Step          string
	Precompile    string
	Success       bool
	ErrorCategory string
}

type OrchestrationDiagnostics struct {
	StepTraces              []OrchestrationStepTrace
	NormalizedErrorCategory string
}

type AllocateFeeOrchestrationResult struct {
	Distribution fees.FeeDistribution
	Diagnostics  OrchestrationDiagnostics
}

type SubmitArbitrationEvidenceOrchestrationResult struct {
	Diagnostics OrchestrationDiagnostics
}

type BatchDisputeStatusesOrchestrationResult struct {
	Statuses    pblockchain.DisputeLifecycleBatchStatusResult
	Diagnostics OrchestrationDiagnostics
}

type ContractOrchestrator interface {
	ExecuteEscrowArbitrationFlow(req EscrowArbitrationOrchestrationRequest) (EscrowArbitrationOrchestrationResult, error)
	// Legacy method preserved for compatibility; deployment tooling should prefer AllocateFeeWithDiagnostics.
	// Example: res, err := orch.AllocateFeeWithDiagnostics(req); category := res.Diagnostics.NormalizedErrorCategory
	AllocateFee(req pblockchain.AllocateFeeRequest) (fees.FeeDistribution, error)
	AllocateFeeWithDiagnostics(req pblockchain.AllocateFeeRequest) (AllocateFeeOrchestrationResult, error)
	// Legacy method preserved for compatibility; deployment tooling should prefer SubmitArbitrationEvidenceWithDiagnostics.
	// Example: res, err := orch.SubmitArbitrationEvidenceWithDiagnostics(req); trace := res.Diagnostics.StepTraces
	SubmitArbitrationEvidence(req pblockchain.ArbitrationSubmitEvidenceRequest) error
	SubmitArbitrationEvidenceWithDiagnostics(req pblockchain.ArbitrationSubmitEvidenceRequest) (SubmitArbitrationEvidenceOrchestrationResult, error)
	// Legacy method preserved for compatibility; deployment tooling should prefer GetBatchDisputeStatusesWithDiagnostics.
	// Example: res, err := orch.GetBatchDisputeStatusesWithDiagnostics(req); statuses := res.Statuses.Statuses
	GetBatchDisputeStatuses(req pblockchain.DisputeLifecycleBatchStatusRequest) (pblockchain.DisputeLifecycleBatchStatusResult, error)
	GetBatchDisputeStatusesWithDiagnostics(req pblockchain.DisputeLifecycleBatchStatusRequest) (BatchDisputeStatusesOrchestrationResult, error)
}

type PrecompileContractOrchestrator struct {
	precompiles vm.PrecompiledContracts
}

func NewPrecompileContractOrchestrator(precompiles vm.PrecompiledContracts) *PrecompileContractOrchestrator {
	if precompiles == nil {
		precompiles = vm.PrecompiledContractsPrague
	}
	return &PrecompileContractOrchestrator{precompiles: precompiles}
}

func (o *PrecompileContractOrchestrator) ExecuteEscrowArbitrationFlow(req EscrowArbitrationOrchestrationRequest) (EscrowArbitrationOrchestrationResult, error) {
	if o == nil || o.precompiles == nil {
		return EscrowArbitrationOrchestrationResult{
			Diagnostics: orchestrationDiagnostics(nil, ErrOrchestrationPrecompileSetNotConfigured),
		}, ErrOrchestrationPrecompileSetNotConfigured
	}
	if req.ArbitratorCount <= 0 {
		err := fmt.Errorf("%w: arbitrator count must be positive", ErrOrchestrationValidation)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(nil, err)}, err
	}

	stepTraces := make([]OrchestrationStepTrace, 0, 4)
	appendStep := func(step string, address common.Address) {
		stepTraces = append(stepTraces, OrchestrationStepTrace{Step: step, Precompile: address.Hex()})
	}
	markSuccess := func() {
		stepTraces[len(stepTraces)-1].Success = true
	}
	markStepError := func(err error) {
		stepTraces[len(stepTraces)-1].ErrorCategory = normalizeOrchestrationError(err)
	}

	appendStep("trigger_dispute", vm.O2ULPrecompileEscrowDispute)
	triggerInput, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
		Escrow:      req.Escrow,
		EvidenceRef: req.EvidenceRef,
		ViewKey:     req.ViewKey,
		DisputeFee:  req.DisputeFee,
	})
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	triggerOutput, err := o.runPrecompile(vm.O2ULPrecompileEscrowDispute, triggerInput)
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	var triggerResp struct {
		DisputeID string `json:"disputeId"`
	}
	if err := json.Unmarshal(triggerOutput, &triggerResp); err != nil {
		decodeErr := fmt.Errorf("%w: %v", ErrOrchestrationResponseDecode, err)
		markStepError(decodeErr)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, decodeErr)}, decodeErr
	}
	if triggerResp.DisputeID == "" {
		shapeErr := fmt.Errorf("%w: dispute id is required in trigger response", ErrOrchestrationResponseShape)
		markStepError(shapeErr)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, shapeErr)}, shapeErr
	}
	markSuccess()

	disputeID := protocol.DisputeID(triggerResp.DisputeID)
	appendStep("select_arbitrators", vm.O2ULPrecompileArbitrationSelect)
	selectInput, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: disputeID,
		Seed:      req.Seed,
		Count:     req.ArbitratorCount,
	})
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	selectOutput, err := o.runPrecompile(vm.O2ULPrecompileArbitrationSelect, selectInput)
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	var selectResp struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOutput, &selectResp); err != nil {
		decodeErr := fmt.Errorf("%w: %v", ErrOrchestrationResponseDecode, err)
		markStepError(decodeErr)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, decodeErr)}, decodeErr
	}
	if len(selectResp.Selected) == 0 {
		shapeErr := fmt.Errorf("%w: selected arbitrators are required in select response", ErrOrchestrationResponseShape)
		markStepError(shapeErr)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, shapeErr)}, shapeErr
	}
	markSuccess()

	appendStep("rule_arbitration", vm.O2ULPrecompileArbitrationRule)
	ruleInput, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  disputeID,
		Arbitrator: protocol.NodeID(selectResp.Selected[0]),
		Decision:   req.Decision,
	})
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	if _, err := o.runPrecompile(vm.O2ULPrecompileArbitrationRule, ruleInput); err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	markSuccess()

	appendStep("settle_escrow", vm.O2ULPrecompileEscrowSettle)
	settleInput, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID:        disputeID,
		Escrow:           req.Escrow,
		Decision:         req.Decision,
		ReleaseSignature: req.ReleaseSignature,
		SettlementFee:    req.SettlementFee,
	})
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	settleOutput, err := o.runPrecompile(vm.O2ULPrecompileEscrowSettle, settleInput)
	if err != nil {
		markStepError(err)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, err)}, err
	}
	var settleResp struct {
		SettlementType string `json:"settlementType"`
		FeeAllocated   bool   `json:"feeAllocated"`
	}
	if err := json.Unmarshal(settleOutput, &settleResp); err != nil {
		decodeErr := fmt.Errorf("%w: %v", ErrOrchestrationResponseDecode, err)
		markStepError(decodeErr)
		return EscrowArbitrationOrchestrationResult{Diagnostics: orchestrationDiagnostics(stepTraces, decodeErr)}, decodeErr
	}
	markSuccess()

	selected := make([]protocol.NodeID, 0, len(selectResp.Selected))
	for _, node := range selectResp.Selected {
		selected = append(selected, protocol.NodeID(node))
	}

	return EscrowArbitrationOrchestrationResult{
		DisputeID:      disputeID,
		Selected:       selected,
		SettlementType: settleResp.SettlementType,
		FeeAllocated:   settleResp.FeeAllocated,
		Diagnostics:    orchestrationDiagnostics(stepTraces, nil),
	}, nil
}

func (o *PrecompileContractOrchestrator) AllocateFee(req pblockchain.AllocateFeeRequest) (fees.FeeDistribution, error) {
	out, err := o.AllocateFeeWithDiagnostics(req)
	if err != nil {
		return fees.FeeDistribution{}, err
	}
	return out.Distribution, nil
}

func (o *PrecompileContractOrchestrator) AllocateFeeWithDiagnostics(req pblockchain.AllocateFeeRequest) (AllocateFeeOrchestrationResult, error) {
	step := OrchestrationStepTrace{Step: "allocate_fee", Precompile: vm.O2ULPrecompileFeeAllocate.Hex()}
	if o == nil || o.precompiles == nil {
		step.ErrorCategory = OrchestrationErrorCategoryConfiguration
		return AllocateFeeOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, ErrOrchestrationPrecompileSetNotConfigured)}, ErrOrchestrationPrecompileSetNotConfigured
	}
	if req.Total.Value == nil || req.Total.Value.Sign() <= 0 {
		err := fmt.Errorf("%w: total must be positive", ErrOrchestrationValidation)
		step.ErrorCategory = OrchestrationErrorCategoryValidation
		return AllocateFeeOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	input, err := json.Marshal(req)
	if err != nil {
		step.ErrorCategory = normalizeOrchestrationError(err)
		return AllocateFeeOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	out, err := o.runPrecompile(vm.O2ULPrecompileFeeAllocate, input)
	if err != nil {
		step.ErrorCategory = normalizeOrchestrationError(err)
		return AllocateFeeOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	var resp struct {
		Total             protocol.Amount `json:"total"`
		ProversValidators protocol.Amount `json:"proversValidators"`
		ArbitratorPool    protocol.Amount `json:"arbitratorPool"`
		DevTreasury       protocol.Amount `json:"devTreasury"`
		Burn              protocol.Amount `json:"burn"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		decodeErr := fmt.Errorf("%w: %v", ErrOrchestrationResponseDecode, err)
		step.ErrorCategory = OrchestrationErrorCategoryResponseDecode
		return AllocateFeeOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, decodeErr)}, decodeErr
	}
	step.Success = true
	return AllocateFeeOrchestrationResult{
		Distribution: fees.FeeDistribution{
			Total:             resp.Total,
			ProversValidators: resp.ProversValidators,
			ArbitratorPool:    resp.ArbitratorPool,
			DevTreasury:       resp.DevTreasury,
			Burn:              resp.Burn,
		},
		Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, nil),
	}, nil
}

func (o *PrecompileContractOrchestrator) SubmitArbitrationEvidence(req pblockchain.ArbitrationSubmitEvidenceRequest) error {
	out, err := o.SubmitArbitrationEvidenceWithDiagnostics(req)
	if err != nil {
		return err
	}
	_ = out
	return nil
}

func (o *PrecompileContractOrchestrator) SubmitArbitrationEvidenceWithDiagnostics(req pblockchain.ArbitrationSubmitEvidenceRequest) (SubmitArbitrationEvidenceOrchestrationResult, error) {
	step := OrchestrationStepTrace{Step: "submit_arbitration_evidence", Precompile: vm.O2ULPrecompileArbitrationSubmit.Hex()}
	if o == nil || o.precompiles == nil {
		step.ErrorCategory = OrchestrationErrorCategoryConfiguration
		return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, ErrOrchestrationPrecompileSetNotConfigured)}, ErrOrchestrationPrecompileSetNotConfigured
	}
	if req.DisputeID == "" {
		err := fmt.Errorf("%w: dispute id is required", ErrOrchestrationValidation)
		step.ErrorCategory = OrchestrationErrorCategoryValidation
		return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	if len(req.EvidenceRef) == 0 {
		err := fmt.Errorf("%w: evidence reference is required", ErrOrchestrationValidation)
		step.ErrorCategory = OrchestrationErrorCategoryValidation
		return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	if len(req.ViewKey) == 0 {
		err := fmt.Errorf("%w: encrypted view key is required", ErrOrchestrationValidation)
		step.ErrorCategory = OrchestrationErrorCategoryValidation
		return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	input, err := json.Marshal(req)
	if err != nil {
		step.ErrorCategory = normalizeOrchestrationError(err)
		return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	_, err = o.runPrecompile(vm.O2ULPrecompileArbitrationSubmit, input)
	if err != nil {
		step.ErrorCategory = normalizeOrchestrationError(err)
		return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	step.Success = true
	return SubmitArbitrationEvidenceOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, nil)}, nil
}

func (o *PrecompileContractOrchestrator) GetBatchDisputeStatuses(req pblockchain.DisputeLifecycleBatchStatusRequest) (pblockchain.DisputeLifecycleBatchStatusResult, error) {
	out, err := o.GetBatchDisputeStatusesWithDiagnostics(req)
	if err != nil {
		return pblockchain.DisputeLifecycleBatchStatusResult{}, err
	}
	return out.Statuses, nil
}

func (o *PrecompileContractOrchestrator) GetBatchDisputeStatusesWithDiagnostics(req pblockchain.DisputeLifecycleBatchStatusRequest) (BatchDisputeStatusesOrchestrationResult, error) {
	step := OrchestrationStepTrace{Step: "get_batch_dispute_statuses", Precompile: vm.O2ULPrecompileDisputeStatusBatch.Hex()}
	if o == nil || o.precompiles == nil {
		step.ErrorCategory = OrchestrationErrorCategoryConfiguration
		return BatchDisputeStatusesOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, ErrOrchestrationPrecompileSetNotConfigured)}, ErrOrchestrationPrecompileSetNotConfigured
	}
	input, err := json.Marshal(req)
	if err != nil {
		step.ErrorCategory = normalizeOrchestrationError(err)
		return BatchDisputeStatusesOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	out, err := o.runPrecompile(vm.O2ULPrecompileDisputeStatusBatch, input)
	if err != nil {
		step.ErrorCategory = normalizeOrchestrationError(err)
		return BatchDisputeStatusesOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, err)}, err
	}
	var payload struct {
		Statuses []struct {
			DisputeID               string   `json:"disputeId"`
			Stage                   string   `json:"stage"`
			KnownToEscrowRegistry   bool     `json:"knownToEscrowRegistry"`
			EvidenceRecorded        bool     `json:"evidenceRecorded"`
			EvidenceSubmitted       bool     `json:"evidenceSubmitted"`
			SelectedArbitrators     []string `json:"selectedArbitrators"`
			SelectionCount          int      `json:"selectionCount"`
			Ruled                   bool     `json:"ruled"`
			RulingDecision          string   `json:"rulingDecision"`
			Settled                 bool     `json:"settled"`
			SettledMetadataRetained bool     `json:"settledMetadataRetained"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		decodeErr := fmt.Errorf("%w: %v", ErrOrchestrationResponseDecode, err)
		step.ErrorCategory = OrchestrationErrorCategoryResponseDecode
		return BatchDisputeStatusesOrchestrationResult{Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, decodeErr)}, decodeErr
	}

	statuses := make([]pblockchain.DisputeLifecycleStatusResult, 0, len(payload.Statuses))
	for _, status := range payload.Statuses {
		selected := make([]protocol.NodeID, 0, len(status.SelectedArbitrators))
		for _, node := range status.SelectedArbitrators {
			selected = append(selected, protocol.NodeID(node))
		}
		statuses = append(statuses, pblockchain.DisputeLifecycleStatusResult{
			DisputeID:               protocol.DisputeID(status.DisputeID),
			Stage:                   status.Stage,
			KnownToEscrowRegistry:   status.KnownToEscrowRegistry,
			EvidenceRecorded:        status.EvidenceRecorded,
			EvidenceSubmitted:       status.EvidenceSubmitted,
			SelectedArbitrators:     selected,
			SelectionCount:          status.SelectionCount,
			Ruled:                   status.Ruled,
			RulingDecision:          protocol.Decision(status.RulingDecision),
			Settled:                 status.Settled,
			SettledMetadataRetained: status.SettledMetadataRetained,
		})
	}

	step.Success = true
	return BatchDisputeStatusesOrchestrationResult{
		Statuses:    pblockchain.DisputeLifecycleBatchStatusResult{Statuses: statuses},
		Diagnostics: orchestrationDiagnostics([]OrchestrationStepTrace{step}, nil),
	}, nil
}

func (o *PrecompileContractOrchestrator) runPrecompile(address common.Address, input []byte) ([]byte, error) {
	if o.precompiles == nil {
		return nil, ErrOrchestrationPrecompileSetNotConfigured
	}
	precompile := o.precompiles[address]
	if precompile == nil {
		return nil, ErrOrchestrationPrecompileMissing
	}
	return precompile.Run(input)
}

func orchestrationDiagnostics(stepTraces []OrchestrationStepTrace, err error) OrchestrationDiagnostics {
	traces := append([]OrchestrationStepTrace(nil), stepTraces...)
	return OrchestrationDiagnostics{
		StepTraces:              traces,
		NormalizedErrorCategory: normalizeOrchestrationError(err),
	}
}

func normalizeOrchestrationError(err error) string {
	if err == nil {
		return OrchestrationErrorCategoryNone
	}
	if errors.Is(err, ErrOrchestrationValidation) {
		return OrchestrationErrorCategoryValidation
	}
	if errors.Is(err, ErrOrchestrationPrecompileSetNotConfigured) {
		return OrchestrationErrorCategoryConfiguration
	}
	if errors.Is(err, ErrOrchestrationPrecompileMissing) {
		return OrchestrationErrorCategoryPrecompileMiss
	}
	if errors.Is(err, ErrOrchestrationResponseDecode) {
		return OrchestrationErrorCategoryResponseDecode
	}
	if errors.Is(err, ErrOrchestrationResponseShape) {
		return OrchestrationErrorCategoryResponseShape
	}
	return OrchestrationErrorCategoryExecution
}
