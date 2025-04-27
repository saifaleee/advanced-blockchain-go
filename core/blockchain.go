package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Blockchain manages the overall blockchain system, including shards and consensus.
type Blockchain struct {
	ShardManager       *ShardManager
	ValidatorManager   *ValidatorManager
	ConsistencyManager *ConsistencyOrchestrator // Correct type name
	Config             BlockchainConfig
	mu                 sync.RWMutex
	stopChan           chan struct{}       // Channel to signal stop
	wg                 sync.WaitGroup      // WaitGroup to manage goroutines
	BlockChains        map[uint64][]*Block // In-memory storage per shard chain - Use uint64 for shard ID consistency
	ChainMu            sync.RWMutex        // Lock specifically for accessing blockChains map
	ChainHeight        int                 // Approximate overall chain height (max shard height)
}

// BlockchainConfig holds configuration parameters for the blockchain.
type BlockchainConfig struct {
	NumValidators            int
	InitialReputation        int64
	PoWDifficulty            int
	TelemetryInterval        time.Duration
	PruneKeepBlocks          int           // Added config for pruning
	ConsistencyCheckInterval time.Duration // Added for Consistency Orchestrator
	// Add other relevant config fields if needed
}

// NewBlockchain creates a new Blockchain instance.
func NewBlockchain(numShards int, smConfig ShardManagerConfig, bcConfig BlockchainConfig, consConfig ConsistencyConfig) (*Blockchain, error) { // Added error return
	vm := NewValidatorManager()
	// Create Telemetry Monitor (needed for Consistency Orchestrator)
	telemetryMonitor := NewNetworkTelemetryMonitor(bcConfig.TelemetryInterval) // Fix argument for NewNetworkTelemetryMonitor

	cm := NewConsistencyOrchestrator(telemetryMonitor, consConfig, bcConfig.ConsistencyCheckInterval) // Pass monitor and interval

	// Initialize Validators (Phase 4)
	for i := 0; i < bcConfig.NumValidators; i++ {
		nodeID := fmt.Sprintf("Validator-%d", i)
		node, err := NewNode(NodeID(nodeID)) // Create node with keys, ensure NodeID type
		if err != nil {
			// Return error instead of fatal log
			return nil, fmt.Errorf("failed to create node %s: %w", nodeID, err)
		}
		// Simulate authentication for now
		node.Authenticate() // Mark as authenticated (Phase 4/8)

		vm.AddValidator(node, bcConfig.InitialReputation) // Fix argument for AddValidator
	}

	// Initialize Shard Manager
	sm, err := NewShardManager(smConfig, numShards) // Correct function call signature and handle error
	if err != nil {
		return nil, fmt.Errorf("failed to create shard manager: %w", err)
	}

	bc := &Blockchain{
		ShardManager:       sm, // Assign initialized ShardManager
		ValidatorManager:   vm,
		ConsistencyManager: cm, // Assign Consistency Manager
		Config:             bcConfig,
		stopChan:           make(chan struct{}),
		BlockChains:        make(map[uint64][]*Block), // Use uint64 key
	}
	return bc, nil // Return blockchain and nil error
}

// Start begins the blockchain's background processes (like shard management).
func (bc *Blockchain) Start() {
	// Start Consistency Manager monitoring
	bc.wg.Add(1)
	go bc.ConsistencyManager.Start() // Adjusted to match the method's definition
	// Start Shard Manager monitoring
	bc.wg.Add(1)
	go bc.ShardManager.StartManagementLoop() // Replace Start with StartManagementLoop
}

// Stop gracefully shuts down the blockchain processes.
func (bc *Blockchain) Stop() {
	close(bc.stopChan)
	bc.wg.Wait()
	log.Println("[Blockchain] All background processes stopped.")
}

// AddTransaction routes a transaction to the appropriate shard using the ShardManager.
func (bc *Blockchain) AddTransaction(tx *Transaction) error {
	if tx == nil || tx.ID == nil {
		return errors.New("cannot add nil or uninitialized transaction")
	}

	// Determine the target shard using the ShardManager's consistent hashing
	// Use the transaction ID as the key for routing.
	shardID, err := bc.ShardManager.DetermineShard(tx.ID)
	if err != nil {
		log.Printf("[Blockchain] Error determining shard for tx %x: %v", tx.ID, err)
		// Decide how to handle routing errors. Maybe route to a default shard or return error.
		// Returning error is safer for now.
		return fmt.Errorf("could not determine shard for transaction %x: %w", tx.ID, err)
	}

	// Get the shard using the determined ID
	shard, ok := bc.ShardManager.GetShard(shardID) // GetShard returns (*Shard, bool)
	if !ok {
		// This indicates an inconsistency, as DetermineShard should only return active shard IDs.
		log.Printf("[Blockchain] CRITICAL: DetermineShard routed tx %x to non-existent shard %d!", tx.ID, shardID)
		// Attempt fallback routing or return a critical error.
		// Fallback might involve re-determining or routing to shard 0 if it exists.
		// Returning error for now.
		return fmt.Errorf("determined shard %d for transaction %x does not exist in ShardManager", shardID, tx.ID)
	}

	log.Printf("[Blockchain] Routing transaction %x to shard %d", tx.ID, shardID)
	// Add the transaction to the determined shard's pool
	addErr := shard.AddTransaction(tx)
	if addErr != nil {
		log.Printf("[Blockchain] Error adding transaction %x to shard %d pool: %v", tx.ID, shardID, addErr)
		return fmt.Errorf("failed to add transaction %x to shard %d: %w", tx.ID, shardID, addErr)
	}

	return nil
}

// MineShardBlock coordinates the process of proposing and finalizing a block for a specific shard.
// Includes selecting a proposer (potentially via VRF) and handling cross-shard TX states.
func (bc *Blockchain) MineShardBlock(shardID uint64) (*Block, error) { // Removed proposerID argument
	shard, ok := bc.ShardManager.GetShard(shardID)
	if !ok {
		return nil, fmt.Errorf("could not get shard %d for mining", shardID)
	}

	// 1. Get Previous Block Info & Determine Next Height
	bc.ChainMu.RLock()
	chain, chainExists := bc.BlockChains[shardID]
	var prevBlockHash []byte
	var prevBlockHeight uint64 = 0
	var prevBlockVC VectorClock = make(VectorClock)
	if chainExists && len(chain) > 0 {
		lastBlock := chain[len(chain)-1]
		prevBlockHash = lastBlock.Hash
		prevBlockHeight = lastBlock.Header.Height
		prevBlockVC = lastBlock.Header.VectorClock
	} else {
		log.Printf("[Blockchain] Shard %d: No previous block found, preparing for first block (Height 1).", shardID)
		// Height 0 is conceptual genesis, first mined block is Height 1
	}
	bc.ChainMu.RUnlock()
	nextBlockHeight := prevBlockHeight + 1

	// 1.5 Select Proposer (Ticket 7 - VRF Integration Point)
	// Use VRF to select a proposer for this specific block height
	seed := prevBlockHash // Use prev block hash as part of the VRF seed
	vrf, err := NewSecureVRF()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize VRF: %w", err)
	}

	vrfInput := append(seed, byte(shardID), byte(nextBlockHeight))
	vrfOutput, err := vrf.Evaluate(vrfInput)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate VRF: %w", err)
	}

	proposerValidator, err := bc.ValidatorManager.SelectDelegateWithVRF(vrfOutput.Output, shardID, nextBlockHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to select proposer with VRF: %w", err)
	}

	proposerID := proposerValidator.Node.ID
	log.Printf("[Blockchain] Shard %d: Selected proposer %s for H:%d.", shardID, proposerID, nextBlockHeight)

	// 2. Get Transactions & Prepare for State Changes
	transactions := shard.GetTransactionsForBlock(10) // Assuming max 10 TXs per block

	// --- 2PC Prepare Phase ---
	preparedTxIDs := make(map[string]bool) // Track prepared TXs
	for _, tx := range transactions {
		if tx.Type == CrossShardInitiateTx {
			log.Printf("[2PC] Shard %d: Preparing CrossShardInitiateTx %x...", shardID, safeSlice(tx.ID, 4))
			tx.Status = "Prepared" // Mark as prepared
			preparedTxIDs[string(tx.ID)] = true
		} else if tx.Type == CrossShardFinalizeTx {
			log.Printf("[2PC] Shard %d: Checking status for CrossShardFinalizeTx %x...", shardID, safeSlice(tx.ID, 4))
			// Assume the corresponding Initiate TX is committed for now
		}
	}

	// 3. Apply State Changes
	finalStateRoot, err := shard.ApplyStateChanges(transactions)
	if err != nil {
		log.Printf("[Blockchain] Shard %d: Failed to apply state changes for H:%d: %v. Aborting mining.", shardID, nextBlockHeight, err)
		// --- 2PC Integration Point (Ticket 3) ---
		// If state application fails *after* preparing, need to trigger 'Abort' for prepared TXs.
		for txIDStr := range preparedTxIDs {
			log.Printf("[2PC Placeholder] Shard %d: Aborting prepared CrossShardInitiateTx %x due to state apply failure.", shardID, safeSlice([]byte(txIDStr), 4))
			// TODO: Implement 2PC Abort Logic (unlock resources, mark TX as Aborted)
		}
		// --- End 2PC Abort Phase ---
		return nil, fmt.Errorf("failed to apply state changes for shard %d H:%d: %w", shardID, nextBlockHeight, err)
	}

	// 4. Propose Block (Create Header, Run PoW)
	newBlock, err := ProposeBlock(shardID, transactions, prevBlockHash, nextBlockHeight, finalStateRoot, bc.Config.PoWDifficulty, proposerID, prevBlockVC)
	if err != nil {
		// Handle potential proposal failure (e.g., error during Merkle tree creation)
		log.Printf("[Blockchain] Shard %d: Failed to propose block H:%d: %v", shardID, nextBlockHeight, err)
		// --- 2PC Integration Point (Ticket 3) ---
		// If block proposal fails *after* preparing, need to trigger 'Abort'.
		for txIDStr := range preparedTxIDs {
			log.Printf("[2PC Placeholder] Shard %d: Aborting prepared CrossShardInitiateTx %x due to block proposal failure.", shardID, safeSlice([]byte(txIDStr), 4))
			// TODO: Implement 2PC Abort Logic
		}
		// --- End 2PC Abort Phase ---
		return nil, fmt.Errorf("failed to propose block for shard %d H:%d: %w", shardID, nextBlockHeight, err)
	}

	log.Printf("[Blockchain] Shard %d: Block H:%d proposed by %s. Hash: %x, Nonce: %d, StateRoot: %x",
		shardID, newBlock.Header.Height, proposerID, safeSlice(newBlock.Hash, 4), newBlock.Header.Nonce, safeSlice(newBlock.Header.StateRoot, 4))

	// 5. Finalize Block (dBFT)
	signedBlock, err := bc.ValidatorManager.FinalizeBlockDBFT(newBlock, shardID)
	if err != nil {
		log.Printf("[Blockchain] Shard %d: Block H:%d Hash:%x proposed by %s FAILED dBFT: %v",
			shardID, newBlock.Header.Height, safeSlice(newBlock.Hash, 4), proposerID, err)
		bc.ValidatorManager.UpdateReputation(proposerID, DbftConsensusPenalty)
		// --- 2PC Integration Point (Ticket 3) ---
		// If dBFT fails, the block is rejected. Trigger 'Abort' for prepared TXs.
		for txIDStr := range preparedTxIDs {
			log.Printf("[2PC Placeholder] Shard %d: Aborting prepared CrossShardInitiateTx %x due to dBFT failure.", shardID, safeSlice([]byte(txIDStr), 4))
			// TODO: Implement 2PC Abort Logic
		}
		// --- End 2PC Abort Phase ---
		return nil, fmt.Errorf("block finalization failed for shard %d H:%d: %w", shardID, newBlock.Header.Height, err)
	}

	finalizedBlock := signedBlock.Block

	// --- 2PC Commit Phase ---
	for _, tx := range finalizedBlock.Transactions {
		if tx.Type == CrossShardInitiateTx && tx.Status == "Prepared" {
			log.Printf("[2PC] Shard %d: Committing CrossShardInitiateTx %x...", shardID, safeSlice(tx.ID, 4))
			tx.Status = "Committed" // Mark as committed
			// Generate commit proof (e.g., Merkle proof)
			proof, _, err := finalizedBlock.GetTransactionMerkleProof(tx.ID)
			if err == nil {
				serializedProof := bytes.Join(proof, []byte{}) // Serialize [][]byte into []byte
				tx.CommitProof = serializedProof
			}
		} else if tx.Type == CrossShardFinalizeTx {
			log.Printf("[2PC] Shard %d: Finalized block includes CrossShardFinalizeTx %x.", shardID, safeSlice(tx.ID, 4))
			tx.Status = "Finalized"
		}
	}

	// 6. Add Finalized Block to Shard Chain & Update Metrics
	bc.ChainMu.Lock()
	if _, exists := bc.BlockChains[shardID]; !exists {
		bc.BlockChains[shardID] = make([]*Block, 0)
		log.Printf("[Blockchain] Initialized chain storage for new shard %d.", shardID)
	}
	// Basic validation: check parent linkage
	currentChain := bc.BlockChains[shardID] // Get chain again after acquiring lock
	isFirstBlock := finalizedBlock.Header.Height == 1

	parentMismatch := false
	if isFirstBlock {
		if len(finalizedBlock.Header.PrevBlockHash) != 0 {
			parentMismatch = true
			log.Printf("[Blockchain] CRITICAL: Shard %d: First block H:1 has non-nil PrevBlockHash (%x). Discarding.",
				shardID, safeSlice(finalizedBlock.Header.PrevBlockHash, 4))
		}
		if len(currentChain) != 0 {
			parentMismatch = true
			log.Printf("[Blockchain] CRITICAL: Shard %d: Attempting to add first block H:1 but chain map is not empty (len:%d). Discarding.",
				shardID, len(currentChain))
		}
	} else { // Block height > 1
		if len(currentChain) == 0 {
			parentMismatch = true
			log.Printf("[Blockchain] CRITICAL: Shard %d: Proposed block H:%d (Prev: %x) has no parent in empty chain map (expected H:%d). Discarding.",
				shardID, finalizedBlock.Header.Height, safeSlice(finalizedBlock.Header.PrevBlockHash, 4), finalizedBlock.Header.Height-1)
		} else if !bytes.Equal(currentChain[len(currentChain)-1].Hash, finalizedBlock.Header.PrevBlockHash) {
			parentMismatch = true
			log.Printf("[Blockchain] CRITICAL: Shard %d: Proposed block H:%d (Prev: %x) does not link to current chain head (H:%d, Hash:%x). Discarding.",
				shardID, finalizedBlock.Header.Height, safeSlice(finalizedBlock.Header.PrevBlockHash, 4),
				currentChain[len(currentChain)-1].Header.Height, safeSlice(currentChain[len(currentChain)-1].Hash, 4))
		}
	}

	if parentMismatch {
		bc.ChainMu.Unlock()
		// --- 2PC Integration Point (Ticket 3) ---
		// If block is discarded due to linkage error *after* commit phase logic,
		// this is a critical state inconsistency. May need manual intervention or complex rollback.
		// For now, just log the error.
		log.Printf("[2PC CRITICAL] Shard %d: Block H:%d discarded due to linkage error after 2PC commit logic executed!", shardID, finalizedBlock.Header.Height)
		// --- End 2PC Critical Note ---
		return nil, fmt.Errorf("proposed block H:%d for shard %d does not link correctly to the chain", finalizedBlock.Header.Height, shardID)
	}

	bc.BlockChains[shardID] = append(bc.BlockChains[shardID], finalizedBlock)
	chainLen := len(bc.BlockChains[shardID])
	bc.ChainMu.Unlock()

	// Update shard metrics
	shard.Metrics.BlockCount.Add(1)

	log.Printf("[Blockchain] Shard %d: Block H:%d Hash:%x successfully mined and added to chain (Chain Length: %d).",
		shardID, finalizedBlock.Header.Height, safeSlice(finalizedBlock.Hash, 4), chainLen)

	// Reward proposer for successful block
	bc.ValidatorManager.UpdateReputation(proposerID, DbftConsensusReward)

	// 7. Prune old state/blocks if necessary
	bc.PruneChain(shardID, bc.Config.PruneKeepBlocks)

	// Update overall chain height (simple max height tracking)
	bc.mu.Lock()
	if int(finalizedBlock.Header.Height) > bc.ChainHeight {
		bc.ChainHeight = int(finalizedBlock.Header.Height)
	}
	bc.mu.Unlock()

	return finalizedBlock, nil
}

// GetBlock retrieves a block from a specific shard's chain.
func (bc *Blockchain) GetBlock(shardID uint64, blockHash []byte) (*Block, error) {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()

	chain, ok := bc.BlockChains[shardID]
	if !ok {
		return nil, fmt.Errorf("shard %d chain not found", shardID)
	}

	for i := len(chain) - 1; i >= 0; i-- { // Search backwards for efficiency
		if bytes.Equal(chain[i].Hash, blockHash) {
			return chain[i], nil
		}
	}

	return nil, fmt.Errorf("block %x not found in shard %d chain", blockHash, shardID)
}

// GetLatestBlock retrieves the latest block from a specific shard's chain.
func (bc *Blockchain) GetLatestBlock(shardID uint64) (*Block, error) {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()

	chain, ok := bc.BlockChains[shardID]
	if !ok || len(chain) == 0 {
		return nil, fmt.Errorf("shard %d chain not found or is empty", shardID)
	}
	return chain[len(chain)-1], nil
}

// GetState retrieves a value from the state DB of a specific shard.
func (bc *Blockchain) GetState(shardID uint64, key string) ([]byte, error) {
	shard, ok := bc.ShardManager.GetShard(shardID)
	if !ok {
		return nil, fmt.Errorf("could not get shard %d to retrieve state", shardID)
	}
	// Use StateDB field
	value, err := shard.StateDB.Get(key) // Pass key as string
	if err != nil {
		// Don't wrap "key not found" errors necessarily, depends on desired behavior
		// return nil, fmt.Errorf("failed to get key '%s' from shard %d state: %w", key, shardID, err)
		return nil, err // Return original error (e.g., key not found)
	}
	return value, nil
}

// PruneChain removes old blocks from a specific shard's chain in memory.
func (bc *Blockchain) PruneChain(shardID uint64, minBlocksToKeep int) {
	if minBlocksToKeep <= 0 {
		return // Pruning disabled or invalid config
	}

	bc.ChainMu.Lock()
	defer bc.ChainMu.Unlock()

	chain, ok := bc.BlockChains[shardID]
	if !ok {
		// log.Printf("[Prune] Shard %d: Chain not found, skipping.", shardID)
		return
	}

	currentLen := len(chain)
	if currentLen <= minBlocksToKeep {
		// log.Printf("[Prune] Shard %d: Chain length %d <= %d, no pruning needed.", shardID, currentLen, minBlocksToKeep)
		return // Not enough blocks to prune
	}

	prunedCount := currentLen - minBlocksToKeep

	// Add state archival logic during pruning
	archivedStates := make(map[uint64][]byte) // Map of block height to state root
	for i := 0; i < prunedCount; i++ {
		block := chain[i]
		archivedStates[block.Header.Height] = block.Header.StateRoot
		log.Printf("[Archive] Archived state for Block H:%d with StateRoot: %x", block.Header.Height, safeSlice(block.Header.StateRoot, 4))
	}
	// TODO: Persist archivedStates to external storage (e.g., database, file system)

	bc.BlockChains[shardID] = chain[prunedCount:] // Keep the last 'minBlocksToKeep' blocks

	newLen := len(bc.BlockChains[shardID])
	firstBlockHeight := uint64(0)
	lastBlockHeight := uint64(0)
	if newLen > 0 {
		firstBlockHeight = bc.BlockChains[shardID][0].Header.Height
		lastBlockHeight = bc.BlockChains[shardID][newLen-1].Header.Height
	}

	log.Printf("[Prune] Shard %d: Pruned %d blocks. New length: %d (Keeping H:%d to H:%d).",
		shardID, prunedCount, newLen, firstBlockHeight, lastBlockHeight)

	// TODO: State pruning would need to happen here too, based on the oldest kept block's state root.
}

// DisplayReputations logs the current reputation scores of all validators.
func (bc *Blockchain) DisplayReputations() {
	// Check consensus.go content - assuming LogReputations exists
	bc.ValidatorManager.LogReputations() // Changed from DisplayReputations
}

// Helper function to safely slice byte arrays for logging
func safeSlice(data []byte, length int) []byte {
	if len(data) < length {
		return data
	}
	return data[:length]
}
