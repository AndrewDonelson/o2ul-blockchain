package o2ulbridge

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestNewContractStorageGovernanceReaderEnvValidation(t *testing.T) {
	t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", "http://127.0.0.1:8545")
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CALLER_MAPPING_SLOT", "7")
	t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_EXECUTABLE_MAPPING_SLOT", "0x09")
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000001006")
	t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_CONTRACT_ADDRESS", "0x0000000000000000000000000000000000002001")

	r, err := newContractStorageGovernanceReader("")
	if err != nil {
		t.Fatalf("new contract storage reader: %v", err)
	}
	if r.governorCallerMapSlot != 7 {
		t.Fatalf("unexpected governor slot: %d", r.governorCallerMapSlot)
	}
	if r.timelockExecMapSlot != 9 {
		t.Fatalf("unexpected timelock slot: %d", r.timelockExecMapSlot)
	}
	if r.rpcEndpoint != "http://127.0.0.1:8545" {
		t.Fatalf("unexpected rpc endpoint: %s", r.rpcEndpoint)
	}
}

func TestNewContractStorageGovernanceReaderRequiresSlots(t *testing.T) {
	t.Setenv("O2UL_FEE_SPLIT_GOVERNANCE_RPC_URL", "http://127.0.0.1:8545")
	t.Setenv("O2UL_FEE_SPLIT_GOVERNOR_CALLER_MAPPING_SLOT", "")
	t.Setenv("O2UL_FEE_SPLIT_TIMELOCK_EXECUTABLE_MAPPING_SLOT", "")

	if _, err := newContractStorageGovernanceReader(""); err == nil {
		t.Fatal("expected slot validation error")
	}
}

func TestDefaultGovernanceRPCEndpointFromNodeDataDir(t *testing.T) {
	endpoint := defaultGovernanceRPCEndpoint("/tmp/o2ul-node")
	if endpoint != "/tmp/o2ul-node/geth.ipc" {
		t.Fatalf("unexpected endpoint: %s", endpoint)
	}
}

func TestMappingStorageSlotHelpers(t *testing.T) {
	address := common.HexToAddress("0x0000000000000000000000000000000000001006")
	proposalHash := crypto.Keccak256Hash([]byte("proposal-1"))

	addrSlot := mappingStorageSlotForAddress(address, 5)
	proposalSlot := mappingStorageSlotForHash(proposalHash, 11)
	if addrSlot == (common.Hash{}) {
		t.Fatal("expected non-zero address mapping slot")
	}
	if proposalSlot == (common.Hash{}) {
		t.Fatal("expected non-zero proposal mapping slot")
	}
	if addrSlot == proposalSlot {
		t.Fatal("expected distinct mapping slots")
	}
}
