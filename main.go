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

	"github.com/saifaleee/advanced-blockchain-go/core" // Adjust import path if necessary
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
	numValidators := 5 // Number of validators to simulate
	initialReputation := int64(10)

	// Shard Manager Config (Adjust thresholds as needed)
	smConfig := core.DefaultShardManagerConfig()
	smConfig.SplitThresholdStateSize = 15
	smConfig.SplitThresholdTxPool = 20
	smConfig.MergeThresholdStateSize = 5
	smConfig.MergeTargetThresholdSize = 5
	smConfig.CheckInterval = 15 * time.Second
	smConfig.MaxShards = 4 // Keep max shards low for demo

	// Configuration for Telemetry and Consistency
	telemetryInterval := 5 * time.Second
	consistencyInterval := 10 * time.Second
	consistencyConfig := core.DefaultConsistencyConfig()

	log.Printf("ShardManager Config: %+v\n", smConfig)
	log.Printf("Consistency Config: %+v\n", consistencyConfig) // Log new config

	// --- Initialization ---
	log.Println("Initializing Nodes and Validators...")
	validatorMgr := core.NewValidatorManager()
	// >> FIX: Added validator creation loop <<
	nodes := make([]*core.Node, numValidators)
	var localNode *core.Node // Declare localNode variable

	log.Printf("Creating %d nodes/validators...", numValidators)
	for i := 0; i < numValidators; i++ {
		node := core.NewNode()
		node.Authenticate() // Authenticate simulated validators
		nodes[i] = node
		if i == 0 {
			localNode = node // Assign the first created node as the local node for this instance
			log.Printf("Local Node ID assigned: %s", localNode.ID)
		}
		// Add the node to the validator manager
		err := validatorMgr.AddValidator(node, initialReputation)
		if err != nil {
			// Log non-fatally, maybe continue? Or log fatal? Fatal is safer for simulation setup.
			log.Fatalf("Failed to add validator %s: %v", node.ID, err)
		}
	}
	// Check if localNode was assigned (should always happen if numValidators > 0)
	if localNode == nil && numValidators > 0 {
		log.Fatalf("Local node was not assigned during validator initialization loop.")
	} else if numValidators == 0 {
		// This case should ideally not happen if we want consensus
		log.Println("Warning: numValidators is 0. Consensus will fail.")
		// Create a localNode anyway if needed for other purposes, even if not a validator
		localNode = core.NewNode()
		localNode.Authenticate()
		log.Printf("Created Local Node ID (not a validator): %s", localNode.ID)
	}
	log.Printf("Initialized and added %d validators.", len(validatorMgr.GetAllValidators()))
	// >> END FIX <<

	// Initialize Telemetry and Consistency Orchestrator
	log.Println("Initializing Telemetry Monitor and Consistency Orchestrator...")
	telemetryMonitor := core.NewNetworkTelemetryMonitor(telemetryInterval)
	consistencyOrchestrator := core.NewConsistencyOrchestrator(telemetryMonitor, consistencyConfig, consistencyInterval)

	log.Println("Initializing Blockchain...")
	// Pass Consistency Orchestrator and LocalNodeID to NewBlockchain
	bc, err := core.NewBlockchain(uint(initialShards), smConfig, validatorMgr, consistencyOrchestrator, localNode.ID)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	log.Printf("Blockchain initialized with %d shards. Initial IDs: %v\n", initialShards, bc.ShardManager.GetAllShardIDs())

	// --- Start Background Processes ---
	log.Println("Starting background loops (Shard Manager, Telemetry, Consistency)...")
	telemetryMonitor.Start()
	consistencyOrchestrator.Start()
	bc.ShardManager.StartManagementLoop() // Start this after the others

	// --- Simulation Control ---
	var wg sync.WaitGroup
	stopSim := make(chan struct{}) // Channel to signal simulation loops to stop

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
					shardIDs := bc.ShardManager.GetAllShardIDs() // Get current shard IDs
					numActiveShards := len(shardIDs)

					// Ensure there are shards before proceeding
					if numActiveShards == 0 {
						// log.Println("[TX Generator] No active shards to send transactions to.")
						time.Sleep(1 * time.Second) // Wait before trying again
						continue
					}

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
						tx.SourceShard = &sourceShardID // Set source shard for routing
						// log.Printf("[TX Generator] Created CrossShardInitiate TX #%d (%d -> %d)", txCounter, sourceShardID, destShardID)

					} else { // Intra-shard transaction
						targetShardIdx := rand.Intn(numActiveShards)
						targetShardID := shardIDs[targetShardIdx] // Pick a target shard for intra-shard
						data := []byte(fmt.Sprintf("IS:%d:Shard%d", txCounter, targetShardID))
						tx = core.NewTransaction(data, core.IntraShard, nil)
						// log.Printf("[TX Generator] Created IntraShard TX #%d for Shard %d", txCounter, targetShardID)
					}

					// Basic placeholder signing
					_ = tx.Sign([]byte("dummy-private-key"))

					// Add transaction (routing handled by blockchain)
					addErr := bc.AddTransaction(tx)
					if addErr != nil {
						log.Printf("[TX Generator] Error adding TX #%d (Type %d): %v", txCounter, tx.Type, addErr)
					} else {
						// log.Printf("[TX Generator] Added TX #%d (Type: %d) via router. TX ID: %x...", txCounter, tx.Type, safeSlice(tx.ID, 4))
					}
					time.Sleep(time.Duration(rand.Intn(30)+10) * time.Millisecond) // Small random delay between txs
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
					// log.Println("[Miner] No active shards for consensus.")
					continue
				}

				// log.Printf("[Miner] === Triggering Consensus Round for Shards: %v ===", shardIDs)
				// Simulate this node trying to run consensus for all shards it knows about
				var roundWg sync.WaitGroup
				for _, shardID := range shardIDs {
					// Run consensus attempts concurrently for different shards in this round
					roundWg.Add(1)
					go func(sID uint64) {
						defer roundWg.Done()
						// Add small random delay to simulate network/processing time variance
						time.Sleep(time.Duration(rand.Intn(100)+10) * time.Millisecond)

						if _, exists := bc.ShardManager.GetShard(sID); !exists {
							return // Shard disappeared
						}

						// MineShardBlock now encapsulates Propose (PoW) + Finalize (dBFT)
						finalizedBlock, mineErr := bc.MineShardBlock(sID)

						if mineErr != nil {
							// Handle specific errors if needed (e.g., no txs, consensus failure)
							if mineErr.Error() == "no transactions to mine" {
								// Normal condition, don't log verbosely
								// log.Printf("[Miner] Shard %d: No transactions available.", sID)
							} else if mineErr.Error() == "dBFT consensus failed (Strong level)" || mineErr.Error() == "dBFT consensus failed (Eventual level)" {
								log.Printf("[Miner] Shard %d: Consensus FAILED for proposed block (%v).", sID, mineErr)
							} else {
								log.Printf("[Miner] Error during consensus for Shard %d: %v", sID, mineErr)
							}
						} else if finalizedBlock != nil {
							// Successful block
							if s, ok := bc.ShardManager.GetShard(sID); ok {
								log.Printf("[Metrics] Shard %d - Blocks: %d, StateSize: %d, PendingTxPool: %d",
									sID, s.Metrics.BlockCount.Load(), s.Metrics.StateSize.Load(), s.Metrics.PendingTxPool.Load())
							}
						}
					}(shardID)
				}
				roundWg.Wait() // Wait for all consensus attempts in this round to finish
				// log.Printf("[Miner] === Consensus Round Finished ===")

			}
		}
	}()

	// Goroutine to periodically print validator reputations
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second) // Print reps every 30s
		defer ticker.Stop()
		lastLevel := core.Strong // Track last printed level to reduce noise
		for {
			select {
			case <-stopSim:
				return
			case <-ticker.C:
				// Log current consistency level
				currentLevel := consistencyOrchestrator.GetCurrentLevel()
				if currentLevel != lastLevel {
					log.Printf("<<<<< CONSISTENCY LEVEL CHANGED TO: %s >>>>>", currentLevel)
					lastLevel = currentLevel
				} else {
					log.Printf("--- Current Consistency Level: %s ---", currentLevel)
				}

				log.Println("--- Validator Reputations ---")
				allValidators := validatorMgr.GetAllValidators()
				if len(allValidators) == 0 {
					log.Println("  No validators registered.")
				} else {
					for _, v := range allValidators {
						idSuffix := string(v.Node.ID[len(v.Node.ID)-4:]) // Last 4 chars of ID
						isActive := v.IsActive.Load()
						isAuth := v.Node.IsAuthenticated.Load()
						log.Printf("  Validator ...%s: Rep = %d (Active: %t, Auth: %t)",
							idSuffix, v.Reputation.Load(), isActive, isAuth)
					}
				}
				log.Println("---------------------------")
			}
		}
	}()

	// Goroutine to monitor shard count changes (unchanged, keep for visibility)
	go func() {
		lastShardCount := initialShards
		// Check slightly less frequently than shard manager
		ticker := time.NewTicker(smConfig.CheckInterval + 2*time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopSim:
				return
			case <-ticker.C:
				currentIDs := bc.ShardManager.GetAllShardIDs()
				currentShardCount := len(currentIDs)
				if currentShardCount != lastShardCount {
					log.Printf("<<<<< SHARD COUNT CHANGED: %d -> %d. Current IDs: %v >>>>>", lastShardCount, currentShardCount, currentIDs)
					lastShardCount = currentShardCount
				}
			}
		}
	}()

	// --- Run Simulation & Handle Shutdown ---
	log.Println(">>> Simulation running... Press Ctrl+C to stop. <<<")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // Block until signal is received

	log.Println("--- Shutdown Signal Received ---")

	log.Println("Stopping simulation goroutines...")
	close(stopSim) // Signal TX Gen, Miner, Rep Printer to stop

	// Stop Telemetry and Consistency Orchestrator first
	log.Println("Stopping Consistency Orchestrator...")
	consistencyOrchestrator.Stop()
	log.Println("Stopping Telemetry Monitor...")
	telemetryMonitor.Stop()

	// Stop the shard manager loop (which also stops shards)
	log.Println("Stopping Shard Manager background loop...")
	bc.ShardManager.StopManagementLoop()

	// Wait for main simulation goroutines to finish
	log.Println("Waiting for main goroutines (TX Gen, Miner, Rep Printer) to finish...")
	wg.Wait()
	log.Println("Main simulation goroutines finished.")

	// Perform final chain validation
	log.Println("Performing final chain validation...")
	bc.IsChainValid() // Log output happens inside the function

	log.Println("--- Simulation Finished ---")

	// Optional: Print final state summary (unchanged)
	finalShardIDs := bc.ShardManager.GetAllShardIDs()
	log.Printf("Final active shard IDs: %v", finalShardIDs)

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
				// Check if genesis block exists even if no others were mined
				bc.ChainMu.RLock()
				genesisChain, genesisExists := bc.BlockChains[shardID]
				bc.ChainMu.RUnlock()
				if genesisExists && len(genesisChain) > 0 && genesisChain[0].Header.Height == 0 {
					lastBlock := genesisChain[0]
					log.Printf("    Shard %d - Only Genesis Block H:%d exists. Hash: %x..., Proposer: ...%s, Finalizers: %d",
						shardID, lastBlock.Header.Height, safeSlice(lastBlock.Hash, 4),
						safeSlice([]byte(lastBlock.Header.ProposerID), 4), len(lastBlock.Header.FinalitySignatures))
				} else {
					log.Printf("    Shard %d - No blocks found.", shardID)
				}
			}
		}
	}
	// Print final reputations
	log.Println("--- Final Validator Reputations ---")
	allValidators := validatorMgr.GetAllValidators()
	if len(allValidators) == 0 {
		log.Println("  No validators were registered.")
	} else {
		for _, v := range allValidators {
			idSuffix := string(v.Node.ID[len(v.Node.ID)-4:])
			log.Printf("  Validator ...%s: Rep = %d", idSuffix, v.Reputation.Load())
		}
	}
	log.Println("---------------------------------")
}
