package core_test

import (
	"bytes"
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

func TestNewShardManager(t *testing.T) {
	initialCount := uint(4)
	sm, err := core.NewShardManager(initialCount)
	if err != nil {
		t.Fatalf("NewShardManager failed: %v", err)
	}
	if sm == nil {
		t.Fatal("NewShardManager returned nil")
	}
	if sm.ShardCount != initialCount {
		t.Errorf("Shard count mismatch: expected %d, got %d", initialCount, sm.ShardCount)
	}
	if len(sm.Shards) != int(initialCount) {
		t.Errorf("Shard map size mismatch: expected %d, got %d", initialCount, len(sm.Shards))
	}

	for i := uint(0); i < initialCount; i++ {
		shardID := core.ShardID(i)
		shard, ok := sm.GetShard(shardID)
		if !ok {
			t.Errorf("Shard %d not found in manager", shardID)
		}
		if shard.ID != shardID {
			t.Errorf("Shard ID mismatch for shard %d: expected %d, got %d", i, shardID, shard.ID)
		}
		if shard.State == nil {
			t.Errorf("Shard %d has nil StateDB", shardID)
		}
	}

	// Test invalid count
	_, err = core.NewShardManager(0)
	if err == nil {
		t.Error("Expected error for zero initial shards, but got nil")
	}
}

func TestDetermineShard(t *testing.T) {
	sm, _ := core.NewShardManager(8) // 8 shards
	data1 := []byte("some data")
	data2 := []byte("other data")
	data3 := []byte("some data") // Same as data1

	shardID1 := sm.DetermineShard(data1)
	shardID2 := sm.DetermineShard(data2)
	shardID3 := sm.DetermineShard(data3)

	if shardID1 < 0 || shardID1 >= core.ShardID(sm.ShardCount) {
		t.Errorf("Shard ID %d out of range [0, %d)", shardID1, sm.ShardCount)
	}
	if shardID2 < 0 || shardID2 >= core.ShardID(sm.ShardCount) {
		t.Errorf("Shard ID %d out of range [0, %d)", shardID2, sm.ShardCount)
	}
	if shardID1 != shardID3 {
		t.Errorf("Deterministic routing failed: expected same shard for same data, got %d and %d", shardID1, shardID3)
	}
	// It's statistically unlikely, but possible, that shardID1 == shardID2 for different data.
	// We cannot reliably test for *different* shards without knowing the hash output.
	t.Logf("Data1 routed to Shard %d", shardID1)
	t.Logf("Data2 routed to Shard %d", shardID2)

}

func TestRouteTransaction(t *testing.T) {
	sm, _ := core.NewShardManager(2) // 2 shards
	tx1 := core.NewTransaction([]byte("tx for shard 0 maybe"))
	tx2 := core.NewTransaction([]byte("tx for shard 1 perhaps"))

	// Determine expected shards first
	expectedShardID1 := sm.DetermineShard(tx1.ID)
	expectedShardID2 := sm.DetermineShard(tx2.ID)

	t.Logf("Routing Tx1 (%x) -> Shard %d", tx1.ID, expectedShardID1)
	t.Logf("Routing Tx2 (%x) -> Shard %d", tx2.ID, expectedShardID2)

	err1 := sm.RouteTransaction(tx1)
	if err1 != nil {
		t.Fatalf("RouteTransaction(tx1) failed: %v", err1)
	}
	err2 := sm.RouteTransaction(tx2)
	if err2 != nil {
		t.Fatalf("RouteTransaction(tx2) failed: %v", err2)
	}

	// Check if transactions landed in the correct shard's pool
	shard1, _ := sm.GetShard(expectedShardID1)
	found1 := false
	shard1.Mu.RLock() // Use the exported Mu field
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
	shard2.Mu.RLock() // Use the exported Mu field
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

	// If shards are different, check tx1 is NOT in shard2's pool
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

// Add tests for TriggerSplit and TriggerMerge once implemented
// Add tests for CrossShardReceipt processing once implemented
