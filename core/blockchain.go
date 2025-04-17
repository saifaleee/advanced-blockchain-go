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
	ShardManager *ShardManager        // Manages all shards
	Difficulty   int                  // Global PoW difficulty (can be per-shard later)
	ChainHeight  int                  // Approximate overall chain height (e.g., max shard height) - needs refinement
	Mu           sync.RWMutex         // Protects global state like ChainHeight
	BlockChains  map[ShardID][]*Block // In-memory storage per shard chain
	ChainMu      sync.RWMutex         // Lock specifically for accessing blockChains map
}

// NewBlockchain creates a new sharded blockchain.
func NewBlockchain(initialShardCount uint, difficulty int) (*Blockchain, error) {
	if difficulty < 1 {
		return nil, errors.New("difficulty must be at least 1")
	}
	if initialShardCount == 0 {
		return nil, errors.New("initial shard count must be positive")
	}

	sm, err := NewShardManager(initialShardCount)
	if err != nil {
		return nil, fmt.Errorf("failed to create shard manager: %w", err)
	}

	bc := &Blockchain{
		ShardManager: sm,
		Difficulty:   difficulty,
		ChainHeight:  0, // Initial height is 0
		BlockChains:  make(map[ShardID][]*Block),
	}

	// Create genesis block for each shard
	bc.ChainMu.Lock() // Lock before modifying blockChains map
	defer bc.ChainMu.Unlock()
	for shardID := range sm.Shards {
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
		return bc.ShardManager.RouteTransaction(tx)
	}
}

// MineShardBlock attempts to mine a new block for a specific shard using transactions from its pool.
func (bc *Blockchain) MineShardBlock(shardID ShardID) (*Block, error) {
	shard, ok := bc.ShardManager.GetShard(shardID)
	if !ok {
		return nil, fmt.Errorf("shard %d not found", shardID)
	}

	// Define max transactions per block (could be configurable)
	maxTxPerBlock := 10
	txs := shard.GetTransactionsForBlock(maxTxPerBlock)

	// Handle Cross-Shard Transactions (Simplified)
	// In a real system, need coordination, proofs, etc.
	var receiptsToProcess []*CrossShardReceipt
	var transactionsForBlock []*Transaction
	for _, tx := range txs {
		if tx.Type == CrossShardTxInit && tx.SourceShard != nil && tx.DestinationShard != nil {
			// Process on source shard:
			// 1. Validate transaction against source shard state
			// 2. Deduct funds/lock state on source shard (using shard.State)
			// 3. Generate a receipt to be sent to the destination shard
			log.Printf("Shard %d: Processing cross-shard init Tx %x for dest %d", shardID, tx.ID, *tx.DestinationShard)
			// Placeholder for state update: shard.State.Put(...)
			// Create receipt but skip processing for now
			// The receipt is left here for future implementation
			_ = &CrossShardReceipt{
				OriginShard:      *tx.SourceShard,
				DestinationShard: *tx.DestinationShard,
				TransactionID:    tx.ID,
				// Add proof data here in a real system
			}
			// How to route the receipt? Could be put into a special pool or broadcast.
			// For simplicity, let's assume another process picks this up.
			log.Printf("Shard %d: Generated receipt for Tx %x", shardID, tx.ID)
			// Include the initiating tx in the block
			transactionsForBlock = append(transactionsForBlock, tx)
		} else if tx.Type == CrossShardTxFinalize {
			// This tx type would likely be generated *in response* to a receipt,
			// not pulled directly from a generic pool like this.
			// Placeholder: Requires receipt handling mechanism first.
			log.Printf("Shard %d: Encountered cross-shard finalize Tx %x (requires receipt processing)", shardID, tx.ID)
			// Skip adding for now, or handle based on included receipt data (not implemented)
		} else {
			// Intra-shard transaction
			// Placeholder for applying tx to shard state: shard.State.Put/Get...
			transactionsForBlock = append(transactionsForBlock, tx)
		}
	}

	// Process incoming receipts (if a mechanism delivered them to receiptsToProcess)
	for _, receipt := range receiptsToProcess {
		err := shard.ProcessCrossShardReceipt(receipt)
		if err != nil {
			log.Printf("Shard %d: Error processing receipt for Tx %x: %v", shardID, receipt.TransactionID, err)
			// Handle error - potentially revert state changes from receipt processing
		}
	}

	if len(transactionsForBlock) == 0 {
		// Optionally allow empty blocks or return specific status
		// log.Printf("Shard %d: No transactions to mine.", shardID)
		// return nil, errors.New("no transactions available for mining")
		// Let's allow empty blocks for simplicity now
	}

	// Get the latest block *for this shard*
	bc.ChainMu.Lock() // Lock map access
	shardChain, chainOk := bc.BlockChains[shardID]
	if !chainOk || len(shardChain) == 0 {
		bc.ChainMu.Unlock()
		// Should not happen if genesis blocks were created correctly
		return nil, fmt.Errorf("shard %d chain not found or is empty", shardID)
	}
	prevBlock := shardChain[len(shardChain)-1]
	bc.ChainMu.Unlock() // Unlock after reading

	newHeight := prevBlock.Height + 1
	newBlock := NewBlock(shardID, transactionsForBlock, prevBlock.Hash, newHeight, bc.Difficulty)

	// Basic validation before adding (already done implicitly in AddBlock logic below)
	pow := NewProofOfWork(newBlock, bc.Difficulty)
	if !pow.Validate() {
		return nil, fmt.Errorf("shard %d: mined block %d failed proof-of-work validation", shardID, newHeight)
	}
	if !ValidateBlockIntegrity(newBlock, prevBlock) { // Use existing validator
		return nil, fmt.Errorf("shard %d: mined block %d failed integrity validation against previous block", shardID, newHeight)
	}

	// Add the successfully mined block to the shard's chain
	bc.ChainMu.Lock() // Lock map access
	bc.BlockChains[shardID] = append(bc.BlockChains[shardID], newBlock)
	currentHeight := newBlock.Height
	bc.ChainMu.Unlock() // Unlock map access

	bc.Mu.Lock() // Lock global state
	if currentHeight > bc.ChainHeight {
		bc.ChainHeight = currentHeight // Update global height if this shard is tallest
	}
	bc.Mu.Unlock() // Unlock global state

	log.Printf("Shard %d: Added Block %d to chain.\n", shardID, newHeight)
	return newBlock, nil
}

// GetLatestBlock returns the most recent block for a specific shard.
func (bc *Blockchain) GetLatestBlock(shardID ShardID) (*Block, error) {
	bc.ChainMu.Lock() // Lock map access
	defer bc.ChainMu.Unlock()

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
	bc.ChainMu.Lock() // Lock map access
	defer bc.ChainMu.Unlock()

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
func ValidateBlockIntegrity(newBlock, prevBlock *Block) bool { /* ... unchanged ... */
	if newBlock == nil || prevBlock == nil {
		log.Println("Error: Cannot validate nil blocks.")
		return false
	}
	if newBlock.ShardID != prevBlock.ShardID {
		log.Printf("Validation Error: Block %d ShardID (%d) does not match Prev Block %d ShardID (%d)\n",
			newBlock.Height, newBlock.ShardID, prevBlock.Height, prevBlock.ShardID)
		return false
	}
	if !bytes.Equal(newBlock.PrevBlockHash, prevBlock.Hash) {
		log.Printf("Validation Error: Shard %d Block %d PrevBlockHash (%x) does not match Block %d Hash (%x)\n",
			newBlock.ShardID, newBlock.Height, newBlock.PrevBlockHash, prevBlock.Height, prevBlock.Hash)
		return false
	}
	if newBlock.Height != prevBlock.Height+1 {
		log.Printf("Validation Error: Shard %d Block %d Height (%d) is not sequential to previous block height %d",
			newBlock.ShardID, newBlock.ShardID, newBlock.Height, prevBlock.Height)
		return false
	}
	// log.Printf("Shard %d: Block %d integrity validated successfully against Block %d.\n", newBlock.ShardID, newBlock.Height, prevBlock.Height)
	return true
}

// IsChainValid now validates each shard's chain independently.
func (bc *Blockchain) IsChainValid() bool {
	bc.ChainMu.Lock() // Lock map access
	defer bc.ChainMu.Unlock()
	overallValid := true

	var wg sync.WaitGroup
	validationErrors := make(chan error, len(bc.BlockChains)) // Channel for errors

	for shardID, chain := range bc.BlockChains {
		wg.Add(1)
		go func(sID ShardID, ch []*Block) {
			defer wg.Done()
			log.Printf("Validating chain for Shard %d...", sID)
			if len(ch) == 0 {
				log.Printf("Shard %d chain is empty, skipping validation.", sID)
				return // Skip empty chains (shouldn't happen with genesis)
			}
			if len(ch) == 1 {
				genesis := ch[0]
				powGenesis := NewProofOfWork(genesis, bc.Difficulty)
				if !powGenesis.Validate() {
					err := fmt.Errorf("shard %d genesis block PoW validation failed", sID)
					log.Println(err)
					validationErrors <- err
					return
				}
				log.Printf("Shard %d Genesis block validated.", sID)
				return
			}

			for i := 1; i < len(ch); i++ {
				currentBlock := ch[i]
				prevBlock := ch[i-1]

				pow := NewProofOfWork(currentBlock, bc.Difficulty)
				if !pow.Validate() {
					err := fmt.Errorf("shard %d PoW validation failed for Block %d (Hash: %x)", sID, currentBlock.Height, currentBlock.Hash)
					log.Println(err)
					validationErrors <- err
					return // Stop validation for this shard on first error
				}

				if !ValidateBlockIntegrity(currentBlock, prevBlock) {
					err := fmt.Errorf("shard %d integrity validation failed between Block %d and Block %d", sID, currentBlock.Height, prevBlock.Height)
					log.Println(err)
					validationErrors <- err
					return // Stop validation for this shard
				}
			}
			log.Printf("Shard %d chain validation successful.", sID)
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
	bc.ChainMu.Lock() // Lock map access
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
			bc.BlockChains[shardID] = chain[pruneHeight:]
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
