package core_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

func TestInMemoryStateDB_PutGet(t *testing.T) {
	db := core.NewInMemoryStateDB()
	key := []byte("testKey")
	value := []byte("testValue")

	err := db.Put(key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	retrievedValue, err := db.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(value, retrievedValue) {
		t.Errorf("Value mismatch: expected %s, got %s", value, retrievedValue)
	}

	// Test getting non-existent key
	_, err = db.Get([]byte("nonExistentKey"))
	if err == nil {
		t.Error("Expected error when getting non-existent key, but got nil")
	}
	if err != core.ErrNotFound {
		t.Errorf("Expected ErrNotFound, but got: %v", err)
	}
}

func TestInMemoryStateDB_Delete(t *testing.T) {
	db := core.NewInMemoryStateDB()
	key := []byte("testKey")
	value := []byte("testValue")

	db.Put(key, value) // Ensure key exists

	err := db.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify key is deleted
	_, err = db.Get(key)
	if err == nil || err != core.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, but got: %v", err)
	}

	// Test deleting non-existent key
	err = db.Delete([]byte("nonExistentKeyAgain"))
	// Depending on desired behavior: check for specific error or nil
	if err != core.ErrNotFound { // Assuming delete should error if key doesn't exist
		t.Errorf("Expected ErrNotFound when deleting non-existent key, got: %v", err)
	}
}

func TestInMemoryStateDB_Overwrite(t *testing.T) {
	db := core.NewInMemoryStateDB()
	key := []byte("testKey")
	value1 := []byte("value1")
	value2 := []byte("value2")

	db.Put(key, value1)
	retrievedValue1, _ := db.Get(key)
	if !bytes.Equal(value1, retrievedValue1) {
		t.Fatalf("Initial Put/Get failed")
	}

	db.Put(key, value2) // Overwrite
	retrievedValue2, err := db.Get(key)
	if err != nil {
		t.Fatalf("Get after overwrite failed: %v", err)
	}
	if !bytes.Equal(value2, retrievedValue2) {
		t.Errorf("Value mismatch after overwrite: expected %s, got %s", value2, retrievedValue2)
	}
}

func TestStateUpdatesAfterMining(t *testing.T) {
	// Create a new blockchain with 2 shards
	config := core.DefaultShardManagerConfig()
	config.SplitThresholdStateSize = 5       // Lower threshold for testing
	bc, err := core.NewBlockchain(2, config) // Use lower difficulty for faster tests
	if err != nil {
		t.Fatalf("Failed to create blockchain: %v", err)
	}

	// Get the IDs of the shards
	shardIDs := bc.ShardManager.GetAllShardIDs()
	if len(shardIDs) != 2 {
		t.Fatalf("Expected 2 shards, got %d", len(shardIDs))
	}

	// Add intra-shard transactions to shard 0
	shard0 := shardIDs[0]

	// Add multiple transactions with specific data to ensure they create state entries
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		data := []byte(fmt.Sprintf("%s:%s", key, value))
		tx := core.NewTransaction(data, core.IntraShard, nil)

		err = bc.AddTransaction(tx)
		if err != nil {
			t.Fatalf("Failed to add transaction to shard 0: %v", err)
		}
	}

	// Get the shard and check initial state
	shard0Inst, ok := bc.ShardManager.GetShard(shard0)
	if !ok {
		t.Fatalf("Failed to get shard %d", shard0)
	}

	initialSize := shard0Inst.StateDB.Size()
	if initialSize != 0 {
		t.Fatalf("Expected initial state size to be 0, got %d", initialSize)
	}

	// Mine a block for shard 0
	block0, err := bc.MineShardBlock(shard0)
	if err != nil {
		t.Fatalf("Failed to mine block for shard 0: %v", err)
	}

	// Verify that state size has increased
	newSize := shard0Inst.StateDB.Size()
	if newSize <= initialSize {
		t.Fatalf("Expected state size to increase after mining, was %d, now %d", initialSize, newSize)
	}

	// Verify that state root is not empty
	if len(block0.Header.StateRoot) == 0 {
		t.Fatalf("Expected non-empty state root, got empty")
	}

	t.Logf("Shard %d: State size increased from %d to %d after mining", shard0, initialSize, newSize)
	t.Logf("Shard %d: State root is %x", shard0, block0.Header.StateRoot)

	// Test cross-shard transactions
	shard1 := shardIDs[1]

	// Create a cross-shard transaction from shard 0 to shard 1
	crossData := []byte("cross-key:cross-value-from-shard0-to-shard1")
	crossTx := core.NewTransaction(crossData, core.CrossShardTxInit, &shard1)
	crossTx.SourceShard = &shard0

	err = bc.AddTransaction(crossTx)
	if err != nil {
		t.Fatalf("Failed to add cross-shard transaction: %v", err)
	}

	// Get the destination shard
	shard1Inst, ok := bc.ShardManager.GetShard(shard1)
	if !ok {
		t.Fatalf("Failed to get shard %d", shard1)
	}

	// Mine a block for shard 0 (initiating shard)
	_, err = bc.MineShardBlock(shard0)
	if err != nil {
		t.Fatalf("Failed to mine block for shard 0 (initiator): %v", err)
	}

	// Check that a finalization transaction was added to shard 1's pool
	finalizeTxFound := false
	for _, tx := range shard1Inst.TxPool {
		if tx.Type == core.CrossShardTxFinalize {
			// Verify it's the right one by checking source/destination
			if tx.SourceShard != nil && *tx.SourceShard == shard0 {
				finalizeTxFound = true
				// Check data matches
				if !bytes.Equal(tx.Data, crossData) {
					t.Errorf("Finalize tx data mismatch: expected %s, got %s", crossData, tx.Data)
				}
				break
			}
		}
	}

	if !finalizeTxFound {
		t.Fatalf("No finalization transaction found in shard %d's pool", shard1)
	}

	// Mine a block for shard 1 (receiving shard)
	initialSize1 := shard1Inst.StateDB.Size()
	_, err = bc.MineShardBlock(shard1)
	if err != nil {
		t.Fatalf("Failed to mine block for shard 1 (receiver): %v", err)
	}

	// Verify that shard 1's state size also increased
	newSize1 := shard1Inst.StateDB.Size()
	if newSize1 <= initialSize1 {
		t.Fatalf("Expected shard 1 state size to increase after mining finalization, was %d, now %d", initialSize1, newSize1)
	}

	t.Logf("Cross-shard transaction test passed: Shard %d state size increased from %d to %d after finalizing",
		shard1, initialSize1, newSize1)
}

func TestStateConsistencyAfterMultipleMining(t *testing.T) {
	// Create a new blockchain with 2 shards
	config := core.DefaultShardManagerConfig()
	bc, err := core.NewBlockchain(2, config) // Use lower difficulty for faster tests
	if err != nil {
		t.Fatalf("Failed to create blockchain: %v", err)
	}

	// Get the IDs of the shards
	shardIDs := bc.ShardManager.GetAllShardIDs()
	shard0 := shardIDs[0]

	// Get the shard
	shard0Inst, ok := bc.ShardManager.GetShard(shard0)
	if !ok {
		t.Fatalf("Failed to get shard %d", shard0)
	}

	// Mine multiple blocks
	stateSizes := make([]int, 0)
	stateRoots := make([][]byte, 0)

	// Record initial state
	stateSizes = append(stateSizes, shard0Inst.StateDB.Size())
	initialRoot, _ := shard0Inst.StateDB.GetStateRoot()
	stateRoots = append(stateRoots, initialRoot)

	// Mine 3 blocks, adding new transactions before each mining
	for i := 0; i < 3; i++ {
		// Add 3 new transactions for each mining round with key:value format
		for j := 0; j < 3; j++ {
			key := fmt.Sprintf("key-block%d-tx%d", i, j)
			value := fmt.Sprintf("value-block%d-tx%d", i, j)
			data := []byte(fmt.Sprintf("%s:%s", key, value))
			tx := core.NewTransaction(data, core.IntraShard, nil)
			err = bc.AddTransaction(tx)
			if err != nil {
				t.Fatalf("Failed to add transaction %d-%d: %v", i, j, err)
			}
		}

		// Mine a block
		_, err := bc.MineShardBlock(shard0)
		if err != nil {
			t.Fatalf("Failed to mine block %d: %v", i, err)
		}

		// Record state after each mining operation
		stateSizes = append(stateSizes, shard0Inst.StateDB.Size())
		root, _ := shard0Inst.StateDB.GetStateRoot()
		stateRoots = append(stateRoots, root)
	}

	// Verify state sizes increase
	for i := 1; i < len(stateSizes); i++ {
		if stateSizes[i] <= stateSizes[i-1] {
			t.Errorf("State size did not increase after mining block %d: %d -> %d",
				i-1, stateSizes[i-1], stateSizes[i])
		}
	}

	// Verify state roots change
	for i := 1; i < len(stateRoots); i++ {
		if bytes.Equal(stateRoots[i], stateRoots[i-1]) {
			t.Errorf("State root did not change after mining block %d", i-1)
		}
	}

	t.Logf("State sizes after mining: %v", stateSizes)

	// Verify chain is valid
	isValid := bc.IsChainValid()
	if !isValid {
		t.Fatalf("Blockchain is not valid after mining")
	}

	t.Logf("Blockchain remains valid after multiple mining operations")
}
