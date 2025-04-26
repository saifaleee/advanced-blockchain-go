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
		log.Printf("Warning: Node %s proposing for shard %d is not a registered validator.", proposerNodeID, shardID)
		proposer = &Validator{Node: &Node{ID: proposerNodeID}} // Temporary, won't be in manager
		proposer.Node.Authenticate()                           // Assume authenticated for simulation
	}

	// --- 2. Select Transactions & Handle Cross-Shard ---
	maxTxPerBlock := 10
	txsRaw := shard.GetTransactionsForBlock(maxTxPerBlock)
	transactionsForProposal := make([]*Transaction, 0, len(txsRaw))

	for _, tx := range txsRaw {
		if tx.Type == CrossShardTxInit && tx.SourceShard != nil && tx.DestinationShard != nil {
			if *tx.SourceShard != shardID {
				log.Printf("Error: CrossShardTxInit %x found in shard %d, but SourceShard field is %d. Re-routing or discarding.", tx.ID, shardID, *tx.SourceShard)
				continue
			}

			log.Printf("Shard %d: Processing cross-shard init Tx %x for dest %d", shardID, tx.ID, *tx.DestinationShard)
			receipt := &CrossShardReceipt{
				SourceShard:      *tx.SourceShard,
				DestinationShard: *tx.DestinationShard,
				TransactionID:    tx.ID,
				Data:             tx.Data,
			}
			finalizeTx := NewTransaction(receipt.Data, CrossShardTxFinalize, &receipt.DestinationShard)
			finalizeTx.SourceShard = &receipt.SourceShard
			finalizeTx.SourceReceiptProof = &ReceiptProof{MerkleProof: [][]byte{receipt.TransactionID}}

			destShard, destOk := bc.ShardManager.GetShard(receipt.DestinationShard)
			if destOk {
				err := destShard.AddTransaction(finalizeTx)
				if err != nil {
					log.Printf("Shard %d: Error routing cross-shard finalize tx %x to shard %d: %v", shardID, finalizeTx.ID, receipt.DestinationShard, err)
				} else {
					log.Printf("Shard %d: Routed finalize tx %x for %x to dest shard %d", shardID, finalizeTx.ID, tx.ID, receipt.DestinationShard)
				}
			} else {
				log.Printf("Shard %d: Cannot route finalize tx %x for %x, dest shard %d not found", shardID, finalizeTx.ID, tx.ID, receipt.DestinationShard)
				log.Printf("Shard %d: Discarding cross-shard init Tx %x due to missing destination shard %d", shardID, tx.ID, *tx.DestinationShard)
				continue
			}
			transactionsForProposal = append(transactionsForProposal, tx)

		} else if tx.Type == CrossShardTxFinalize {
			if tx.DestinationShard == nil || *tx.DestinationShard != shardID {
				log.Printf("Error: CrossShardTxFinalize %x found in shard %d, but DestinationShard field is missing or incorrect (%v). Discarding.", tx.ID, shardID, tx.DestinationShard)
				continue
			}
			log.Printf("Shard %d: Including cross-shard finalize Tx %x", shardID, tx.ID)
			transactionsForProposal = append(transactionsForProposal, tx)
		} else if tx.Type == IntraShard {
			transactionsForProposal = append(transactionsForProposal, tx)
		} else {
			log.Printf("Shard %d: Skipping transaction %x with unknown type %d", shardID, tx.ID, tx.Type)
		}
	}

	bc.ChainMu.RLock()
	shardChain, chainOk := bc.BlockChains[shardID]
	bc.ChainMu.RUnlock()

	if !chainOk || len(shardChain) == 0 {
		err := bc.initializeShardChain(shardID)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize chain for shard %d: %w", shardID, err)
		}
		bc.ChainMu.RLock()
		shardChain, chainOk = bc.BlockChains[shardID]
		bc.ChainMu.RUnlock()
		if !chainOk || len(shardChain) == 0 {
			return nil, fmt.Errorf("shard %d chain initialization failed or chain empty", shardID)
		}
	}

	bc.ChainMu.RLock()
	prevBlock := shardChain[len(shardChain)-1]
	bc.ChainMu.RUnlock()
	newHeight := prevBlock.Header.Height + 1

	stateRoot, err := shard.ApplyStateChanges(transactionsForProposal)
	if err != nil {
		return nil, fmt.Errorf("failed to apply state changes for shard %d, block %d: %w", shardID, newHeight, err)
	}

	proposedBlock, err := ProposeBlock(shardID, transactionsForProposal, prevBlock.Hash, newHeight, stateRoot, bc.Difficulty, proposerNodeID, prevBlock.Header.VectorClock)
	if err != nil {
		log.Printf("Shard %d: Failed to propose block H:%d: %v", shardID, newHeight, err)
		return nil, fmt.Errorf("block proposal failed for shard %d: %w", shardID, err)
	}

	currentConsistencyLevel := bc.Consistency.GetCurrentLevel()
	log.Printf("Shard %d: Running consensus with %s consistency level.", shardID, currentConsistencyLevel)

	consensusReached, agreeingValidators := bc.ValidatorMgr.SimulateDBFT(
		proposedBlock,
		proposer,
		currentConsistencyLevel,
		bc.Consistency,
	)

	if !consensusReached {
		log.Printf("Shard %d: Consensus failed for proposed block H:%d under %s consistency. Discarding block.",
			shardID, newHeight, currentConsistencyLevel)
		return nil, fmt.Errorf("dBFT consensus failed (%s level)", currentConsistencyLevel)
	}

	proposedBlock.Finalize(agreeingValidators)

	if !ValidateBlockIntegrity(proposedBlock, prevBlock, bc.ValidatorMgr) {
		log.Printf("Shard %d: CRITICAL - Finalized block H:%d failed integrity validation!", shardID, newHeight)
		return nil, fmt.Errorf("finalized block %d failed integrity check", newHeight)
	}

	bc.ChainMu.Lock()
	bc.BlockChains[shardID] = append(bc.BlockChains[shardID], proposedBlock)
	currentHeight := proposedBlock.Header.Height
	bc.ChainMu.Unlock()

	bc.Mu.Lock()
	if int(currentHeight) > bc.ChainHeight {
		bc.ChainHeight = int(currentHeight)
	}
	bc.Mu.Unlock()

	shard.Metrics.BlockCount.Add(1)

	log.Printf("Shard %d: ===> Successfully Proposed, Finalized, and Added Block H:%d. Hash: %x... VC: %v <===",
		shardID, newHeight, safeSlice(proposedBlock.Hash, 4), proposedBlock.Header.VectorClock)

	return proposedBlock, nil
}

// initializeShardChain creates a genesis block for a shard and initializes its blockchain
// Uses the Blockchain's GenesisProposer ID.
func (bc *Blockchain) initializeShardChain(shardID uint64) error {
	if _, ok := bc.ShardManager.GetShard(shardID); !ok {
		return fmt.Errorf("cannot initialize chain: shard %d not found in shard manager", shardID)
	}

	bc.ChainMu.Lock()
	defer bc.ChainMu.Unlock()

	if _, exists := bc.BlockChains[shardID]; exists {
		return nil
	}

	log.Printf("Initializing chain for dynamically added Shard %d...", shardID)
	genesis := NewGenesisBlock(shardID, nil, bc.Difficulty, bc.GenesisProposer)

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

// ValidateBlockIntegrity includes checks for proposer, finality, and vector clock causality.
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

	if newBlock.Header.ProposerID == "" {
		log.Printf("Validation Error: Shard %d Block %d has empty ProposerID.", newBlock.Header.ShardID, newBlock.Header.Height)
		return false
	}
	proposer, exists := vm.GetValidator(newBlock.Header.ProposerID)
	if !exists {
	} else if !proposer.Node.IsAuthenticated.Load() {
		log.Printf("Validation Error: Proposer %s for Shard %d Block %d is not authenticated.", newBlock.Header.ProposerID, newBlock.Header.ShardID, newBlock.Header.Height)
		return false
	}

	isGenesis := newBlock.Header.Height == 0
	if !isGenesis && len(newBlock.Header.FinalitySignatures) == 0 {
		log.Printf("Validation Error: Shard %d Block %d (non-genesis) has no finality signatures.", newBlock.Header.ShardID, newBlock.Header.Height)
		return false
	}

	comparison := newBlock.Header.VectorClock.Compare(prevBlock.Header.VectorClock)
	if comparison == -1 {
		log.Printf("Validation Error: Shard %d Block %d VectorClock (%v) is causally BEFORE previous block (%v).",
			newBlock.Header.ShardID, newBlock.Header.Height, newBlock.Header.VectorClock, prevBlock.Header.VectorClock)
		return false
	}
	newShardEntry := newBlock.Header.VectorClock[newBlock.Header.ShardID]
	prevShardEntry := prevBlock.Header.VectorClock[prevBlock.Header.ShardID]
	if newShardEntry != prevShardEntry+1 {
		log.Printf("Validation Error: Shard %d Block %d VectorClock entry for shard %d (%d) was not incremented correctly from previous block (%d).",
			newBlock.Header.ShardID, newBlock.Header.Height, newBlock.Header.ShardID, newShardEntry, prevShardEntry)
		return false
	}
	for shardID, prevVal := range prevBlock.Header.VectorClock {
		if shardID == newBlock.Header.ShardID {
			continue
		}
		newVal, ok := newBlock.Header.VectorClock[shardID]
		if !ok || newVal < prevVal {
			log.Printf("Validation Error: Shard %d Block %d VectorClock entry for other shard %d (%d) is less than previous block's entry (%d).",
				newBlock.Header.ShardID, newBlock.Header.Height, shardID, newVal, prevVal)
			return false
		}
	}

	return true
}

// IsChainValid now includes PoW, integrity, proposer, finality, and vector clock checks.
func (bc *Blockchain) IsChainValid() bool {
	bc.ChainMu.RLock()
	defer bc.ChainMu.RUnlock()
	overallValid := true

	var wg sync.WaitGroup
	errorChan := make(chan error, len(bc.BlockChains))

	chainsToValidate := make(map[uint64][]*Block)
	for id, chain := range bc.BlockChains {
		copiedChain := make([]*Block, len(chain))
		copy(copiedChain, chain)
		chainsToValidate[id] = copiedChain
	}

	for shardID, chain := range chainsToValidate {
		wg.Add(1)
		go func(sID uint64, ch []*Block, vm *ValidatorManager) {
			defer wg.Done()
			if len(ch) == 0 {
				return
			}

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
			if len(genesis.Header.FinalitySignatures) == 0 {
				errorChan <- fmt.Errorf("shard %d genesis block has no finality signatures", sID)
				return
			}
			if genesis.Header.VectorClock == nil || genesis.Header.VectorClock[sID] != 1 {
				errorChan <- fmt.Errorf("shard %d genesis block has invalid vector clock: %v", sID, genesis.Header.VectorClock)
				return
			}

			if len(ch) == 1 {
				return
			}

			for i := 1; i < len(ch); i++ {
				currentBlock := ch[i]
				prevBlock := ch[i-1]

				pow := NewProofOfWork(currentBlock)
				if !pow.Validate() {
					errorChan <- fmt.Errorf("shard %d PoW validation failed for Block H:%d (Hash: %x)", sID, currentBlock.Header.Height, currentBlock.Hash)
					return
				}

				if !ValidateBlockIntegrity(currentBlock, prevBlock, vm) {
					errorChan <- fmt.Errorf("shard %d integrity validation failed between Block H:%d and H:%d", sID, prevBlock.Header.Height, currentBlock.Header.Height)
					return
				}
			}
		}(shardID, chain, bc.ValidatorMgr)
	}

	wg.Wait()
	close(errorChan)

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
	log.Println("Placeholder: Pruning logic finished (simplified).")
}

// GetShardChain remains the same.
func (bc *Blockchain) GetShardChain(shardID uint64) []*Block {
	return nil
}
