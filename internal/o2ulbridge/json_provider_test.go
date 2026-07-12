package o2ulbridge

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/AndrewDonelson/o2ul-proprietary/pkg/arbitration"
	pblockchain "github.com/AndrewDonelson/o2ul-proprietary/pkg/blockchain"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/consensus"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/escrow"
	"github.com/AndrewDonelson/o2ul-proprietary/pkg/fees"
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

func newConsensusTestRuntimeBridge(t *testing.T) *pblockchain.RuntimeBridge {
	t.Helper()
	proofSys := proofs.NewHashProofSystem(0)
	consensusAdapter := consensus.NewBasicEngineWithConfig(proofSys, consensus.BasicEngineConfig{
		CircuitID:      protocol.CircuitID("proof"),
		GenesisHash:    protocol.Hash("genesis-hash"),
		RegisteredNode: []protocol.NodeID{"node-a"},
	})
	bridge, err := pblockchain.NewRuntimeBridge(pblockchain.RuntimeBridgeDeps{
		Proofs:       proofSys,
		Shielded:     shielded.NewInMemoryPool(),
		NFT:          nft.NewInMemoryRegistry(),
		NFTOwnership: nft.NewHashOwnershipVerifier(),
		Threshold:    threshold.NewSimpleSigner(),
		ViewKeys:     viewkeys.NewSimpleManager(),
		Consensus:    consensusAdapter,
	})
	if err != nil {
		t.Fatalf("new consensus test runtime bridge: %v", err)
	}
	return bridge
}

func newFeeTestRuntimeBridge(t *testing.T) *pblockchain.RuntimeBridge {
	t.Helper()
	bridge, err := pblockchain.NewRuntimeBridge(pblockchain.RuntimeBridgeDeps{
		Proofs:       proofs.NewHashProofSystem(0),
		Shielded:     shielded.NewInMemoryPool(),
		NFT:          nft.NewInMemoryRegistry(),
		NFTOwnership: nft.NewHashOwnershipVerifier(),
		Threshold:    threshold.NewSimpleSigner(),
		ViewKeys:     viewkeys.NewSimpleManager(),
		Fees:         fees.NewInMemoryDistributionLedger(),
	})
	if err != nil {
		t.Fatalf("new fee test runtime bridge: %v", err)
	}
	return bridge
}

func newDisputeFlowRuntimeBridge(t *testing.T) *pblockchain.RuntimeBridge {
	t.Helper()
	arb := arbitration.NewInMemoryEngine(big.NewInt(1))
	if err := arb.Register("node-a", protocol.Amount{Value: big.NewInt(10)}); err != nil {
		t.Fatalf("register arbitrator: %v", err)
	}
	bridge, err := pblockchain.NewRuntimeBridge(pblockchain.RuntimeBridgeDeps{
		Proofs:       proofs.NewHashProofSystem(0),
		Shielded:     shielded.NewInMemoryPool(),
		NFT:          nft.NewInMemoryRegistry(),
		NFTOwnership: nft.NewHashOwnershipVerifier(),
		Threshold:    threshold.NewSimpleSigner(),
		ViewKeys:     viewkeys.NewSimpleManager(),
		Fees:         fees.NewInMemoryDistributionLedger(),
		Escrow:       escrow.NewInMemoryManager(),
		Arbitration:  arb,
	})
	if err != nil {
		t.Fatalf("new dispute flow runtime bridge: %v", err)
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

func TestJSONRuntimeProviderConsensusVerifyAndAttestationPolicies(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newConsensusTestRuntimeBridge(t))

	proof, err := proofs.NewHashProofSystem(0).Prove(protocol.CircuitID("proof"), protocol.Witness("block-1"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	verifyReq, err := json.Marshal(pblockchain.ConsensusVerifyBlockRequest{
		Header: protocol.BlockHeader{
			Number:    1,
			Hash:      protocol.Hash("block-1"),
			Parent:    protocol.Hash("genesis-hash"),
			Timestamp: 1,
		},
		Proof: proof,
	})
	if err != nil {
		t.Fatalf("marshal verify request: %v", err)
	}
	verifyOut, err := provider.VerifyConsensusBlockHook(verifyReq)
	if err != nil {
		t.Fatalf("verify consensus block hook: %v", err)
	}
	var verifyResp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(verifyOut, &verifyResp); err != nil {
		t.Fatalf("unmarshal verify response: %v", err)
	}
	if !verifyResp.OK {
		t.Fatal("expected consensus verify response OK")
	}

	badAttReq, err := json.Marshal(pblockchain.ConsensusSubmitAttestationRequest{
		NodeID:    protocol.NodeID("node-b"),
		BlockHash: protocol.Hash("block-1"),
	})
	if err != nil {
		t.Fatalf("marshal bad attestation request: %v", err)
	}
	_, err = provider.SubmitConsensusAttestationHook(badAttReq)
	if !errors.Is(err, consensus.ErrUnregisteredNode) {
		t.Fatalf("expected unregistered node error, got %v", err)
	}

	goodAttReq, err := json.Marshal(pblockchain.ConsensusSubmitAttestationRequest{
		NodeID:    protocol.NodeID("node-a"),
		BlockHash: protocol.Hash("block-1"),
	})
	if err != nil {
		t.Fatalf("marshal good attestation request: %v", err)
	}
	_, err = provider.SubmitConsensusAttestationHook(goodAttReq)
	if err != nil {
		t.Fatalf("expected first valid attestation to pass, got %v", err)
	}
	_, err = provider.SubmitConsensusAttestationHook(goodAttReq)
	if !errors.Is(err, consensus.ErrDuplicateAttestation) {
		t.Fatalf("expected duplicate attestation error, got %v", err)
	}
}

func TestJSONRuntimeProviderAllocateFeeHook(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newFeeTestRuntimeBridge(t))
	req, err := json.Marshal(pblockchain.AllocateFeeRequest{
		Total: protocol.Amount{Value: big.NewInt(1000001)},
	})
	if err != nil {
		t.Fatalf("marshal allocate fee request: %v", err)
	}
	out, err := provider.AllocateFeeHook(req)
	if err != nil {
		t.Fatalf("allocate fee hook: %v", err)
	}

	var resp struct {
		Total             protocol.Amount `json:"total"`
		ProversValidators protocol.Amount `json:"proversValidators"`
		ArbitratorPool    protocol.Amount `json:"arbitratorPool"`
		DevTreasury       protocol.Amount `json:"devTreasury"`
		Burn              protocol.Amount `json:"burn"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal allocate fee response: %v", err)
	}

	sum := big.NewInt(0)
	sum.Add(sum, resp.ProversValidators.Value)
	sum.Add(sum, resp.ArbitratorPool.Value)
	sum.Add(sum, resp.DevTreasury.Value)
	sum.Add(sum, resp.Burn.Value)
	if sum.Cmp(resp.Total.Value) != 0 {
		t.Fatalf("allocation sum mismatch: sum=%s total=%s", sum, resp.Total.Value)
	}
}

func TestJSONRuntimeProviderAllocateFeeHookRequiresFeeAdapter(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newTestRuntimeBridge(t))
	req, err := json.Marshal(pblockchain.AllocateFeeRequest{
		Total: protocol.Amount{Value: big.NewInt(1)},
	})
	if err != nil {
		t.Fatalf("marshal allocate fee request: %v", err)
	}
	_, err = provider.AllocateFeeHook(req)
	if err == nil {
		t.Fatal("expected fee adapter requirement error")
	}
}

func TestJSONRuntimeProviderFeeDistributionSplitHooks(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newFeeTestRuntimeBridge(t))

	configureReq, err := json.Marshal(pblockchain.ConfigureFeeDistributionSplitRequest{Split: fees.DistributionSplit{
		ProversValidatorsBps: 4500,
		ArbitratorPoolBps:    2500,
		DevTreasuryBps:       2500,
		BurnBps:              500,
	}})
	if err != nil {
		t.Fatalf("marshal fee split configure request: %v", err)
	}
	configureOut, err := provider.ConfigureFeeDistributionSplitHook(configureReq)
	if err != nil {
		t.Fatalf("configure fee split hook: %v", err)
	}
	var configureResp struct {
		Split fees.DistributionSplit `json:"split"`
	}
	if err := json.Unmarshal(configureOut, &configureResp); err != nil {
		t.Fatalf("unmarshal fee split configure response: %v", err)
	}
	if configureResp.Split.ProversValidatorsBps != 4500 || configureResp.Split.ArbitratorPoolBps != 2500 || configureResp.Split.DevTreasuryBps != 2500 || configureResp.Split.BurnBps != 500 {
		t.Fatalf("unexpected configured split response: %+v", configureResp.Split)
	}

	getOut, err := provider.GetFeeDistributionSplitHook(nil)
	if err != nil {
		t.Fatalf("get fee split hook: %v", err)
	}
	var getResp struct {
		Split fees.DistributionSplit `json:"split"`
	}
	if err := json.Unmarshal(getOut, &getResp); err != nil {
		t.Fatalf("unmarshal fee split get response: %v", err)
	}
	if getResp.Split != configureResp.Split {
		t.Fatalf("fee split mismatch: configure=%+v get=%+v", configureResp.Split, getResp.Split)
	}

	invalidReq, err := json.Marshal(pblockchain.ConfigureFeeDistributionSplitRequest{Split: fees.DistributionSplit{
		ProversValidatorsBps: 4000,
		ArbitratorPoolBps:    2500,
		DevTreasuryBps:       3000,
		BurnBps:              400,
	}})
	if err != nil {
		t.Fatalf("marshal invalid fee split configure request: %v", err)
	}
	_, err = provider.ConfigureFeeDistributionSplitHook(invalidReq)
	if !errors.Is(err, fees.ErrDistributionSplitMustTotal10000) {
		t.Fatalf("expected split-total sentinel error, got %v", err)
	}
}

func TestJSONRuntimeProviderTriggerEscrowDisputeAndAllocateHook(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	req, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(100)},
		},
		EvidenceRef: protocol.Hash("ev-1"),
		ViewKey:     protocol.EncryptedViewKey("vk-1"),
		DisputeFee:  protocol.Amount{Value: big.NewInt(1000001)},
	})
	if err != nil {
		t.Fatalf("marshal trigger dispute request: %v", err)
	}
	out, err := provider.TriggerEscrowDisputeAndAllocateHook(req)
	if err != nil {
		t.Fatalf("trigger escrow dispute and allocate hook: %v", err)
	}

	var resp struct {
		DisputeID    string `json:"disputeId"`
		FeeAllocated bool   `json:"feeAllocated"`
		Distribution struct {
			Total             protocol.Amount `json:"total"`
			ProversValidators protocol.Amount `json:"proversValidators"`
			ArbitratorPool    protocol.Amount `json:"arbitratorPool"`
			DevTreasury       protocol.Amount `json:"devTreasury"`
			Burn              protocol.Amount `json:"burn"`
		} `json:"distribution"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal trigger dispute response: %v", err)
	}
	if resp.DisputeID == "" {
		t.Fatal("expected dispute id in response")
	}
	if !resp.FeeAllocated {
		t.Fatal("expected dispute fee allocation to be applied")
	}
	sum := big.NewInt(0)
	sum.Add(sum, resp.Distribution.ProversValidators.Value)
	sum.Add(sum, resp.Distribution.ArbitratorPool.Value)
	sum.Add(sum, resp.Distribution.DevTreasury.Value)
	sum.Add(sum, resp.Distribution.Burn.Value)
	if sum.Cmp(resp.Distribution.Total.Value) != 0 {
		t.Fatalf("distribution invariant broken: sum=%s total=%s", sum, resp.Distribution.Total.Value)
	}
}

func TestJSONRuntimeProviderArbitrationSelectAndRuleHooks(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	triggerReq, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		EvidenceRef: protocol.Hash("ev-10"),
		ViewKey:     protocol.EncryptedViewKey("vk-10"),
		DisputeFee:  protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal trigger dispute request: %v", err)
	}
	triggerOut, err := provider.TriggerEscrowDisputeAndAllocateHook(triggerReq)
	if err != nil {
		t.Fatalf("trigger dispute hook: %v", err)
	}
	var triggered struct {
		DisputeID string `json:"disputeId"`
	}
	if err := json.Unmarshal(triggerOut, &triggered); err != nil {
		t.Fatalf("unmarshal trigger dispute response: %v", err)
	}
	if triggered.DisputeID == "" {
		t.Fatal("expected dispute id from trigger dispute hook")
	}

	selectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: protocol.DisputeID(triggered.DisputeID),
		Seed:      protocol.VRFSeed("seed-10"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal arbitration select request: %v", err)
	}
	selectOut, err := provider.SelectArbitratorsHook(selectReq)
	if err != nil {
		t.Fatalf("select arbitrators hook: %v", err)
	}
	var selectedResp struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOut, &selectedResp); err != nil {
		t.Fatalf("unmarshal selected response: %v", err)
	}
	if len(selectedResp.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator, got %d", len(selectedResp.Selected))
	}

	evReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   protocol.DisputeID(triggered.DisputeID),
		EvidenceRef: protocol.Hash("ev-10"),
		ViewKey:     protocol.EncryptedViewKey("vk-10"),
	})
	if err != nil {
		t.Fatalf("marshal submit evidence request: %v", err)
	}
	if _, err := provider.SubmitArbitrationEvidenceHook(evReq); err == nil {
		t.Fatal("expected duplicate evidence submission to fail for trigger-created dispute")
	}
	mismatchEvReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   protocol.DisputeID(triggered.DisputeID),
		EvidenceRef: protocol.Hash("ev-10-mismatch"),
		ViewKey:     protocol.EncryptedViewKey("vk-10"),
	})
	if err != nil {
		t.Fatalf("marshal mismatch submit evidence request: %v", err)
	}
	if _, err := provider.SubmitArbitrationEvidenceHook(mismatchEvReq); err == nil {
		t.Fatal("expected evidence mismatch rejection for trigger-created dispute")
	}

	ruleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  protocol.DisputeID(triggered.DisputeID),
		Arbitrator: protocol.NodeID(selectedResp.Selected[0]),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal rule request: %v", err)
	}
	if _, err := provider.RuleArbitrationHook(ruleReq); err != nil {
		t.Fatalf("rule arbitration hook: %v", err)
	}

	unknownSelectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: "unknown-dispute",
		Seed:      protocol.VRFSeed("seed-unknown"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal unknown select request: %v", err)
	}
	if _, err := provider.SelectArbitratorsHook(unknownSelectReq); err == nil {
		t.Fatal("expected unknown dispute rejection for select")
	}

	unknownSubmitReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   "unknown-dispute",
		EvidenceRef: protocol.Hash("ev-unknown"),
		ViewKey:     protocol.EncryptedViewKey("vk-unknown"),
	})
	if err != nil {
		t.Fatalf("marshal unknown submit request: %v", err)
	}
	if _, err := provider.SubmitArbitrationEvidenceHook(unknownSubmitReq); err == nil {
		t.Fatal("expected unknown dispute rejection for evidence submit")
	}

	unknownRuleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  "unknown-dispute",
		Arbitrator: protocol.NodeID("node-a"),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal unknown rule request: %v", err)
	}
	if _, err := provider.RuleArbitrationHook(unknownRuleReq); err == nil {
		t.Fatal("expected unknown dispute rejection for rule")
	}
}

func TestJSONRuntimeProviderSettleEscrowFromArbitrationHook(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))

	triggerDispute := func(label string) protocol.DisputeID {
		t.Helper()
		req, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
			Escrow: protocol.EscrowNote{
				Buyer:  protocol.PublicKey("buyer"),
				Seller: protocol.PublicKey("seller"),
				Amount: protocol.Amount{Value: big.NewInt(10)},
			},
			EvidenceRef: protocol.Hash("ev-" + label),
			ViewKey:     protocol.EncryptedViewKey("vk-" + label),
			DisputeFee:  protocol.Amount{Value: big.NewInt(0)},
		})
		if err != nil {
			t.Fatalf("marshal trigger dispute request: %v", err)
		}
		out, err := provider.TriggerEscrowDisputeAndAllocateHook(req)
		if err != nil {
			t.Fatalf("trigger dispute hook: %v", err)
		}
		var resp struct {
			DisputeID string `json:"disputeId"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal trigger dispute response: %v", err)
		}
		if resp.DisputeID == "" {
			t.Fatal("expected dispute id from trigger dispute hook")
		}
		return protocol.DisputeID(resp.DisputeID)
	}

	settleDispute1 := triggerDispute("settle-direct-1")
	settleDispute2 := triggerDispute("settle-direct-2")

	selectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{DisputeID: settleDispute1, Seed: protocol.VRFSeed("seed-settle-direct-1"), Count: 1})
	if err != nil {
		t.Fatalf("marshal arbitration select request: %v", err)
	}
	selectOut, err := provider.SelectArbitratorsHook(selectReq)
	if err != nil {
		t.Fatalf("select arbitrators hook: %v", err)
	}
	var selected struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOut, &selected); err != nil {
		t.Fatalf("unmarshal arbitration select response: %v", err)
	}
	if len(selected.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator, got %d", len(selected.Selected))
	}
	ruleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{DisputeID: settleDispute1, Arbitrator: protocol.NodeID(selected.Selected[0]), Decision: protocol.Decision("approve")})
	if err != nil {
		t.Fatalf("marshal arbitration rule request: %v", err)
	}
	if _, err := provider.RuleArbitrationHook(ruleReq); err != nil {
		t.Fatalf("rule arbitration hook: %v", err)
	}

	selectReq2, err := json.Marshal(pblockchain.ArbitrationSelectRequest{DisputeID: settleDispute2, Seed: protocol.VRFSeed("seed-settle-direct-2"), Count: 1})
	if err != nil {
		t.Fatalf("marshal arbitration select request 2: %v", err)
	}
	selectOut2, err := provider.SelectArbitratorsHook(selectReq2)
	if err != nil {
		t.Fatalf("select arbitrators hook 2: %v", err)
	}
	if err := json.Unmarshal(selectOut2, &selected); err != nil {
		t.Fatalf("unmarshal arbitration select response 2: %v", err)
	}
	if len(selected.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator for second dispute, got %d", len(selected.Selected))
	}
	ruleReq2, err := json.Marshal(pblockchain.ArbitrationRuleRequest{DisputeID: settleDispute2, Arbitrator: protocol.NodeID(selected.Selected[0]), Decision: protocol.Decision("deny")})
	if err != nil {
		t.Fatalf("marshal arbitration rule request 2: %v", err)
	}
	if _, err := provider.RuleArbitrationHook(ruleReq2); err != nil {
		t.Fatalf("rule arbitration hook 2: %v", err)
	}

	mismatchReleaseReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: settleDispute1,
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("intruder"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-1"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal mismatched release settlement request: %v", err)
	}
	if _, err := provider.SettleEscrowFromArbitrationHook(mismatchReleaseReq); err == nil {
		t.Fatal("expected settlement escrow mismatch rejection")
	}

	releaseReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: settleDispute1,
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-1"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(1000001)},
	})
	if err != nil {
		t.Fatalf("marshal release settlement request: %v", err)
	}
	releaseOut, err := provider.SettleEscrowFromArbitrationHook(releaseReq)
	if err != nil {
		t.Fatalf("settle escrow release hook: %v", err)
	}
	var releaseResp struct {
		SettlementType string `json:"settlementType"`
		ReleaseTx      struct {
			Proof protocol.Proof `json:"proof"`
		} `json:"releaseTx"`
		FeeAllocated bool `json:"feeAllocated"`
		Distribution struct {
			Total             protocol.Amount `json:"total"`
			ProversValidators protocol.Amount `json:"proversValidators"`
			ArbitratorPool    protocol.Amount `json:"arbitratorPool"`
			DevTreasury       protocol.Amount `json:"devTreasury"`
			Burn              protocol.Amount `json:"burn"`
		} `json:"distribution"`
	}
	if err := json.Unmarshal(releaseOut, &releaseResp); err != nil {
		t.Fatalf("unmarshal release settlement response: %v", err)
	}
	if releaseResp.SettlementType != "release" {
		t.Fatalf("expected release settlement type, got %q", releaseResp.SettlementType)
	}
	if len(releaseResp.ReleaseTx.Proof) == 0 {
		t.Fatal("expected release tx payload")
	}
	if !releaseResp.FeeAllocated {
		t.Fatal("expected fee allocation on release settlement")
	}

	reclaimReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: settleDispute2,
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:      protocol.Decision("deny"),
		SettlementFee: protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal reclaim settlement request: %v", err)
	}
	reclaimOut, err := provider.SettleEscrowFromArbitrationHook(reclaimReq)
	if err != nil {
		t.Fatalf("settle escrow reclaim hook: %v", err)
	}
	var reclaimResp struct {
		SettlementType string `json:"settlementType"`
		ReclaimTx      struct {
			Proof protocol.Proof `json:"proof"`
		} `json:"reclaimTx"`
		FeeAllocated bool `json:"feeAllocated"`
	}
	if err := json.Unmarshal(reclaimOut, &reclaimResp); err != nil {
		t.Fatalf("unmarshal reclaim settlement response: %v", err)
	}
	if reclaimResp.SettlementType != "reclaim" {
		t.Fatalf("expected reclaim settlement type, got %q", reclaimResp.SettlementType)
	}
	if len(reclaimResp.ReclaimTx.Proof) == 0 {
		t.Fatal("expected reclaim tx payload")
	}
	if reclaimResp.FeeAllocated {
		t.Fatal("did not expect fee allocation for zero settlement fee")
	}

	forgedSelectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{DisputeID: "forged-direct-settle", Seed: protocol.VRFSeed("seed-forged-direct"), Count: 1})
	if err != nil {
		t.Fatalf("marshal forged select request: %v", err)
	}
	if _, err := provider.SelectArbitratorsHook(forgedSelectReq); err == nil {
		t.Fatal("expected forged dispute select rejection")
	}
	forgedRuleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{DisputeID: "forged-direct-settle", Arbitrator: protocol.NodeID("node-a"), Decision: protocol.Decision("approve")})
	if err != nil {
		t.Fatalf("marshal forged rule request: %v", err)
	}
	forgedEvidenceReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   "forged-direct-settle",
		EvidenceRef: protocol.Hash("ev-forged-direct"),
		ViewKey:     protocol.EncryptedViewKey("vk-forged-direct"),
	})
	if err != nil {
		t.Fatalf("marshal forged evidence request: %v", err)
	}
	if _, err := provider.SubmitArbitrationEvidenceHook(forgedEvidenceReq); err == nil {
		t.Fatal("expected forged dispute evidence rejection")
	}
	if _, err := provider.RuleArbitrationHook(forgedRuleReq); err == nil {
		t.Fatal("expected forged dispute rule rejection")
	}
	forgedSettleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: "forged-direct-settle",
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-forged"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal forged settle request: %v", err)
	}
	if _, err := provider.SettleEscrowFromArbitrationHook(forgedSettleReq); err == nil {
		t.Fatal("expected forged dispute origin rejection")
	}
}

func TestJSONRuntimeProviderDisputeAndSettlementPrecompileRouting(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	selectPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationSelect]
	if selectPC == nil {
		t.Fatal("expected arbitration select precompile")
	}

	disputeReq, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(100)},
		},
		EvidenceRef: protocol.Hash("ev-precompile-1"),
		ViewKey:     protocol.EncryptedViewKey("vk-precompile-1"),
		DisputeFee:  protocol.Amount{Value: big.NewInt(1000001)},
	})
	if err != nil {
		t.Fatalf("marshal escrow dispute request: %v", err)
	}
	disputePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileEscrowDispute]
	if disputePC == nil {
		t.Fatal("expected escrow dispute precompile")
	}
	disputeOut, err := disputePC.Run(disputeReq)
	if err != nil {
		t.Fatalf("run escrow dispute precompile: %v", err)
	}
	var disputeResp struct {
		DisputeID string `json:"disputeId"`
	}
	if err := json.Unmarshal(disputeOut, &disputeResp); err != nil {
		t.Fatalf("unmarshal escrow dispute response: %v", err)
	}
	if disputeResp.DisputeID == "" {
		t.Fatal("expected dispute id from escrow dispute precompile")
	}

	selectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: protocol.DisputeID(disputeResp.DisputeID),
		Seed:      protocol.VRFSeed("seed-precompile-1"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal arbitration select request: %v", err)
	}
	selectOut, err := selectPC.Run(selectReq)
	if err != nil {
		t.Fatalf("run arbitration select precompile: %v", err)
	}
	var selected struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOut, &selected); err != nil {
		t.Fatalf("unmarshal arbitration select response: %v", err)
	}
	if len(selected.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator, got %d", len(selected.Selected))
	}

	selectReq2, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: protocol.DisputeID(disputeResp.DisputeID),
		Seed:      protocol.VRFSeed("seed-precompile-1-dispute"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal arbitration select for dispute request: %v", err)
	}
	selectOut2, err := selectPC.Run(selectReq2)
	if err != nil {
		t.Fatalf("run arbitration select precompile for dispute: %v", err)
	}
	var selected2 struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOut2, &selected2); err != nil {
		t.Fatalf("unmarshal arbitration select for dispute response: %v", err)
	}
	if len(selected2.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator for dispute, got %d", len(selected2.Selected))
	}
	rulePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationRule]
	if rulePC == nil {
		t.Fatal("expected arbitration rule precompile")
	}
	ruleReq2, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  protocol.DisputeID(disputeResp.DisputeID),
		Arbitrator: protocol.NodeID(selected2.Selected[0]),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal arbitration rule for dispute request: %v", err)
	}
	if _, err := rulePC.Run(ruleReq2); err != nil {
		t.Fatalf("run arbitration rule precompile for dispute: %v", err)
	}

	settleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: protocol.DisputeID(disputeResp.DisputeID),
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(100)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-precompile-1"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(1000001)},
	})
	if err != nil {
		t.Fatalf("marshal escrow settle request: %v", err)
	}
	settlePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileEscrowSettle]
	if settlePC == nil {
		t.Fatal("expected escrow settle precompile")
	}
	settleOut, err := settlePC.Run(settleReq)
	if err != nil {
		t.Fatalf("run escrow settle precompile: %v", err)
	}
	var settleResp struct {
		SettlementType string `json:"settlementType"`
	}
	if err := json.Unmarshal(settleOut, &settleResp); err != nil {
		t.Fatalf("unmarshal escrow settle response: %v", err)
	}
	if settleResp.SettlementType != "release" {
		t.Fatalf("expected release settlement type, got %q", settleResp.SettlementType)
	}
}

func TestJSONRuntimeProviderPrecompileSequentialArbitrationAndSettlementFlow(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	selectPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationSelect]
	submitPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationSubmit]
	rulePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileArbitrationRule]
	disputePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileEscrowDispute]
	settlePC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileEscrowSettle]
	if selectPC == nil || submitPC == nil || rulePC == nil || disputePC == nil || settlePC == nil {
		t.Fatal("expected arbitration/settlement precompiles")
	}

	triggerDispute := func(label string) protocol.DisputeID {
		t.Helper()
		req, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
			Escrow: protocol.EscrowNote{
				Buyer:  protocol.PublicKey("buyer"),
				Seller: protocol.PublicKey("seller"),
				Amount: protocol.Amount{Value: big.NewInt(10)},
			},
			EvidenceRef: protocol.Hash("ev-" + label),
			ViewKey:     protocol.EncryptedViewKey("vk-" + label),
			DisputeFee:  protocol.Amount{Value: big.NewInt(0)},
		})
		if err != nil {
			t.Fatalf("marshal precompile dispute request: %v", err)
		}
		out, err := disputePC.Run(req)
		if err != nil {
			t.Fatalf("run precompile dispute hook: %v", err)
		}
		var resp struct {
			DisputeID string `json:"disputeId"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal precompile dispute response: %v", err)
		}
		if resp.DisputeID == "" {
			t.Fatal("expected dispute id from precompile dispute hook")
		}
		return protocol.DisputeID(resp.DisputeID)
	}

	seqDispute1 := triggerDispute("seq-dispute-1")

	// Happy path: select -> submit evidence -> rule -> settle release.
	selectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: seqDispute1,
		Seed:      protocol.VRFSeed("seed-seq-1"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal arbitration select request: %v", err)
	}
	selectOut, err := selectPC.Run(selectReq)
	if err != nil {
		t.Fatalf("run arbitration select precompile: %v", err)
	}
	var selected struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOut, &selected); err != nil {
		t.Fatalf("unmarshal arbitration select response: %v", err)
	}
	if len(selected.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator, got %d", len(selected.Selected))
	}

	mismatchSubmitReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   seqDispute1,
		EvidenceRef: protocol.Hash("ev-seq-1-mismatch"),
		ViewKey:     protocol.EncryptedViewKey("vk-seq-1"),
	})
	if err != nil {
		t.Fatalf("marshal mismatch evidence submit request: %v", err)
	}
	if _, err := submitPC.Run(mismatchSubmitReq); err == nil {
		t.Fatal("expected precompile evidence mismatch rejection")
	}

	ruleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  seqDispute1,
		Arbitrator: protocol.NodeID(selected.Selected[0]),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal arbitration rule request: %v", err)
	}
	if _, err := rulePC.Run(ruleReq); err != nil {
		t.Fatalf("run arbitration rule precompile: %v", err)
	}

	mismatchSettleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: seqDispute1,
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("intruder"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-seq-1"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal mismatched settlement request: %v", err)
	}
	if _, err := settlePC.Run(mismatchSettleReq); err == nil {
		t.Fatal("expected precompile settlement escrow mismatch rejection")
	}

	settleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: seqDispute1,
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-seq-1"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(1)},
	})
	if err != nil {
		t.Fatalf("marshal settlement request: %v", err)
	}
	settleOut, err := settlePC.Run(settleReq)
	if err != nil {
		t.Fatalf("run settlement precompile: %v", err)
	}
	var settleResp struct {
		SettlementType string `json:"settlementType"`
	}
	if err := json.Unmarshal(settleOut, &settleResp); err != nil {
		t.Fatalf("unmarshal settlement response: %v", err)
	}
	if settleResp.SettlementType != "release" {
		t.Fatalf("expected release settlement type, got %q", settleResp.SettlementType)
	}

	if _, err := settlePC.Run(settleReq); err == nil {
		t.Fatal("expected duplicate settlement replay to fail")
	}

	settledSelectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: seqDispute1,
		Seed:      protocol.VRFSeed("seed-after-settle"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal settled dispute select request: %v", err)
	}
	if _, err := selectPC.Run(settledSelectReq); err == nil {
		t.Fatal("expected settled dispute select rejection")
	}

	settledSubmitReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   seqDispute1,
		EvidenceRef: protocol.Hash("ev-seq-1"),
		ViewKey:     protocol.EncryptedViewKey("vk-seq-1"),
	})
	if err != nil {
		t.Fatalf("marshal settled dispute submit request: %v", err)
	}
	if _, err := submitPC.Run(settledSubmitReq); err == nil {
		t.Fatal("expected settled dispute evidence rejection")
	}

	settledRuleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  seqDispute1,
		Arbitrator: protocol.NodeID("node-a"),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal settled dispute rule request: %v", err)
	}
	if _, err := rulePC.Run(settledRuleReq); err == nil {
		t.Fatal("expected settled dispute rule rejection")
	}

	// Negative path: duplicate arbitration rule.
	if _, err := rulePC.Run(ruleReq); err == nil {
		t.Fatal("expected duplicate arbitration rule to fail")
	}

	forgedSelectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: "forged-precompile-dispute",
		Seed:      protocol.VRFSeed("seed-forged-precompile"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal forged precompile select request: %v", err)
	}
	if _, err := selectPC.Run(forgedSelectReq); err == nil {
		t.Fatal("expected forged precompile dispute select rejection")
	}
	forgedSubmitReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   "forged-precompile-dispute",
		EvidenceRef: protocol.Hash("ev-forged-precompile"),
		ViewKey:     protocol.EncryptedViewKey("vk-forged-precompile"),
	})
	if err != nil {
		t.Fatalf("marshal forged precompile submit request: %v", err)
	}
	if _, err := submitPC.Run(forgedSubmitReq); err == nil {
		t.Fatal("expected forged precompile dispute evidence rejection")
	}
	forgedRuleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  "forged-precompile-dispute",
		Arbitrator: protocol.NodeID("node-a"),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal forged precompile rule request: %v", err)
	}
	if _, err := rulePC.Run(forgedRuleReq); err == nil {
		t.Fatal("expected forged precompile dispute rule rejection")
	}
	forgedSettleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: "forged-precompile-dispute",
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-forged-precompile"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal forged precompile settle request: %v", err)
	}
	if _, err := settlePC.Run(forgedSettleReq); err == nil {
		t.Fatal("expected forged precompile dispute origin rejection")
	}

	unknownSelectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: "unknown-precompile-dispute",
		Seed:      protocol.VRFSeed("seed-unknown-precompile"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal unknown precompile select request: %v", err)
	}
	if _, err := selectPC.Run(unknownSelectReq); err == nil {
		t.Fatal("expected unknown dispute rejection for precompile select")
	}

	unknownSubmitReq, err := json.Marshal(pblockchain.ArbitrationSubmitEvidenceRequest{
		DisputeID:   "unknown-precompile-dispute",
		EvidenceRef: protocol.Hash("ev-unknown-precompile"),
		ViewKey:     protocol.EncryptedViewKey("vk-unknown-precompile"),
	})
	if err != nil {
		t.Fatalf("marshal unknown precompile submit request: %v", err)
	}
	if _, err := submitPC.Run(unknownSubmitReq); err == nil {
		t.Fatal("expected unknown dispute rejection for precompile submit")
	}

	unknownRuleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  "unknown-precompile-dispute",
		Arbitrator: protocol.NodeID("node-a"),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal unknown precompile rule request: %v", err)
	}
	if _, err := rulePC.Run(unknownRuleReq); err == nil {
		t.Fatal("expected unknown dispute rejection for precompile rule")
	}

	// Negative path: wrong arbitrator for selected dispute.
	seqDispute2 := triggerDispute("seq-dispute-2")
	selectReq2, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: seqDispute2,
		Seed:      protocol.VRFSeed("seed-seq-2"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal arbitration select request 2: %v", err)
	}
	if _, err := selectPC.Run(selectReq2); err != nil {
		t.Fatalf("run arbitration select precompile 2: %v", err)
	}
	wrongRuleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  seqDispute2,
		Arbitrator: protocol.NodeID("node-b"),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal wrong arbitrator rule request: %v", err)
	}
	if _, err := rulePC.Run(wrongRuleReq); err == nil {
		t.Fatal("expected wrong arbitrator rule to fail")
	}

	// Negative path: unsupported settlement decision.
	unsupportedSettleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: seqDispute2,
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:      protocol.Decision("unsupported"),
		SettlementFee: protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal unsupported settlement request: %v", err)
	}
	if _, err := settlePC.Run(unsupportedSettleReq); err == nil {
		t.Fatal("expected unsupported settlement decision to fail")
	}
}

func TestJSONRuntimeProviderDisputeLifecycleStatusHookAndPrecompileParity(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	statusReq, err := json.Marshal(pblockchain.DisputeLifecycleStatusRequest{DisputeID: "unknown"})
	if err != nil {
		t.Fatalf("marshal unknown status request: %v", err)
	}
	unknownOut, err := provider.GetDisputeLifecycleStatusHook(statusReq)
	if err != nil {
		t.Fatalf("get unknown status: %v", err)
	}
	var unknownResp struct {
		Stage                 string `json:"stage"`
		KnownToEscrowRegistry bool   `json:"knownToEscrowRegistry"`
		Settled               bool   `json:"settled"`
	}
	if err := json.Unmarshal(unknownOut, &unknownResp); err != nil {
		t.Fatalf("unmarshal unknown status response: %v", err)
	}
	if unknownResp.Stage != "unknown" || unknownResp.KnownToEscrowRegistry || unknownResp.Settled {
		t.Fatalf("unexpected unknown status response: %+v", unknownResp)
	}

	triggerReq, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		EvidenceRef: protocol.Hash("ev-status-precompile"),
		ViewKey:     protocol.EncryptedViewKey("vk-status-precompile"),
		DisputeFee:  protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal trigger request: %v", err)
	}
	triggerOut, err := provider.TriggerEscrowDisputeAndAllocateHook(triggerReq)
	if err != nil {
		t.Fatalf("trigger dispute: %v", err)
	}
	var triggerResp struct {
		DisputeID string `json:"disputeId"`
	}
	if err := json.Unmarshal(triggerOut, &triggerResp); err != nil {
		t.Fatalf("unmarshal trigger response: %v", err)
	}
	if triggerResp.DisputeID == "" {
		t.Fatal("expected dispute id from trigger response")
	}

	selectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{
		DisputeID: protocol.DisputeID(triggerResp.DisputeID),
		Seed:      protocol.VRFSeed("seed-status-precompile"),
		Count:     1,
	})
	if err != nil {
		t.Fatalf("marshal select request: %v", err)
	}
	selectOut, err := provider.SelectArbitratorsHook(selectReq)
	if err != nil {
		t.Fatalf("select arbitrator: %v", err)
	}
	var selectResp struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal(selectOut, &selectResp); err != nil {
		t.Fatalf("unmarshal select response: %v", err)
	}
	if len(selectResp.Selected) != 1 {
		t.Fatalf("expected one selected arbitrator, got %d", len(selectResp.Selected))
	}

	ruleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{
		DisputeID:  protocol.DisputeID(triggerResp.DisputeID),
		Arbitrator: protocol.NodeID(selectResp.Selected[0]),
		Decision:   protocol.Decision("approve"),
	})
	if err != nil {
		t.Fatalf("marshal rule request: %v", err)
	}
	if _, err := provider.RuleArbitrationHook(ruleReq); err != nil {
		t.Fatalf("rule arbitrator: %v", err)
	}

	settleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
		DisputeID: protocol.DisputeID(triggerResp.DisputeID),
		Escrow: protocol.EscrowNote{
			Buyer:  protocol.PublicKey("buyer"),
			Seller: protocol.PublicKey("seller"),
			Amount: protocol.Amount{Value: big.NewInt(10)},
		},
		Decision:         protocol.Decision("approve"),
		ReleaseSignature: protocol.Signature("sig-status-precompile"),
		SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
	})
	if err != nil {
		t.Fatalf("marshal settle request: %v", err)
	}
	if _, err := provider.SettleEscrowFromArbitrationHook(settleReq); err != nil {
		t.Fatalf("settle dispute: %v", err)
	}

	settledStatusReq, err := json.Marshal(pblockchain.DisputeLifecycleStatusRequest{DisputeID: protocol.DisputeID(triggerResp.DisputeID)})
	if err != nil {
		t.Fatalf("marshal settled status request: %v", err)
	}
	directOut, err := provider.GetDisputeLifecycleStatusHook(settledStatusReq)
	if err != nil {
		t.Fatalf("get settled status direct: %v", err)
	}
	var directResp struct {
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
	if err := json.Unmarshal(directOut, &directResp); err != nil {
		t.Fatalf("unmarshal settled status direct response: %v", err)
	}
	if directResp.Stage != "settled" || !directResp.Settled || !directResp.SettledMetadataRetained {
		t.Fatalf("unexpected settled direct status response: %+v", directResp)
	}
	if directResp.DisputeID != triggerResp.DisputeID {
		t.Fatalf("expected dispute id %s, got %s", triggerResp.DisputeID, directResp.DisputeID)
	}

	statusPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileDisputeStatus]
	if statusPC == nil {
		t.Fatal("expected dispute status precompile")
	}
	precompileOut, err := statusPC.Run(settledStatusReq)
	if err != nil {
		t.Fatalf("run dispute status precompile: %v", err)
	}
	if string(precompileOut) != string(directOut) {
		t.Fatalf("expected direct and precompile dispute status payload parity; direct=%s precompile=%s", string(directOut), string(precompileOut))
	}
}

func TestJSONRuntimeProviderBatchDisputeLifecycleStatusesHookAndPrecompileParity(t *testing.T) {
	provider := NewJSONRuntimeHookProvider(newDisputeFlowRuntimeBridge(t))
	vm.SetO2ULRuntimeHookProvider(provider)
	t.Cleanup(func() { vm.SetO2ULRuntimeHookProvider(nil) })

	triggerDispute := func(label string) protocol.DisputeID {
		t.Helper()
		req, err := json.Marshal(pblockchain.EscrowTriggerDisputeAndAllocateRequest{
			Escrow: protocol.EscrowNote{
				Buyer:  protocol.PublicKey("buyer"),
				Seller: protocol.PublicKey("seller"),
				Amount: protocol.Amount{Value: big.NewInt(10)},
			},
			EvidenceRef: protocol.Hash("ev-batch-status-" + label),
			ViewKey:     protocol.EncryptedViewKey("vk-batch-status-" + label),
			DisputeFee:  protocol.Amount{Value: big.NewInt(0)},
		})
		if err != nil {
			t.Fatalf("marshal trigger request: %v", err)
		}
		out, err := provider.TriggerEscrowDisputeAndAllocateHook(req)
		if err != nil {
			t.Fatalf("trigger dispute %s: %v", label, err)
		}
		var resp struct {
			DisputeID string `json:"disputeId"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("unmarshal trigger response: %v", err)
		}
		return protocol.DisputeID(resp.DisputeID)
	}

	settleOne := func(disputeID protocol.DisputeID, label string) {
		t.Helper()
		selectReq, err := json.Marshal(pblockchain.ArbitrationSelectRequest{DisputeID: disputeID, Seed: protocol.VRFSeed("seed-batch-status-" + label), Count: 1})
		if err != nil {
			t.Fatalf("marshal select request: %v", err)
		}
		selectOut, err := provider.SelectArbitratorsHook(selectReq)
		if err != nil {
			t.Fatalf("select dispute %s: %v", label, err)
		}
		var selected struct {
			Selected []string `json:"selected"`
		}
		if err := json.Unmarshal(selectOut, &selected); err != nil {
			t.Fatalf("unmarshal select response: %v", err)
		}
		if len(selected.Selected) != 1 {
			t.Fatalf("expected one selected arbitrator, got %d", len(selected.Selected))
		}

		ruleReq, err := json.Marshal(pblockchain.ArbitrationRuleRequest{DisputeID: disputeID, Arbitrator: protocol.NodeID(selected.Selected[0]), Decision: protocol.Decision("approve")})
		if err != nil {
			t.Fatalf("marshal rule request: %v", err)
		}
		if _, err := provider.RuleArbitrationHook(ruleReq); err != nil {
			t.Fatalf("rule dispute %s: %v", label, err)
		}

		settleReq, err := json.Marshal(pblockchain.SettleEscrowFromArbitrationRequest{
			DisputeID: disputeID,
			Escrow: protocol.EscrowNote{
				Buyer:  protocol.PublicKey("buyer"),
				Seller: protocol.PublicKey("seller"),
				Amount: protocol.Amount{Value: big.NewInt(10)},
			},
			Decision:         protocol.Decision("approve"),
			ReleaseSignature: protocol.Signature("sig-batch-status-" + label),
			SettlementFee:    protocol.Amount{Value: big.NewInt(0)},
		})
		if err != nil {
			t.Fatalf("marshal settle request: %v", err)
		}
		if _, err := provider.SettleEscrowFromArbitrationHook(settleReq); err != nil {
			t.Fatalf("settle dispute %s: %v", label, err)
		}
	}

	d1 := triggerDispute("1")
	d2 := triggerDispute("2")
	settleOne(d1, "1")

	batchReq, err := json.Marshal(pblockchain.DisputeLifecycleBatchStatusRequest{DisputeIDs: []protocol.DisputeID{d2, "unknown-batch-precompile", d1, d1}})
	if err != nil {
		t.Fatalf("marshal batch status request: %v", err)
	}
	directOut, err := provider.GetDisputeLifecycleStatusesHook(batchReq)
	if err != nil {
		t.Fatalf("get batch status direct: %v", err)
	}

	var directResp struct {
		Statuses []struct {
			DisputeID string `json:"disputeId"`
			Stage     string `json:"stage"`
			Settled   bool   `json:"settled"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(directOut, &directResp); err != nil {
		t.Fatalf("unmarshal direct batch status response: %v", err)
	}
	if len(directResp.Statuses) != 3 {
		t.Fatalf("expected 3 unique batch statuses, got %d", len(directResp.Statuses))
	}
	ids := make([]string, 0, len(directResp.Statuses))
	for _, status := range directResp.Statuses {
		ids = append(ids, status.DisputeID)
	}
	expectedIDs := append([]string(nil), ids...)
	sort.Strings(expectedIDs)
	if !reflect.DeepEqual(ids, expectedIDs) {
		t.Fatalf("expected deterministic sorted batch ids %v, got %v", expectedIDs, ids)
	}

	statusByID := make(map[string]struct {
		Stage   string
		Settled bool
	})
	for _, status := range directResp.Statuses {
		statusByID[status.DisputeID] = struct {
			Stage   string
			Settled bool
		}{Stage: status.Stage, Settled: status.Settled}
	}
	if st := statusByID[string(d1)]; st.Stage != "settled" || !st.Settled {
		t.Fatalf("expected settled status for d1, got %+v", st)
	}
	if st := statusByID[string(d2)]; st.Stage != "triggered" || st.Settled {
		t.Fatalf("expected triggered status for d2, got %+v", st)
	}
	if st := statusByID["unknown-batch-precompile"]; st.Stage != "unknown" || st.Settled {
		t.Fatalf("expected unknown status for unknown dispute, got %+v", st)
	}

	statusBatchPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileDisputeStatusBatch]
	if statusBatchPC == nil {
		t.Fatal("expected batch dispute status precompile")
	}
	precompileOut, err := statusBatchPC.Run(batchReq)
	if err != nil {
		t.Fatalf("run batch dispute status precompile: %v", err)
	}
	if string(precompileOut) != string(directOut) {
		t.Fatalf("expected direct and precompile batch status payload parity; direct=%s precompile=%s", string(directOut), string(precompileOut))
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
	t.Setenv("O2UL_CONSENSUS_NETWORK_TYPE", "")

	cfg, err := RuntimeBackendConfigFromEnv()
	if err != nil {
		t.Fatalf("runtime backend config from env: %v", err)
	}
	if cfg.Proofs != BackendModeProduction || cfg.Threshold != BackendModeProduction {
		t.Fatalf("unexpected parsed modes: %+v", cfg)
	}
	if cfg.Consensus.RequiredCircuitID != "" {
		t.Fatalf("expected empty consensus required circuit when env not set, got: %s", cfg.Consensus.RequiredCircuitID)
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

func TestRuntimeBackendConfigFromEnvRejectsInvalidConsensusNetworkType(t *testing.T) {
	t.Setenv("O2UL_CONSENSUS_NETWORK_TYPE", "invalid-network")
	_, err := RuntimeBackendConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid consensus network type error")
	}
}

func TestRuntimeBackendConfigFromEnvParsesConsensusNetworkType(t *testing.T) {
	t.Setenv("O2UL_CONSENSUS_NETWORK_TYPE", "o2ul-testnet")
	t.Setenv("O2UL_CONSENSUS_REGISTERED_NODES", "node-a,node-b")
	t.Setenv("O2UL_CONSENSUS_GENESIS_HASH", "genesis-hash")
	cfg, err := RuntimeBackendConfigFromEnv()
	if err != nil {
		t.Fatalf("runtime backend config from env: %v", err)
	}
	if cfg.Consensus.NetworkType != "o2ul-testnet" {
		t.Fatalf("unexpected consensus network type: %+v", cfg.Consensus)
	}
	if cfg.Consensus.RequiredCircuitID != protocol.CircuitID("consensus.block.verify.v1.testnet") {
		t.Fatalf("unexpected consensus required circuit: %s", cfg.Consensus.RequiredCircuitID)
	}
	if len(cfg.Consensus.RegisteredNodes) != 2 {
		t.Fatalf("unexpected consensus registered nodes: %+v", cfg.Consensus.RegisteredNodes)
	}
	if string(cfg.Consensus.GenesisHash) != "genesis-hash" {
		t.Fatalf("unexpected consensus genesis hash: %s", string(cfg.Consensus.GenesisHash))
	}
}

func TestInstallRuntimeBridgeFromEnvWiresConsensusAttestationPolicy(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_CONSENSUS_NETWORK_TYPE", "o2ul-testnet")
	t.Setenv("O2UL_CONSENSUS_REGISTERED_NODES", "node-a")
	t.Setenv("O2UL_CONSENSUS_GENESIS_HASH", "genesis-hash")

	cfg, err := RuntimeBackendConfigFromEnv()
	if err != nil {
		t.Fatalf("runtime backend config from env: %v", err)
	}
	bridge, err := newRuntimeBridgeWithConfig(cfg, "")
	if err != nil {
		t.Fatalf("new runtime bridge with config: %v", err)
	}
	provider := NewJSONRuntimeHookProvider(bridge)

	proof, err := proofs.NewHashProofSystem(0).Prove(protocol.CircuitID("consensus.block.verify.v1.testnet"), protocol.Witness("block-1"))
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	verifyReq, err := json.Marshal(pblockchain.ConsensusVerifyBlockRequest{
		Header: protocol.BlockHeader{
			Number:    1,
			Hash:      protocol.Hash("block-1"),
			Parent:    protocol.Hash("genesis-hash"),
			Timestamp: 1,
		},
		Proof: proof,
	})
	if err != nil {
		t.Fatalf("marshal verify request: %v", err)
	}
	if _, err := provider.VerifyConsensusBlockHook(verifyReq); err != nil {
		t.Fatalf("verify consensus block hook: %v", err)
	}

	badAttReq, err := json.Marshal(pblockchain.ConsensusSubmitAttestationRequest{
		NodeID:    protocol.NodeID("node-b"),
		BlockHash: protocol.Hash("block-1"),
	})
	if err != nil {
		t.Fatalf("marshal bad attestation request: %v", err)
	}
	_, err = provider.SubmitConsensusAttestationHook(badAttReq)
	if !errors.Is(err, consensus.ErrUnregisteredNode) {
		t.Fatalf("expected unregistered node error, got %v", err)
	}
}

func TestInstallRuntimeBridgeFromEnvEnforcesConsensusCircuitPolicy(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_CONSENSUS_NETWORK_TYPE", "o2ul-testnet")

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
	proof, err := production.Prove(protocol.CircuitID("consensus.block.verify.v1.testnet"), protocol.Witness("witness"))
	if err != nil {
		t.Fatalf("production prove: %v", err)
	}

	reqOK, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("consensus.block.verify.v1.testnet"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal proof verify request: %v", err)
	}
	out, err := pc.Run(reqOK)
	if err != nil {
		t.Fatalf("run proof precompile (ok path): %v", err)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected successful proof verification for required consensus circuit")
	}

	reqMismatch, err := json.Marshal(pblockchain.ProofVerifyRequest{
		Circuit:      protocol.CircuitID("consensus.block.verify.v1.mainnet"),
		Proof:        proof,
		PublicInputs: protocol.PublicInputs("witness"),
	})
	if err != nil {
		t.Fatalf("marshal mismatch proof verify request: %v", err)
	}
	_, err = pc.Run(reqMismatch)
	if err == nil {
		t.Fatal("expected mismatch path to return consensus circuit policy error")
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

func TestInstallRuntimeBridgeFromEnvRejectsAmbiguousExternalProviderConfig(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "production")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_PROOFS_PRODUCTION_FLAVOR", "external")
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_URL", "http://example.invalid")
	t.Setenv("O2UL_PROOFS_EXTERNAL_PROVIDER_CMD", "echo provider")

	path := filepath.Join(t.TempDir(), "proof-circuits.json")
	if err := os.WriteFile(path, []byte(`{"circuits":{"proof":"6b65792d31"}}`), 0o600); err != nil {
		t.Fatalf("write circuit key file: %v", err)
	}
	t.Setenv("O2UL_PROOFS_CIRCUIT_KEYS_JSON", path)

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected ambiguous provider config error")
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

func TestInstallRuntimeBridgeFromEnvRejectsInvalidViewKeysDisclosureKeyHex(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "production")
	t.Setenv("O2UL_VIEWKEYS_DISCLOSURE_KEY_HEX", "bad-hex")

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected invalid viewkeys disclosure key env error")
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

func TestInstallRuntimeBridgeFromEnvRejectsInvalidNFTProvenanceKeyHex(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "production")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_NFT_PROVENANCE_KEY_HEX", "bad-hex")

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected invalid nft provenance key env error")
	}
}

func TestInstallRuntimeBridgeFromEnvSupportsNFTProductionPathWithCustomProvenanceKey(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "production")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "deterministic")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_NFT_PROVENANCE_KEY_HEX", "6b65792d637573746f6d")

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

	defaultProof, err := nft.NewHashProductionOwnershipVerifier().CreateProof(mintResp.Note)
	if err != nil {
		t.Fatalf("create default ownership proof: %v", err)
	}
	verifyPC := vm.PrecompiledContractsPrague[vm.O2ULPrecompileNFTOwnershipVerify]
	if verifyPC == nil {
		t.Fatal("expected nft ownership verify precompile")
	}
	defaultVerifyReq, err := json.Marshal(pblockchain.NFTOwnershipVerifyRequest{Note: mintResp.Note, Proof: defaultProof})
	if err != nil {
		t.Fatalf("marshal default verify request: %v", err)
	}
	defaultVerifyOut, err := verifyPC.Run(defaultVerifyReq)
	if err != nil {
		t.Fatalf("run default verify precompile: %v", err)
	}
	var defaultVerifyResp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(defaultVerifyOut, &defaultVerifyResp); err != nil {
		t.Fatalf("unmarshal default verify response: %v", err)
	}
	if defaultVerifyResp.OK {
		t.Fatal("expected default production verifier proof to fail when custom provenance key is configured")
	}

	customVerifier, err := nft.NewProvenanceProductionOwnershipVerifierWithKey([]byte("key-custom"))
	if err != nil {
		t.Fatalf("new custom provenance verifier: %v", err)
	}
	customProof, err := customVerifier.CreateProof(mintResp.Note)
	if err != nil {
		t.Fatalf("create custom ownership proof: %v", err)
	}
	customVerifyReq, err := json.Marshal(pblockchain.NFTOwnershipVerifyRequest{Note: mintResp.Note, Proof: customProof})
	if err != nil {
		t.Fatalf("marshal custom verify request: %v", err)
	}
	customVerifyOut, err := verifyPC.Run(customVerifyReq)
	if err != nil {
		t.Fatalf("run custom verify precompile: %v", err)
	}
	var customVerifyResp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(customVerifyOut, &customVerifyResp); err != nil {
		t.Fatalf("unmarshal custom verify response: %v", err)
	}
	if !customVerifyResp.OK {
		t.Fatal("expected successful nft ownership verification with custom provenance key")
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

func TestInstallRuntimeBridgeFromEnvRejectsInvalidThresholdProductionKeyHex(t *testing.T) {
	t.Setenv("O2UL_BACKEND_PROOFS", "deterministic")
	t.Setenv("O2UL_BACKEND_SHIELDED", "deterministic")
	t.Setenv("O2UL_BACKEND_NFT", "deterministic")
	t.Setenv("O2UL_BACKEND_THRESHOLD", "production")
	t.Setenv("O2UL_BACKEND_VIEWKEYS", "deterministic")
	t.Setenv("O2UL_THRESHOLD_PRODUCTION_KEY_HEX", "bad-hex")

	err := InstallRuntimeBridgeFromEnv()
	if err == nil {
		t.Fatal("expected invalid threshold production key env error")
	}
}
