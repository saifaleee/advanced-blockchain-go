package core_test

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

// Helper to create sample transactions
func createSampleTransactions(count int) []*core.Transaction {
	txs := make([]*core.Transaction, count)
	for i := 0; i < count; i++ {
		txs[i] = core.NewTransaction([]byte(fmt.Sprintf("Test TX %d", i+1)))
	}
	return txs
}

func TestNewBlock(t *testing.T) {
	prevHash := []byte("previousblockhash")
	height := 1
	difficulty := 5 // Low difficulty for faster testing
	txs := createSampleTransactions(2)

	block := core.NewBlock(txs, prevHash, height, difficulty)

	if block == nil {
		t.Fatal("NewBlock returned nil")
	}
	if block.Height != height {
		t.Errorf("Block height mismatch: expected %d, got %d", height, block.Height)
	}
	if !bytes.Equal(block.PrevBlockHash, prevHash) {
		t.Errorf("Block PrevBlockHash mismatch: expected %x, got %x", prevHash, block.PrevBlockHash)
	}
	if len(block.Transactions) != len(txs) {
		t.Errorf("Block transaction count mismatch: expected %d, got %d", len(txs), len(block.Transactions))
	}
	if len(block.Hash) == 0 {
		t.Error("Block hash is empty after mining")
	}
	if len(block.MerkleRoot) == 0 {
		t.Error("Block MerkleRoot is empty")
	}
	if block.Nonce <= 0 && difficulty > 0 { // Nonce should be > 0 if mining occurred
		t.Error("Block nonce is not positive after mining")
	}

	// Verify PoW
	pow := core.NewProofOfWork(block, difficulty)
	if !pow.Validate() {
		t.Errorf("Newly created block failed PoW validation. Hash: %x", block.Hash)
	}

	// Verify Merkle Root Calculation
	expectedMerkleRoot := block.CalculateMerkleRoot() // Re-calculate to check consistency
	if !bytes.Equal(block.MerkleRoot, expectedMerkleRoot) {
		t.Errorf("Block MerkleRoot mismatch on recalculation: stored %x, calculated %x", block.MerkleRoot, expectedMerkleRoot)
	}
}

func TestProofOfWork(t *testing.T) {
	difficulty := 8 // Moderate difficulty
	block := &core.Block{
		Timestamp:     time.Now().Unix(),
		Transactions:  createSampleTransactions(1),
		PrevBlockHash: []byte("dummyprevhash"),
		Height:        1,
	}
	block.MerkleRoot = block.CalculateMerkleRoot() // Need Merkle root for PoW data prep

	pow := core.NewProofOfWork(block, difficulty)
	nonce, hash := pow.Run()

	if nonce < 0 {
		t.Fatalf("PoW Run returned negative nonce: %d", nonce)
	}
	if len(hash) == 0 {
		t.Fatal("PoW Run returned empty hash")
	}

	// Set the results back on the block for validation check
	block.Nonce = nonce
	block.Hash = hash

	// Verify the hash meets the target difficulty
	var hashInt big.Int
	hashInt.SetBytes(hash)
	target := big.NewInt(1)
	target.Lsh(target, uint(256-difficulty))

	if hashInt.Cmp(target) != -1 { // Should be strictly less than target
		t.Errorf("PoW hash %x does not meet target difficulty %d (target: %x)", hash, difficulty, target)
	}

	// Validate using the Validate method
	if !pow.Validate() { // pow already has the block with nonce/hash set now
		t.Error("PoW validation failed for a correctly mined block")
	}

	// Test Validation Failure with incorrect nonce
	block.Nonce++                                         // Tamper with nonce
	powTampered := core.NewProofOfWork(block, difficulty) // Create new pow instance with tampered block
	if powTampered.Validate() {
		t.Error("PoW validation succeeded for a block with incorrect nonce")
	}
}

func TestBlockSerialization(t *testing.T) {
	prevHash := []byte("prevhash")
	txs := createSampleTransactions(1)
	block := core.NewBlock(txs, prevHash, 1, 4) // Low diff for speed

	serialized := block.Serialize()
	if len(serialized) == 0 {
		t.Fatal("Serialization resulted in empty byte slice")
	}

	deserializedBlock, err := core.DeserializeBlock(serialized)
	if err != nil {
		t.Fatalf("Deserialization failed: %v", err)
	}

	// Compare fields (Can't compare Transactions directly as pointers will differ)
	if block.Timestamp != deserializedBlock.Timestamp {
		t.Errorf("Timestamp mismatch: expected %d, got %d", block.Timestamp, deserializedBlock.Timestamp)
	}
	if !bytes.Equal(block.PrevBlockHash, deserializedBlock.PrevBlockHash) {
		t.Errorf("PrevBlockHash mismatch: expected %x, got %x", block.PrevBlockHash, deserializedBlock.PrevBlockHash)
	}
	if !bytes.Equal(block.Hash, deserializedBlock.Hash) {
		t.Errorf("Hash mismatch: expected %x, got %x", block.Hash, deserializedBlock.Hash)
	}
	if !bytes.Equal(block.MerkleRoot, deserializedBlock.MerkleRoot) {
		t.Errorf("MerkleRoot mismatch: expected %x, got %x", block.MerkleRoot, deserializedBlock.MerkleRoot)
	}
	if block.Nonce != deserializedBlock.Nonce {
		t.Errorf("Nonce mismatch: expected %d, got %d", block.Nonce, deserializedBlock.Nonce)
	}
	if block.Height != deserializedBlock.Height {
		t.Errorf("Height mismatch: expected %d, got %d", block.Height, deserializedBlock.Height)
	}
	if len(block.Transactions) != len(deserializedBlock.Transactions) {
		t.Fatalf("Transaction count mismatch: expected %d, got %d", len(block.Transactions), len(deserializedBlock.Transactions))
	}
	// Compare transaction IDs as a proxy for content
	for i := range block.Transactions {
		if !bytes.Equal(block.Transactions[i].ID, deserializedBlock.Transactions[i].ID) {
			t.Errorf("Transaction ID mismatch at index %d: expected %x, got %x", i, block.Transactions[i].ID, deserializedBlock.Transactions[i].ID)
		}
	}
}
