package core

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
)

// ShardID represents the identifier for a shard.
type ShardID uint

// Shard represents a partition of the blockchain's state and processing.
type Shard struct {
	ID        ShardID
	State     StateDB        // State specific to this shard
	BlockChan chan *Block    // Channel to receive blocks for this shard
	TxPool    []*Transaction // Simple transaction pool for the shard
	Mu        sync.RWMutex   // Exported mutex for transaction pool access
	// Add metrics like transaction count, state size for dynamic splitting/merging decisions
}

// NewShard creates a new shard instance.
func NewShard(id ShardID) *Shard {
	return &Shard{
		ID:        id,
		State:     NewInMemoryStateDB(),   // Each shard gets its own state DB
		BlockChan: make(chan *Block, 100), // Buffered channel
		TxPool:    make([]*Transaction, 0),
	}
}

// AddTransaction adds a transaction to the shard's pool (basic implementation).
func (s *Shard) AddTransaction(tx *Transaction) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.TxPool = append(s.TxPool, tx)
	// In a real system, validate tx against shard state here
	log.Printf("Shard %d: Added Tx %x to pool", s.ID, tx.ID)
}

// GetTransactionsForBlock retrieves transactions to be included in the next block for this shard.
func (s *Shard) GetTransactionsForBlock(maxTx int) []*Transaction {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	count := len(s.TxPool)
	if count == 0 {
		return []*Transaction{}
	}
	if count > maxTx {
		count = maxTx
	}

	txs := make([]*Transaction, count)
	copy(txs, s.TxPool[:count])
	s.TxPool = s.TxPool[count:] // Remove retrieved transactions

	return txs
}

// GetTxPoolSize safely returns the current size of the transaction pool
func (s *Shard) GetTxPoolSize() int {
	s.Mu.RLock()
	defer s.Mu.RUnlock()
	return len(s.TxPool)
}

// ShardManager manages the different shards in the blockchain.
type ShardManager struct {
	Shards       map[ShardID]*Shard
	ShardCount   uint // Current number of active shards
	mu           sync.RWMutex
	routingTable map[ShardID]*Shard // Fast lookup
	// Add configuration for sharding (e.g., max transactions per shard before split)
}

// NewShardManager creates and initializes the shard manager.
func NewShardManager(initialShardCount uint) (*ShardManager, error) {
	if initialShardCount == 0 {
		return nil, errors.New("initial shard count must be greater than 0")
	}
	sm := &ShardManager{
		Shards:       make(map[ShardID]*Shard),
		ShardCount:   initialShardCount,
		routingTable: make(map[ShardID]*Shard),
	}
	for i := uint(0); i < initialShardCount; i++ {
		shardID := ShardID(i)
		shard := NewShard(shardID)
		sm.Shards[shardID] = shard
		sm.routingTable[shardID] = shard
	}
	log.Printf("ShardManager initialized with %d shards.\n", initialShardCount)
	return sm, nil
}

// DetermineShard maps data (e.g., account address, transaction ID) to a ShardID.
// Basic implementation using modulo. A real system might use hash prefixes.
func (sm *ShardManager) DetermineShard(data []byte) ShardID {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.ShardCount == 0 {
		log.Panic("Shard count is zero, cannot determine shard.")
	}
	// Simple hash-based routing for demonstration
	hash := sha256.Sum256(data)
	// Use a portion of the hash to determine the shard
	val := new(big.Int).SetBytes(hash[:8]) // Use first 8 bytes for distribution
	shardIndex := val.Mod(val, big.NewInt(int64(sm.ShardCount))).Uint64()
	return ShardID(shardIndex)
}

// RouteTransaction sends a transaction to the appropriate shard's pool.
func (sm *ShardManager) RouteTransaction(tx *Transaction) error {
	sm.mu.RLock() // Lock for reading shard count and routing table
	// Determine target shard based on transaction content (e.g., sender address)
	// For simplicity, let's use the transaction ID for now.
	shardID := sm.DetermineShard(tx.ID)
	targetShard, ok := sm.routingTable[shardID]
	sm.mu.RUnlock() // Unlock after getting shard info

	if !ok {
		return fmt.Errorf("shard %d not found in routing table", shardID)
	}

	targetShard.AddTransaction(tx) // AddTransaction is internally locked
	return nil
}

// GetShard retrieves a shard by its ID.
func (sm *ShardManager) GetShard(id ShardID) (*Shard, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	shard, ok := sm.Shards[id]
	return shard, ok
}

// --- Dynamic Sharding (Placeholders) ---

// TriggerSplit checks if a shard needs splitting and initiates the process.
// Placeholder: In a real system, this would involve complex state migration and consensus.
func (sm *ShardManager) TriggerSplit(shardID ShardID) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	_, ok := sm.Shards[shardID]
	if !ok {
		return fmt.Errorf("cannot split non-existent shard %d", shardID)
	}

	// Check conditions (e.g., shard size, transaction volume)
	// Condition = true (for demonstration)
	log.Printf("Placeholder: Triggering split for Shard %d\n", shardID)

	// 1. Create new ShardID (e.g., sm.ShardCount)
	// 2. Create new Shard instance
	// 3. Redistribute state/accounts from old shard to new shard
	// 4. Update Merkle roots/accumulators
	// 5. Update routing table
	// 6. Increment sm.ShardCount
	// 7. Requires consensus mechanism agreement

	log.Printf("Placeholder: Shard splitting logic not implemented yet.\n")
	return errors.New("dynamic split not implemented")

}

// TriggerMerge checks if shards can be merged and initiates the process.
// Placeholder: Requires complex coordination and state merging.
func (sm *ShardManager) TriggerMerge(shardID1, shardID2 ShardID) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check conditions (e.g., low load on both shards)
	log.Printf("Placeholder: Triggering merge for Shards %d and %d\n", shardID1, shardID2)

	// 1. Ensure shards exist
	// 2. Migrate state from one shard to the other
	// 3. Update Merkle roots/accumulators
	// 4. Remove one shard
	// 5. Update routing table
	// 6. Decrement sm.ShardCount
	// 7. Requires consensus mechanism agreement

	log.Printf("Placeholder: Shard merging logic not implemented yet.\n")
	return errors.New("dynamic merge not implemented")
}

// --- Cross-Shard Communication (Basic Placeholder) ---

// CrossShardReceipt represents evidence of a transaction part executed on one shard,
// needed for completion on another shard.
type CrossShardReceipt struct {
	OriginShard      ShardID
	DestinationShard ShardID
	TransactionID    []byte
	// Include necessary state proofs or commitments
}

// ProcessCrossShardReceipt processes a receipt received from another shard.
// Placeholder: This logic would be integrated into block processing.
func (s *Shard) ProcessCrossShardReceipt(receipt *CrossShardReceipt) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if receipt.DestinationShard != s.ID {
		return errors.New("receipt destined for wrong shard")
	}

	log.Printf("Shard %d: Processing cross-shard receipt for Tx %x from Shard %d\n",
		s.ID, receipt.TransactionID, receipt.OriginShard)

	// 1. Verify receipt authenticity (using state proofs/commitments - not implemented)
	// 2. Apply state changes based on the receipt (e.g., credit account)
	// 3. Update shard state (s.State.Put(...))

	log.Printf("Placeholder: Cross-shard receipt processing logic not fully implemented.\n")
	return nil // Placeholder success
}
