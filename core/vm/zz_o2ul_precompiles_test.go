package vm

import (
	"errors"
	"testing"
)

type fixedO2ULProvider struct{}

func (fixedO2ULProvider) VerifyProofHook(input []byte) ([]byte, error) { return []byte("ok"), nil }
func (fixedO2ULProvider) CreateShieldedNoteHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) SpendShieldedNoteHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) VerifyShieldedTransactionHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) MintNFTHook(input []byte) ([]byte, error)     { return []byte("ok"), nil }
func (fixedO2ULProvider) TransferNFTHook(input []byte) ([]byte, error) { return []byte("ok"), nil }
func (fixedO2ULProvider) VerifyNFTOwnershipHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) GenerateThresholdGroupKeyHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) SignThresholdPartialHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) AggregateThresholdPartialsHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}
func (fixedO2ULProvider) GenerateViewKeyHook(input []byte) ([]byte, error) { return []byte("ok"), nil }
func (fixedO2ULProvider) DiscloseViewKeyHook(input []byte) ([]byte, error) { return []byte("ok"), nil }
func (fixedO2ULProvider) IsDisclosureReplayHook(input []byte) ([]byte, error) {
	return []byte("ok"), nil
}

func TestO2ULPrecompilesRegisteredInCancunAndPrague(t *testing.T) {
	addresses := []struct {
		name string
		addr [20]byte
	}{
		{"proof", O2ULPrecompileProofVerify},
		{"shielded create", O2ULPrecompileShieldedCreate},
		{"shielded spend", O2ULPrecompileShieldedSpend},
		{"shielded verify", O2ULPrecompileShieldedVerifyTx},
		{"nft mint", O2ULPrecompileNFTMint},
		{"nft transfer", O2ULPrecompileNFTTransfer},
		{"nft verify", O2ULPrecompileNFTOwnershipVerify},
		{"threshold generate", O2ULPrecompileThresholdGenerate},
		{"threshold sign", O2ULPrecompileThresholdSign},
		{"threshold aggregate", O2ULPrecompileThresholdAggregate},
		{"viewkey generate", O2ULPrecompileViewKeyGenerate},
		{"viewkey disclose", O2ULPrecompileViewKeyDisclose},
		{"viewkey replay", O2ULPrecompileViewKeyReplayCheck},
	}
	for _, a := range addresses {
		if _, ok := PrecompiledContractsCancun[a.addr]; !ok {
			t.Fatalf("missing %s precompile in Cancun", a.name)
		}
		if _, ok := PrecompiledContractsPrague[a.addr]; !ok {
			t.Fatalf("missing %s precompile in Prague", a.name)
		}
	}
}

func TestO2ULPrecompileRequiresRuntimeProvider(t *testing.T) {
	SetO2ULRuntimeHookProvider(nil)
	pc := PrecompiledContractsPrague[O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof precompile registered")
	}
	_, err := pc.Run([]byte("payload"))
	if !errors.Is(err, ErrO2ULRuntimeProviderNotSet) {
		t.Fatalf("expected ErrO2ULRuntimeProviderNotSet, got %v", err)
	}
}

func TestO2ULPrecompileRoutesToRuntimeProvider(t *testing.T) {
	SetO2ULRuntimeHookProvider(fixedO2ULProvider{})
	t.Cleanup(func() { SetO2ULRuntimeHookProvider(nil) })

	pc := PrecompiledContractsPrague[O2ULPrecompileProofVerify]
	if pc == nil {
		t.Fatal("expected proof precompile registered")
	}
	out, err := pc.Run([]byte("payload"))
	if err != nil {
		t.Fatalf("run precompile: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}
