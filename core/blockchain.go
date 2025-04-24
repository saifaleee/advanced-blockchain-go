package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"sync"
)

// Blockchain now manages multiple shards.
type Blockchain struct {
	ShardManager *ShardManager       // Manages all shards
	Difficulty   int                 // Global PoW difficulty (can be per-shard later)
	ChainHeight  int                 // Approximate overall chain height (e.g., max shard height) - needs refinement
	Mu           sync.RWMutex        // Protects global state like ChainHeight
	BlockChains  map[uint64][]*Block // In-memory storage per shard chain
	ChainMu      sync.RWMutex        // Lock specifically for accessing blockChains map
}

// NewBlockchain creates a new sharded blockchain.
func NewBlockchain(initialShardCount uint, config ShardManagerConfig) (*Blockchain, error) {
	difficulty := 16 // Default difficulty
	if difficulty < 1 {
		return nil, errors.New("difficulty must be at least 1")
	}

	// Create shard manager with config
	sm, err := NewShardManager(config, int(initialShardCount))
	if err != nil {
		return nil, fmt.Errorf("failed to create shard manager: %w", err)
	}

	bc := &Blockchain{
		ShardManager: sm,
		Difficulty:   difficulty,
		ChainHeight:  0, // Initial height is 0
		BlockChains:  make(map[uint64][]*Block),
	}

	// Create genesis block for each shard
	bc.ChainMu.Lock() // Lock before modifying blockChains map
	defer bc.ChainMu.Unlock()

	// Get all shard IDs from the manager
	shardIDs := sm.GetAllShardIDs()
	for _, shardID := range shardIDs {
		genesis := NewGenesisBlock(shardID, nil, difficulty)
		bc.BlockChains[shardID] = []*Block{genesis}
		log.Printf("Created Genesis Block for Shard %d\n", shardID)
	}

	return bc, nil
}

// AddTransaction routes a transaction to the appropriate shard.
func (bc *Blockchain) AddTransaction(tx *Transaction) error {
	// Basic routing - could involve more complex logic like checking account shards
	if tx.Type == CrossShardTxInit && tx.SourceShard != nil {
		// Route initialization to the source shard
		sourceShard, ok := bc.ShardManager.GetShard(*tx.SourceShard)
		if !ok {
			return fmt.Errorf("source shard %d not found for cross-shard tx %x", *tx.SourceShard, tx.ID)
		}
		sourceShard.AddTransaction(tx) // AddTransaction handles internal locking
		log.Printf("Routed cross-shard init tx %x to source shard %d", tx.ID, *tx.SourceShard)
		return nil
	} else {
		// Route intra-shard or other types based on general routing rule (e.g., ID)
		shardID, err := bc.ShardManager.DetermineShard(tx.ID)
		if err != nil {
			return fmt.Errorf("failed to determine shard for tx %x: %w", tx.ID, err)
		}
		shard, ok := bc.ShardManager.GetShard(shardID)
		if !ok {
			return fmt.Errorf("determined shard %d not found for tx %x", shardID, tx.ID)
		}
		return shard.AddTransaction(tx)
	}
}

// MineShardBlock attempts to mine a new block for a specific shard using transactions from its pool.
func (bc *Blockchain) MineShardBlock(shardID uint64) (*Block, error) {
	shard, ok := bc.ShardManager.GetShard(shardID)
	if !ok {
		return nil, fmt.Errorf("shard %d not found", shardID)
	}

	// Define max transactions per block (could be configurable)
	maxTxPerBlock := 10
	txs := shard.GetTransactionsForBlock(maxTxPerBlock)

	// Handle Cross-Shard Transactions (Simplified)
	// In a real system, need coordination, proofs, etc.
	var receiptsToProcess []*CrossShardReceipt // This remains empty as receipt relay isn't implemented
	var transactionsForBlock []*Transaction
	for _, tx := range txs {
		if tx.Type == CrossShardTxInit && tx.SourceShard != nil && tx.DestinationShard != nil {
			// Process on source shard:
			// 1. Validate transaction against source shard state
			// 2. Deduct funds/lock state on source shard (using shard.State)
			// 3. Generate a receipt to be sent to the destination shard
			log.Printf("Shard %d: Processing cross-shard init Tx %x for dest %d", shardID, tx.ID, *tx.DestinationShard)

			// Create a proper cross-shard receipt
			receipt := &CrossShardReceipt{
				SourceShard:      *tx.SourceShard,
				DestinationShard: *tx.DestinationShard,
				TransactionID:    tx.ID,
				Data:             tx.Data, // Include transaction data for destination
				// In a real system, we would also include:
				// SourceBlockHash: Will be set after block is mined
				// SourceBlockHeight: Will be set after block is mined
				// Proof: Merkle proof of inclusion (generated after block creation)
			}

			// Route the receipt by creating a finalization transaction for the destination shard
			finalizeTx := NewTransaction(
				receipt.Data,
				CrossShardTxFinalize,
				&receipt.DestinationShard,
			)

			// Set source reference in the transaction
			finalizeTx.SourceShard = &receipt.SourceShard
			// Set a reference to the original transaction ID
			finalizeTx.SourceReceiptProof = &ReceiptProof{
				MerkleProof: [][]byte{receipt.TransactionID}, // Simplified, should be actual proof
			}

			// Add the finalization transaction to the destination shard
			destShard, destOk := bc.ShardManager.GetShard(receipt.DestinationShard)
			if destOk {
				err := destShard.AddTransaction(finalizeTx)
				if err != nil {
					log.Printf("Shard %d: Error routing cross-shard finalize tx to shard %d: %v",
						shardID, receipt.DestinationShard, err)
				} else {
					log.Printf("Shard %d: Successfully routed cross-shard finalize tx %x to shard %d",
						shardID, finalizeTx.ID, receipt.DestinationShard)
				}
			} else {
				log.Printf("Shard %d: Cannot route cross-shard tx, destination shard %d not found",
					shardID, receipt.DestinationShard)
			}

			// Include the initiating tx in the block
			transactionsForBlock = append(transactionsForBlock, tx)
		} else if tx.Type == CrossShardTxFinalize {
			// Process finalization transaction
			log.Printf("Shard %d: Processing cross-shard finalize Tx %x", shardID, tx.ID)

			// Include the finalization transaction in the block
			transactionsForBlock = append(transactionsForBlock, tx)
		} else {
			// Intra-shard transaction
			// Placeholder for applying tx to shard state: shard.State.Put/Get...
			transactionsForBlock = append(transactionsForBlock, tx)
		}
	}

	// Process incoming receipts (if a mechanism delivered them to receiptsToProcess)
	for _, receipt := range receiptsToProcess { // This loop will currently do nothing
		// Skip processing for now as the method doesn't exist
		log.Printf("Shard %d: Would process receipt for Tx %x (once implemented)", shardID, receipt.TransactionID)
		// Later: err := shard.ProcessCrossShardReceipt(receipt)
	}

	if len(transactionsForBlock) == 0 {
		// Optionally allow empty blocks or return specific status
		// log.Printf("Shard %d: No transactions to mine.", shardID)
		// return nil, errors.New("no transactions available for mining")
		// Let's allow empty blocks for simplicity now
	}

	// Get the latest block *for this shard*
	bc.ChainMu.RLock() // Use RLock for reading
	shardChain, chainOk := bc.BlockChains[shardID]
	bc.ChainMu.RUnlock() // Unlock after reading

	// If shard chain doesn't exist yet, initialize it with a genesis block
	if !chainOk || len(shardChain) == 0 {
		err := bc.initializeShardChain(shardID)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize chain for shard %d: %w", shardID, err)
		}

		// Get the chain again now that it's initialized
		bc.ChainMu.RLock()
		shardChain, chainOk = bc.BlockChains[shardID]
		if !chainOk || len(shardChain) == 0 {
			bc.ChainMu.RUnlock()
			return nil, fmt.Errorf("shard %d chain initialization failed", shardID)
		}
		bc.ChainMu.RUnlock()
	}

	bc.ChainMu.RLock()
	prevBlock := shardChain[len(shardChain)-1]
	bc.ChainMu.RUnlock()

	newHeight := prevBlock.Header.Height + 1

	// Apply state changes for all transactions in the block
	stateRoot, err := shard.ApplyStateChanges(transactionsForBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to apply state changes: %w", err)
	}

	newBlock, err := NewBlock(shardID, transactionsForBlock, prevBlock.Hash, newHeight, stateRoot, bc.Difficulty)
	if err != nil {
		return nil, fmt.Errorf("failed to create new block: %w", err)
	}

	// Basic validation before adding
	pow := NewProofOfWork(newBlock)
	if !pow.Validate() {
		return nil, fmt.Errorf("shard %d: mined block %d failed proof-of-work validation", shardID, newHeight)
	}
	if !ValidateBlockIntegrity(newBlock, prevBlock) { // Use existing validator
		return nil, fmt.Errorf("shard %d: mined block %d failed integrity validation against previous block", shardID, newHeight)
	}

	// Add the successfully mined block to the shard's chain
	bc.ChainMu.Lock() // Lock map access for writing
	bc.BlockChains[shardID] = append(bc.BlockChains[shardID], newBlock)
	currentHeight := newBlock.Header.Height
	bc.ChainMu.Unlock() // Unlock map access

	bc.Mu.Lock() // Lock global state
	if int(currentHeight) > bc.ChainHeight {
		bc.ChainHeight = int(currentHeight) // Update global height if this shard is tallest
	}
	bc.Mu.Unlock() // Unlock global state

	// Update shard metrics after block is added
	// Commented out since it seems this method doesn't exist yet
	// shard.UpdateLastBlockTimestamp(newBlock.Header.Timestamp)

	log.Printf("Shard %d: Added Block %d to chain.\n", shardID, newHeight)
	return newBlock, nil
}

// initializeShardChain creates a genesis block for a shard and initializes its blockchain
func (bc *Blockchain) initializeShardChain(shardID uint64) error {
	// Check if the shard exists in the shard manager
	_, ok := bc.ShardManager.GetShard(shardID)
	if !ok {
		return fmt.Errorf("shard %d not found in shard manager", shardID)
	}

	// Check if chain already exists
	bc.ChainMu.RLock()
	_, exists := bc.BlockChains[shardID]
	bc.ChainMu.RUnlock()

	if exists {
		return nil // Chain already initialized
	}

	// Create genesis block for the shard
	genesis := NewGenesisBlock(shardID, nil, bc.Difficulty)

	// Add the genesis block to the blockchain
	bc.ChainMu.Lock()
	bc.BlockChains[shardID] = []*Block{genesis}
	bc.ChainMu.Unlock()

	log.Printf("Created Genesis Block for Shard %d\n", shardID)
	return nil
}

// GetLatestBlock returns the most recent block for a specific shard.
func (bc *Blockchain) GetLatestBlock(shardID uint64) (*Block, error) {
	bc.ChainMu.RLock() // Use RLock for reading
	defer bc.ChainMu.RUnlock()

	shardChain, ok := bc.BlockChains[shardID]
	if !ok {
		return nil, fmt.Errorf("shard %d not found", shardID)
	}
	if len(shardChain) == 0 {
		return nil, fmt.Errorf("shard %d chain is empty", shardID) // Should have genesis
	}
	return shardChain[len(shardChain)-1], nil
}

// GetBlock retrieves a specific block by hash (searches all shards - inefficient).
// A real system needs indexing or routing hints.
func (bc *Blockchain) GetBlock(hash []byte) (*Block, error) {
	bc.ChainMu.RLock() // Use RLock for reading
	defer bc.ChainMu.RUnlock()

	// Search in all shard chains
	for _, chain := range bc.BlockChains {
		for _, block := range chain {
			if bytes.Equal(block.Hash, hash) {
				return block, nil
			}
		}
	}
	return nil, fmt.Errorf("block with hash %x not found in any shard", hash)
}

// ValidateBlockIntegrity remains the same as before (checks height and prev hash link)
func ValidateBlockIntegrity(newBlock, prevBlock *Block) bool {
	if newBlock == nil || prevBlock == nil {
		log.Println("Error: Cannot validate nil blocks.")
		return false
	}
	if newBlock.Header.ShardID != prevBlock.Header.ShardID {
		log.Printf("Validation Error: Block %d ShardID (%d) does not match Prev Block %d ShardID (%d)\n",
			newBlock.Header.Height, newBlock.Header.ShardID, prevBlock.Header.Height, prevBlock.Header.ShardID)
		return false
	}
	if !bytes.Equal(newBlock.Header.PrevBlockHash, prevBlock.Hash) {
		log.Printf("Validation Error: Shard %d Block %d PrevBlockHash (%x) does not match Block %d Hash (%x)\n",
			newBlock.Header.ShardID, newBlock.Header.Height, newBlock.Header.PrevBlockHash, prevBlock.Header.Height, prevBlock.Hash)
		return false
	}
	// *** FIX: Corrected log format string for height validation ***
	if newBlock.Header.Height != prevBlock.Header.Height+1 {
		log.Printf("Validation Error: Shard %d Block %d Height (%d) is not sequential to previous block height %d\n",
			newBlock.Header.ShardID, newBlock.Header.Height, newBlock.Header.Height, prevBlock.Header.Height) // Corrected second %d to newBlock.Height
		return false
	}
	// log.Printf("Shard %d: Block %d integrity validated successfully against Block %d.\n", newBlock.ShardID, newBlock.Height, prevBlock.Height)
	return true
}

// IsChainValid now validates each shard's chain independently.
func (bc *Blockchain) IsChainValid() bool {
	bc.ChainMu.RLock() // Use RLock for reading map structure
	defer bc.ChainMu.RUnlock()
	overallValid := true

	var wg sync.WaitGroup
	validationErrors := make(chan error, len(bc.BlockChains)) // Channel for errors

	// Create a copy of the chains to validate, so we don't hold the lock during validation
	chainsToValidate := make(map[uint64][]*Block)
	for id, chain := range bc.BlockChains {
		chainsToValidate[id] = chain
	}

	for shardID, chain := range chainsToValidate {
		wg.Add(1)
		go func(sID uint64, ch []*Block) {
			defer wg.Done()
			// log.Printf("Validating chain for Shard %d...", sID) // Reduce log noise
			if len(ch) == 0 {
				// log.Printf("Shard %d chain is empty, skipping validation.", sID)
				return // Skip empty chains (shouldn't happen with genesis)
			}
			if len(ch) == 1 {
				genesis := ch[0]
				powGenesis := NewProofOfWork(genesis)
				if !powGenesis.Validate() {
					err := fmt.Errorf("shard %d genesis block PoW validation failed", sID)
					log.Println(err)
					validationErrors <- err
					return
				}
				// log.Printf("Shard %d Genesis block validated.", sID)
				return
			}

			for i := 1; i < len(ch); i++ {
				currentBlock := ch[i]
				prevBlock := ch[i-1]

				pow := NewProofOfWork(currentBlock)
				if !pow.Validate() {
					err := fmt.Errorf("shard %d PoW validation failed for Block %d (Hash: %x)", sID, currentBlock.Header.Height, currentBlock.Hash)
					log.Println(err)
					validationErrors <- err
					return // Stop validation for this shard on first error
				}

				if !ValidateBlockIntegrity(currentBlock, prevBlock) {
					err := fmt.Errorf("shard %d integrity validation failed between Block %d and Block %d", sID, currentBlock.Header.Height, prevBlock.Header.Height)
					log.Println(err)
					validationErrors <- err
					return // Stop validation for this shard
				}
			}
			// log.Printf("Shard %d chain validation successful.", sID)
		}(shardID, chain)
	}

	wg.Wait() // Wait for all shard validations to complete
	close(validationErrors)

	// Check if any errors occurred
	if len(validationErrors) > 0 {
		log.Printf("Blockchain validation failed. Errors encountered:")
		for err := range validationErrors {
			log.Printf("- %v", err)
		}
		overallValid = false
	} else {
		log.Println("All shard chains validated successfully. Blockchain is valid.")
	}

	return overallValid
}

// --- State Pruning (Ticket 10 - Placeholder) ---

// PruneChain attempts to remove old blocks from shard chains below a certain height,
// keeping headers or necessary state roots (not fully implemented).
func (bc *Blockchain) PruneChain(pruneHeight int) {
	bc.ChainMu.Lock() // Lock map access for writing
	defer bc.ChainMu.Unlock()

	log.Printf("Placeholder: Attempting to prune chains below height %d...", pruneHeight)
	if pruneHeight <= 0 {
		log.Println("Pruning skipped: Height must be positive.")
		return
	}

	for shardID, chain := range bc.BlockChains {
		if len(chain) > pruneHeight {
			// Basic pruning: Just remove the old blocks from the slice.
			// A real implementation needs to preserve headers/roots in a separate index
			// and ensure state corresponding to pruned blocks is handled (e.g., snapshots).
			log.Printf("Shard %d: Pruning %d blocks (keeping %d).", shardID, pruneHeight, len(chain)-pruneHeight)
			// Keep blocks from pruneHeight onwards
			bc.BlockChains[shardID] = chain[pruneHeight:] // This modifies the map entry directly
			// Update the PrevBlockHash of the new first block (now at index 0) if necessary?
			// No, the PrevBlockHash should still point to the *actual* previous block's hash,
			// even if that block data is discarded. This requires storing headers separately.
			// For this simplified slice removal, we implicitly lose that history link for the
			// *very first* remaining block if we don't store headers.
		} else {
			// log.Printf("Shard %d: No pruning needed (chain length %d <= prune height %d).", shardID, len(chain), pruneHeight)
		}
	}
	log.Println("Placeholder: Pruning logic finished (simplified).")
}

// GetShardChain returns the blockchain for a specific shard.
func (bc *Blockchain) GetShardChain(shardID uint64) []*Block {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()

	chain, exists := bc.BlockChains[shardID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]*Block, len(chain))
	copy(result, chain)
	return result
}
