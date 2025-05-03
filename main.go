package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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
	log.Println("--- Advanced Go Blockchain PoC - Phase 6 Simulation ---") // Updated title
	rand.Seed(time.Now().UnixNano())

	// --- Configuration ---
	initialShards := 2
	numValidators := 5 // Number of validators to simulate
	initialReputation := int64(10)
	simulationDuration := 2 * time.Minute // Run simulation for 2 minutes
	pruneKeepBlocks := 10                 // Keep the last 10 blocks when pruning
	powDifficulty := 4                    // Example PoW difficulty

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

	// Blockchain Config (Pass NumValidators and InitialReputation here)
	bcConfig := core.BlockchainConfig{
		NumValidators:            numValidators,     // Used by NewBlockchain
		InitialReputation:        initialReputation, // Used by NewBlockchain
		PoWDifficulty:            powDifficulty,
		TelemetryInterval:        telemetryInterval,
		PruneKeepBlocks:          pruneKeepBlocks,
		ConsistencyCheckInterval: consistencyInterval,
	}

	log.Printf("ShardManager Config: %+v\n", smConfig)
	log.Printf("Consistency Config: %+v\n", consistencyConfig)
	log.Printf("Blockchain Config: %+v\n", bcConfig)

	// --- Initialization ---
	log.Println("Initializing Blockchain and Components...")

	// Initialize Telemetry and Consistency Orchestrator first
	telemetryMonitor := core.NewNetworkTelemetryMonitor(telemetryInterval)
	consistencyOrchestrator := core.NewConsistencyOrchestrator(telemetryMonitor, consistencyConfig, consistencyInterval)

	// Create Blockchain instance (this will initialize ValidatorManager and Shards)
	bc, err := core.NewBlockchain(initialShards, smConfig, bcConfig, consistencyConfig)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	// Link components
	bc.ConsistencyManager = consistencyOrchestrator // Link consistency manager
	bc.ShardManager.SetBlockchainLink(bc)           // Link shard manager back to bc

	log.Printf("Blockchain initialized with %d shards. Initial IDs: %v\n", initialShards, bc.ShardManager.GetAllShardIDs())

	// --- FIX START: Assign localNode AFTER Blockchain init ---
	var localNode *core.Node
	// Get the node from the blockchain's validator manager
	if numValidators > 0 {
		// Assuming Node-0 corresponds to Validator-0 created in NewBlockchain
		// A more robust approach might involve searching by a known ID or index.
		firstValidatorID := core.NodeID(fmt.Sprintf("Validator-%d", 0))
		if validator, ok := bc.ValidatorManager.GetValidator(firstValidatorID); ok {
			localNode = validator.Node
			log.Printf("Local Node ID assigned: %s", localNode.ID)
		} else {
			log.Fatalf("Failed to get validator %s to assign as local node.", firstValidatorID)
		}
	} else {
		// Handle case with 0 validators if needed (e.g., create a non-validator local node)
		log.Println("Warning: numValidators is 0. No local node assigned from validators.")
		// Optionally create a standalone node for local operations if simulation requires it
	}
	// --- FIX END ---

	// --- FIX START: Remove validator creation loop from main ---
	// log.Println("Initializing Nodes and Validators...")
	// validatorMgr := core.NewValidatorManager() // Temporary manager, not needed
	// nodes := make([]*core.Node, numValidators) // Not needed here

	// log.Printf("Creating %d nodes/validators...", numValidators)
	// for i := 0; i < numValidators; i++ {
	// 	nodeID := core.NodeID(fmt.Sprintf("Node-%d", i)) // Use "Validator-%d" for consistency?
	// 	node, err := core.NewNode(nodeID)
	// 	if err != nil {
	// 		log.Fatalf("Failed to create node %s: %v", nodeID, err)
	// 	}
	// 	node.Authenticate() // Authenticate simulated validators
	// 	nodes[i] = node
	// 	if i == 0 {
	// 		localNode = node // Assign the first created node as the local node
	// 		log.Printf("Local Node ID assigned: %s", localNode.ID)
	// 	}
	// 	// Add the node to the temporary validator manager (REMOVED)
	// 	// validatorMgr.AddValidator(node, initialReputation)
	// }
	// // Assign the blockchain's actual validator manager
	// // bc.ValidatorManager = validatorMgr // Assigning the wrong one!

	// log.Printf("Initialized and added %d validators.", len(bc.ValidatorManager.GetAllValidators())) // Log count from bc's manager
	// --- FIX END ---

	// Ensure Shard 0 has a Genesis Block before starting miner
	// This might be needed if NewBlockchain doesn't guarantee genesis creation
	bc.ChainMu.Lock()
	if _, ok := bc.BlockChains[0]; !ok {
		log.Println("Manually creating Genesis Block for Shard 0 in main")
		genesisBlock := core.NewGenesisBlock(0, nil, powDifficulty, "GENESIS_MAIN")
		if genesisBlock != nil {
			bc.BlockChains[0] = []*core.Block{genesisBlock}
			if shard0, shardOk := bc.ShardManager.GetShard(0); shardOk {
				shard0.Metrics.BlockCount.Add(1) // Increment genesis block count
			}
		} else {
			log.Println("Failed to create genesis block for shard 0")
		}

	}
	// Ensure Shard 1 has a Genesis Block
	if _, ok := bc.BlockChains[1]; !ok {
		log.Println("Manually creating Genesis Block for Shard 1 in main")
		genesisBlock := core.NewGenesisBlock(1, nil, powDifficulty, "GENESIS_MAIN")
		if genesisBlock != nil {
			bc.BlockChains[1] = []*core.Block{genesisBlock}
			if shard1, shardOk := bc.ShardManager.GetShard(1); shardOk {
				shard1.Metrics.BlockCount.Add(1) // Increment genesis block count
			}
		} else {
			log.Println("Failed to create genesis block for shard 1")
		}
	}
	bc.ChainMu.Unlock()

	// --- Start Background Processes ---
	log.Println("Starting background loops (Shard Manager, Telemetry, Consistency)...")
	consistencyOrchestrator.Start()
	bc.ShardManager.StartManagementLoop()
	telemetryMonitor.Start()

	// --- Simulation Control ---
	var wg sync.WaitGroup
	stopSim := make(chan struct{}) // Channel to signal simulation loops to stop

	// Goroutine for generating transactions
	wg.Add(1)
	go func() {
		// ... (rest of TX generator goroutine is unchanged)
		defer wg.Done()
		log.Println("[TX Generator] Starting...")
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		txCounter := 0

		for {
			select {
			case <-stopSim:
				log.Println("[TX Generator] Stopping...")
				return
			case <-ticker.C:
				numTx := rand.Intn(5) + 1
				for i := 0; i < numTx; i++ {
					txCounter++
					var tx *core.Transaction
					txTypeRand := rand.Float32()
					shardIDs := bc.ShardManager.GetAllShardIDs()
					numActiveShards := len(shardIDs)

					if numActiveShards == 0 {
						log.Println("[TX Generator] No active shards to send transactions to. Sleeping.")
						time.Sleep(1 * time.Second) // Wait if no shards are available
						continue
					}

					if numActiveShards > 1 && txTypeRand < 0.3 {
						sourceShardIdx := rand.Intn(numActiveShards)
						destShardIdx := rand.Intn(numActiveShards)
						for destShardIdx == sourceShardIdx {
							destShardIdx = rand.Intn(numActiveShards)
						}
						sourceShardID := shardIDs[sourceShardIdx]
						destShardID := shardIDs[destShardIdx]

						data := []byte(fmt.Sprintf("CS:%d:%d->%d", txCounter, sourceShardID, destShardID))
						tx = core.NewTransaction(core.CrossShardInitiateTx, data, uint32(sourceShardID), uint32(destShardID))
					} else {
						targetShardIdx := rand.Intn(numActiveShards)
						targetShardID := shardIDs[targetShardIdx]
						data := []byte(fmt.Sprintf("IS:%d:Shard%d", txCounter, targetShardID))
						tx = core.NewTransaction(core.IntraShardTx, data, uint32(targetShardID), uint32(targetShardID))
					}

					// Use AddTransaction from Blockchain instance
					addErr := bc.AddTransaction(tx)
					if addErr != nil {
						log.Printf("[TX Generator] Error adding TX #%d (Type %d): %v", txCounter, tx.Type, addErr)
					} else {
						// log.Printf("[TX Generator] Added TX #%d (Type %d) to shard %d", txCounter, tx.Type, tx.ToShard)
					}
					// Add slight delay between sending transactions
					time.Sleep(time.Duration(rand.Intn(30)+10) * time.Millisecond)
				}
			}
		}
	}()

	// Goroutine for triggering mining
	wg.Add(1)
	go func() {
		// ... (rest of Miner goroutine is unchanged)
		defer wg.Done()
		log.Println("[Miner] Starting consensus loop...")
		ticker := time.NewTicker(7 * time.Second) // Adjust mining interval if needed
		defer ticker.Stop()

		for {
			select {
			case <-stopSim:
				log.Println("[Miner] Stopping consensus loop...")
				return
			case <-ticker.C:
				shardIDs := bc.ShardManager.GetAllShardIDs()
				if len(shardIDs) == 0 {
					// log.Println("[Miner] No active shards to mine on.")
					continue
				}

				var roundWg sync.WaitGroup
				for _, shardID := range shardIDs {
					roundWg.Add(1)
					go func(sID uint64) {
						defer roundWg.Done()
						// Add slight random delay before attempting to mine
						time.Sleep(time.Duration(rand.Intn(100)+10) * time.Millisecond)

						// Check if shard still exists (it might have been merged)
						if _, exists := bc.ShardManager.GetShard(sID); !exists {
							// log.Printf("[Miner] Shard %d no longer exists, skipping mining.", sID)
							return
						}

						// Call MineShardBlock from Blockchain instance
						finalizedBlock, mineErr := bc.MineShardBlock(sID) // Pass only shard ID

						if mineErr != nil {
							// Handle specific errors without excessive logging
							if mineErr.Error() == "no transactions to mine" {
								// Don't log this every time, it's normal
							} else if mineErr.Error() == "block finalization failed" {
								log.Printf("[Miner] Shard %d: Consensus FAILED for proposed block.", sID)
							} else {
								// Log other unexpected errors
								log.Printf("[Miner] Error during consensus process for Shard %d: %v", sID, mineErr)
							}
						} else if finalizedBlock != nil {
							// Log shard metrics after successful mining
							if s, ok := bc.ShardManager.GetShard(sID); ok {
								log.Printf("[Metrics] Shard %d - Blocks: %d, StateSize: %d, PendingTxPool: %d",
									sID, s.Metrics.BlockCount.Load(), s.Metrics.StateSize.Load(), s.Metrics.PendingTxPool.Load())
							}
							// Pruning logic remains the same
							if bc.Config.PruneKeepBlocks > 0 && finalizedBlock.Header.Height%uint64(bc.Config.PruneKeepBlocks) == 0 {
								// Note: Pruning is now called within MineShardBlock if successful
								// bc.PruneChain(sID, bc.Config.PruneKeepBlocks) // Call prune from bc
							}
						}
					}(shardID)
				}
				roundWg.Wait() // Wait for all shard mining attempts in this round
			}
		}
	}()

	// Goroutine to periodically print validator reputations
	wg.Add(1)
	go func() {
		// ... (rest of Reputation printer goroutine is unchanged)
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		lastLevel := core.Strong // Assuming initial level is Strong

		for {
			select {
			case <-stopSim:
				return
			case <-ticker.C:
				// Log Consistency Level
				currentLevel := consistencyOrchestrator.GetCurrentLevel()
				if currentLevel != lastLevel {
					log.Printf("<<<<< CONSISTENCY LEVEL CHANGED TO: %s >>>>>", currentLevel)
					lastLevel = currentLevel
				} else {
					// log.Printf("--- Current Consistency Level: %s ---", currentLevel)
				}

				// Log Validator Reputations from Blockchain's Validator Manager
				log.Println("--- Validator Reputations ---")
				allValidators := bc.ValidatorManager.GetAllValidators() // Use bc's manager
				if len(allValidators) == 0 {
					log.Println("  No validators registered.")
				} else {
					for _, v := range allValidators {
						// Shorten ID for readability
						idSuffix := string(v.Node.ID)
						if len(v.Node.ID) > 10 { // Adjust length as needed
							idSuffix = "..." + string(v.Node.ID[len(v.Node.ID)-6:])
						}
						isActive := v.IsActive.Load()
						isAuth := v.Node.IsAuthenticated()
						log.Printf("  Validator %s: Rep = %d (Active: %t, Auth: %t, Trust: %.2f)",
							idSuffix, v.Reputation.Load(), isActive, isAuth, v.Node.TrustScore)
					}
				}
				log.Println("---------------------------")

				// Log Shard Info
				log.Println("--- Shard Information ---")
				currentShardIDs := bc.ShardManager.GetAllShardIDs()
				log.Printf("  Active Shard IDs: %v", currentShardIDs)
				for _, sID := range currentShardIDs {
					if shard, ok := bc.ShardManager.GetShard(sID); ok {
						stateR, _ := shard.StateDB.GetStateRoot()
						log.Printf("  Shard %d: Blocks=%d, StateSize=%d, PendingTX=%d, StateRoot=%x...",
							sID,
							shard.Metrics.BlockCount.Load(),
							shard.Metrics.StateSize.Load(),
							shard.Metrics.PendingTxPool.Load(),
							safeSlice(stateR, 4))
					}
				}
				log.Println("-------------------------")

			}
		}
	}()

	// Goroutine for periodic authentication challenges
	wg.Add(1)
	go func() {
		// ... (rest of Auth challenger goroutine is unchanged)
		defer wg.Done()
		// Initial delay before starting challenges
		time.Sleep(20 * time.Second)
		ticker := time.NewTicker(45 * time.Second) // Interval for challenge rounds
		defer ticker.Stop()

		for {
			select {
			case <-stopSim:
				log.Println("[Auth Challenger] Stopping authentication challenges.")
				return
			case <-ticker.C:
				log.Println("[Auth Challenger] --- Starting Authentication Round ---")
				validatorsToChallenge := bc.ValidatorManager.GetAllValidators() // Use bc's manager
				var authWg sync.WaitGroup

				if len(validatorsToChallenge) == 0 {
					log.Println("[Auth Challenger] No validators to challenge.")
					continue
				}

				for _, v := range validatorsToChallenge {
					// Only challenge active and authenticated validators? Or all? Challenge all for now.
					if !v.IsActive.Load() { // Example: Only challenge active ones
						// log.Printf("[Auth Challenger] Skipping challenge for inactive validator %s", v.Node.ID)
						continue
					}

					authWg.Add(1)
					go func(validator *core.Validator) {
						defer authWg.Done()
						nodeID := validator.Node.ID
						challenge, err := bc.ValidatorManager.ChallengeValidator(nodeID) // Use bc's manager
						if err != nil {
							// Log error but don't crash; might be expected if already pending
							// log.Printf("[Auth Challenger] Error creating challenge for %s: %v", nodeID, err)

							// Attempt verification with nil response if challenge failed (to potentially penalize)
							verifyErr := bc.ValidatorManager.VerifyResponse(nodeID, nil) // Use bc's manager
							if verifyErr != nil {
								// Log this secondary error as well
								// log.Printf("[Auth Challenger] Error verifying nil response for %s after challenge error: %v", nodeID, verifyErr)
							}
							return
						}

						// Node signs the challenge data
						// Note: SignData expects the HASH of the data. ChallengeValidator needs to coordinate.
						// Assuming VerifyResponse handles hashing internally based on pending challenge.
						response, err := validator.Node.SignData(challenge) // Sign the raw challenge bytes for now
						if err != nil {
							log.Printf("[Auth Challenger] Error node %s signing challenge: %v", nodeID, err)
							// Verify with nil response to trigger potential penalty
							verifyErr := bc.ValidatorManager.VerifyResponse(nodeID, nil) // Use bc's manager
							if verifyErr != nil {
								// log.Printf("[Auth Challenger] Error verifying nil response for %s after signing error: %v", nodeID, verifyErr)
							}
							return
						}

						// Simulate slight delay before manager verifies
						time.Sleep(time.Duration(5+rand.Intn(20)) * time.Millisecond)

						// Manager verifies the response
						verifyErr := bc.ValidatorManager.VerifyResponse(nodeID, response) // Use bc's manager
						if verifyErr != nil {
							// Log verification error (e.g., invalid signature, expired)
							// VerifyResponse handles reputation updates internally
							log.Printf("[Auth Challenger] Verification failed for %s: %v", nodeID, verifyErr)
						} else {
							// log.Printf("[Auth Challenger] Verification successful for %s", nodeID)
						}

					}(v)
				}
				authWg.Wait() // Wait for all challenges in this round to complete
				log.Println("[Auth Challenger] --- Authentication Round Finished ---")

				// Cleanup expired challenges that might not have been handled
				bc.ValidatorManager.CleanupExpiredChallenges()

			}
		}
	}()

	// Goroutine to monitor shard count changes
	// ... (rest of Shard monitor goroutine is unchanged)
	go func() {
		lastShardCount := initialShards
		// Use a ticker slightly longer than the check interval to avoid race conditions
		checkInterval := smConfig.CheckInterval
		if checkInterval <= 0 {
			checkInterval = 10 * time.Second // Default if config is zero
		}
		ticker := time.NewTicker(checkInterval + 2*time.Second)
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
	simTimer := time.NewTimer(simulationDuration)

	select {
	case <-sigChan:
		log.Println("Received shutdown signal.")
	case <-simTimer.C:
		log.Printf("Simulation duration (%s) reached.", simulationDuration)
	}

	log.Println("--- Shutdown Signal Received ---")

	log.Println("Stopping simulation goroutines...")
	close(stopSim) // Signal all simulation goroutines to stop

	log.Println("Stopping Blockchain background processes (Consistency, ShardManager, Telemetry)...")
	consistencyOrchestrator.Stop()
	bc.ShardManager.StopManagementLoop() // Stops loop and managed shards
	telemetryMonitor.Stop()

	log.Println("Waiting for main simulation goroutines (TX Gen, Miner, Rep Printer, Auth Challenger) to finish...")
	wg.Wait() // Wait for the primary simulation goroutines controlled by the main waitgroup
	log.Println("Main simulation goroutines finished.")

	log.Println("--- Simulation Finished ---")

	// --- Final State Reporting ---
	finalShardIDs := bc.ShardManager.GetAllShardIDs()
	log.Printf("Final active shard IDs: %v", finalShardIDs)

	for _, shardID := range finalShardIDs {
		if s, ok := bc.ShardManager.GetShard(shardID); ok {
			finalStateRoot, stateErr := s.StateDB.GetStateRoot()
			if stateErr != nil {
				log.Printf("  Shard %d - Error getting final state root: %v", shardID, stateErr)
			} else {
				log.Printf("  Shard %d - Final State Size: %d, Final State Root: %x, Blocks Mined: %d",
					shardID, s.Metrics.StateSize.Load(), finalStateRoot, s.Metrics.BlockCount.Load())
			}

			// Access blockchain data safely
			bc.ChainMu.RLock()
			chain, chainExists := bc.BlockChains[shardID]
			var lastBlock *core.Block
			chainLength := 0
			if chainExists && len(chain) > 0 {
				lastBlock = chain[len(chain)-1]
				chainLength = len(chain)
			}
			bc.ChainMu.RUnlock()

			log.Printf("    Shard %d - Final Chain Length (in memory): %d", shardID, chainLength)
			if lastBlock != nil {
				proposerIDSuffix := string(lastBlock.Header.ProposerID)
				if len(proposerIDSuffix) > 8 { // Shorten proposer ID if long
					proposerIDSuffix = "..." + proposerIDSuffix[len(proposerIDSuffix)-6:]
				}
				log.Printf("      Last Block H:%d, Hash: %x..., Proposer: %s, Finalizers: %d, StateRoot: %x...",
					lastBlock.Header.Height, safeSlice(lastBlock.Hash, 4),
					proposerIDSuffix, len(lastBlock.Header.FinalitySignatures), safeSlice(lastBlock.Header.StateRoot, 4))
			} else {
				log.Printf("      Shard %d - No blocks found in blockchain map.", shardID)
			}
		} else {
			log.Printf("  Shard %d - Could not retrieve shard info at end.", shardID)
		}
	}

	log.Println("--- Final Validator Reputations ---")
	allValidatorsFinal := bc.ValidatorManager.GetAllValidators() // Use bc's manager
	if len(allValidatorsFinal) == 0 {
		log.Println("  No validators were registered.")
	} else {
		for _, v := range allValidatorsFinal {
			idSuffix := string(v.Node.ID)
			if len(v.Node.ID) > 10 {
				idSuffix = "..." + string(v.Node.ID[len(v.Node.ID)-6:])
			}
			log.Printf("  Validator %s: Rep = %d (Auth: %t, Trust: %.2f)",
				idSuffix, v.Reputation.Load(), v.Node.IsAuthenticated(), v.Node.TrustScore)
		}
	}
	log.Println("---------------------------------")
}
