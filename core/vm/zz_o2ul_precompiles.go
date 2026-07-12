package vm

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
)

const (
	o2ulPrecompileBaseGas    uint64 = 150
	o2ulPrecompilePerByteGas uint64 = 2
)

var (
	ErrO2ULRuntimeProviderNotSet = errors.New("o2ul runtime hook provider not set")
)

var (
	O2ULPrecompileProofVerify        = common.HexToAddress("0x0000000000000000000000000000000000000100")
	O2ULPrecompileShieldedCreate     = common.HexToAddress("0x0000000000000000000000000000000000000101")
	O2ULPrecompileShieldedSpend      = common.HexToAddress("0x0000000000000000000000000000000000000102")
	O2ULPrecompileShieldedVerifyTx   = common.HexToAddress("0x0000000000000000000000000000000000000103")
	O2ULPrecompileNFTMint            = common.HexToAddress("0x0000000000000000000000000000000000000104")
	O2ULPrecompileNFTTransfer        = common.HexToAddress("0x0000000000000000000000000000000000000105")
	O2ULPrecompileNFTOwnershipVerify = common.HexToAddress("0x0000000000000000000000000000000000000106")
	O2ULPrecompileThresholdGenerate  = common.HexToAddress("0x0000000000000000000000000000000000000107")
	O2ULPrecompileThresholdSign      = common.HexToAddress("0x0000000000000000000000000000000000000108")
	O2ULPrecompileThresholdAggregate = common.HexToAddress("0x0000000000000000000000000000000000000109")
	O2ULPrecompileViewKeyGenerate    = common.HexToAddress("0x000000000000000000000000000000000000010a")
	O2ULPrecompileViewKeyDisclose    = common.HexToAddress("0x000000000000000000000000000000000000010b")
	O2ULPrecompileViewKeyReplayCheck = common.HexToAddress("0x000000000000000000000000000000000000010c")
	O2ULPrecompileFeeAllocate        = common.HexToAddress("0x000000000000000000000000000000000000010d")
	O2ULPrecompileArbitrationSelect  = common.HexToAddress("0x000000000000000000000000000000000000010e")
	O2ULPrecompileArbitrationSubmit  = common.HexToAddress("0x000000000000000000000000000000000000010f")
	O2ULPrecompileArbitrationRule    = common.HexToAddress("0x0000000000000000000000000000000000000110")
	O2ULPrecompileEscrowDispute      = common.HexToAddress("0x0000000000000000000000000000000000000111")
	O2ULPrecompileEscrowSettle       = common.HexToAddress("0x0000000000000000000000000000000000000112")
	O2ULPrecompileDisputeStatus      = common.HexToAddress("0x0000000000000000000000000000000000000113")
	O2ULPrecompileDisputeStatusBatch = common.HexToAddress("0x0000000000000000000000000000000000000114")
	O2ULPrecompileFeeConfigureSplit  = common.HexToAddress("0x0000000000000000000000000000000000000115")
	O2ULPrecompileFeeGetSplit        = common.HexToAddress("0x0000000000000000000000000000000000000116")
)

// O2ULRuntimeHookProvider provides the runtime hook entrypoints invoked by O2UL precompiles.
// Implementations in downstream integrations can decode/encode ABI payloads as needed.
type O2ULRuntimeHookProvider interface {
	VerifyProofHook(input []byte) ([]byte, error)
	CreateShieldedNoteHook(input []byte) ([]byte, error)
	SpendShieldedNoteHook(input []byte) ([]byte, error)
	VerifyShieldedTransactionHook(input []byte) ([]byte, error)
	MintNFTHook(input []byte) ([]byte, error)
	TransferNFTHook(input []byte) ([]byte, error)
	VerifyNFTOwnershipHook(input []byte) ([]byte, error)
	GenerateThresholdGroupKeyHook(input []byte) ([]byte, error)
	SignThresholdPartialHook(input []byte) ([]byte, error)
	AggregateThresholdPartialsHook(input []byte) ([]byte, error)
	GenerateViewKeyHook(input []byte) ([]byte, error)
	DiscloseViewKeyHook(input []byte) ([]byte, error)
	IsDisclosureReplayHook(input []byte) ([]byte, error)
	AllocateFeeHook(input []byte) ([]byte, error)
	ConfigureFeeDistributionSplitHook(input []byte) ([]byte, error)
	GetFeeDistributionSplitHook(input []byte) ([]byte, error)
	SelectArbitratorsHook(input []byte) ([]byte, error)
	SubmitArbitrationEvidenceHook(input []byte) ([]byte, error)
	RuleArbitrationHook(input []byte) ([]byte, error)
	TriggerEscrowDisputeAndAllocateHook(input []byte) ([]byte, error)
	SettleEscrowFromArbitrationHook(input []byte) ([]byte, error)
	GetDisputeLifecycleStatusHook(input []byte) ([]byte, error)
	GetDisputeLifecycleStatusesHook(input []byte) ([]byte, error)
}

var o2ulRuntimeHooks O2ULRuntimeHookProvider

func SetO2ULRuntimeHookProvider(provider O2ULRuntimeHookProvider) {
	o2ulRuntimeHooks = provider
}

func o2ulRequiredGas(input []byte) uint64 {
	return o2ulPrecompileBaseGas + uint64(len(input))*o2ulPrecompilePerByteGas
}

type o2ulHookPrecompile struct {
	run func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error)
}

func (p *o2ulHookPrecompile) RequiredGas(input []byte) uint64 {
	return o2ulRequiredGas(input)
}

func (p *o2ulHookPrecompile) Run(input []byte) ([]byte, error) {
	if o2ulRuntimeHooks == nil {
		return nil, ErrO2ULRuntimeProviderNotSet
	}
	return p.run(o2ulRuntimeHooks, input)
}

func registerO2ULPrecompiles(target PrecompiledContracts) {
	target[O2ULPrecompileProofVerify] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.VerifyProofHook(input)
	}}
	target[O2ULPrecompileShieldedCreate] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.CreateShieldedNoteHook(input)
	}}
	target[O2ULPrecompileShieldedSpend] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.SpendShieldedNoteHook(input)
	}}
	target[O2ULPrecompileShieldedVerifyTx] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.VerifyShieldedTransactionHook(input)
	}}
	target[O2ULPrecompileNFTMint] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.MintNFTHook(input)
	}}
	target[O2ULPrecompileNFTTransfer] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.TransferNFTHook(input)
	}}
	target[O2ULPrecompileNFTOwnershipVerify] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.VerifyNFTOwnershipHook(input)
	}}
	target[O2ULPrecompileThresholdGenerate] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.GenerateThresholdGroupKeyHook(input)
	}}
	target[O2ULPrecompileThresholdSign] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.SignThresholdPartialHook(input)
	}}
	target[O2ULPrecompileThresholdAggregate] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.AggregateThresholdPartialsHook(input)
	}}
	target[O2ULPrecompileViewKeyGenerate] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.GenerateViewKeyHook(input)
	}}
	target[O2ULPrecompileViewKeyDisclose] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.DiscloseViewKeyHook(input)
	}}
	target[O2ULPrecompileViewKeyReplayCheck] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.IsDisclosureReplayHook(input)
	}}
	target[O2ULPrecompileFeeAllocate] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.AllocateFeeHook(input)
	}}
	target[O2ULPrecompileFeeConfigureSplit] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.ConfigureFeeDistributionSplitHook(input)
	}}
	target[O2ULPrecompileFeeGetSplit] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.GetFeeDistributionSplitHook(input)
	}}
	target[O2ULPrecompileArbitrationSelect] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.SelectArbitratorsHook(input)
	}}
	target[O2ULPrecompileArbitrationSubmit] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.SubmitArbitrationEvidenceHook(input)
	}}
	target[O2ULPrecompileArbitrationRule] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.RuleArbitrationHook(input)
	}}
	target[O2ULPrecompileEscrowDispute] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.TriggerEscrowDisputeAndAllocateHook(input)
	}}
	target[O2ULPrecompileEscrowSettle] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.SettleEscrowFromArbitrationHook(input)
	}}
	target[O2ULPrecompileDisputeStatus] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.GetDisputeLifecycleStatusHook(input)
	}}
	target[O2ULPrecompileDisputeStatusBatch] = &o2ulHookPrecompile{run: func(provider O2ULRuntimeHookProvider, input []byte) ([]byte, error) {
		return provider.GetDisputeLifecycleStatusesHook(input)
	}}
}

func rebuildPrecompileAddressSets() {
	PrecompiledAddressesHomestead = nil
	PrecompiledAddressesByzantium = nil
	PrecompiledAddressesIstanbul = nil
	PrecompiledAddressesBerlin = nil
	PrecompiledAddressesCancun = nil
	PrecompiledAddressesPrague = nil
	for k := range PrecompiledContractsHomestead {
		PrecompiledAddressesHomestead = append(PrecompiledAddressesHomestead, k)
	}
	for k := range PrecompiledContractsByzantium {
		PrecompiledAddressesByzantium = append(PrecompiledAddressesByzantium, k)
	}
	for k := range PrecompiledContractsIstanbul {
		PrecompiledAddressesIstanbul = append(PrecompiledAddressesIstanbul, k)
	}
	for k := range PrecompiledContractsBerlin {
		PrecompiledAddressesBerlin = append(PrecompiledAddressesBerlin, k)
	}
	for k := range PrecompiledContractsCancun {
		PrecompiledAddressesCancun = append(PrecompiledAddressesCancun, k)
	}
	for k := range PrecompiledContractsPrague {
		PrecompiledAddressesPrague = append(PrecompiledAddressesPrague, k)
	}
}

func init() {
	registerO2ULPrecompiles(PrecompiledContractsCancun)
	registerO2ULPrecompiles(PrecompiledContractsPrague)
	registerO2ULPrecompiles(PrecompiledContractsBLS)
	registerO2ULPrecompiles(PrecompiledContractsVerkle)
	rebuildPrecompileAddressSets()
}
