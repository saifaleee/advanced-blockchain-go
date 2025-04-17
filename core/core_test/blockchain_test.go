package core_test

import (
	"bytes"
	"fmt" // Import fmt
	"sync"
	"testing"

	//"time"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

// Helper function to create a test blockchain
func setupShardedBlockchain(t *testing.T, numShards uint, difficulty int) *core.Blockchain {
	t.Helper() // Marks this as a test helper
	bc, err := core.NewBlockchain(numShards, difficulty)
	if err != nil {
		t.Fatalf("Setup: NewBlockchain failed: %v", err)
	}
	return bc
}

func TestNewShardedBlockchain(t *testing.T) {
	numShards := uint(2)
	difficulty := 4
	bc := setupShardedBlockchain(t, numShards, difficulty)

	if bc == nil {
		t.Fatal("NewBlockchain returned nil")
	}
	if bc.ShardManager == nil {
		t.Fatal("Blockchain has nil ShardManager")
	}
	if bc.ShardManager.ShardCount != numShards {
		t.Fatalf("ShardManager count mismatch: expected %d, got %d", numShards, bc.ShardManager.ShardCount)
	}
	if bc.Difficulty != difficulty {
		t.Errorf("Blockchain difficulty mismatch: expected %d, got %d", difficulty, bc.Difficulty)
	}

	// Check genesis block for each shard
	for i := uint(0); i < numShards; i++ {
		shardID := core.ShardID(i)
		latestBlock, err := bc.GetLatestBlock(shardID)
		if err != nil {
			t.Errorf("Error getting latest block for shard %d: %v", shardID, err)
			continue
		}
		if latestBlock.Height != 0 {
			t.Errorf("Shard %d genesis block height should be 0, got %d", shardID, latestBlock.Height)
		}
		if len(latestBlock.PrevBlockHash) != 0 {
			t.Errorf("Shard %d genesis block PrevBlockHash should be empty, got %x", shardID, latestBlock.PrevBlockHash)
		}
		if latestBlock.ShardID != shardID {
			t.Errorf("Shard %d genesis block has wrong ShardID %d", shardID, latestBlock.ShardID)
		}
		// Validate Genesis PoW
		powGenesis := core.NewProofOfWork(latestBlock, difficulty)
		if !powGenesis.Validate() {
			t.Errorf("Shard %d Genesis block failed PoW validation", shardID)
		}
	}
}

func TestAddTransactionAndMineShardBlock(t *testing.T) {
	numShards := uint(2)
	difficulty := 6 // Low difficulty for testing
	bc := setupShardedBlockchain(t, numShards, difficulty)

	// Create transactions expected to land in different shards (likely, not guaranteed)
	tx1 := core.NewTransaction([]byte("Transaction Data One"))
	tx2 := core.NewTransaction([]byte("Transaction Data Two Different"))
	tx3 := core.NewTransaction([]byte("More Transaction Data Three"))

	// Determine target shards
	targetShard1 := bc.ShardManager.DetermineShard(tx1.ID)
	targetShard2 := bc.ShardManager.DetermineShard(tx2.ID)
	targetShard3 := bc.ShardManager.DetermineShard(tx3.ID)
	t.Logf("Tx1 (%x) -> Shard %d", tx1.ID, targetShard1)
	t.Logf("Tx2 (%x) -> Shard %d", tx2.ID, targetShard2)
	t.Logf("Tx3 (%x) -> Shard %d", tx3.ID, targetShard3)

	// Add transactions
	if err := bc.AddTransaction(tx1); err != nil {
		t.Fatalf("Add tx1 failed: %v", err)
	}
	if err := bc.AddTransaction(tx2); err != nil {
		t.Fatalf("Add tx2 failed: %v", err)
	}
	if err := bc.AddTransaction(tx3); err != nil {
		t.Fatalf("Add tx3 failed: %v", err)
	}

	// Mine blocks for each shard where transactions landed
	minedShards := make(map[core.ShardID]bool)
	minedShards[targetShard1] = true
	minedShards[targetShard2] = true
	minedShards[targetShard3] = true

	minedBlocks := make(map[core.ShardID]*core.Block)

	for shardID := range minedShards {
		t.Logf("Attempting to mine block for Shard %d...", shardID)
		minedBlock, err := bc.MineShardBlock(shardID)
		if err != nil {
			// It's possible a shard got no txs if DetermineShard mapped them all elsewhere
			shard, _ := bc.ShardManager.GetShard(shardID)
			shard.TxPoolMu.RLock()
			poolSize := len(shard.TxPool)
			shard.TxPoolMu.RUnlock()
			if poolSize == 0 {
				t.Logf("Shard %d had no transactions, mining skipped or resulted in empty block (expected). Err: %v", shardID, err)
				// If empty blocks are allowed, MineShardBlock shouldn't error here unless PoW fails
				// If empty blocks NOT allowed, this error might be expected. Adjust test based on MineShardBlock behavior.
				// Let's assume empty blocks are mined for now. Check if MineShardBlock returns nil or error.
				if minedBlock != nil { // If it *did* mine an empty block
					t.Logf("Shard %d mined an empty block.", shardID)
					minedBlocks[shardID] = minedBlock
				} else {
					t.Logf("Shard %d mining resulted in error (likely no txs and empty blocks disallowed): %v", shardID, err)
					// Skip further checks for this shard if mining failed as expected/handled
					continue
				}

			} else {
				t.Fatalf("MineShardBlock for shard %d failed unexpectedly: %v", shardID, err)
			}
		} else if minedBlock == nil {
			t.Fatalf("MineShardBlock for shard %d returned nil block without error", shardID)
		} else {
			t.Logf("Shard %d mined block %d successfully.", shardID, minedBlock.Height)
			minedBlocks[shardID] = minedBlock // Store successfully mined block
		}
	}

	// Validate the mined blocks
	for shardID, minedBlock := range minedBlocks {
		if minedBlock.Height != 1 {
			t.Errorf("Shard %d mined block height should be 1, got %d", shardID, minedBlock.Height)
		}
		if minedBlock.ShardID != shardID {
			t.Errorf("Shard %d mined block has wrong ShardID %d", shardID, minedBlock.ShardID)
		}

		genesis, _ := bc.GetLatestBlock(shardID) // This should now be the genesis block
		genesis = genesis                        // Hack: Re-fetch genesis from the stored chain directly for comparison
		bc.ChainMu.RLock()
		genesis = bc.BlockChains[shardID][0]
		bc.ChainMu.RUnlock()

		if !bytes.Equal(minedBlock.PrevBlockHash, genesis.Hash) {
			t.Errorf("Shard %d block PrevBlockHash %x does not match genesis hash %x", shardID, minedBlock.PrevBlockHash, genesis.Hash)
		}

		// Check if the correct transactions are in the block
		foundTxCount := 0
		if shardID == targetShard1 {
			for _, tx := range minedBlock.Transactions {
				if bytes.Equal(tx.ID, tx1.ID) {
					foundTxCount++
				}
			}
		}
		if shardID == targetShard2 {
			for _, tx := range minedBlock.Transactions {
				if bytes.Equal(tx.ID, tx2.ID) {
					foundTxCount++
				}
			}
		}
		if shardID == targetShard3 {
			for _, tx := range minedBlock.Transactions {
				if bytes.Equal(tx.ID, tx3.ID) {
					foundTxCount++
				}
			}
		}

		// Check if *at least* the expected tx landed here (could be others if hash collision)
		expectedMinTx := 0
		if shardID == targetShard1 {
			expectedMinTx++
		}
		if shardID == targetShard2 {
			expectedMinTx++
		}
		if shardID == targetShard3 {
			expectedMinTx++
		}

		// This check is tricky because DetermineShard might map multiple tx to the same shard.
		// A better check is to verify *all* transactions in the block belong to this shard.
		blockTxIds := make(map[string]bool)
		for _, tx := range minedBlock.Transactions {
			if bc.ShardManager.DetermineShard(tx.ID) != shardID {
				t.Errorf("Shard %d block contains transaction %x belonging to a different shard (%d)",
					shardID, tx.ID, bc.ShardManager.DetermineShard(tx.ID))
			}
			blockTxIds[string(tx.ID)] = true
		}

		// Now verify our specific test transactions landed correctly
		if shardID == targetShard1 && !blockTxIds[string(tx1.ID)] {
			t.Errorf("Tx1 (%x) was expected in Shard %d block but not found", tx1.ID, shardID)
		}
		if shardID == targetShard2 && !blockTxIds[string(tx2.ID)] {
			t.Errorf("Tx2 (%x) was expected in Shard %d block but not found", tx2.ID, shardID)
		}
		if shardID == targetShard3 && !blockTxIds[string(tx3.ID)] {
			t.Errorf("Tx3 (%x) was expected in Shard %d block but not found", tx3.ID, shardID)
		}

		// Validate PoW
		pow := core.NewProofOfWork(minedBlock, difficulty)
		if !pow.Validate() {
			t.Errorf("Shard %d mined block failed PoW validation", shardID)
		}
	}
}

func TestIsChainValidSharded(t *testing.T) {
	numShards := uint(2)
	difficulty := 5
	bc := setupShardedBlockchain(t, numShards, difficulty)

	// Add some blocks to different shards
	var wg sync.WaitGroup
	numBlocksToAdd := 3
	for i := 0; i < numBlocksToAdd; i++ {
		for j := uint(0); j < numShards; j++ {
			wg.Add(1)
			go func(shardID core.ShardID) {
				defer wg.Done()
				tx := core.NewTransaction([]byte(fmt.Sprintf("Shard %d - Tx %d", shardID, i+1)))
				err := bc.AddTransaction(tx)
				if err != nil {
					t.Errorf("Error adding tx for shard %d: %v", shardID, err)
					return
				}
				_, err = bc.MineShardBlock(shardID)
				if err != nil {
					t.Errorf("Error mining block for shard %d: %v", shardID, err)
				}
			}(core.ShardID(j))
		}
		wg.Wait() // Wait for blocks at this 'height' across shards to finish mining
	}

	// Test valid chain
	if !bc.IsChainValid() {
		t.Error("IsChainValid returned false for a presumably valid sharded chain")
	}

	// Tamper with a block in one shard
	targetShardID := core.ShardID(0)
	bc.ChainMu.Lock() // Need lock to modify block slice
	if len(bc.BlockChains[targetShardID]) > 1 {
		t.Logf("Tampering with Shard %d Block 1 hash...", targetShardID)
		originalHash := append([]byte{}, bc.BlockChains[targetShardID][1].Hash...) // Copy
		bc.BlockChains[targetShardID][1].Hash = []byte("tamperedhash123")
		bc.ChainMu.Unlock() // Unlock after modification

		if bc.IsChainValid() {
			t.Errorf("IsChainValid returned true for a chain with a tampered block hash in shard %d", targetShardID)
		}

		// Restore
		bc.ChainMu.Lock()
		bc.BlockChains[targetShardID][1].Hash = originalHash
		bc.ChainMu.Unlock()

		// Re-validate after restore
		if !bc.IsChainValid() {
			t.Error("IsChainValid returned false after restoring tampered block hash")
		}

	} else {
		bc.ChainMu.Unlock()
		t.Logf("Skipping tamper test on shard %d: Not enough blocks", targetShardID)
	}
}

// TestCrossShardTransaction (Basic Placeholder Simulation)
func TestCrossShardTransaction(t *testing.T) {
	numShards := uint(2)
	difficulty := 4
	bc := setupShardedBlockchain(t, numShards, difficulty)

	sourceShard := core.ShardID(0)
	destShard := core.ShardID(1)

	// 1. Create and add the initiating transaction to the source shard
	crossTxData := []byte("Transfer 10 from AccA (Shard 0) to AccB (Shard 1)")
	initTx := core.NewCrossShardInitTransaction(crossTxData, sourceShard, destShard)
	err := bc.AddTransaction(initTx) // Should route to sourceShard
	if err != nil {
		t.Fatalf("Failed to add cross-shard init tx: %v", err)
	}
	if bc.ShardManager.DetermineShard(initTx.ID) != sourceShard {
		// This check depends on how AddTransaction routes cross-shard tx.
		// Our current AddTransaction routes based on SourceShard field if present.
		// Verify tx is in source shard pool:
		shard, _ := bc.ShardManager.GetShard(sourceShard)
		shard.TxPoolMu.RLock()
		found := false
		for _, ptx := range shard.TxPool {
			if bytes.Equal(ptx.ID, initTx.ID) {
				found = true
				break
			}
		}
		shard.TxPoolMu.RUnlock()
		if !found {
			t.Errorf("Cross-shard init tx %x not found in source shard %d pool", initTx.ID, sourceShard)
		}

	}

	// 2. Mine a block on the source shard, which should process the init tx
	//    and (in theory) generate a receipt (our MineShardBlock logs this placeholder)
	sourceBlock, err := bc.MineShardBlock(sourceShard)
	if err != nil {
		t.Fatalf("Failed to mine source shard block: %v", err)
	}
	if sourceBlock == nil {
		t.Fatal("Source shard block is nil after mining")
	}
	foundInitTxInBlock := false
	for _, tx := range sourceBlock.Transactions {
		if bytes.Equal(tx.ID, initTx.ID) {
			foundInitTxInBlock = true
			break
		}
	}
	if !foundInitTxInBlock {
		t.Errorf("Cross-shard init tx %x not found in mined source block", initTx.ID)
	}
	t.Logf("Mined source block %d on Shard %d containing cross-shard init tx %x", sourceBlock.Height, sourceShard, initTx.ID)

	// 3. Simulate receipt generation and delivery
	//    (In reality, this requires inter-shard communication protocol)
	receipt := &core.CrossShardReceipt{
		OriginShard:      sourceShard,
		DestinationShard: destShard,
		TransactionID:    initTx.ID,
		// Proof data omitted
	}
	t.Logf("Simulating delivery of receipt for Tx %x to Shard %d", initTx.ID, destShard)

	// 4. Simulate processing the receipt on the destination shard
	//    (This would normally trigger state updates and potentially a finalize tx)
	destShardInstance, _ := bc.ShardManager.GetShard(destShard)
	err = destShardInstance.ProcessCrossShardReceipt(receipt) // Uses placeholder implementation
	if err != nil {
		t.Fatalf("Destination shard failed to process receipt: %v", err)
	}
	// Placeholder: Check destination shard state if ProcessCrossShardReceipt updated it
	// e.g., stateValue, _ := destShardInstance.State.Get([]byte("AccB_Balance"))

	// 5. Mine a block on the destination shard
	//    (This block might contain a finalize transaction if one was created from the receipt)
	destBlock, err := bc.MineShardBlock(destShard)
	if err != nil {
		t.Fatalf("Failed to mine destination shard block: %v", err)
	}
	if destBlock == nil {
		t.Fatal("Destination shard block is nil after mining")
	}
	t.Logf("Mined destination block %d on Shard %d (may contain finalize tx - not implemented)", destBlock.Height, destShard)
	// Placeholder: Check destBlock.Transactions for a finalize transaction related to initTx.ID

	// This test mainly verifies the placeholder functions run without crashing.
	// Full cross-shard testing requires implementing the actual logic and communication.
}

// TestPruningPlaceholder tests the basic pruning mechanism runs.
func TestPruningPlaceholder(t *testing.T) {
	numShards := uint(1)
	difficulty := 4
	bc := setupShardedBlockchain(t, numShards, difficulty)
	shardID := core.ShardID(0)

	// Add several blocks
	numBlocks := 5
	for i := 0; i < numBlocks; i++ {
		bc.AddTransaction(core.NewTransaction([]byte(fmt.Sprintf("Prune Test Tx %d", i))))
		_, err := bc.MineShardBlock(shardID)
		if err != nil {
			t.Fatalf("Failed to mine block %d for pruning test: %v", i+1, err)
		}
	}

	bc.ChainMu.RLock()
	initialLength := len(bc.BlockChains[shardID])
	bc.ChainMu.RUnlock()
	if initialLength != numBlocks+1 { // +1 for genesis
		t.Fatalf("Expected %d blocks before pruning, found %d", numBlocks+1, initialLength)
	}

	pruneHeight := 3
	bc.PruneChain(pruneHeight)

	bc.ChainMu.RLock()
	finalLength := len(bc.BlockChains[shardID])
	expectedLengthAfterPrune := (numBlocks + 1) - pruneHeight
	bc.ChainMu.RUnlock()

	if finalLength != expectedLengthAfterPrune {
		t.Errorf("Expected chain length %d after pruning at height %d, but got %d", expectedLengthAfterPrune, pruneHeight, finalLength)
	}
	t.Logf("Pruning finished. Initial length: %d, Pruned below: %d, Final length: %d", initialLength, pruneHeight, finalLength)

	// Verify the first remaining block's height (if any remain)
	bc.ChainMu.RLock()
	if len(bc.BlockChains[shardID]) > 0 {
		firstRemainingBlock := bc.BlockChains[shardID][0]
		if firstRemainingBlock.Height != pruneHeight {
			t.Errorf("First block after pruning has height %d, expected height %d", firstRemainingBlock.Height, pruneHeight)
		}
	}
	bc.ChainMu.RUnlock()
}
