package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time" // Ensure time is imported

	"github.com/saifaleee/advanced-blockchain-go/core"
)

// safeSlice helper (keep as is)
func safeSlice(data []byte, n int) []byte {
	if len(data) < n {
		return data
	}
	return data[:n]
}

func main() {
	log.Println("--- Advanced Go Blockchain PoC - Phase 4+ Simulation ---") // Updated title
	rand.Seed(time.Now().UnixNano())

	// --- Configuration ---
	initialShards := 2
	// numValidators := 5
	// initialReputation := int64(10)

	smConfig := core.DefaultShardManagerConfig()
	smConfig.SplitThresholdStateSize = 15
	smConfig.SplitThresholdTxPool = 20
	smConfig.MergeThresholdStateSize = 5
	smConfig.MergeTargetThresholdSize = 5
	smConfig.CheckInterval = 15 * time.Second
	smConfig.MaxShards = 4

	// >> NEW: Configuration for Telemetry and Consistency
	telemetryInterval := 5 * time.Second
	consistencyInterval := 10 * time.Second
	consistencyConfig := core.DefaultConsistencyConfig()
	// << END NEW

	log.Printf("ShardManager Config: %+v\n", smConfig)
	log.Printf("Consistency Config: %+v\n", consistencyConfig) // Log new config

	// --- Initialization ---
	log.Println("Initializing Nodes and Validators...")
	validatorMgr := core.NewValidatorManager()
	// ... (validator initialization code remains the same) ...
	log.Printf("Initialized %d validators.", len(validatorMgr.GetAllValidators()))

	// >> NEW: Initialize Telemetry and Consistency Orchestrator
	log.Println("Initializing Telemetry Monitor and Consistency Orchestrator...")
	telemetryMonitor := core.NewNetworkTelemetryMonitor(telemetryInterval)
	consistencyOrchestrator := core.NewConsistencyOrchestrator(telemetryMonitor, consistencyConfig, consistencyInterval)
	// << END NEW

	log.Println("Initializing Blockchain...")
	// >> UPDATE: Pass Consistency Orchestrator to NewBlockchain
	// Before Initializing Blockchain...
	localNode := core.NewNode()
	localNode.Authenticate()
	log.Printf("Local Node ID: %s", localNode.ID)
	// If the local node should also be a validator:
	// err := validatorMgr.AddValidator(localNode, initialReputation)
	// if err != nil { log.Fatalf(...) }

	// Then the NewBlockchain call will work:
	bc, err := core.NewBlockchain(uint(initialShards), smConfig, validatorMgr, consistencyOrchestrator, localNode.ID)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	log.Printf("Blockchain initialized with %d shards. Initial IDs: %v\n", initialShards, bc.ShardManager.GetAllShardIDs())

	// --- Start Background Processes ---
	log.Println("Starting background loops (Shard Manager, Telemetry, Consistency)...")
	// >> NEW: Start Telemetry and Consistency loops
	telemetryMonitor.Start()
	consistencyOrchestrator.Start()
	// << END NEW
	bc.ShardManager.StartManagementLoop() // Start this after the others

	// --- Simulation Control ---
	var wg sync.WaitGroup
	stopSim := make(chan struct{})

	// >> UPDATE: Add WaitGroup entries for new loops
	wg.Add(5) // 1 (TX Gen) + 1 (Miner) + 1 (Reputation Printer) + 1 (Telemetry) + 1 (Consistency) - Adjust if ShardManager/Telemetry/Consistency handle their own waits internally better. Let's assume StartManagementLoop, Start, Start handle internal waits. So just 3 standard ones.
	// wg.Add(3) // TxGen, Miner, RepPrinter

	// Goroutine for generating transactions (mostly unchanged)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[TX Generator] Starting...")
		ticker := time.NewTicker(1 * time.Second) // Generate TXs more frequently
		defer ticker.Stop()
		txCounter := 0

		for {
			select {
			case <-stopSim:
				log.Println("[TX Generator] Stopping...")
				return
			case <-ticker.C:
				numTx := rand.Intn(5) + 1 // Generate 1-5 transactions per tick
				for i := 0; i < numTx; i++ {
					txCounter++
					var tx *core.Transaction
					txTypeRand := rand.Float32()
					shardIDs := bc.ShardManager.GetAllShardIDs()
					numActiveShards := len(shardIDs)

					// Create different transaction types
					if numActiveShards > 1 && txTypeRand < 0.3 { // ~30% chance of cross-shard
						sourceShardIdx := rand.Intn(numActiveShards)
						destShardIdx := rand.Intn(numActiveShards)
						for destShardIdx == sourceShardIdx {
							destShardIdx = rand.Intn(numActiveShards)
						}
						sourceShardID := shardIDs[sourceShardIdx] // Select a source shard
						destShardID := shardIDs[destShardIdx]     // Select a destination shard

						data := []byte(fmt.Sprintf("CS:%d:%d->%d", txCounter, sourceShardID, destShardID))
						tx = core.NewTransaction(data, core.CrossShardTxInit, &destShardID)
						// *** Crucially, set the source shard field for routing ***
						tx.SourceShard = &sourceShardID
						// log.Printf("[TX Generator] Created CrossShardInitiate TX #%d (%d -> %d)", txCounter, sourceShardID, destShardID)

					} else { // Intra-shard transaction
						data := []byte(fmt.Sprintf("IS:%d", txCounter))
						tx = core.NewTransaction(data, core.IntraShard, nil)
						// log.Printf("[TX Generator] Created IntraShard TX #%d", txCounter)
					}

					// Basic placeholder signing (no change)
					_ = tx.Sign([]byte("dummy-private-key"))

					// Add transaction (routing handled by blockchain)
					addErr := bc.AddTransaction(tx)
					if addErr != nil {
						log.Printf("[TX Generator] Error adding TX #%d (Type %d): %v", txCounter, tx.Type, addErr)
					} else {
						// log.Printf("[TX Generator] Added TX #%d (Type: %d) via router. TX ID: %x...", txCounter, tx.Type, safeSlice(tx.ID, 4))
					}
					time.Sleep(20 * time.Millisecond) // Shorter delay
				}
			}
		}
	}()

	// Goroutine for triggering mining (now Propose/Finalize) periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[Miner] Starting consensus loop...")
		// Mine more frequently than metric checks
		ticker := time.NewTicker(7 * time.Second) // Interval for triggering consensus round
		defer ticker.Stop()

		for {
			select {
			case <-stopSim:
				log.Println("[Miner] Stopping consensus loop...")
				return
			case <-ticker.C:
				shardIDs := bc.ShardManager.GetAllShardIDs()
				if len(shardIDs) == 0 {
					log.Println("[Miner] No active shards for consensus.")
					continue
				}

				log.Printf("[Miner] === Triggering Consensus Round for Shards: %v ===", shardIDs)
				// Simulate this node trying to run consensus for all shards it knows about
				for _, shardID := range shardIDs {
					// Check if shard still exists (might have been merged)
					// Add small random delay to simulate network/processing time variance
					time.Sleep(time.Duration(rand.Intn(50)+10) * time.Millisecond)

					if _, exists := bc.ShardManager.GetShard(shardID); !exists {
						// log.Printf("[Miner] Shard %d no longer exists, skipping consensus.", shardID)
						continue
					}

					// log.Printf("[Miner] Attempting consensus for Shard %d...", shardID)
					// MineShardBlock now encapsulates Propose (PoW) + Finalize (dBFT)
					finalizedBlock, mineErr := bc.MineShardBlock(shardID)

					if mineErr != nil {
						// Handle specific errors if needed (e.g., no txs, consensus failure)
						if mineErr.Error() == "no transactions to mine" {
							// log.Printf("[Miner] Shard %d: No transactions available.", shardID)
						} else if mineErr.Error() == "dBFT consensus failed" {
							log.Printf("[Miner] Shard %d: Consensus FAILED for proposed block.", shardID)
						} else {
							log.Printf("[Miner] Error during consensus for Shard %d: %v", shardID, mineErr)
						}
					} else if finalizedBlock != nil {
						// Log success (logging moved inside MineShardBlock for clarity)
						// log.Printf("[Miner] Successfully Finalized Block H:%d for Shard %d! Hash: %x...",
						//  finalizedBlock.Header.Height, shardID, safeSlice(finalizedBlock.Hash, 4))

						// Print metrics after successful block finalization
						if s, ok := bc.ShardManager.GetShard(shardID); ok {
							log.Printf("[Metrics] Shard %d - Blocks: %d, StateSize: %d, PendingTxPool: %d",
								shardID, s.Metrics.BlockCount.Load(), s.Metrics.StateSize.Load(), s.Metrics.PendingTxPool.Load())
						}
					}
					// Add a small delay between attempts on different shards
					time.Sleep(50 * time.Millisecond)
				}
				log.Printf("[Miner] === Consensus Round Finished ===")

			}
		}
	}()

	// Goroutine to periodically print validator reputations
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second) // Print reps every 30s
		defer ticker.Stop()
		for {
			select {
			case <-stopSim:
				return
			case <-ticker.C:
				log.Println("--- Validator Reputations ---")
				allValidators := validatorMgr.GetAllValidators()
				for _, v := range allValidators {
					idSuffix := string(v.Node.ID[len(v.Node.ID)-4:]) // Last 4 chars of ID
					isActive := v.IsActive.Load()
					isAuth := v.Node.IsAuthenticated.Load()
					log.Printf("  Validator ...%s: Rep = %d (Active: %t, Auth: %t)",
						idSuffix, v.Reputation.Load(), isActive, isAuth)
				}
				log.Println("---------------------------")
			}
		}
	}()

	// --- Run Simulation & Handle Shutdown ---
	log.Println(">>> Simulation running... Press Ctrl+C to stop. <<<")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("--- Shutdown Signal Received ---")

	log.Println("Stopping simulation goroutines...")
	close(stopSim) // Signal TX Gen, Miner, Rep Printer to stop

	// >> NEW: Stop Telemetry and Consistency Orchestrator first
	log.Println("Stopping Consistency Orchestrator...")
	consistencyOrchestrator.Stop()
	log.Println("Stopping Telemetry Monitor...")
	telemetryMonitor.Stop()
	// << END NEW

	// Stop the shard manager loop (which also stops shards)
	log.Println("Stopping Shard Manager background loop...")
	bc.ShardManager.StopManagementLoop()

	// Wait for main simulation goroutines to finish (if using WaitGroup for them)
	// log.Println("Waiting for main goroutines to finish...")
	// wg.Wait() // Uncomment if wg.Add was used for TxGen, Miner, RepPrinter
	// log.Println("Main simulation goroutines finished.")

	// Perform final chain validation
	log.Println("Performing final chain validation...")
	bc.IsChainValid()

	log.Println("--- Simulation Finished ---")

	// Optional: Print final state summary (unchanged)
	finalShardIDs := bc.ShardManager.GetAllShardIDs()
	log.Printf("Final active shard IDs: %v", finalShardIDs)
	// ... (rest of final state printing as before) ...
	for _, shardID := range finalShardIDs {
		if s, ok := bc.ShardManager.GetShard(shardID); ok {
			finalStateRoot, _ := s.StateDB.GetStateRoot()
			log.Printf("  Shard %d - Final State Size: %d, Final State Root: %x, Blocks: %d",
				shardID, s.Metrics.StateSize.Load(), finalStateRoot, s.Metrics.BlockCount.Load())

			bc.ChainMu.RLock()
			chain, exists := bc.BlockChains[shardID]
			bc.ChainMu.RUnlock()
			if exists && len(chain) > 0 {
				lastBlock := chain[len(chain)-1]
				log.Printf("    Shard %d - Last Block H:%d, Hash: %x..., Proposer: ...%s, Finalizers: %d",
					shardID, lastBlock.Header.Height, safeSlice(lastBlock.Hash, 4),
					safeSlice([]byte(lastBlock.Header.ProposerID), 4), len(lastBlock.Header.FinalitySignatures))
			} else {
				log.Printf("    Shard %d - No blocks found.", shardID)
			}
		}
	}
	// Print final reputations
	log.Println("--- Final Validator Reputations ---")
	allValidators := validatorMgr.GetAllValidators()
	for _, v := range allValidators {
		idSuffix := string(v.Node.ID[len(v.Node.ID)-4:])
		log.Printf("  Validator ...%s: Rep = %d", idSuffix, v.Reputation.Load())
	}
	log.Println("---------------------------------")
}
