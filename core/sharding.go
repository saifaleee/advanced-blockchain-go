package core

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"sync/atomic" // For atomic metric updates
	"time"        // For potential time-based metrics
)

// ShardID represents the identifier for a shard.
type ShardID uint

// ShardMetrics holds load information for a shard.
type ShardMetrics struct {
	// Using atomic operations for thread-safe updates without locking the main shard mutex constantly
	TransactionCount    atomic.Int64 // Number of transactions processed or currently in pool (adjust definition as needed)
	StateSizeBytes      atomic.Int64 // Approximate size of the state data
	LastBlockTimestamp  atomic.Int64 // Timestamp of the last block added
	PendingTxCount      atomic.Int64 // Number of transactions currently in the pool
}

// Shard represents a partition of the blockchain's state and processing.
type Shard struct {
	ID        ShardID
	State     StateDB        // State specific to this shard (needs Size() method for metrics)
	BlockChan chan *Block    // Channel to receive blocks for this shard
	TxPool    []*Transaction // Simple transaction pool for the shard
	Mu        sync.RWMutex   // Exported mutex for TxPool and potentially other direct access
	Metrics   ShardMetrics   // Load metrics for dynamic sharding decisions
	stopChan  chan struct{}  // Channel to signal shard shutdown (e.g., during merge)
}

// NewShard creates a new shard instance.
func NewShard(id ShardID) *Shard {
	return &Shard{
		ID:        id,
		State:     NewInMemoryStateDB(),   // Each shard gets its own state DB
		BlockChan: make(chan *Block, 100), // Buffered channel
		TxPool:    make([]*Transaction, 0),
		Metrics:   ShardMetrics{}, // Initialize metrics
		stopChan:  make(chan struct{}),
	}
}

// AddTransaction adds a transaction to the shard's pool and updates metrics.
func (s *Shard) AddTransaction(tx *Transaction) {
	s.Mu.Lock()
	s.TxPool = append(s.TxPool, tx)
	pendingCount := len(s.TxPool)
	s.Mu.Unlock() // Unlock before logging and atomic ops

	s.Metrics.PendingTxCount.Store(int64(pendingCount)) // Update pending count
	s.Metrics.TransactionCount.Add(1) // Increment total processed/added count

	log.Printf("Shard %d: Added Tx %x to pool (Pending: %d)", s.ID, tx.ID, pendingCount)
}

// GetTransactionsForBlock retrieves transactions and updates metrics.
func (s *Shard) GetTransactionsForBlock(maxTx int) []*Transaction {
	s.Mu.Lock() // Lock TxPool access

	count := len(s.TxPool)
	if count == 0 {
		s.Mu.Unlock()
		s.Metrics.PendingTxCount.Store(0) // Ensure pending count is 0
		return []*Transaction{}
	}
	if count > maxTx {
		count = maxTx
	}

	txs := make([]*Transaction, count)
	copy(txs, s.TxPool[:count])
	s.TxPool = s.TxPool[count:] // Remove retrieved transactions
	pendingCount := len(s.TxPool)

	s.Mu.Unlock() // Unlock before atomic ops

	s.Metrics.PendingTxCount.Store(int64(pendingCount)) // Update pending count after removal

	return txs
}

// GetTxPoolSize safely returns the current size of the transaction pool
func (s *Shard) GetTxPoolSize() int {
	// Use atomic load for quick read if strict consistency with Mu isn't needed here
	// return int(s.Metrics.PendingTxCount.Load())
	// Or use the mutex for guaranteed consistency with Add/Get:
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return len(s.TxPool)
}

// UpdateStateSizeMetric updates the approximate state size metric.
// This should be called periodically or after significant state changes.
func (s *Shard) UpdateStateSizeMetric() {
	// Check if StateDB supports Size() method
	if sizeableDB, ok := s.State.(interface{ Size() int64 }); ok {
		size := sizeableDB.Size()
		s.Metrics.StateSizeBytes.Store(size)
		// log.Printf("Shard %d: Updated state size metric to %d bytes", s.ID, size)
	} else {
		// log.Printf("Shard %d: StateDB does not support Size() method for metrics.", s.ID)
		// Could approximate based on transaction count or other heuristics if needed
	}
}

// UpdateLastBlockTimestamp updates the timestamp metric.
func (s *Shard) UpdateLastBlockTimestamp(timestamp int64) {
	s.Metrics.LastBlockTimestamp.Store(timestamp)
}

// Stop signals the shard to stop processing (e.g., before merging).
func (s *Shard) Stop() {
	close(s.stopChan) // Signal shutdown
	log.Printf("Shard %d: Stop signal received.", s.ID)
	// Add logic here to gracefully stop any shard-specific goroutines if needed
}

// --- Shard Manager ---

// ShardManagerConfig holds configuration for dynamic sharding.
type ShardManagerConfig struct {
	SplitTxThreshold     int64 // Trigger split if PendingTxCount exceeds this
	MergeTxThreshold     int64 // Consider merge if PendingTxCount below this
	SplitStateThreshold  int64 // Trigger split if StateSizeBytes exceeds this
	MergeStateThreshold  int64 // Consider merge if StateSizeBytes below this
	CheckInterval        time.Duration // How often to check shard metrics
	MaxShards            uint          // Maximum number of shards allowed
	MinShards            uint          // Minimum number of shards allowed
}

// DefaultShardManagerConfig provides sensible defaults.
func DefaultShardManagerConfig() ShardManagerConfig {
	return ShardManagerConfig{
		SplitTxThreshold:     1000, // Example: Split if > 1000 pending tx
		MergeTxThreshold:     50,   // Example: Merge if < 50 pending tx
		SplitStateThreshold:  1024 * 1024 * 100, // Example: Split if > 100MB state
		MergeStateThreshold:  1024 * 1024 * 10,  // Example: Merge if < 10MB state
		CheckInterval:        30 * time.Second,  // Example: Check every 30 seconds
		MaxShards:            256,               // Example: Limit max shards
		MinShards:            1,                 // Example: Must have at least 1 shard
	}
}

// ShardManager manages the different shards in the blockchain.
type ShardManager struct {
	Shards         map[ShardID]*Shard
	ShardCount     uint // Current number of active shards (use atomic for reads outside lock?)
	Config         ShardManagerConfig
	Mu             sync.RWMutex
	RoutingTable   map[ShardID]*Shard // Fast lookup for active shards
	nextShardID    ShardID            // Counter for assigning new shard IDs
	managementStop chan struct{}      // Channel to stop the management loop
	IsManaging     atomic.Bool        // Flag to indicate if management loop is running
}

// NewShardManager creates and initializes the shard manager.
func NewShardManager(initialShardCount uint, config ShardManagerConfig) (*ShardManager, error) {
	if initialShardCount == 0 || initialShardCount < config.MinShards || initialShardCount > config.MaxShards {
		return nil, fmt.Errorf("initial shard count (%d) must be between MinShards (%d) and MaxShards (%d)",
			initialShardCount, config.MinShards, config.MaxShards)
	}
	sm := &ShardManager{
		Shards:         make(map[ShardID]*Shard),
		ShardCount:     initialShardCount,
		Config:         config,
		RoutingTable:   make(map[ShardID]*Shard),
		nextShardID:    ShardID(initialShardCount), // Start assigning IDs after initial ones
		managementStop: make(chan struct{}),
	}
	for i := uint(0); i < initialShardCount; i++ {
		shardID := ShardID(i)
		shard := NewShard(shardID)
		sm.Shards[shardID] = shard
		sm.RoutingTable[shardID] = shard
	}
	log.Printf("ShardManager initialized with %d shards.\n", initialShardCount)
	return sm, nil
}

// StartManagementLoop periodically checks shard metrics and triggers splits/merges.
func (sm *ShardManager) StartManagementLoop() {
	if !sm.IsManaging.CompareAndSwap(false, true) {
		log.Println("Shard management loop already running.")
		return
	}
	log.Println("Starting shard management loop...")
	go func() {
		ticker := time.NewTicker(sm.Config.CheckInterval)
		defer ticker.Stop()
		defer func() {
			sm.IsManaging.Store(false) // Ensure flag is reset on exit
			log.Println("Shard management loop stopped.")
		}()

		for {
			select {
			case <-ticker.C:
				log.Println("Checking shard metrics for dynamic management...")
				sm.CheckAndManageShards()
			case <-sm.managementStop:
				return // Exit loop
			}
		}
	}()
}

// StopManagementLoop signals the management loop to terminate.
func (sm *ShardManager) StopManagementLoop() {
	if sm.IsManaging.CompareAndSwap(true, false) { // Only close if running
		close(sm.managementStop)
		log.Println("Stop signal sent to shard management loop.")
	} else {
		log.Println("Shard management loop not running.")
	}
}

// CheckAndManageShards iterates through shards and triggers split/merge based on config.
func (sm *ShardManager) CheckAndManageShards() {
	sm.Mu.Lock() // Lock for the duration of checking and potentially modifying shards map/count
	defer sm.Mu.Unlock()

	// Flag to track if any splits happened during this check
	splitOccurred := false

	// --- Check for Splits ---
	// Create a list of shard IDs to iterate over to avoid issues if map changes during iteration (though lock prevents this here)
	currentShardIDs := make([]ShardID, 0, len(sm.Shards))
	for id := range sm.Shards {
		currentShardIDs = append(currentShardIDs, id)
	}

	for _, shardID := range currentShardIDs {
		shard, exists := sm.Shards[shardID]
		if !exists {
			continue // Shard might have been merged in a previous iteration of this check
		}

		// Update metrics before checking (especially state size)
		shard.UpdateStateSizeMetric()

		pendingTx := shard.Metrics.PendingTxCount.Load()
		stateSize := shard.Metrics.StateSizeBytes.Load()

		// Check split conditions
		needsSplit := false
		if sm.Config.SplitTxThreshold > 0 && pendingTx > sm.Config.SplitTxThreshold {
			log.Printf("Shard %d needs split: Pending Txs (%d) > Threshold (%d)",
				shardID, pendingTx, sm.Config.SplitTxThreshold)
			needsSplit = true
		}
		if !needsSplit && sm.Config.SplitStateThreshold > 0 && stateSize > sm.Config.SplitStateThreshold {
			log.Printf("Shard %d needs split: State Size (%d) > Threshold (%d)",
				shardID, stateSize, sm.Config.SplitStateThreshold)
			needsSplit = true
		}

		// Trigger split if needed and possible
		if needsSplit {
			if sm.ShardCount >= sm.Config.MaxShards {
				log.Printf("Split needed for Shard %d, but MaxShards (%d) reached. Skipping split.",
					shardID, sm.Config.MaxShards)
			} else {
				// Call the actual split function (which is still mostly placeholder)
				err := sm.triggerSplitUnsafe(shardID) // Use unsafe version as lock is already held
				if err != nil {
					log.Printf("Error triggering split for shard %d: %v", shardID, err)
				} else {
					// If split successful, set flag to skip merge checks in this cycle
					splitOccurred = true
				}
			}
		}
	}

	// --- Check for Merges ---
	// Skip merge checks if a split occurred in this cycle or merging is disabled (MergeTxThreshold = 0)
	if splitOccurred || sm.Config.MergeTxThreshold <= 0 {
		return
	}
	
	// Merge logic is more complex: needs to find suitable pairs of shards.
	// Simple approach: find any shard below threshold and try to merge it with a neighbor (if topology is defined)
	// or another low-load shard.
	// For now, let's just identify shards that *could* be merged.

	mergeCandidates := make([]ShardID, 0)
	currentShardIDs = make([]ShardID, 0, len(sm.Shards)) // Refresh list in case splits occurred
	for id := range sm.Shards {
		currentShardIDs = append(currentShardIDs, id)
	}

	for _, shardID := range currentShardIDs {
		shard, exists := sm.Shards[shardID]
		if !exists {
			continue
		}

		pendingTx := shard.Metrics.PendingTxCount.Load()
		stateSize := shard.Metrics.StateSizeBytes.Load()

		// Check merge conditions (both must be below threshold)
		canMerge := false
		if sm.Config.MergeTxThreshold > 0 && pendingTx < sm.Config.MergeTxThreshold &&
			sm.Config.MergeStateThreshold > 0 && stateSize < sm.Config.MergeStateThreshold {
			canMerge = true
		} else if sm.Config.MergeTxThreshold > 0 && pendingTx < sm.Config.MergeTxThreshold && sm.Config.MergeStateThreshold <= 0 {
			// Only Tx threshold matters
			canMerge = true
		} else if sm.Config.MergeStateThreshold > 0 && stateSize < sm.Config.MergeStateThreshold && sm.Config.MergeTxThreshold <= 0 {
			// Only State threshold matters
			canMerge = true
		}

		if canMerge {
			if sm.ShardCount > sm.Config.MinShards {
				mergeCandidates = append(mergeCandidates, shardID)
			} else {
				// log.Printf("Shard %d is below merge threshold, but MinShards (%d) reached. Cannot merge.",
				// 	shardID, sm.Config.MinShards)
			}
		}
	}

	// Simple pairing logic: Try merging adjacent candidates if possible
	if len(mergeCandidates) >= 2 {
		// Need a defined topology or pairing strategy. Let's try merging the first two candidates found.
		shardID1 := mergeCandidates[0]
		shardID2 := mergeCandidates[1]

		log.Printf("Potential merge candidates identified: %v. Attempting merge for %d and %d.", mergeCandidates, shardID1, shardID2)
		// Ensure both shards still exist (might have changed if splits happened)
		if _, ok1 := sm.Shards[shardID1]; ok1 {
			if _, ok2 := sm.Shards[shardID2]; ok2 {
				// Call the actual merge function (mostly placeholder)
				err := sm.triggerMergeUnsafe(shardID1, shardID2) // Use unsafe version
				if err != nil {
					log.Printf("Error triggering merge for shards %d and %d: %v", shardID1, shardID2, err)
				} else {
					// Merge successful, potentially stop checking for merges this round
					// Or update mergeCandidates and continue
				}
			}
		}
	}
}

// DetermineShard maps data to a ShardID.
// WARNING: Simple modulo routing breaks down with dynamic shard counts unless remapping occurs.
// A production system needs consistent hashing or range-based routing.
func (sm *ShardManager) DetermineShard(data []byte) ShardID {
	sm.Mu.RLock()
	currentShardCount := sm.ShardCount // Read count under lock
	sm.Mu.RUnlock()

	if currentShardCount == 0 {
		log.Panic("Shard count is zero, cannot determine shard.")
	}
	hash := sha256.Sum256(data)
	val := new(big.Int).SetBytes(hash[:8])
	shardIndex := val.Mod(val, big.NewInt(int64(currentShardCount))).Uint64()

	// Need to map the index back to an *actual* active ShardID from routingTable
	// This simple modulo doesn't guarantee the target shard exists if shards were merged/split non-contiguously.
	// For now, return the raw index as ShardID and rely on routingTable lookup later.
	// A better approach maps hash ranges to specific ShardIDs.
	targetID := ShardID(shardIndex)
	// log.Printf("Determined raw shard index %d for data %x (ShardCount: %d)", targetID, data[:4], currentShardCount)

	// Quick check if the target ID likely exists (doesn't guarantee it if merges happened)
	sm.Mu.RLock()
	_, exists := sm.RoutingTable[targetID]
	sm.Mu.RUnlock()
	if !exists {
		// Fallback or re-routing needed. For now, just log and return the calculated ID.
		// A simple fallback could be modulo len(sm.routingTable), but keys aren't sequential.
		// Or just route to shard 0.
		log.Printf("Warning: Determined shard %d does not exist in routing table. Routing may fail or fallback.", targetID)
		// Fallback to shard 0 for now in this basic example
		// return ShardID(0)
	}

	return targetID // Return the calculated ID, RouteTransaction must handle non-existence
}

// RouteTransaction sends a transaction to the appropriate shard's pool.
func (sm *ShardManager) RouteTransaction(tx *Transaction) error {
	// Determine target shard based on transaction content
	shardID := sm.DetermineShard(tx.ID) // Uses updated DetermineShard

	sm.Mu.RLock() // Lock for reading routing table
	targetShard, ok := sm.RoutingTable[shardID]
	
	if !ok {
		// If the target shard doesn't exist (e.g., due to merges), 
		// use the first available shard as fallback
		log.Printf("Routing Error: Target shard %d determined for Tx %x does not exist in routing table. Using fallback.", shardID, tx.ID)
		
		// Find any available shard (shard 0 preferably, but any if 0 doesn't exist)
		targetShard = nil
		
		// Try shard 0 first
		if shard0, exists := sm.RoutingTable[ShardID(0)]; exists {
			targetShard = shard0
			log.Printf("Routing Tx %x to fallback shard 0", tx.ID)
		} else {
			// If shard 0 doesn't exist, use the first available shard from the map
			for id, shard := range sm.RoutingTable {
				targetShard = shard
				log.Printf("Routing Tx %x to fallback shard %d", tx.ID, id)
				break
			}
		}
		
		if targetShard == nil {
			sm.Mu.RUnlock() // Make sure to unlock before returning error
			return fmt.Errorf("critical routing error: no shards available for tx %x", tx.ID)
		}
	}
	
	// We've found the target shard. Now unlock before adding transaction
	sm.Mu.RUnlock()

	targetShard.AddTransaction(tx) // AddTransaction is internally locked
	return nil
}

// GetShard retrieves a shard by its ID.
func (sm *ShardManager) GetShard(id ShardID) (*Shard, bool) {
	sm.Mu.RLock()
	defer sm.Mu.RUnlock()
	shard, ok := sm.Shards[id] // Check main map
	return shard, ok
}

// --- Dynamic Sharding Implementation (Structural) ---

// triggerSplitUnsafe performs the structural parts of splitting a shard.
// Assumes sm.mu is already locked.
func (sm *ShardManager) triggerSplitUnsafe(shardToSplitID ShardID) error {
	if sm.ShardCount >= sm.Config.MaxShards {
		return fmt.Errorf("cannot split shard %d: MaxShards (%d) reached", shardToSplitID, sm.Config.MaxShards)
	}

	_, exists := sm.Shards[shardToSplitID]
	if !exists {
		return fmt.Errorf("cannot split non-existent shard %d", shardToSplitID)
	}

	// 1. Determine and assign new ShardID
	newShardID := sm.nextShardID
	sm.nextShardID++ // Increment for the next split

	log.Printf("Initiating split of Shard %d into new Shard %d", shardToSplitID, newShardID)

	// 2. Create new Shard instance
	newShard := NewShard(newShardID)

	// --- Placeholder: State Redistribution ---
	// This is the complex part requiring careful state iteration, locking,
	// potential temporary suspension of operations on the original shard,
	// and cryptographic commitments.
	log.Printf("Placeholder: Redistributing state from Shard %d to Shard %d...", shardToSplitID, newShardID)
	// Example: Iterate originalShard.State, decide which keys go to newShard.State, move them.
	// err := migrateState(originalShard, newShard)
	// if err != nil { return fmt.Errorf("state migration failed: %w", err) }
	log.Printf("Placeholder: State redistribution logic not implemented.")
	// --- End Placeholder ---

	// 4. Update Merkle roots/accumulators (Placeholder)
	// After state migration, the state roots/accumulators for both shards need recalculation.
	log.Printf("Placeholder: Updating state roots/accumulators for shards %d and %d.", shardToSplitID, newShardID)

	// 5. Update ShardManager maps and count
	sm.Shards[newShardID] = newShard
	sm.RoutingTable[newShardID] = newShard // Add new shard to routing
	sm.ShardCount++

	log.Printf("Shard split complete (structurally). Shard count: %d. Shard %d added.", sm.ShardCount, newShardID)

	// 6. Consensus (Placeholder)
	log.Printf("Placeholder: Consensus required to finalize shard split (%d -> %d).", shardToSplitID, newShardID)

	return nil
}

// triggerMergeUnsafe performs the structural parts of merging two shards.
// Assumes sm.mu is already locked.
func (sm *ShardManager) triggerMergeUnsafe(shardID1, shardID2 ShardID) error {
	if sm.ShardCount <= sm.Config.MinShards {
		return fmt.Errorf("cannot merge shards %d, %d: MinShards (%d) reached", shardID1, shardID2, sm.Config.MinShards)
	}
	if shardID1 == shardID2 {
		return fmt.Errorf("cannot merge a shard with itself (%d)", shardID1)
	}

	_, exists1 := sm.Shards[shardID1]
	shard2, exists2 := sm.Shards[shardID2]

	if !exists1 || !exists2 {
		return fmt.Errorf("cannot merge non-existent shards (%d exists: %t, %d exists: %t)", shardID1, exists1, shardID2, exists2)
	}

	// Convention: Merge shard2 *into* shard1. Shard2 will be removed.
	log.Printf("Initiating merge of Shard %d into Shard %d", shardID2, shardID1)

	// --- Placeholder: State Migration ---
	// Stop processing on shard2, migrate its state to shard1, handle conflicts.
	shard2.Stop() // Signal shard 2 to stop activity
	log.Printf("Placeholder: Migrating state from Shard %d to Shard %d...", shardID2, shardID1)
	// Example: Iterate shard2.State, add keys/values to shard1.State, resolve conflicts.
	// err := consolidateState(shard1, shard2)
	// if err != nil { return fmt.Errorf("state consolidation failed: %w", err) }
	log.Printf("Placeholder: State migration logic not implemented.")
	// --- End Placeholder ---

	// 3. Update Merkle roots/accumulators (Placeholder)
	log.Printf("Placeholder: Updating state root/accumulator for shard %d after merge.", shardID1)

	// 4. Remove shard2 from manager
	delete(sm.Shards, shardID2)
	delete(sm.RoutingTable, shardID2) // Remove from routing
	sm.ShardCount--

	log.Printf("Shard merge complete (structurally). Shard count: %d. Shard %d removed.", sm.ShardCount, shardID2)

	// 7. Consensus (Placeholder)
	log.Printf("Placeholder: Consensus required to finalize shard merge (%d into %d).", shardID2, shardID1)

	return nil
}

// --- Cross-Shard Communication (Placeholders - Unchanged) ---

type CrossShardReceipt struct {
	OriginShard      ShardID
	DestinationShard ShardID
	TransactionID    []byte
	// Include necessary state proofs or commitments
}

func (s *Shard) ProcessCrossShardReceipt(receipt *CrossShardReceipt) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if receipt.DestinationShard != s.ID {
		return errors.New("receipt destined for wrong shard")
	}
	log.Printf("Shard %d: Processing cross-shard receipt for Tx %x from Shard %d\n",
		s.ID, receipt.TransactionID, receipt.OriginShard)
	log.Printf("Placeholder: Cross-shard receipt processing logic not fully implemented.\n")
	return nil
}
