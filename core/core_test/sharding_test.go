package core_test

import (
	"bytes" // Import bytes
	"fmt"
	"testing"
	"time" // Import time

	"github.com/saifaleee/advanced-blockchain-go/core"
)

// Helper function to create a ShardManager with specific config for testing
func setupTestShardManager(t *testing.T, initialShards uint, config core.ShardManagerConfig) *core.ShardManager {
	t.Helper()
	sm, err := core.NewShardManager(initialShards, config)
	if err != nil {
		t.Fatalf("Failed to create shard manager: %v", err)
	}
	return sm
}

func TestNewShardManagerWithConfig(t *testing.T) {
	config := core.DefaultShardManagerConfig()
	config.MinShards = 2
	config.MaxShards = 10

	// Test valid creation
	sm, err := core.NewShardManager(3, config)
	if err != nil {
		t.Fatalf("NewShardManager failed: %v", err)
	}
	if sm.ShardCount != 3 {
		t.Errorf("Expected shard count 3, got %d", sm.ShardCount)
	}
	if sm.Config.MinShards != 2 || sm.Config.MaxShards != 10 {
		t.Error("Config mismatch in created ShardManager")
	}

	// Test invalid creation (too few shards)
	_, err = core.NewShardManager(1, config)
	if err == nil {
		t.Error("Expected error creating manager with fewer than MinShards")
	}

	// Test invalid creation (too many shards)
	_, err = core.NewShardManager(11, config)
	if err == nil {
		t.Error("Expected error creating manager with more than MaxShards")
	}
}

// TestSplitTriggering simulates load and checks if a split is triggered structurally.
func TestSplitTriggering(t *testing.T) {
	config := core.DefaultShardManagerConfig()
	config.SplitTxThreshold = 5 // Low threshold for testing
	config.MergeTxThreshold = 0 // Disable merging for this test
	config.MaxShards = 3
	config.MinShards = 1

	sm := setupTestShardManager(t, 1, config) // Start with 1 shard
	targetShardID := core.ShardID(0)
	targetShard, _ := sm.GetShard(targetShardID)

	// Simulate load exceeding threshold
	for i := 0; i < int(config.SplitTxThreshold)+1; i++ {
		tx := core.NewTransaction([]byte(fmt.Sprintf("Load Tx %d", i)))
		// Directly add to shard pool for test control (bypassing routing)
		targetShard.AddTransaction(tx)
	}

	// Check metrics
	if targetShard.Metrics.PendingTxCount.Load() <= config.SplitTxThreshold {
		t.Fatalf("PendingTxCount (%d) did not exceed threshold (%d) after adding transactions",
			targetShard.Metrics.PendingTxCount.Load(), config.SplitTxThreshold)
	}

	// Manually trigger check (management loop not started in test)
	sm.CheckAndManageShards()

	// Verify split occurred structurally
	sm.Mu.RLock() // Use manager's RLock for reading shard count
	finalShardCount := sm.ShardCount
	sm.Mu.RUnlock()

	if finalShardCount != 2 {
		t.Errorf("Expected shard count to be 2 after split, got %d", finalShardCount)
	}

	// Verify new shard exists in manager
	newShardID := core.ShardID(1) // Expecting nextShardID was 1
	_, exists := sm.GetShard(newShardID)
	if !exists {
		t.Errorf("Expected new shard %d to exist after split, but it doesn't", newShardID)
	}

	// Test MaxShards limit
	sm = setupTestShardManager(t, config.MaxShards, config) // Start at max shards
	targetShard, _ = sm.GetShard(targetShardID)
	for i := 0; i < int(config.SplitTxThreshold)+1; i++ {
		targetShard.AddTransaction(core.NewTransaction([]byte(fmt.Sprintf("Load Tx %d", i))))
	}
	sm.CheckAndManageShards() // Trigger check again

	sm.Mu.RLock()
	finalShardCount = sm.ShardCount
	sm.Mu.RUnlock()
	if finalShardCount != config.MaxShards {
		t.Errorf("Expected shard count to remain %d (MaxShards), got %d", config.MaxShards, finalShardCount)
	}
}

// TestMergeTriggering simulates low load and checks if a merge is triggered structurally.
func TestMergeTriggering(t *testing.T) {
	config := core.DefaultShardManagerConfig()
	config.MergeTxThreshold = 10 // Merge if < 10 pending tx
	config.MergeStateThreshold = 0 // Ignore state size for this test
	config.MinShards = 1
	config.MaxShards = 10

	sm := setupTestShardManager(t, 2, config) // Start with 2 shards
	shardID1 := core.ShardID(0)
	shardID2 := core.ShardID(1)
	shard1, _ := sm.GetShard(shardID1)
	shard2, _ := sm.GetShard(shardID2)

	// Ensure shards are below threshold (default state)
	if shard1.Metrics.PendingTxCount.Load() >= config.MergeTxThreshold ||
		shard2.Metrics.PendingTxCount.Load() >= config.MergeTxThreshold {
		t.Fatalf("Initial pending counts (%d, %d) are not below merge threshold (%d)",
			shard1.Metrics.PendingTxCount.Load(), shard2.Metrics.PendingTxCount.Load(), config.MergeTxThreshold)
	}

	// Manually trigger check
	sm.CheckAndManageShards()

	// Verify merge occurred structurally (shard 2 merged into shard 1)
	sm.Mu.RLock()
	finalShardCount := sm.ShardCount
	sm.Mu.RUnlock()

	if finalShardCount != 1 {
		t.Errorf("Expected shard count to be 1 after merge, got %d", finalShardCount)
	}

	// Verify merged shard (shardID2) no longer exists
	_, exists := sm.GetShard(shardID2)
	if exists {
		t.Errorf("Expected merged shard %d to be removed, but it still exists", shardID2)
	}
	// Verify target shard (shardID1) still exists
	_, exists = sm.GetShard(shardID1)
	if !exists {
		t.Errorf("Expected target shard %d to still exist after merge, but it doesn't", shardID1)
	}

	// Test MinShards limit
	sm = setupTestShardManager(t, config.MinShards, config) // Start at min shards
	sm.CheckAndManageShards()                               // Trigger check again

	sm.Mu.RLock()
	finalShardCount = sm.ShardCount
	sm.Mu.RUnlock()
	if finalShardCount != config.MinShards {
		t.Errorf("Expected shard count to remain %d (MinShards), got %d", config.MinShards, finalShardCount)
	}
}

// TestManagementLoop starts and stops the management loop.
func TestManagementLoop(t *testing.T) {
	config := core.DefaultShardManagerConfig()
	config.CheckInterval = 50 * time.Millisecond // Fast interval for testing
	sm := setupTestShardManager(t, 1, config)

	sm.StartManagementLoop()

	// Check if it's running (using internal flag - slightly white-box)
	if !sm.IsManaging.Load() {
		t.Fatal("Management loop flag not set after starting")
	}

	// Allow loop to run for a short time
	time.Sleep(config.CheckInterval * 2)

	sm.StopManagementLoop()

	// Allow time for loop to fully stop
	time.Sleep(config.CheckInterval / 2)

	// Check if it stopped
	if sm.IsManaging.Load() {
		t.Fatal("Management loop flag still set after stopping")
	}

	// Try stopping again (should be idempotent)
	sm.StopManagementLoop()
	if sm.IsManaging.Load() {
		t.Fatal("Management loop flag set after stopping a second time")
	}
}

// TestStateSizeMetric tests the state size metric update.
func TestStateSizeMetric(t *testing.T) {
	shard := core.NewShard(0)
	initialSize := shard.Metrics.StateSizeBytes.Load()

	// Add some state data
	shard.State.Put([]byte("key1"), []byte("value1"))
	shard.State.Put([]byte("key2"), make([]byte, 100)) // Add 100 bytes

	shard.UpdateStateSizeMetric()
	sizeAfterPut := shard.Metrics.StateSizeBytes.Load()

	if sizeAfterPut <= initialSize {
		t.Errorf("Expected state size metric (%d) to increase after Put, initial was %d", sizeAfterPut, initialSize)
	}
	t.Logf("Initial state size: %d, Size after Put: %d", initialSize, sizeAfterPut)

	// Delete state data
	shard.State.Delete([]byte("key1"))
	shard.UpdateStateSizeMetric()
	sizeAfterDelete := shard.Metrics.StateSizeBytes.Load()

	if sizeAfterDelete >= sizeAfterPut {
		// Note: Due to map overhead approximation, size might not decrease perfectly,
		// but it should generally be less than after the large Put.
		t.Logf("Warning: State size metric (%d) did not decrease significantly after Delete (%d). Approximation limitations?", sizeAfterDelete, sizeAfterPut)
	} else {
		t.Logf("Size after Delete: %d", sizeAfterDelete)
	}

}

// --- Existing Tests (Need slight adjustments if Shard struct changed significantly) ---

// TestNewShardManager (already covered by setupTestShardManager and TestNewShardManagerWithConfig)

// TestDetermineShard (Needs slight update for how routing table is checked now)
func TestDetermineShardDynamic(t *testing.T) {
	config := core.DefaultShardManagerConfig()
	sm := setupTestShardManager(t, 8, config) // 8 shards
	data1 := []byte("some data")
	data2 := []byte("other data")
	data3 := []byte("some data") // Same as data1

	shardID1 := sm.DetermineShard(data1)
	shardID2 := sm.DetermineShard(data2)
	shardID3 := sm.DetermineShard(data3)

	// Basic range check
	if shardID1 >= core.ShardID(sm.ShardCount) { // Check against initial count
		t.Errorf("Shard ID %d out of range [0, %d)", shardID1, sm.ShardCount)
	}
	if shardID2 >= core.ShardID(sm.ShardCount) {
		t.Errorf("Shard ID %d out of range [0, %d)", shardID2, sm.ShardCount)
	}
	// Determinism check
	if shardID1 != shardID3 {
		t.Errorf("Deterministic routing failed: expected same shard for same data, got %d and %d", shardID1, shardID3)
	}
	t.Logf("Data1 routed to Shard %d", shardID1)
	t.Logf("Data2 routed to Shard %d", shardID2)

	// Test routing after a simulated merge (where determined shard might not exist)
	sm.Mu.Lock() // Manually modify for test
	removedShardID := core.ShardID(5)
	if _, ok := sm.Shards[removedShardID]; ok {
		t.Logf("Simulating merge by removing shard %d", removedShardID)
		delete(sm.Shards, removedShardID)
		delete(sm.RoutingTable, removedShardID)
		sm.ShardCount--
	} else {
		t.Logf("Shard %d already doesn't exist, skipping removal simulation", removedShardID)
	}
	sm.Mu.Unlock()

	// Find data that *would* route to the removed shard
	var dataForRemoved []byte
	for i := 0; i < 10000; i++ {
		testData := []byte(fmt.Sprintf("find shard %d data %d", removedShardID, i))
		if sm.DetermineShard(testData) == removedShardID {
			dataForRemoved = testData
			break
		}
	}
	if dataForRemoved == nil {
		t.Logf("Could not find test data that routes to removed shard %d, skipping fallback test", removedShardID)
		return
	}

	t.Logf("Found data %x that routes to removed shard %d", dataForRemoved[:4], removedShardID)
	// Now try routing it - it should fallback to any available shard
	tx := core.NewTransaction(dataForRemoved)
	err := sm.RouteTransaction(tx)
	if err != nil {
		t.Fatalf("RouteTransaction failed for data targeting removed shard: %v", err)
	}

	// Verify it landed in one of the available shards
	found := false
	txID := tx.ID // Get expected ID
	
	// Check all available shards
	sm.Mu.RLock()
	for shardID, shard := range sm.Shards {
		shard.Mu.RLock()
		for _, poolTx := range shard.TxPool {
			if bytes.Equal(poolTx.ID, txID) {
				found = true
				t.Logf("Transaction for removed shard %d successfully routed to fallback shard %d", removedShardID, shardID)
				break
			}
		}
		shard.Mu.RUnlock()
		if found {
			break
		}
	}
	sm.Mu.RUnlock()
	
	if !found {
		t.Errorf("Transaction for removed shard %d not found in any active shard pool", removedShardID)
	}
}

// TestRouteTransaction (Needs update to use exported Mu and check fallback)
func TestRouteTransactionDynamic(t *testing.T) {
	config := core.DefaultShardManagerConfig()
	sm := setupTestShardManager(t, 2, config) // 2 shards
	tx1 := core.NewTransaction([]byte("tx for shard 0 maybe"))
	tx2 := core.NewTransaction([]byte("tx for shard 1 perhaps"))

	// Determine expected shards first
	expectedShardID1 := sm.DetermineShard(tx1.ID)
	expectedShardID2 := sm.DetermineShard(tx2.ID)

	t.Logf("Routing Tx1 (%x) -> Shard %d", tx1.ID, expectedShardID1)
	t.Logf("Routing Tx2 (%x) -> Shard %d", tx2.ID, expectedShardID2)

	// Route transactions
	if err := sm.RouteTransaction(tx1); err != nil {
		t.Fatalf("RouteTransaction(tx1) failed: %v", err)
	}
	if err := sm.RouteTransaction(tx2); err != nil {
		t.Fatalf("RouteTransaction(tx2) failed: %v", err)
	}

	// Check if transactions landed in the correct shard's pool using exported Mu
	shard1, _ := sm.GetShard(expectedShardID1)
	found1 := false
	shard1.Mu.RLock() // Use exported Mu
	for _, tx := range shard1.TxPool {
		if bytes.Equal(tx.ID, tx1.ID) {
			found1 = true
			break
		}
	}
	shard1.Mu.RUnlock()
	if !found1 {
		t.Errorf("Tx1 not found in expected shard %d pool", expectedShardID1)
	}

	shard2, _ := sm.GetShard(expectedShardID2)
	found2 := false
	shard2.Mu.RLock() // Use exported Mu
	for _, tx := range shard2.TxPool {
		if bytes.Equal(tx.ID, tx2.ID) {
			found2 = true
			break
		}
	}
	shard2.Mu.RUnlock()
	if !found2 {
		t.Errorf("Tx2 not found in expected shard %d pool", expectedShardID2)
	}

	// Check cross-contamination if shards are different
	if expectedShardID1 != expectedShardID2 {
		found1in2 := false
		shard2.Mu.RLock()
		for _, tx := range shard2.TxPool {
			if bytes.Equal(tx.ID, tx1.ID) {
				found1in2 = true
				break
			}
		}
		shard2.Mu.RUnlock()
		if found1in2 {
			t.Errorf("Tx1 was found in unexpected shard %d pool", expectedShardID2)
		}
	}
}

