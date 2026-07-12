package o2ulbridge

import (
	"encoding/json"
	"errors"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/protocol"
	"github.com/ethereum/go-ethereum/core/vm"
)

var ErrRuntimeBridgeNotSet = errors.New("runtime bridge is required")

// JSONRuntimeHookProvider adapts proprietary RuntimeBridge handlers to vm precompile hooks
// using a JSON wire format for request/response payloads.
type JSONRuntimeHookProvider struct {
	bridge *pblockchain.RuntimeBridge
}

func NewJSONRuntimeHookProvider(bridge *pblockchain.RuntimeBridge) *JSONRuntimeHookProvider {
	return &JSONRuntimeHookProvider{bridge: bridge}
}

func InstallRuntimeBridge(bridge *pblockchain.RuntimeBridge) {
	vm.SetO2ULRuntimeHookProvider(NewJSONRuntimeHookProvider(bridge))
}

func (p *JSONRuntimeHookProvider) VerifyProofHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ProofVerifyRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ok, err := p.bridge.VerifyProofHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: ok})
}

func (p *JSONRuntimeHookProvider) VerifyConsensusBlockHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ConsensusVerifyBlockRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ok, err := p.bridge.VerifyConsensusBlockHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: ok})
}

func (p *JSONRuntimeHookProvider) SubmitConsensusAttestationHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ConsensusSubmitAttestationRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	if err := p.bridge.SubmitConsensusAttestationHook(req); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (p *JSONRuntimeHookProvider) CreateShieldedNoteHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ShieldedCreateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	note, commitment, err := p.bridge.CreateShieldedNoteHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Note       interface{} `json:"note"`
		Commitment interface{} `json:"commitment"`
	}{Note: note, Commitment: commitment})
}

func (p *JSONRuntimeHookProvider) SpendShieldedNoteHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ShieldedSpendRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	nullifier, err := p.bridge.SpendShieldedNoteHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Nullifier interface{} `json:"nullifier"`
	}{Nullifier: nullifier})
}

func (p *JSONRuntimeHookProvider) VerifyShieldedTransactionHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ShieldedVerifyRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ok, err := p.bridge.VerifyShieldedTransactionHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: ok})
}

func (p *JSONRuntimeHookProvider) MintNFTHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.NFTMintRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	assetID, note, err := p.bridge.MintNFTHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		AssetID interface{} `json:"assetId"`
		Note    interface{} `json:"note"`
	}{AssetID: assetID, Note: note})
}

func (p *JSONRuntimeHookProvider) TransferNFTHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.NFTTransferRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	note, nullifier, err := p.bridge.TransferNFTHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Note      interface{} `json:"note"`
		Nullifier interface{} `json:"nullifier"`
	}{Note: note, Nullifier: nullifier})
}

func (p *JSONRuntimeHookProvider) VerifyNFTOwnershipHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.NFTOwnershipVerifyRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ok, err := p.bridge.VerifyNFTOwnershipHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: ok})
}

func (p *JSONRuntimeHookProvider) GenerateThresholdGroupKeyHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ThresholdGenerateGroupKeyRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	groupKey, err := p.bridge.GenerateThresholdGroupKeyHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		GroupKey interface{} `json:"groupKey"`
	}{GroupKey: groupKey})
}

func (p *JSONRuntimeHookProvider) SignThresholdPartialHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ThresholdSignPartialRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	partial, err := p.bridge.SignThresholdPartialHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Partial interface{} `json:"partial"`
	}{Partial: partial})
}

func (p *JSONRuntimeHookProvider) AggregateThresholdPartialsHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ThresholdAggregateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	sig, err := p.bridge.AggregateThresholdPartialsHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Signature interface{} `json:"signature"`
	}{Signature: sig})
}

func (p *JSONRuntimeHookProvider) GenerateViewKeyHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ViewKeyGenerateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	viewKey, err := p.bridge.GenerateViewKeyHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		ViewKey interface{} `json:"viewKey"`
	}{ViewKey: viewKey})
}

func (p *JSONRuntimeHookProvider) DiscloseViewKeyHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ViewKeyDiscloseRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	disclosure, err := p.bridge.DiscloseViewKeyHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Disclosure interface{} `json:"disclosure"`
	}{Disclosure: disclosure})
}

func (p *JSONRuntimeHookProvider) IsDisclosureReplayHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ViewKeyReplayCheckRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	ok, err := p.bridge.IsDisclosureReplayHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: ok})
}

func (p *JSONRuntimeHookProvider) AllocateFeeHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.AllocateFeeRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	distribution, err := p.bridge.AllocateFeeHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Total             interface{} `json:"total"`
		ProversValidators interface{} `json:"proversValidators"`
		ArbitratorPool    interface{} `json:"arbitratorPool"`
		DevTreasury       interface{} `json:"devTreasury"`
		Burn              interface{} `json:"burn"`
	}{
		Total:             distribution.Total,
		ProversValidators: distribution.ProversValidators,
		ArbitratorPool:    distribution.ArbitratorPool,
		DevTreasury:       distribution.DevTreasury,
		Burn:              distribution.Burn,
	})
}

func (p *JSONRuntimeHookProvider) SelectArbitratorsHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ArbitrationSelectRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	selected, err := p.bridge.SelectArbitratorsHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Selected []string `json:"selected"`
	}{Selected: toStrings(selected)})
}

func (p *JSONRuntimeHookProvider) SubmitArbitrationEvidenceHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ArbitrationSubmitEvidenceRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	if err := p.bridge.SubmitArbitrationEvidenceHook(req); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (p *JSONRuntimeHookProvider) RuleArbitrationHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.ArbitrationRuleRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	if err := p.bridge.RuleArbitrationHook(req); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		OK bool `json:"ok"`
	}{OK: true})
}

func (p *JSONRuntimeHookProvider) TriggerEscrowDisputeAndAllocateHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.EscrowTriggerDisputeAndAllocateRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	out, err := p.bridge.TriggerEscrowDisputeAndAllocateHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		DisputeID    string      `json:"disputeId"`
		FeeAllocated bool        `json:"feeAllocated"`
		Distribution interface{} `json:"distribution"`
	}{
		DisputeID:    string(out.DisputeID),
		FeeAllocated: out.FeeAllocated,
		Distribution: out.Distribution,
	})
}

func (p *JSONRuntimeHookProvider) SettleEscrowFromArbitrationHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.SettleEscrowFromArbitrationRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	out, err := p.bridge.SettleEscrowFromArbitrationHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		SettlementType string      `json:"settlementType"`
		ReleaseTx      interface{} `json:"releaseTx"`
		ReclaimTx      interface{} `json:"reclaimTx"`
		FeeAllocated   bool        `json:"feeAllocated"`
		Distribution   interface{} `json:"distribution"`
	}{
		SettlementType: out.SettlementType,
		ReleaseTx:      out.ReleaseTx,
		ReclaimTx:      out.ReclaimTx,
		FeeAllocated:   out.FeeAllocated,
		Distribution:   out.Distribution,
	})
}

func (p *JSONRuntimeHookProvider) GetDisputeLifecycleStatusHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.DisputeLifecycleStatusRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	out, err := p.bridge.GetDisputeLifecycleStatusHook(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
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
	}{
		DisputeID:               string(out.DisputeID),
		Stage:                   out.Stage,
		KnownToEscrowRegistry:   out.KnownToEscrowRegistry,
		EvidenceRecorded:        out.EvidenceRecorded,
		EvidenceSubmitted:       out.EvidenceSubmitted,
		SelectedArbitrators:     toStrings(out.SelectedArbitrators),
		SelectionCount:          out.SelectionCount,
		Ruled:                   out.Ruled,
		RulingDecision:          string(out.RulingDecision),
		Settled:                 out.Settled,
		SettledMetadataRetained: out.SettledMetadataRetained,
	})
}

func (p *JSONRuntimeHookProvider) GetDisputeLifecycleStatusesHook(input []byte) ([]byte, error) {
	if p.bridge == nil {
		return nil, ErrRuntimeBridgeNotSet
	}
	var req pblockchain.DisputeLifecycleBatchStatusRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}
	out, err := p.bridge.GetDisputeLifecycleStatusesHook(req)
	if err != nil {
		return nil, err
	}
	type jsonStatus struct {
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
	}
	statuses := make([]jsonStatus, 0, len(out.Statuses))
	for _, status := range out.Statuses {
		statuses = append(statuses, jsonStatus{
			DisputeID:               string(status.DisputeID),
			Stage:                   status.Stage,
			KnownToEscrowRegistry:   status.KnownToEscrowRegistry,
			EvidenceRecorded:        status.EvidenceRecorded,
			EvidenceSubmitted:       status.EvidenceSubmitted,
			SelectedArbitrators:     toStrings(status.SelectedArbitrators),
			SelectionCount:          status.SelectionCount,
			Ruled:                   status.Ruled,
			RulingDecision:          string(status.RulingDecision),
			Settled:                 status.Settled,
			SettledMetadataRetained: status.SettledMetadataRetained,
		})
	}

	return json.Marshal(struct {
		Statuses []jsonStatus `json:"statuses"`
	}{Statuses: statuses})
}

func toStrings(in []protocol.NodeID) []string {
	out := make([]string, 0, len(in))
	for _, n := range in {
		out = append(out, string(n))
	}
	return out
}
