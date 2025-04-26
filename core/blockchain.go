package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
	// Add import for NodeID if not already present implicitly
)

// Blockchain now manages multiple shards and includes consensus components.
type Blockchain struct {
	ShardManager    *ShardManager
	ValidatorMgr    *ValidatorManager        // Added Validator Manager
	Consistency     *ConsistencyOrchestrator // <<< ADDED for Ticket 4
	Difficulty      int                      // Global PoW difficulty (can be per-shard later)
	ChainHeight     int                      // Approximate overall chain height (max shard height)
	Mu              sync.RWMutex             // Protects global state like ChainHeight
	BlockChains     map[uint64][]*Block      // In-memory storage per shard chain
	ChainMu         sync.RWMutex             // Lock specifically for accessing blockChains map
	LocalNodeID     NodeID                   // ID of the node running this blockchain instance (for proposing)
	GenesisProposer NodeID                   // ID used for genesis blocks
}

// NewBlockchain creates a new sharded blockchain with consensus mechanisms.
func NewBlockchain(initialShardCount uint, config ShardManagerConfig, validatorMgr *ValidatorManager, consistency *ConsistencyOrchestrator, localNodeID NodeID) (*Blockchain, error) {
	difficulty := 16 // Default difficulty, adjust as needed
	if difficulty < 1 {
		return nil, errors.New("difficulty must be at least 1")
	}
	if validatorMgr == nil {
		return nil, errors.New("validator manager cannot be nil")
	}
	if localNodeID == "" {
		log.Println("Warning: Local node ID not set for blockchain instance.")
		// Assign a default or generate one? For simulation, allow empty but log warning.
	}

	// Add nil check for consistency orchestrator:
	if consistency == nil {
		return nil, errors.New("consistency orchestrator cannot be nil")
	}
	if validatorMgr == nil { // Keep existing checks too
		return nil, errors.New("validator manager cannot be nil")
	}
	if localNodeID == "" {
		log.Println("Warning: Local node ID not set for blockchain instance.")
	}

	sm, err := NewShardManager(config, int(initialShardCount))
	if err != nil {
		return nil, fmt.Errorf("failed to create shard manager: %w", err)
	}

	// Initialize Blockchain struct - Add Consistency field:
	bc := &Blockchain{
		ShardManager:    sm,
		ValidatorMgr:    validatorMgr,
		Consistency:     consistency, // <<< ADDED Assignment
		Difficulty:      difficulty,
		ChainHeight:     0,
		BlockChains:     make(map[uint64][]*Block),
		LocalNodeID:     localNodeID,
		GenesisProposer: NodeID("GENESIS_" + localNodeID),
	}

	// Link ShardManager back to Blockchain
	sm.SetBlockchainLink(bc)

	// --- Explicitly manage lock and add checks/logging for genesis creation ---
	bc.ChainMu.Lock() // Lock before modifying the map

	shardIDs := sm.GetAllShardIDs()
	log.Printf("[NewBlockchain] Starting genesis block creation for %d shards: %v", len(shardIDs), shardIDs)

	for _, shardID := range shardIDs {
		log.Printf("[NewBlockchain] Creating genesis for Shard %d...", shardID)
		genesis := NewGenesisBlock(shardID, nil, bc.Difficulty, bc.GenesisProposer)

		// Defensive check: Ensure genesis block was actually created
		if genesis == nil {
			log.Printf("[NewBlockchain] CRITICAL ERROR: NewGenesisBlock returned nil for Shard %d", shardID)
			bc.ChainMu.Unlock() // Unlock before returning error
			return nil, fmt.Errorf("genesis block creation failed (returned nil) for shard %d", shardID)
		}

		// Assign the genesis block to the map
		bc.BlockChains[shardID] = []*Block{genesis}

		// Add log to confirm assignment and length immediately after
		log.Printf("[NewBlockchain] Assigned genesis to bc.BlockChains[%d]. Map entry length: %d", shardID, len(bc.BlockChains[shardID]))
	}

	log.Printf("[NewBlockchain] Finished genesis block loop. Releasing lock.")
	bc.ChainMu.Unlock() // Unlock *after* the loop finishes, before returning
	// --------------------------------------------------------------------

	log.Printf("[NewBlockchain] Initialization complete. Returning Blockchain instance.")
	return bc, nil
}

// AddTransaction - Routing logic remains mostly the same.
// dBFT interaction happens during block creation, not transaction submission.
func (bc *Blockchain) AddTransaction(tx *Transaction) error {
	// Route CrossShardTxInit to source shard (if specified)
	if tx.Type == CrossShardTxInit && tx.SourceShard != nil {
		shardID := *tx.SourceShard
		shard, ok := bc.ShardManager.GetShard(shardID)
		if !ok {
			// Fallback routing if source shard doesn't exist (e.g., after merge)
			fallbackShardID, err := bc.ShardManager.DetermineShard(tx.ID)
			if err != nil {
				return fmt.Errorf("source shard %d not found and failed to determine fallback for tx %x: %w", shardID, tx.ID, err)
			}
			shard, ok = bc.ShardManager.GetShard(fallbackShardID)
			if !ok {
				return fmt.Errorf("source shard %d not found and fallback shard %d also not found for tx %x", shardID, fallbackShardID, tx.ID)
			}
			log.Printf("Warning: Source shard %d not found for tx %x, routing to fallback shard %d", shardID, tx.ID, fallbackShardID)
			// Update the transaction's source shard field? Or let processor handle it? Let processor handle.
		}
		return shard.AddTransaction(tx)
	}

	// Route other transactions based on consistent hashing of TX ID
	targetShardID, err := bc.ShardManager.DetermineShard(tx.ID)
	if err != nil {
		return fmt.Errorf("failed to determine shard for tx %x: %w", tx.ID, err)
	}
	shard, ok := bc.ShardManager.GetShard(targetShardID)
	if !ok {
		// Add fallback mechanism if shard disappears between determination and get
		log.Printf("Warning: Target shard %d determined for tx %x but not found (race condition?). Retrying determination.", targetShardID, tx.ID)
		time.Sleep(50 * time.Millisecond) // Small delay
		retryShardID, retryErr := bc.ShardManager.DetermineShard(tx.ID)
		if retryErr != nil {
			return fmt.Errorf("failed to determine shard for tx %x on retry: %w", tx.ID, retryErr)
		}
		shard, ok = bc.ShardManager.GetShard(retryShardID)
		if !ok {
			return fmt.Errorf("determined shard %d (retry %d) not found for tx %x", targetShardID, retryShardID, tx.ID)
		}
		log.Printf("Successfully routed tx %x to shard %d on retry.", tx.ID, retryShardID)
		targetShardID = retryShardID // Update targetShardID for logging consistency if needed
	}

	// Add transaction to the determined shard's pool
	addErr := shard.AddTransaction(tx)
	if addErr != nil {
		log.Printf("Error adding TX %x to shard %d pool: %v", tx.ID, targetShardID, addErr)
		return addErr
	}
	// log.Printf("Routed TX %x (Type: %d) to shard %d", tx.ID, tx.Type, targetShardID)
	return nil
}

// MineShardBlock performs the hybrid PoW/dBFT consensus process for a shard.
func (bc *Blockchain) MineShardBlock(shardID uint64) (*Block, error) {
	shard, ok := bc.ShardManager.GetShard(shardID)
	if !ok {
		return nil, fmt.Errorf("shard %d not found", shardID)
	}

	// --- 1. Get Proposer Information ---
	proposerNodeID := bc.LocalNodeID // Assume this node instance is the proposer
	if proposerNodeID == "" {
		return nil, errors.New("cannot propose block: local node ID is not set")
	}
	proposer, ok := bc.ValidatorMgr.GetValidator(proposerNodeID)
	if !ok {
		// If the local node isn't a validator, it shouldn't propose blocks in dBFT.
		// For simulation, allow non-validators to propose PoW, but they might fail dBFT.
		log.Printf("Warning: Node %s proposing for shard %d is not a registered validator.", proposerNodeID, shardID)
		// Create a temporary validator struct for reputation tracking if needed, or disallow.
		// Let's disallow for stricter simulation:
		// return nil, fmt.Errorf("node %s cannot propose block for shard %d: not a validator", proposerNodeID, shardID)
		// OR allow but create a temporary validator for simulation:
		proposer = &Validator{Node: &Node{ID: proposerNodeID}} // Temporary, won't be in manager
		proposer.Node.Authenticate()                           // Assume authenticated for simulation
	}

	// --- 2. Select Transactions & Handle Cross-Shard ---
	maxTxPerBlock := 10
	txsRaw := shard.GetTransactionsForBlock(maxTxPerBlock)
	transactionsForProposal := make([]*Transaction, 0, len(txsRaw))

	// Process raw transactions, handle cross-shard init, create final list
	for _, tx := range txsRaw {
		if tx.Type == CrossShardTxInit && tx.SourceShard != nil && tx.DestinationShard != nil {
			// Check if initiating shard matches current shard
			if *tx.SourceShard != shardID {
				log.Printf("Error: CrossShardTxInit %x found in shard %d, but SourceShard field is %d. Re-routing or discarding.", tx.ID, shardID, *tx.SourceShard)
				// Option 1: Discard (simplest)
				continue
				// Option 2: Try to re-route (adds complexity)
				// bc.AddTransaction(tx) // Could cause loops if routing is flawed
				// continue
			}

			log.Printf("Shard %d: Processing cross-shard init Tx %x for dest %d", shardID, tx.ID, *tx.DestinationShard)
			// Create finalize TX and route it to destination shard
			receipt := &CrossShardReceipt{ /* ... fields ... */
				SourceShard:      *tx.SourceShard,
				DestinationShard: *tx.DestinationShard,
				TransactionID:    tx.ID,
				Data:             tx.Data,
			}
			finalizeTx := NewTransaction(receipt.Data, CrossShardTxFinalize, &receipt.DestinationShard)
			finalizeTx.SourceShard = &receipt.SourceShard
			finalizeTx.SourceReceiptProof = &ReceiptProof{MerkleProof: [][]byte{receipt.TransactionID}} // Placeholder proof

			destShard, destOk := bc.ShardManager.GetShard(receipt.DestinationShard)
			if destOk {
				err := destShard.AddTransaction(finalizeTx) // Route finalization TX
				if err != nil {
					log.Printf("Shard %d: Error routing cross-shard finalize tx %x to shard %d: %v", shardID, finalizeTx.ID, receipt.DestinationShard, err)
					// If routing fails, should we abort including the init tx? Or proceed?
					// Proceed for now, but log error. Destination state won't be updated.
				} else {
					log.Printf("Shard %d: Routed finalize tx %x for %x to dest shard %d", shardID, finalizeTx.ID, tx.ID, receipt.DestinationShard)
				}
			} else {
				log.Printf("Shard %d: Cannot route finalize tx %x for %x, dest shard %d not found", shardID, finalizeTx.ID, tx.ID, receipt.DestinationShard)
				// Abort including the init tx if destination doesn't exist? Safer.
				log.Printf("Shard %d: Discarding cross-shard init Tx %x due to missing destination shard %d", shardID, tx.ID, *tx.DestinationShard)
				continue // Skip adding this init tx to the block
			}
			// Include the initiating tx in the block proposal
			transactionsForProposal = append(transactionsForProposal, tx)

		} else if tx.Type == CrossShardTxFinalize {
			// Check if destination shard matches current shard
			if tx.DestinationShard == nil || *tx.DestinationShard != shardID {
				log.Printf("Error: CrossShardTxFinalize %x found in shard %d, but DestinationShard field is missing or incorrect (%v). Discarding.", tx.ID, shardID, tx.DestinationShard)
				continue
			}
			log.Printf("Shard %d: Including cross-shard finalize Tx %x", shardID, tx.ID)
			transactionsForProposal = append(transactionsForProposal, tx)
		} else if tx.Type == IntraShard {
			// Standard intra-shard transaction
			transactionsForProposal = append(transactionsForProposal, tx)
		} else {
			log.Printf("Shard %d: Skipping transaction %x with unknown type %d", shardID, tx.ID, tx.Type)
		}
	}

	// Allow empty blocks? Let's require at least one TX for now unless it's the genesis block.
	// if len(transactionsForProposal) == 0 {
	//  log.Printf("Shard %d: No valid transactions selected for block proposal.", shardID)
	//  return nil, errors.New("no transactions to mine")
	// }

	// --- 3. Get Previous Block and Apply State ---
	bc.ChainMu.RLock()
	shardChain, chainOk := bc.BlockChains[shardID]
	bc.ChainMu.RUnlock()

	if !chainOk || len(shardChain) == 0 {
		// Attempt to initialize if missing (e.g., for dynamically added shards)
		err := bc.initializeShardChain(shardID)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize chain for shard %d: %w", shardID, err)
		}
		bc.ChainMu.RLock()
		shardChain, chainOk = bc.BlockChains[shardID]
		bc.ChainMu.RUnlock()
		if !chainOk || len(shardChain) == 0 { // Check again after init attempt
			return nil, fmt.Errorf("shard %d chain initialization failed or chain empty", shardID)
		}
	}

	bc.ChainMu.RLock()
	prevBlock := shardChain[len(shardChain)-1]
	bc.ChainMu.RUnlock()
	newHeight := prevBlock.Header.Height + 1

	// Apply state changes *before* proposing block to get the correct StateRoot
	stateRoot, err := shard.ApplyStateChanges(transactionsForProposal)
	if err != nil {
		// If state application fails, we cannot propose the block.
		// TODO: Need to roll back any partial state changes if StateDB supports it.
		// For InMemoryStateDB, changes are likely committed. This is a limitation.
		log.Printf("CRITICAL: Shard %d failed to apply state changes for potential block %d: %v. Transactions potentially lost state.", shardID, newHeight, err)
		// Return the transactions to the pool? Requires careful handling.
		// For now, fail the mining attempt.
		return nil, fmt.Errorf("failed to apply state changes for shard %d, block %d: %w", shardID, newHeight, err)
	}

	// --- 4. Propose Block (PoW) ---
	proposedBlock, err := ProposeBlock(shardID, transactionsForProposal, prevBlock.Hash, newHeight, stateRoot, bc.Difficulty, proposerNodeID)
	if err != nil {
		log.Printf("Shard %d: Failed to propose block H:%d: %v", shardID, newHeight, err)
		// If proposing fails (e.g., PoW timeout), need to decide how to handle txs.
		// Re-add txs to pool? For simulation, just fail.
		return nil, fmt.Errorf("block proposal failed for shard %d: %w", shardID, err)
	}

	// Inside func (bc *Blockchain) MineShardBlock(shardID uint64) (*Block, error)

	// ... (after block proposal, before consensus) ...

	// --- 5. Finalize Block (Simulated dBFT with Adaptive Consistency) ---
	currentConsistencyLevel := bc.Consistency.GetCurrentLevel() // <<< GET Current Level
	log.Printf("Shard %d: Running consensus with %s consistency level.", shardID, currentConsistencyLevel)

	// Modify the SimulateDBFT call:
	consensusReached, agreeingValidators := bc.ValidatorMgr.SimulateDBFT(
		proposedBlock,
		proposer,
		currentConsistencyLevel, // <<< PASS Level
		bc.Consistency,          // <<< PASS Orchestrator (for timeouts etc.)
	)

	if !consensusReached {
		log.Printf("Shard %d: Consensus failed for proposed block H:%d under %s consistency. Discarding block.",
			shardID, newHeight, currentConsistencyLevel) // Log level
		// TODO: Handle transactions from failed block (e.g., return to pool)
		// For now, they are effectively dropped.
		return nil, fmt.Errorf("dBFT consensus failed (%s level)", currentConsistencyLevel) // Include level in error
	}

	// --- 6. Mark Block as Final & Validate ---
	proposedBlock.Finalize(agreeingValidators)

	// Perform final validation checks (including new fields)
	if !ValidateBlockIntegrity(proposedBlock, prevBlock, bc.ValidatorMgr) { // Pass ValidatorMgr
		log.Printf("Shard %d: CRITICAL - Finalized block H:%d failed integrity validation!", shardID, newHeight)
		// This should ideally not happen if dBFT simulation was correct, but check anyway.
		// Need robust handling here (panic? halt shard?)
		return nil, fmt.Errorf("finalized block %d failed integrity check", newHeight)
	}
	// PoW validation was implicitly checked during dBFT simulation, but can re-check.
	// powCheck := NewProofOfWork(proposedBlock)
	// if !powCheck.Validate() {
	//  return nil, fmt.Errorf("finalized block %d failed PoW check", newHeight)
	// }

	// --- 7. Add Finalized Block to Chain ---
	bc.ChainMu.Lock()
	bc.BlockChains[shardID] = append(bc.BlockChains[shardID], proposedBlock)
	currentHeight := proposedBlock.Header.Height
	bc.ChainMu.Unlock()

	// Update global chain height metric
	bc.Mu.Lock()
	if int(currentHeight) > bc.ChainHeight {
		bc.ChainHeight = int(currentHeight)
	}
	bc.Mu.Unlock()

	// Update shard metrics (e.g., block count)
	shard.Metrics.BlockCount.Add(1)

	log.Printf("Shard %d: ===> Successfully Proposed, Finalized, and Added Block H:%d. Hash: %x... <===",
		shardID, newHeight, safeSlice(proposedBlock.Hash, 4))

	return proposedBlock, nil
}

// initializeShardChain creates a genesis block for a shard and initializes its blockchain
// Uses the Blockchain's GenesisProposer ID.
func (bc *Blockchain) initializeShardChain(shardID uint64) error {
	// Check if the shard exists in the shard manager
	if _, ok := bc.ShardManager.GetShard(shardID); !ok {
		return fmt.Errorf("cannot initialize chain: shard %d not found in shard manager", shardID)
	}

	bc.ChainMu.Lock() // Lock for potential map modification
	defer bc.ChainMu.Unlock()

	// Double-check if chain was created between the initial check and acquiring the lock
	if _, exists := bc.BlockChains[shardID]; exists {
		return nil // Chain already initialized
	}

	// Create genesis block for the shard using the designated proposer
	log.Printf("Initializing chain for dynamically added Shard %d...", shardID)
	genesis := NewGenesisBlock(shardID, nil, bc.Difficulty, bc.GenesisProposer)

	// Add the genesis block to the blockchain map
	bc.BlockChains[shardID] = []*Block{genesis}

	log.Printf("Created Genesis Block for Shard %d by %s\n", shardID, bc.GenesisProposer)
	return nil
}

// GetLatestBlock remains the same.
func (bc *Blockchain) GetLatestBlock(shardID uint64) (*Block, error) {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()
	shardChain, ok := bc.BlockChains[shardID]
	if !ok {
		return nil, fmt.Errorf("shard %d chain not found", shardID)
	}
	if len(shardChain) == 0 {
		return nil, fmt.Errorf("shard %d chain is empty", shardID)
	}
	return shardChain[len(shardChain)-1], nil
}

// GetBlock remains the same (inefficient search).
func (bc *Blockchain) GetBlock(hash []byte) (*Block, error) {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()
	for _, chain := range bc.BlockChains {
		for _, block := range chain {
			if bytes.Equal(block.Hash, hash) {
				return block, nil
			}
		}
	}
	return nil, fmt.Errorf("block with hash %x not found in any shard", hash)
}

// ValidateBlockIntegrity includes checks for proposer and finality (basic checks).
func ValidateBlockIntegrity(newBlock, prevBlock *Block, vm *ValidatorManager) bool {
	if newBlock == nil || prevBlock == nil {
		log.Println("Validation Error: Cannot validate nil blocks.")
		return false
	}
	if newBlock.Header.ShardID != prevBlock.Header.ShardID {
		log.Printf("Validation Error: Block %d ShardID (%d) mismatch Prev Block %d ShardID (%d)",
			newBlock.Header.Height, newBlock.Header.ShardID, prevBlock.Header.Height, prevBlock.Header.ShardID)
		return false
	}
	if !bytes.Equal(newBlock.Header.PrevBlockHash, prevBlock.Hash) {
		log.Printf("Validation Error: Shard %d Block %d PrevBlockHash (%x) mismatch Block %d Hash (%x)",
			newBlock.Header.ShardID, newBlock.Header.Height, newBlock.Header.PrevBlockHash, prevBlock.Header.Height, prevBlock.Hash)
		return false
	}
	if newBlock.Header.Height != prevBlock.Header.Height+1 {
		log.Printf("Validation Error: Shard %d Block %d Height (%d) not sequential to Block %d Height (%d)",
			newBlock.Header.ShardID, newBlock.Header.Height, newBlock.Header.Height, prevBlock.Header.Height)
		return false
	}

	// --- Phase 4 Checks ---
	if newBlock.Header.ProposerID == "" {
		log.Printf("Validation Error: Shard %d Block %d has empty ProposerID.", newBlock.Header.ShardID, newBlock.Header.Height)
		return false
	}
	// Check if proposer exists and is authenticated (optional, depends on strictness)
	proposer, exists := vm.GetValidator(newBlock.Header.ProposerID)
	if !exists {
		// Allow non-validator proposers for now if node ID is set, but log warning
		// log.Printf("Validation Warning: Proposer %s for Shard %d Block %d is not a registered validator.", newBlock.Header.ProposerID, newBlock.Header.ShardID, newBlock.Header.Height)
		// If strict, return false here.
	} else if !proposer.Node.IsAuthenticated.Load() {
		log.Printf("Validation Error: Proposer %s for Shard %d Block %d is not authenticated.", newBlock.Header.ProposerID, newBlock.Header.ShardID, newBlock.Header.Height)
		return false // Proposer must be authenticated
	}

	// Check if finality signatures exist (basic check)
	// Genesis block might have special finality.
	isGenesis := newBlock.Header.Height == 0
	if !isGenesis && len(newBlock.Header.FinalitySignatures) == 0 {
		log.Printf("Validation Error: Shard %d Block %d (non-genesis) has no finality signatures.", newBlock.Header.ShardID, newBlock.Header.Height)
		return false
	}
	// Could add check for minimum number of signatures, matching against active validators etc.

	// log.Printf("Shard %d: Block %d integrity validated successfully against Block %d.", newBlock.Header.ShardID, newBlock.Header.Height, prevBlock.Header.Height)
	return true
}

// IsChainValid now includes PoW, integrity, proposer, and finality checks.
func (bc *Blockchain) IsChainValid() bool {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()
	overallValid := true

	var wg sync.WaitGroup
	errorChan := make(chan error, len(bc.BlockChains))

	// Copy chains to avoid holding lock during long validation
	chainsToValidate := make(map[uint64][]*Block)
	for id, chain := range bc.BlockChains {
		copiedChain := make([]*Block, len(chain))
		copy(copiedChain, chain)
		chainsToValidate[id] = copiedChain
	}

	for shardID, chain := range chainsToValidate {
		wg.Add(1)
		go func(sID uint64, ch []*Block, vm *ValidatorManager) { // Pass validator manager
			defer wg.Done()
			if len(ch) == 0 {
				return // Skip empty
			}

			// Validate Genesis Block
			genesis := ch[0]
			powGenesis := NewProofOfWork(genesis)
			if !powGenesis.Validate() {
				errorChan <- fmt.Errorf("shard %d genesis PoW validation failed (Hash: %x)", sID, genesis.Hash)
				return
			}
			if genesis.Header.ProposerID == "" {
				errorChan <- fmt.Errorf("shard %d genesis block has empty ProposerID", sID)
				return
			}
			// Genesis finality check (allow special "GENESIS" marker)
			if len(genesis.Header.FinalitySignatures) == 0 {
				errorChan <- fmt.Errorf("shard %d genesis block has no finality signatures", sID)
				return
			}

			if len(ch) == 1 {
				return // Only genesis, already checked
			}

			// Validate subsequent blocks
			for i := 1; i < len(ch); i++ {
				currentBlock := ch[i]
				prevBlock := ch[i-1]

				// Check PoW
				pow := NewProofOfWork(currentBlock)
				if !pow.Validate() {
					errorChan <- fmt.Errorf("shard %d PoW validation failed for Block H:%d (Hash: %x)", sID, currentBlock.Header.Height, currentBlock.Hash)
					return
				}

				// Check Integrity (includes proposer/finality checks)
				if !ValidateBlockIntegrity(currentBlock, prevBlock, vm) { // Pass vm
					// Specific error logged within ValidateBlockIntegrity
					errorChan <- fmt.Errorf("shard %d integrity validation failed between Block H:%d and H:%d", sID, prevBlock.Header.Height, currentBlock.Header.Height)
					return
				}
			}
			// log.Printf("Shard %d chain validation successful.", sID)
		}(shardID, chain, bc.ValidatorMgr) // Pass validator manager to goroutine
	}

	wg.Wait()
	close(errorChan)

	// Collect and report errors
	errCount := 0
	for err := range errorChan {
		if err != nil {
			log.Printf("- Validation Error: %v", err)
			overallValid = false
			errCount++
		}
	}

	if overallValid {
		log.Println("All shard chains validated successfully. Blockchain is valid.")
	} else {
		log.Printf("Blockchain validation failed. %d errors found.", errCount)
	}

	return overallValid
}

// PruneChain remains the same conceptual placeholder.
func (bc *Blockchain) PruneChain(pruneHeight int) {
	// ... (implementation as before) ...
	log.Println("Placeholder: Pruning logic finished (simplified).")
}

// GetShardChain remains the same.
func (bc *Blockchain) GetShardChain(shardID uint64) []*Block {
	// ... (implementation as before) ...
	return nil // Placeholder return
}

// --- Helper for cross-shard in MineShardBlock ---
// Placeholder, implement actual receipt logic if needed
// type CrossShardReceipt struct {
// 	SourceShard      uint64
// 	DestinationShard uint64
// 	TransactionID    []byte
// 	Data             []byte
// 	// Add SourceBlockHash, Height, Proof etc.
// }

// // Placeholder
// type ReceiptProof struct {
// 	MerkleProof [][]byte
// }
