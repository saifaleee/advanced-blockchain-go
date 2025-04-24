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

	"github.com/saifaleee/advanced-blockchain-go/core" // Import your core package
)

// safeSlice safely returns up to n bytes from a slice, handles empty slices
func safeSlice(data []byte, n int) []byte {
	if len(data) < n {
		return data
	}
	return data[:n]
}

func main() {
	log.Println("--- Advanced Go Blockchain PoC - Main Simulation ---")
	rand.Seed(time.Now().UnixNano())

	// --- Configuration ---
	initialShards := 2
	// Configure dynamic sharding thresholds LOW for demonstration purposes
	smConfig := core.ShardManagerConfig{
		SplitThresholdStateSize:   5,                // Split if > 5 keys in state DB
		SplitThresholdTxPool:      10,               // Split if > 10 txs in pool
		MergeThresholdStateSize:   2,                // Merge if < 2 keys in state DB (Shard A)
		MergeTargetThresholdSize:  2,                // AND target shard B also < 2 keys
		CheckInterval:             10 * time.Second, // Check metrics every 10 seconds
		NumReplicasConsistentHash: 20,
		MaxShards:                 5, // Limit max shards for demo
		MinShards:                 1, // Need at least 1 shard
	}
	log.Printf("ShardManager Config: %+v\n", smConfig)

	// --- Initialization ---
	log.Println("Initializing Blockchain...")
	bc, err := core.NewBlockchain(uint(initialShards), smConfig)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	log.Printf("Blockchain initialized with %d shards. Initial IDs: %v\n", initialShards, bc.ShardManager.GetAllShardIDs())

	// --- Start Dynamic Sharding Management ---
	log.Println("Starting Shard Manager background loop...")
	bc.ShardManager.StartManagementLoop()

	// --- Simulation Control ---
	var wg sync.WaitGroup
	stopSim := make(chan struct{}) // Channel to signal simulation loops to stop

	// Goroutine for generating transactions
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[TX Generator] Starting...")
		ticker := time.NewTicker(2 * time.Second) // Generate TXs every 2 seconds
		defer ticker.Stop()
		txCounter := 0

		for {
			select {
			case <-stopSim:
				log.Println("[TX Generator] Stopping...")
				return
			case <-ticker.C:
				numTx := rand.Intn(4) + 1 // Generate 1-4 transactions per tick
				for i := 0; i < numTx; i++ {
					txCounter++
					var tx *core.Transaction
					txTypeRand := rand.Float32()
					shardIDs := bc.ShardManager.GetAllShardIDs() // Get current shards for cross-shard TXs
					numActiveShards := len(shardIDs)

					// Create different transaction types
					if numActiveShards > 1 && txTypeRand < 0.3 { // 30% chance of cross-shard TX if possible
						sourceShardIdx := rand.Intn(numActiveShards)
						destShardIdx := rand.Intn(numActiveShards)
						// Ensure source and destination are different
						for destShardIdx == sourceShardIdx {
							destShardIdx = rand.Intn(numActiveShards)
						}
						destShardID := shardIDs[destShardIdx]
						data := []byte(fmt.Sprintf("Cross-Shard Data %d to Shard %d", txCounter, destShardID))
						tx = core.NewTransaction(data, core.CrossShardTxInit, &destShardID)
						log.Printf("[TX Generator] Created CrossShardInitiate TX #%d (Dest: %d)", txCounter, destShardID)
					} else { // Intra-shard transaction
						data := []byte(fmt.Sprintf("Intra-Shard Data %d", txCounter))
						tx = core.NewTransaction(data, core.IntraShard, nil)
						// log.Printf("[TX Generator] Created IntraShard TX #%d", txCounter)
					}

					// Basic placeholder signing
					_ = tx.Sign([]byte("dummy-private-key"))

					// Add transaction to the blockchain (will be routed by ShardManager)
					addErr := bc.AddTransaction(tx)
					if addErr != nil {
						log.Printf("[TX Generator] Error adding TX #%d: %v", txCounter, addErr)
					} else {
						// Log without the routed shard ID since it's not returned anymore
						log.Printf("[TX Generator] Added TX #%d (Type: %d) via router. TX ID: %x...", txCounter, tx.Type, safeSlice(tx.ID, 4))
					}
					time.Sleep(50 * time.Millisecond) // Small delay between TXs
				}
			}
		}
	}()

	// Goroutine for triggering mining periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("[Miner] Starting...")
		// Mine more frequently than metric checks to build up state/load
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopSim:
				log.Println("[Miner] Stopping...")
				return
			case <-ticker.C:
				shardIDs := bc.ShardManager.GetAllShardIDs()
				if len(shardIDs) == 0 {
					log.Println("[Miner] No active shards to mine.")
					continue
				}
				// In a real system, nodes would mine their assigned shards.
				// Here, we simulate by picking one or iterating. Let's mine all.
				log.Printf("[Miner] Triggering mining round for shards: %v", shardIDs)
				for _, shardID := range shardIDs {
					// Check if shard still exists (might have been merged)
					if _, exists := bc.ShardManager.GetShard(shardID); !exists {
						log.Printf("[Miner] Shard %d no longer exists, skipping mining.", shardID)
						continue
					}

					log.Printf("[Miner] Attempting to mine block for Shard %d...", shardID)
					minedBlock, mineErr := bc.MineShardBlock(shardID)
					if mineErr != nil {
						if mineErr.Error() == "no transactions to mine" { // Handle expected case gracefully
							log.Printf("[Miner] No transactions to mine for Shard %d.", shardID)
						} else {
							log.Printf("[Miner] Error mining block for Shard %d: %v", shardID, mineErr)
						}
					} else if minedBlock != nil {
						log.Printf("[Miner] Successfully mined Block H:%d for Shard %d! Hash: %x..., TXs: %d, StateRoot: %x...",
							minedBlock.Header.Height, shardID, safeSlice(minedBlock.Hash, 4), len(minedBlock.Transactions), safeSlice(minedBlock.Header.StateRoot, 4))
						// Print metrics after mining
						if s, ok := bc.ShardManager.GetShard(shardID); ok {
							log.Printf("[Metrics] Shard %d - StateSize: %d, PendingTxPool: %d", shardID, s.Metrics.StateSize.Load(), s.Metrics.PendingTxPool.Load())
						}
					}
					time.Sleep(100 * time.Millisecond) // Small delay between mining shards in simulation
				}
			}
		}
	}()

	// --- Run Simulation & Handle Shutdown ---
	log.Println(">>> Simulation running... Press Ctrl+C to stop. <<<")
	// Keep track of shard count changes
	go func() {
		lastShardCount := initialShards
		ticker := time.NewTicker(smConfig.CheckInterval + 1*time.Second) // Check slightly after management loop runs
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

	// Wait for interrupt signal (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan // Block until signal is received

	log.Println("--- Shutdown Signal Received ---")

	// Signal simulation goroutines to stop
	log.Println("Stopping simulation goroutines...")
	close(stopSim)

	// Wait for simulation goroutines to finish
	log.Println("Waiting for goroutines to finish...")
	wg.Wait()
	log.Println("Goroutines finished.")

	// Stop the shard manager loop
	log.Println("Stopping Shard Manager background loop...")
	bc.ShardManager.StopManagementLoop() // This also stops individual shards

	log.Println("--- Simulation Finished ---")

	// Optional: Print final state summary
	finalShardIDs := bc.ShardManager.GetAllShardIDs()
	log.Printf("Final active shard IDs: %v", finalShardIDs)
	for _, shardID := range finalShardIDs {
		if s, ok := bc.ShardManager.GetShard(shardID); ok {
			finalStateRoot, _ := s.StateDB.GetStateRoot()
			log.Printf("  Shard %d - Final State Size: %d, Final State Root: %x",
				shardID, s.Metrics.StateSize.Load(), finalStateRoot)

			// You could also try and access some blockchain data if needed
			bc.ChainMu.RLock()
			chain, exists := bc.BlockChains[shardID]
			bc.ChainMu.RUnlock()

			if exists && len(chain) > 0 {
				lastBlock := chain[len(chain)-1]
				log.Printf("    Shard %d - Last Block Height: %d, Hash: %x...", shardID, lastBlock.Header.Height, safeSlice(lastBlock.Hash, 4))
			} else {
				log.Printf("    Shard %d - No blocks found (or chain not accessible).", shardID)
			}
		}
	}
}
