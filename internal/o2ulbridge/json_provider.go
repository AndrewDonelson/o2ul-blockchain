package o2ulbridge

import (
	"encoding/json"
	"errors"

	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
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
