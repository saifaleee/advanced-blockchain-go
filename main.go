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

	// Blockchain Config
	bcConfig := core.BlockchainConfig{
		NumValidators:            numValidators,
		InitialReputation:        initialReputation,
		PoWDifficulty:            powDifficulty,
		TelemetryInterval:        telemetryInterval,
		PruneKeepBlocks:          pruneKeepBlocks,
		ConsistencyCheckInterval: consistencyInterval,
	}

	log.Printf("ShardManager Config: %+v\n", smConfig)
	log.Printf("Consistency Config: %+v\n", consistencyConfig)
	log.Printf("Blockchain Config: %+v\n", bcConfig) // Log bcConfig

	// --- Initialization ---
	log.Println("Initializing Nodes and Validators...")
	validatorMgr := core.NewValidatorManager()
	nodes := make([]*core.Node, numValidators)
	var localNode *core.Node // Declare localNode variable

	log.Printf("Creating %d nodes/validators...", numValidators)
	for i := 0; i < numValidators; i++ {
		nodeID := core.NodeID(fmt.Sprintf("Node-%d", i))
		node, err := core.NewNode(nodeID)
		if err != nil {
			log.Fatalf("Failed to create node %s: %v", nodeID, err)
		}
		node.Authenticate() // Authenticate simulated validators
		nodes[i] = node
		if i == 0 {
			localNode = node // Assign the first created node as the local node for this instance
			log.Printf("Local Node ID assigned: %s", localNode.ID)
		}
		// Add the node to the validator manager
		validatorMgr.AddValidator(node, initialReputation) // Corrected call signature
	}
	if localNode == nil && numValidators > 0 {
		log.Fatalf("Local node was not assigned during validator initialization loop.")
	} else if numValidators == 0 {
		log.Println("Warning: numValidators is 0. Consensus will fail.")
		// Create a local node even if no validators are specified
		localNodeID := core.NodeID("LocalNode-Solo")
		var err error
		localNode, err = core.NewNode(localNodeID)
		if err != nil {
			log.Fatalf("Failed to create local node: %v", err)
		}
		localNode.Authenticate()
		log.Printf("Created Local Node ID (not a validator): %s", localNode.ID)
	}
	log.Printf("Initialized and added %d validators.", len(validatorMgr.GetAllValidators()))

	// Initialize Telemetry and Consistency Orchestrator
	log.Println("Initializing Telemetry Monitor and Consistency Orchestrator...")
	telemetryMonitor := core.NewNetworkTelemetryMonitor(telemetryInterval) // Corrected call signature
	consistencyOrchestrator := core.NewConsistencyOrchestrator(telemetryMonitor, consistencyConfig, consistencyInterval)

	log.Println("Initializing Blockchain...")
	bc, err := core.NewBlockchain(initialShards, smConfig, bcConfig, consistencyConfig)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	bc.ValidatorManager = validatorMgr
	bc.ConsistencyManager = consistencyOrchestrator
	bc.ShardManager.SetBlockchainLink(bc)

	log.Printf("Blockchain initialized with %d shards. Initial IDs: %v\n", initialShards, bc.ShardManager.GetAllShardIDs())

	// --- Start Background Processes ---
	log.Println("Starting background loops (Shard Manager, Telemetry, Consistency)...")
	consistencyOrchestrator.Start()       // Corrected call signature
	bc.ShardManager.StartManagementLoop() // Corrected call signature
	telemetryMonitor.Start()              // Start telemetry monitor

	// --- Simulation Control ---
	var wg sync.WaitGroup
	stopSim := make(chan struct{}) // Channel to signal simulation loops to stop

	// Goroutine for generating transactions
	wg.Add(1)
	go func() {
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
						time.Sleep(1 * time.Second)
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

					addErr := bc.AddTransaction(tx)
					if addErr != nil {
						log.Printf("[TX Generator] Error adding TX #%d (Type %d): %v", txCounter, tx.Type, addErr)
					}
					time.Sleep(time.Duration(rand.Intn(30)+10) * time.Millisecond)
				}
			}
		}
	}()

	// Goroutine for triggering mining
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[Miner] Starting consensus loop...")
		ticker := time.NewTicker(7 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopSim:
				log.Println("[Miner] Stopping consensus loop...")
				return
			case <-ticker.C:
				shardIDs := bc.ShardManager.GetAllShardIDs()
				if len(shardIDs) == 0 {
					continue
				}

				var roundWg sync.WaitGroup
				for _, shardID := range shardIDs {
					roundWg.Add(1)
					go func(sID uint64) {
						defer roundWg.Done()
						time.Sleep(time.Duration(rand.Intn(100)+10) * time.Millisecond)

						if _, exists := bc.ShardManager.GetShard(sID); !exists {
							return
						}

						// Call MineShardBlock with only the shard ID
						finalizedBlock, mineErr := bc.MineShardBlock(sID)

						if mineErr != nil {
							if mineErr.Error() == "no transactions to mine" {
							} else if mineErr.Error() == "block finalization failed" {
								log.Printf("[Miner] Shard %d: Consensus FAILED for proposed block (%v).", sID, mineErr)
							} else {
								log.Printf("[Miner] Error during consensus for Shard %d: %v", sID, mineErr)
							}
						} else if finalizedBlock != nil {
							if s, ok := bc.ShardManager.GetShard(sID); ok {
								log.Printf("[Metrics] Shard %d - Blocks: %d, StateSize: %d, PendingTxPool: %d",
									sID, s.Metrics.BlockCount.Load(), s.Metrics.StateSize.Load(), s.Metrics.PendingTxPool.Load())
							}
							if finalizedBlock.Header.Height%uint64(pruneKeepBlocks) == 0 {
								bc.PruneChain(sID, pruneKeepBlocks)
							}
						}
					}(shardID)
				}
				roundWg.Wait()
			}
		}
	}()

	// Goroutine to periodically print validator reputations
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		lastLevel := core.Strong
		for {
			select {
			case <-stopSim:
				return
			case <-ticker.C:
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
						idSuffix := string(v.Node.ID)
						if len(v.Node.ID) > 4 {
							idSuffix = string(v.Node.ID[len(v.Node.ID)-4:])
						}
						isActive := v.IsActive.Load()
						isAuth := v.Node.IsAuthenticated()
						log.Printf("  Validator ...%s: Rep = %d (Active: %t, Auth: %t)",
							idSuffix, v.Reputation.Load(), isActive, isAuth)
					}
				}
				log.Println("---------------------------")
			}
		}
	}()

	// Goroutine for periodic authentication challenges
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Second)
		ticker := time.NewTicker(45 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopSim:
				log.Println("[Auth Challenger] Stopping authentication challenges.")
				return
			case <-ticker.C:
				log.Println("[Auth Challenger] --- Starting Authentication Round ---")
				validatorsToChallenge := validatorMgr.GetAllValidators()
				var authWg sync.WaitGroup
				for _, v := range validatorsToChallenge {
					if !v.IsActive.Load() {
						continue
					}

					authWg.Add(1)
					go func(validator *core.Validator) {
						defer authWg.Done()
						nodeID := validator.Node.ID
						challenge, err := validatorMgr.ChallengeValidator(nodeID)
						if err != nil {
							log.Printf("[Auth Challenger] Error creating challenge for %s: %v", nodeID, err)
							verifyErr := validatorMgr.VerifyResponse(nodeID, nil) // Corrected call
							if verifyErr != nil {
								log.Printf("[Auth Challenger] Error verifying nil response for %s after challenge error: %v", nodeID, verifyErr)
							}
							return
						}

						response, err := validator.Node.SignData(challenge)
						if err != nil {
							log.Printf("[Auth Challenger] Error getting response from %s: %v", nodeID, err)
							verifyErr := validatorMgr.VerifyResponse(nodeID, nil) // Corrected call
							if verifyErr != nil {
								log.Printf("[Auth Challenger] Error verifying nil response for %s after signing error: %v", nodeID, verifyErr)
							}
							return
						}

						time.Sleep(time.Duration(5+rand.Intn(20)) * time.Millisecond)
						verifyErr := validatorMgr.VerifyResponse(nodeID, response) // Corrected call
						if verifyErr != nil {
							log.Printf("[Auth Challenger] Error verifying response for %s: %v", nodeID, verifyErr)
						}

					}(v)
				}
				authWg.Wait()
				log.Println("[Auth Challenger] --- Authentication Round Finished ---")
			}
		}
	}()

	// Goroutine to monitor shard count changes
	go func() {
		lastShardCount := initialShards
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
	simTimer := time.NewTimer(simulationDuration)

	select {
	case <-sigChan:
		log.Println("Received shutdown signal.")
	case <-simTimer.C:
		log.Printf("Simulation duration (%s) reached.", simulationDuration)
	}

	log.Println("--- Shutdown Signal Received ---")

	log.Println("Stopping simulation goroutines...")
	close(stopSim)

	log.Println("Stopping Blockchain background processes (Consistency, ShardManager, Telemetry)...")
	consistencyOrchestrator.Stop()
	bc.ShardManager.StopManagementLoop() // Stops loop and managed shards
	telemetryMonitor.Stop()

	log.Println("Waiting for main simulation goroutines (TX Gen, Miner, Rep Printer, Auth Challenger) to finish...")
	wg.Wait()
	log.Println("Main simulation goroutines finished.")

	log.Println("--- Simulation Finished ---")

	finalShardIDs := bc.ShardManager.GetAllShardIDs()
	log.Printf("Final active shard IDs: %v", finalShardIDs)

	for _, shardID := range finalShardIDs {
		if s, ok := bc.ShardManager.GetShard(shardID); ok {
			finalStateRoot, stateErr := s.StateDB.GetStateRoot() // Corrected method name
			if stateErr != nil {
				log.Printf("  Shard %d - Error getting final state root: %v", shardID, stateErr)
			}
			log.Printf("  Shard %d - Final State Size: %d, Final State Root: %x, Blocks: %d",
				shardID, s.Metrics.StateSize.Load(), finalStateRoot, s.Metrics.BlockCount.Load())

			bc.ChainMu.RLock()
			chain, chainExists := bc.BlockChains[shardID]
			var lastBlock *core.Block
			if chainExists && len(chain) > 0 {
				lastBlock = chain[len(chain)-1]
			}
			bc.ChainMu.RUnlock()

			if lastBlock != nil {
				proposerIDSuffix := string(lastBlock.Header.ProposerID)
				if len(proposerIDSuffix) > 4 {
					proposerIDSuffix = string(lastBlock.Header.ProposerID[len(lastBlock.Header.ProposerID)-4:])
				}
				log.Printf("    Shard %d - Last Block H:%d, Hash: %x..., Proposer: ...%s, Finalizers: %d",
					shardID, lastBlock.Header.Height, safeSlice(lastBlock.Hash, 4),
					proposerIDSuffix, len(lastBlock.Header.FinalitySignatures))
			} else {
				log.Printf("    Shard %d - No blocks found in blockchain map.", shardID)
			}
		}
	}
	log.Println("--- Final Validator Reputations ---")
	allValidators := validatorMgr.GetAllValidators()
	if len(allValidators) == 0 {
		log.Println("  No validators were registered.")
	} else {
		for _, v := range allValidators {
			idSuffix := string(v.Node.ID)
			if len(v.Node.ID) > 4 {
				idSuffix = string(v.Node.ID[len(v.Node.ID)-4:])
			}
			log.Printf("  Validator ...%s: Rep = %d (Auth: %t)", idSuffix, v.Reputation.Load(), v.Node.IsAuthenticated())
		}
	}
	log.Println("---------------------------------")
}
