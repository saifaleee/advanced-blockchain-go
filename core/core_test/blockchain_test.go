package core_test

import (
	"bytes"
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core" // Adjust import path
)

func TestNewBlockchain(t *testing.T) {
	difficulty := 4
	bc, err := core.NewBlockchain(difficulty)
	if err != nil {
		t.Fatalf("NewBlockchain failed: %v", err)
	}

	if bc == nil {
		t.Fatal("NewBlockchain returned nil blockchain")
	}
	if len(bc.Blocks) != 1 {
		t.Fatalf("Blockchain should have 1 block (genesis) after creation, found %d", len(bc.Blocks))
	}
	genesis := bc.Blocks[0]
	if genesis.Height != 0 {
		t.Errorf("Genesis block height should be 0, got %d", genesis.Height)
	}
	if len(genesis.PrevBlockHash) != 0 {
		t.Errorf("Genesis block PrevBlockHash should be empty, got %x", genesis.PrevBlockHash)
	}
	if bc.Difficulty != difficulty {
		t.Errorf("Blockchain difficulty mismatch: expected %d, got %d", difficulty, bc.Difficulty)
	}

	// Validate Genesis PoW
	powGenesis := core.NewProofOfWork(genesis, difficulty)
	if !powGenesis.Validate() {
		t.Errorf("Genesis block failed PoW validation")
	}
}

func TestAddBlock(t *testing.T) {
	difficulty := 5 // Low difficulty for faster testing
	bc, err := core.NewBlockchain(difficulty)
	if err != nil {
		t.Fatalf("Setup: NewBlockchain failed: %v", err)
	}

	txs := createSampleTransactions(3)
	err = bc.AddBlock(txs)
	if err != nil {
		t.Fatalf("AddBlock failed: %v", err)
	}

	if len(bc.Blocks) != 2 {
		t.Fatalf("Blockchain should have 2 blocks after adding one, found %d", len(bc.Blocks))
	}

	genesisBlock := bc.Blocks[0]
	newBlock := bc.Blocks[1]

	if newBlock.Height != 1 {
		t.Errorf("New block height should be 1, got %d", newBlock.Height)
	}
	if !bytes.Equal(newBlock.PrevBlockHash, genesisBlock.Hash) {
		t.Errorf("New block PrevBlockHash %x does not match genesis block hash %x", newBlock.PrevBlockHash, genesisBlock.Hash)
	}
	if len(newBlock.Transactions) != len(txs) {
		t.Errorf("New block transaction count mismatch: expected %d, got %d", len(txs), len(newBlock.Transactions))
	}

	// Validate PoW of the new block
	powNew := core.NewProofOfWork(newBlock, difficulty)
	if !powNew.Validate() {
		t.Errorf("Newly added block failed PoW validation")
	}
}

func TestGetLatestBlock(t *testing.T) {
	difficulty := 4
	bc, _ := core.NewBlockchain(difficulty)
	latest := bc.GetLatestBlock()
	if latest == nil || latest != bc.Blocks[0] {
		t.Fatal("GetLatestBlock did not return the genesis block initially")
	}

	bc.AddBlock(createSampleTransactions(1))
	latest = bc.GetLatestBlock()
	if latest == nil || latest != bc.Blocks[1] {
		t.Fatal("GetLatestBlock did not return the second block after adding")
	}
}

func TestIsChainValid(t *testing.T) {
	difficulty := 6 // Slightly higher to make tampering more obvious failure
	bc, _ := core.NewBlockchain(difficulty)
	bc.AddBlock(createSampleTransactions(2))
	bc.AddBlock(createSampleTransactions(1))

	// Test valid chain
	if !bc.IsChainValid() {
		t.Error("IsChainValid returned false for a known valid chain")
	}

	// Tamper with a block's data (e.g., change a transaction)
	if len(bc.Blocks) > 1 && len(bc.Blocks[1].Transactions) > 0 {
		t.Log("Tampering with Block 1 transaction data...")
		// IMPORTANT: Changing transaction data invalidates the Merkle root and the block hash.
		// The IsChainValid function primarily checks PoW and PrevBlockHash links.
		// A more robust validation would re-calculate Merkle roots and hashes.
		// For this test, we'll tamper with the hash directly, which IsChainValid *does* check via PoW.
		originalHash := append([]byte{}, bc.Blocks[1].Hash...) // Make a copy
		bc.Blocks[1].Hash = []byte("tamperedhash123")

		if bc.IsChainValid() {
			t.Error("IsChainValid returned true for a chain with a tampered block hash")
		}
		bc.Blocks[1].Hash = originalHash // Restore hash

		// Restore valid state before next test
		if !bc.IsChainValid() {
			t.Error("Chain should be valid after restoring hash, but IsChainValid failed")
		}

	} else {
		t.Log("Skipping tamper test: Not enough blocks or transactions")
	}

	// Tamper with the PrevBlockHash link
	if len(bc.Blocks) > 1 {
		t.Log("Tampering with Block 2 PrevBlockHash link...")
		originalPrevHash := append([]byte{}, bc.Blocks[2].PrevBlockHash...) // Make a copy
		bc.Blocks[2].PrevBlockHash = []byte("brokenlinkabc")

		if bc.IsChainValid() {
			t.Error("IsChainValid returned true for a chain with a broken PrevBlockHash link")
		}
		bc.Blocks[2].PrevBlockHash = originalPrevHash // Restore

		// Restore valid state before next test
		if !bc.IsChainValid() {
			t.Error("Chain should be valid after restoring prev hash, but IsChainValid failed")
		}
	}
}
