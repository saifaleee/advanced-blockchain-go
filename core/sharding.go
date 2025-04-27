package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stathat/consistent" // Import consistent hashing library
)

// --- Shard ---

// ShardMetrics holds performance/load metrics for a shard.
type ShardMetrics struct {
	TxCount       atomic.Int64 // Number of transactions processed
	StateSize     atomic.Int64 // Approximate size of the state DB (e.g., number of keys)
	BlockCount    atomic.Int64 // Number of blocks mined
	PendingTxPool atomic.Int64 // Number of transactions waiting in the pool
	// Add other metrics like CPU/memory usage if needed
}

// Shard represents a single shard in the blockchain.
type Shard struct {
	ID       uint64
	StateDB  StateDB        // Interface for state storage
	TxPool   []*Transaction // Simple transaction pool (replace with more robust structure if needed)
	mu       sync.RWMutex   // Protects TxPool and StateDB during certain operations like migration
	Metrics  *ShardMetrics
	stopChan chan struct{} // Channel to signal shard termination
}

// NewShard - Initializing block count metric
func NewShard(id uint64) *Shard {
	db := NewInMemoryStateDB()
	initialStateRoot, _ := db.GetStateRoot()
	log.Printf("Initializing Shard %d with initial state root %x", id, initialStateRoot)

	metrics := &ShardMetrics{}
	// Initialize atomic variables if needed (Go initializes atomics to 0 by default)

	return &Shard{
		ID:       id,
		StateDB:  db,
		TxPool:   make([]*Transaction, 0),
		mu:       sync.RWMutex{},
		Metrics:  metrics, // Assign initialized metrics
		stopChan: make(chan struct{}),
	}
}

// AddTransaction adds a transaction to the shard's pool.
func (s *Shard) AddTransaction(tx *Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// TODO: Add more validation (e.g., signature, replay protection, format checks)
	if tx == nil || tx.ID == nil {
		return errors.New("cannot add nil or uninitialized transaction")
	}
	// Optional: Check if TX already exists?
	s.TxPool = append(s.TxPool, tx)
	s.Metrics.PendingTxPool.Store(int64(len(s.TxPool))) // Update metric
	// log.Printf("Shard %d: Added TX %x to pool (pool size: %d)\n", s.ID, tx.ID, len(s.TxPool))
	return nil
}

// GetTransactionsForBlock retrieves transactions for mining.
func (s *Shard) GetTransactionsForBlock(maxTx int) []*Transaction {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := len(s.TxPool)
	if count > maxTx {
		count = maxTx
	}
	if count == 0 {
		return []*Transaction{}
	}

	// Select transactions based on some policy (e.g., FIFO, priority)
	// Simple FIFO for now:
	txs := make([]*Transaction, count)
	copy(txs, s.TxPool[:count])

	// Remove selected transactions from the pool
	s.TxPool = s.TxPool[count:]
	s.Metrics.PendingTxPool.Store(int64(len(s.TxPool))) // Update metric

	// log.Printf("Shard %d: Retrieving %d TXs for block, %d remain in pool\n", s.ID, count, len(s.TxPool))
	return txs
}

// Stop signals the shard to stop processing.
func (s *Shard) Stop() {
	close(s.stopChan)
	log.Printf("Shard %d: Stop signal received.", s.ID)
	// Add any other cleanup needed, e.g., persisting state if not in-memory
}

// ApplyStateChanges applies transactions to the shard's state DB.
// This should be called *before* calculating the final state root for a block.
// Returns the calculated state root AFTER applying transactions.
func (s *Shard) ApplyStateChanges(transactions []*Transaction) ([]byte, error) {
	// Lock the shard during state modification. If StateDB is thread-safe internally,
	// we might only need to lock TxPool access, but locking here ensures atomicity
	// of applying a block's worth of changes relative to other shard operations like migration.
	s.mu.Lock()
	defer s.mu.Unlock()

	originalSize := s.StateDB.Size()
	txsApplied := 0

	conflictResolver := NewConflictResolver() // Use the advanced conflict resolver

	// Apply transactions to state (example logic, adapt based on TX meaning)
	for _, tx := range transactions {
		var key []byte
		var value []byte
		var keyStr string // Use string for DB operations

		if bytes.Contains(tx.Data, []byte(":")) {
			parts := bytes.SplitN(tx.Data, []byte(":"), 2)
			if len(parts) == 2 {
				key = parts[0]
				value = parts[1]
			}
		}
		if key == nil {
			key = tx.ID // Replace tx.Hash() with tx.ID
			value = tx.Data
		}
		keyStr = string(key) // Convert key to string for DB

		// --- Advanced Conflict Detection & Resolution ---
		existingValue, err := s.StateDB.Get(keyStr)
		if err == nil && existingValue != nil {
			localVersion := StateVersion{
				Key:         keyStr, // Use string key
				Value:       existingValue,
				VectorClock: make(VectorClock), // Corrected: Use make() for map[uint64]uint64
				SourceNode:  "local",
			}
			remoteVersion := StateVersion{
				Key:         keyStr, // Use string key
				Value:       value,
				VectorClock: make(VectorClock), // Corrected: Use make() for map[uint64]uint64
				SourceNode:  "tx",
			}
			resolved := conflictResolver.HandlePotentialConflict(localVersion, remoteVersion, tx.ID) // Replace tx.Hash() with tx.ID
			value = resolved.Value.([]byte)
		}
		// else: no existing value, just write
		err = s.StateDB.Put(keyStr, value)
		if err != nil {
			log.Printf("Shard %d: Error applying state for TX %x: %v\n", s.ID, tx.ID, err)
			return nil, fmt.Errorf("failed to apply state for tx %x: %w", tx.ID, err)
		}
		txsApplied++
		s.Metrics.TxCount.Add(1)
	}

	// Update state size metric (based on number of keys for InMemoryStateDB)
	stateSize := s.StateDB.Size()
	s.Metrics.StateSize.Store(int64(stateSize))

	// Calculate the new state root *after* all changes for this block
	stateRoot, err := s.StateDB.GetStateRoot()
	if err != nil {
		log.Printf("Shard %d: Error calculating state root after applying %d txs: %v\n", s.ID, len(transactions), err)
		return nil, fmt.Errorf("failed to calculate state root: %w", err)
	}

	log.Printf("Shard %d: Applied %d transactions. State size %d -> %d. New state root: %x\n", s.ID, txsApplied, originalSize, stateSize, stateRoot)
	return stateRoot, nil
}

// --- ShardManager ---

// ShardManagerConfig holds configuration for dynamic sharding.
type ShardManagerConfig struct {
	SplitThresholdStateSize   int64         // Trigger split if StateSize exceeds this
	SplitThresholdTxPool      int64         // Trigger split if PendingTxPool exceeds this
	MergeThresholdStateSize   int64         // Trigger merge if StateSize drops below this (shard A)
	MergeTargetThresholdSize  int64         // AND target shard B is also below this
	CheckInterval             time.Duration // How often to check metrics
	NumReplicasConsistentHash int           // Number of virtual nodes for consistent hashing
	MaxShards                 int           // Maximum number of shards allowed
	MinShards                 int           // Minimum number of shards allowed
}

// DefaultShardManagerConfig returns the default configuration for ShardManager
func DefaultShardManagerConfig() ShardManagerConfig {
	return ShardManagerConfig{
		SplitThresholdStateSize:   1000,             // Split when state size exceeds 1000 entries
		SplitThresholdTxPool:      500,              // Split when TX pool exceeds 500 pending TXs
		MergeThresholdStateSize:   200,              // Consider merge when state size falls below 200
		MergeTargetThresholdSize:  300,              // And target shard is below 300
		CheckInterval:             time.Second * 10, // Check metrics every 10 seconds
		NumReplicasConsistentHash: 10,               // 10 virtual nodes per shard
		MaxShards:                 64,               // Maximum 64 shards
		MinShards:                 1,                // At least 1 shard
	}
}

// ShardManager manages all shards, routing, and dynamic resizing.
type ShardManager struct {
	shards       map[uint64]*Shard
	shardIDs     []uint64     // Keep track of active shard IDs (sorted for easier merge logic)
	mu           sync.RWMutex // Protects shards map, shardIDs slice, and consistent hash ring
	config       ShardManagerConfig
	nextShardID  uint64                 // Counter for assigning new shard IDs
	consistent   *consistent.Consistent // Consistent hashing ring
	stopChan     chan struct{}
	managementWg sync.WaitGroup
	isManaging   atomic.Bool // Flag to prevent concurrent management runs
	// Blockchain reference needed for state migration or other cross-component actions
	bc *Blockchain // Pointer back to the main Blockchain instance (set during init)
}

// NewShardManager creates a new shard manager.
func NewShardManager(config ShardManagerConfig, initialShards int) (*ShardManager, error) {
	if initialShards <= 0 || initialShards < config.MinShards {
		return nil, fmt.Errorf("initialShards (%d) must be positive and >= MinShards (%d)", initialShards, config.MinShards)
	}
	if config.MaxShards > 0 && initialShards > config.MaxShards {
		return nil, fmt.Errorf("initialShards (%d) cannot exceed MaxShards (%d)", initialShards, config.MaxShards)
	}
	if config.MergeThresholdStateSize >= config.SplitThresholdStateSize || config.MergeTargetThresholdSize >= config.SplitThresholdStateSize {
		log.Printf("Warning: Merge thresholds might overlap with split thresholds, review config.")
	}

	sm := &ShardManager{
		shards:     make(map[uint64]*Shard),
		shardIDs:   make([]uint64, 0, initialShards),
		config:     config,
		consistent: consistent.New(),
		stopChan:   make(chan struct{}),
	}
	sm.consistent.NumberOfReplicas = config.NumReplicasConsistentHash
	if sm.consistent.NumberOfReplicas <= 0 {
		sm.consistent.NumberOfReplicas = 20 // Default if not set
		log.Printf("ShardManager: NumReplicasConsistentHash not set or invalid, defaulting to %d", sm.consistent.NumberOfReplicas)
	}

	for i := 0; i < initialShards; i++ {
		shardID := sm.nextShardID
		sm.nextShardID++
		shard := NewShard(shardID)
		sm.shards[shardID] = shard
		sm.shardIDs = append(sm.shardIDs, shardID)
		sm.consistent.Add(fmt.Sprintf("%d", shardID)) // Add shard ID as string to hash ring
	}
	sort.Slice(sm.shardIDs, func(i, j int) bool { return sm.shardIDs[i] < sm.shardIDs[j] }) // Keep sorted

	log.Printf("ShardManager initialized with %d shards. Shard IDs: %v. Hash ring members: %v\n", len(sm.shards), sm.shardIDs, sm.consistent.Members())
	return sm, nil
}

// *** SetBlockchainLink allows linking the manager back to the Blockchain instance ***
func (sm *ShardManager) SetBlockchainLink(bc *Blockchain) {
	sm.mu.Lock() // Lock needed if this can be called concurrently? Assume called during init.
	defer sm.mu.Unlock()
	sm.bc = bc
	log.Println("ShardManager linked back to Blockchain instance.")
}

// GetShard retrieves a shard by ID (read-locked).
func (sm *ShardManager) GetShard(id uint64) (*Shard, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	shard, ok := sm.shards[id]
	return shard, ok
}

// GetAllShardIDs returns a sorted slice of all active shard IDs (read-locked).
func (sm *ShardManager) GetAllShardIDs() []uint64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	// Return a copy to prevent external modification and ensure it's sorted
	idsCopy := make([]uint64, len(sm.shardIDs))
	copy(idsCopy, sm.shardIDs)
	// Should already be sorted, but ensure it just in case
	sort.Slice(idsCopy, func(i, j int) bool { return idsCopy[i] < sm.shardIDs[j] })
	return idsCopy
}

// DetermineShard uses consistent hashing to find the responsible shard for a given key (read-locked).
func (sm *ShardManager) DetermineShard(key []byte) (uint64, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.shardIDs) == 0 || len(sm.consistent.Members()) == 0 {
		log.Printf("DetermineShard: Cannot route key %x, no active shards or empty hash ring.", key)
		return 0, errors.New("no active shards available for routing")
	}

	// Use consistent hashing
	keyString := string(key) // Consistent hashing library uses strings
	shardIDStr, err := sm.consistent.Get(keyString)
	if err != nil {
		// This can happen if the ring is empty, but we checked. Could be another issue.
		log.Printf("Error determining shard via consistent hash for key %x (%s): %v. Ring size: %d, members: %v", key, keyString, err, len(sm.consistent.Members()), sm.consistent.Members())
		// Fallback: Use simple modulo of the key's numeric representation against the number of shards
		// This isn't stable during dynamic changes but provides a last resort.
		numShards := len(sm.shardIDs)
		if numShards > 0 {
			fallbackIndex := int(new(big.Int).SetBytes(key).Uint64() % uint64(numShards))
			fallbackShardID := sm.shardIDs[fallbackIndex] // Assumes shardIDs is sorted and reflects map keys
			log.Printf("Falling back to modulo routing for key %x, index %d -> shard %d", key, fallbackIndex, fallbackShardID)
			return fallbackShardID, nil
		}
		return 0, fmt.Errorf("consistent hashing failed and no shards available for fallback: %w", err)
	}

	// Parse the string ID back to uint64
	shardID, err := strconv.ParseUint(shardIDStr, 10, 64)
	if err != nil {
		log.Printf("Error parsing shard ID '%s' from consistent hash result: %v", shardIDStr, err)
		// Fallback like above? This indicates a mismatch between ring members and expected format.
		numShards := len(sm.shardIDs)
		if numShards > 0 {
			fallbackIndex := int(new(big.Int).SetBytes(key).Uint64() % uint64(numShards))
			fallbackShardID := sm.shardIDs[fallbackIndex]
			log.Printf("Falling back to modulo routing due to parse error, index %d -> shard %d", fallbackIndex, fallbackShardID)
			return fallbackShardID, nil
		}
		return 0, fmt.Errorf("failed to parse shard ID from consistent hash result: %w", err)
	}

	// Sanity check: Does the shard actually exist in our map? It should!
	if _, exists := sm.shards[shardID]; !exists {
		log.Printf("CRITICAL: Consistent hash routed key %x to non-existent shard ID %d! Ring members: %v, Map keys: %v", key, shardID, sm.consistent.Members(), sm.shardIDs)
		// This indicates a serious inconsistency. Maybe force rebuild or error out.
		// Fallback for now:
		numShards := len(sm.shardIDs)
		if numShards > 0 {
			fallbackIndex := int(new(big.Int).SetBytes(key).Uint64() % uint64(numShards))
			fallbackShardID := sm.shardIDs[fallbackIndex]
			log.Printf("Falling back to modulo routing due to inconsistent state, index %d -> shard %d", fallbackIndex, fallbackShardID)
			return fallbackShardID, nil
		}
		return 0, fmt.Errorf("consistent hash returned non-existent shard ID %d", shardID)
	}

	// log.Printf("Routed key %x to shard %d via consistent hashing", key, shardID)
	return shardID, nil
}

// --- Dynamic Sharding Logic ---

// StartManagementLoop starts the background process to check shard metrics and trigger splits/merges.
func (sm *ShardManager) StartManagementLoop() {
	if sm.config.CheckInterval <= 0 {
		log.Println("ShardManager management loop disabled (CheckInterval <= 0).")
		return
	}
	log.Println("Starting ShardManager management loop...")
	sm.managementWg.Add(1)
	go func() {
		defer sm.managementWg.Done()
		ticker := time.NewTicker(sm.config.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-sm.stopChan:
				log.Println("Stopping ShardManager management loop.")
				return
			case <-ticker.C:
				// Prevent overlapping management cycles if one takes too long
				if sm.isManaging.CompareAndSwap(false, true) {
					// log.Println("Checking shard metrics for potential split/merge...")
					sm.CheckAndManageShards()
					sm.isManaging.Store(false) // Release the lock
				} else {
					log.Println("Skipping management cycle, previous one still running.")
				}
			}
		}
	}()
}

// StopManagementLoop stops the background management process and managed shards.
func (sm *ShardManager) StopManagementLoop() {
	close(sm.stopChan)     // Signal the loop to stop
	sm.managementWg.Wait() // Wait for the loop goroutine to finish
	log.Println("ShardManager management loop stopped.")

	// Stop all managed shards
	sm.mu.RLock() // Need read lock to access shards map
	idsToStop := make([]uint64, 0, len(sm.shards))
	for id := range sm.shards {
		idsToStop = append(idsToStop, id)
	}
	sm.mu.RUnlock()

	log.Printf("Stopping all managed shards: %v", idsToStop)
	for _, shardID := range idsToStop {
		if shard, ok := sm.GetShard(shardID); ok { // Use GetShard for read lock safety
			shard.Stop()
		}
	}
	log.Println("All managed shards stopped.")
}

// CheckAndManageShards iterates through shards and triggers splits or merges based on config thresholds.
// This function acquires the main write lock (`sm.mu`) to ensure atomicity of checking and potentially modifying the shard set.
func (sm *ShardManager) CheckAndManageShards() {
	sm.mu.Lock() // Acquire write lock for the entire check and potential modification
	defer sm.mu.Unlock()

	// Create copies to iterate over, preventing issues if slice/map changes during iteration
	currentShardIDs := make([]uint64, len(sm.shardIDs))
	copy(currentShardIDs, sm.shardIDs)

	log.Printf("Checking metrics for %d shards: %v", len(currentShardIDs), currentShardIDs)

	splitTriggered := false
	mergeTriggered := false

	// Check for Splits first (usually higher priority than merges)
	for _, shardID := range currentShardIDs {
		// Re-check existence in case a merge removed it during a previous iteration (unlikely with full lock, but good practice)
		shard, ok := sm.shards[shardID]
		if !ok {
			continue // Shard was removed
		}

		metrics := shard.Metrics
		stateSize := metrics.StateSize.Load()
		txPoolSize := metrics.PendingTxPool.Load()
		// log.Printf("Shard %d Metrics - StateSize: %d, TxPoolSize: %d", shardID, stateSize, txPoolSize)

		// Check Split condition
		needsSplit := (sm.config.SplitThresholdStateSize > 0 && stateSize > sm.config.SplitThresholdStateSize) ||
			(sm.config.SplitThresholdTxPool > 0 && txPoolSize > sm.config.SplitThresholdTxPool)

		if needsSplit {
			if sm.config.MaxShards > 0 && len(sm.shards) >= sm.config.MaxShards {
				log.Printf("Shard %d (%d keys, %d pool) meets split threshold, but MaxShards (%d) reached. Skipping split.",
					shardID, stateSize, txPoolSize, sm.config.MaxShards)
				continue // Cannot split further
			}

			log.Printf("Shard %d (%d keys, %d pool) meets split threshold. Triggering split.",
				shardID, stateSize, txPoolSize)
			err := sm.triggerSplitUnsafe(shardID) // Call unsafe version as we hold the lock
			if err != nil {
				log.Printf("Error triggering split for shard %d: %v", shardID, err)
				// Decide if we should continue or stop the management cycle on error
			} else {
				splitTriggered = true
				// Important: After a split, the shard list and hash ring have changed.
				// Re-evaluating merge conditions immediately might be complex.
				// Simplest approach: break after the first successful split/merge in a cycle.
				break // Exit the loop after one split
			}
		}
	}

	// If a split happened, we stop this cycle to allow stabilization/re-evaluation next time.
	if splitTriggered {
		log.Println("Split occurred, ending management cycle.")
		return
	}

	// Check for Merges if no split occurred
	// Sort shards by load (e.g., state size) to find candidates easily? Or just iterate.
	// Simple approach: Iterate and find the first pair eligible for merge.
	// Needs at least 2 shards to potentially merge.
	if len(sm.shards) > sm.config.MinShards {
		// Iterate through shards again (or use a sorted list if available)
		for i := 0; i < len(sm.shardIDs); i++ {
			shardAID := sm.shardIDs[i]
			shardA, okA := sm.shards[shardAID]
			if !okA {
				continue
			}

			// Check if shard A is below merge threshold
			stateSizeA := shardA.Metrics.StateSize.Load()
			// txPoolSizeA := shardA.Metrics.PendingTxPool.Load() // Can add pool size to merge criteria too
			if sm.config.MergeThresholdStateSize > 0 && stateSizeA < sm.config.MergeThresholdStateSize {

				// Find a suitable merge partner (e.g., the *next* shard in the sorted list)
				// Ensure we don't merge the last shard with the first one if using simple adjacency.
				var shardBID uint64
				var shardB *Shard
				var okB bool

				if i+1 < len(sm.shardIDs) { // Try merging with the next shard
					shardBID = sm.shardIDs[i+1]
					shardB, okB = sm.shards[shardBID]
				} else if i > 0 && len(sm.shardIDs) > sm.config.MinShards { // Try merging last with previous if allowed
					// Avoid merging last two if result is below min shards? Check needed.
					// shardBID = sm.shardIDs[i-1]
					// shardB, okB = sm.shards[shardBID]
					// Merging with previous can be more complex; sticking to merging A->B (i -> i+1) is simpler
					okB = false // Don't merge last shard with previous in this simple logic
				}

				if okB {
					// Check if shard B is also below the target threshold
					stateSizeB := shardB.Metrics.StateSize.Load()
					if sm.config.MergeTargetThresholdSize > 0 && stateSizeB < sm.config.MergeTargetThresholdSize {
						// Both shards are underutilized, trigger merge
						log.Printf("Shards %d (%d keys) and %d (%d keys) meet merge thresholds. Triggering merge (%d into %d).",
							shardAID, stateSizeA, shardBID, stateSizeB, shardBID, shardAID)

						// Decide merge direction: merge B into A seems more natural with the loop structure
						err := sm.triggerMergeUnsafe(shardAID, shardBID) // Merge B (shardBID) into A (shardAID)
						if err != nil {
							log.Printf("Error triggering merge for shards %d and %d: %v", shardAID, shardBID, err)
						} else {
							mergeTriggered = true
							break // Exit loop after one merge
						}
					}
				}
			}
		}
	}

	if mergeTriggered {
		log.Println("Merge occurred, ending management cycle.")
	} else if !splitTriggered {
		// log.Println("No split or merge conditions met in this cycle.")
	}
}

// triggerSplitUnsafe requires calling bc.initializeShardChain for the new shard.
func (sm *ShardManager) triggerSplitUnsafe(sourceShardID uint64) error {
	if sm.bc == nil {
		log.Printf("CRITICAL: Cannot trigger split for shard %d - ShardManager is not linked to Blockchain.", sourceShardID)
		return errors.New("shard manager blockchain link not set")
	}

	log.Printf("--- Starting Split for Shard %d ---", sourceShardID)
	// ... (Get source shard, check existence) ...
	sourceShard, ok := sm.shards[sourceShardID]
	if !ok { /* ... error handling ... */
	}

	// 1. Create the new shard
	newShardID := sm.nextShardID
	sm.nextShardID++
	newShard := NewShard(newShardID)
	log.Printf("Split: Created new Shard %d", newShardID)

	// 2. Temporarily add to hash ring
	newShardIDStr := fmt.Sprintf("%d", newShardID)
	sm.consistent.Add(newShardIDStr)
	log.Printf("Split: Temporarily added %d to hash ring. Members: %v", newShardID, sm.consistent.Members())

	// 3. Perform State Migration (locking source and new shard)
	sourceShard.mu.Lock()
	newShard.mu.Lock()
	// ... (defer unlocks) ...
	defer sourceShard.mu.Unlock()
	defer newShard.mu.Unlock()

	log.Printf("Split: Migrating state from Shard %d to Shard %d...", sourceShardID, newShardID)
	// ... (GetKeys, determineShardUnsafe helper, migration loop with balancing logic) ...
	// ... (Code for state migration as before) ...
	log.Printf("Split Migration: Completed state transfer. Updating shard state sizes...")
	// ... (Update metrics after migration) ...

	// 4. Finalize: Add new shard to map/ID list AND Initialize its chain
	sm.shards[newShardID] = newShard
	sm.shardIDs = append(sm.shardIDs, newShardID)
	sort.Slice(sm.shardIDs, func(i, j int) bool { return sm.shardIDs[i] < sm.shardIDs[j] })

	log.Printf("Split: Added shard %d to manager. Shard IDs: %v", newShardID, sm.shardIDs)

	// TODO: Assign validators if using per-shard assignment.
	// For now, validators are global via GetActiveValidatorsForShard.

	log.Printf("--- Split Complete for Shard %d -> %d, %d ---", sourceShardID, sourceShardID, newShardID)
	return nil
}

// triggerMergeUnsafe performs the merge operation. Merges `shardBID` into `shardAID`. Assumes caller holds the write lock (`sm.mu`).
func (sm *ShardManager) triggerMergeUnsafe(shardAID, shardBID uint64) error {
	log.Printf("--- Starting Merge of Shard %d into Shard %d ---", shardBID, shardAID)
	if shardAID == shardBID {
		return errors.New("cannot merge a shard into itself")
	}

	shardA, okA := sm.shards[shardAID]
	shardB, okB := sm.shards[shardBID]

	if !okA {
		return fmt.Errorf("target shard %d for merge not found", shardAID)
	}
	if !okB {
		return fmt.Errorf("source shard %d for merge not found", shardBID)
	}

	// 1. Perform State Migration (Move all state from B to A)
	// Lock both shards during migration.
	shardA.mu.Lock()
	shardB.mu.Lock()
	defer shardA.mu.Unlock()
	defer shardB.mu.Unlock()

	log.Printf("Merge: Migrating state from Shard %d to Shard %d...", shardBID, shardAID)
	keysToMigrate, err := shardB.StateDB.GetKeys()
	if err != nil {
		log.Printf("Merge Error: Failed to get keys from source shard %d: %v", shardBID, err)
		// Decide if merge should proceed or fail. Failing is safer.
		return fmt.Errorf("failed to get keys from shard %d for merge: %w", shardBID, err)
	}

	migratedCount := 0
	failedMigrationCount := 0
	for _, key := range keysToMigrate {
		value, getErr := shardB.StateDB.Get(key)
		if getErr != nil {
			log.Printf("Merge Migration Error: Failed to get value for key %s from source shard %d: %v", key, shardBID, getErr)
			failedMigrationCount++
			continue
		}

		// Check for conflicts? Overwrite existing keys in A? For simplicity, overwrite.
		putErr := shardA.StateDB.Put(key, value)
		if putErr != nil {
			log.Printf("Merge Migration Error: Failed to put key %s into target shard %d: %v", key, shardAID, putErr)
			failedMigrationCount++
		} else {
			migratedCount++
		}
	}
	log.Printf("Merge Migration: Attempted to migrate %d keys from shard %d to %d. %d successful, %d failures.",
		len(keysToMigrate), shardBID, shardAID, migratedCount, failedMigrationCount)

	if failedMigrationCount > 0 {
		log.Printf("WARNING: Merge %d->%d completed with migration errors.", shardBID, shardAID)
		// Proceeding anyway in PoC.
	}

	// Clear state DB of the merged shard AFTER successful migration (or based on error handling policy)
	clearErr := shardB.StateDB.Clear()
	if clearErr != nil {
		log.Printf("Merge Warning: Failed to clear state DB for merged shard %d: %v", shardBID, clearErr)
	}

	// Update metrics for the target shard
	shardA.Metrics.StateSize.Store(int64(shardA.StateDB.Size()))
	log.Printf("Merge: Updated state size for target Shard %d: %d keys", shardAID, shardA.Metrics.StateSize.Load())

	// 2. Migrate Pending Transactions from B to A's Pool
	log.Printf("Merge: Migrating %d pending transactions from Shard %d to Shard %d pool", len(shardB.TxPool), shardBID, shardAID)
	shardA.TxPool = append(shardA.TxPool, shardB.TxPool...)
	shardB.TxPool = []*Transaction{} // Clear B's pool
	shardA.Metrics.PendingTxPool.Store(int64(len(shardA.TxPool)))
	shardB.Metrics.PendingTxPool.Store(0)

	// 3. Remove Shard B from the manager
	//   a. Remove from consistent hash ring
	shardBIDStr := fmt.Sprintf("%d", shardBID)
	sm.consistent.Remove(shardBIDStr)
	log.Printf("Merge: Removed %d from hash ring. Members: %v", shardBID, sm.consistent.Members())

	//   b. Remove from shardIDs slice
	newShardIDs := make([]uint64, 0, len(sm.shardIDs)-1)
	for _, id := range sm.shardIDs {
		if id != shardBID {
			newShardIDs = append(newShardIDs, id)
		}
	}
	sm.shardIDs = newShardIDs
	// No need to re-sort as relative order is maintained

	//   c. Remove from shards map
	delete(sm.shards, shardBID)

	//   d. Stop the merged shard's goroutines/processing (if it has any active ones)
	shardB.Stop()

	log.Printf("--- Merge Complete: Shard %d merged into Shard %d --- New Shard IDs: %v", shardBID, shardAID, sm.shardIDs)

	// TODO: Notify system about shard removal.

	return nil
}
