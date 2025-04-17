package core_test

import (
	"bytes"
	"fmt" // Import fmt
	"math/big"
	"testing"
	"time"

	"advanced-blockchain-go/v1/core"
	// Import bloom for testing filter
)

// Helper to create sample transactions (unchanged)
func createSampleTransactions(count int) []*core.Transaction {
	txs := make([]*core.Transaction, count)
	for i := 0; i < count; i++ {
		txs[i] = core.NewTransaction([]byte(fmt.Sprintf("Test TX %d", i+1)))
	}
	return txs
}

func TestNewBlockSharded(t *testing.T) { // Renamed for clarity
	shardID := core.ShardID(1)
	prevHash := []byte("previousblockhashshard1")
	height := 1
	difficulty := 5
	txs := createSampleTransactions(2)

	block := core.NewBlock(shardID, txs, prevHash, height, difficulty)

	if block == nil {
		t.Fatal("NewBlock returned nil")
	}
	if block.ShardID != shardID {
		t.Errorf("Block ShardID mismatch: expected %d, got %d", shardID, block.ShardID)
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
	if block.Nonce <= 0 && difficulty > 0 {
		t.Error("Block nonce is not positive after mining")
	}
	if block.BloomFilter == nil || len(block.BloomFilter.BitSet) == 0 {
		t.Error("Block BloomFilter is nil or empty after creation")
	}

	// Verify PoW
	pow := core.NewProofOfWork(block, difficulty)
	if !pow.Validate() {
		t.Errorf("Newly created block failed PoW validation. Hash: %x", block.Hash)
	}

	// Verify Bloom Filter Content (basic check)
	filter := block.GetBloomFilter()
	if filter == nil {
		t.Fatal("Failed to reconstruct Bloom filter from block")
	}
	if !filter.Test(txs[0].ID) {
		t.Errorf("Bloom filter failed to test positive for included transaction %x", txs[0].ID)
	}
	if !filter.Test(txs[1].ID) {
		t.Errorf("Bloom filter failed to test positive for included transaction %x", txs[1].ID)
	}
	// Test for an item not included (should be false, barring false positives)
	if filter.Test([]byte("not included transaction")) {
		t.Logf("Warning: Bloom filter returned false positive for item not included (expected behavior sometimes)")
	}
}

// TestProofOfWork needs minimal change if prepareData is updated correctly
func TestProofOfWorkSharded(t *testing.T) { // Renamed
	shardID := core.ShardID(0)
	difficulty := 8
	block := &core.Block{
		ShardID:       shardID,
		Timestamp:     time.Now().Unix(),
		Transactions:  createSampleTransactions(1),
		PrevBlockHash: []byte("dummyprevhashshard0"),
		Height:        1,
	}
	block.MerkleRoot = block.CalculateMerkleRoot()
	block.BloomFilter = block.BuildBloomFilter() // Ensure bloom filter is built

	pow := core.NewProofOfWork(block, difficulty)
	nonce, hash := pow.Run()

	// --- Assertions remain the same as before ---
	if nonce < 0 {
		t.Fatalf("PoW Run returned negative nonce: %d", nonce)
	}
	if len(hash) == 0 {
		t.Fatal("PoW Run returned empty hash")
	}
	block.Nonce = nonce
	block.Hash = hash
	var hashInt big.Int
	hashInt.SetBytes(hash)
	target := big.NewInt(1)
	target.Lsh(target, uint(256-difficulty))
	if hashInt.Cmp(target) != -1 {
		t.Errorf("PoW hash %x does not meet target difficulty %d (target: %x)", hash, difficulty, target)
	}
	if !pow.Validate() {
		t.Error("PoW validation failed for a correctly mined block")
	}
	block.Nonce++
	powTampered := core.NewProofOfWork(block, difficulty)
	if powTampered.Validate() {
		t.Error("PoW validation succeeded for a block with incorrect nonce")
	}
}

func TestBlockSerializationSharded(t *testing.T) { // Renamed
	shardID := core.ShardID(2)
	prevHash := []byte("prevhashshard2")
	txs := createSampleTransactions(1)
	block := core.NewBlock(shardID, txs, prevHash, 1, 4)

	serialized := block.Serialize()
	if len(serialized) == 0 {
		t.Fatal("Serialization resulted in empty byte slice")
	}

	deserializedBlock, err := core.DeserializeBlock(serialized)
	if err != nil {
		t.Fatalf("Deserialization failed: %v", err)
	}

	// Compare fields
	if block.ShardID != deserializedBlock.ShardID {
		t.Errorf("ShardID mismatch: expected %d, got %d", block.ShardID, deserializedBlock.ShardID)
	}
	// ... (compare other fields: Timestamp, PrevBlockHash, Hash, MerkleRoot, Nonce, Height) ...
	if block.Timestamp != deserializedBlock.Timestamp {
		t.Errorf("Timestamp mismatch")
	}
	if !bytes.Equal(block.PrevBlockHash, deserializedBlock.PrevBlockHash) {
		t.Errorf("PrevBlockHash mismatch")
	}
	if !bytes.Equal(block.Hash, deserializedBlock.Hash) {
		t.Errorf("Hash mismatch")
	}
	if !bytes.Equal(block.MerkleRoot, deserializedBlock.MerkleRoot) {
		t.Errorf("MerkleRoot mismatch")
	}
	if block.Nonce != deserializedBlock.Nonce {
		t.Errorf("Nonce mismatch")
	}
	if block.Height != deserializedBlock.Height {
		t.Errorf("Height mismatch")
	}

	// Compare BloomFilterData
	if block.BloomFilter == nil || deserializedBlock.BloomFilter == nil {
		if !(block.BloomFilter == nil && deserializedBlock.BloomFilter == nil) { // Only okay if both are nil
			t.Fatalf("BloomFilter nil mismatch: original=%v, deserialized=%v", block.BloomFilter == nil, deserializedBlock.BloomFilter == nil)
		}
	} else {
		if block.BloomFilter.M != deserializedBlock.BloomFilter.M {
			t.Errorf("BloomFilter M mismatch")
		}
		if block.BloomFilter.K != deserializedBlock.BloomFilter.K {
			t.Errorf("BloomFilter K mismatch")
		}
		if !bytes.Equal(block.BloomFilter.BitSet, deserializedBlock.BloomFilter.BitSet) {
			t.Errorf("BloomFilter BitSet mismatch")
		}
	}

	// Compare Transactions
	if len(block.Transactions) != len(deserializedBlock.Transactions) {
		t.Fatalf("Transaction count mismatch")
	}
	for i := range block.Transactions {
		if !bytes.Equal(block.Transactions[i].ID, deserializedBlock.Transactions[i].ID) {
			t.Errorf("Transaction ID mismatch at index %d", i)
		}
		// Could compare other tx fields if necessary
	}
}
