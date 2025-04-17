package main

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

func main() {
	difficulty := 16     // PoW difficulty
	numShards := uint(4) // Number of shards
	log.Printf("Starting sharded blockchain with %d shards, difficulty: %d\n", numShards, difficulty)

	bc, err := core.NewBlockchain(numShards, difficulty)
	if err != nil {
		log.Fatalf("Failed to create blockchain: %v", err)
	}
	log.Println("Blockchain created successfully.")

	// --- Simulate adding transactions ---
	log.Println("\n--- Adding Transactions ---")
	numTx := 20
	txs := make([]*core.Transaction, numTx)
	for i := 0; i < numTx; i++ {
		// Introduce some cross-shard transactions randomly
		if rand.Intn(5) == 0 && numShards > 1 { // ~20% chance if multiple shards exist
			source := core.ShardID(rand.Intn(int(numShards)))
			dest := core.ShardID(rand.Intn(int(numShards)))
			// Ensure source != dest for a meaningful cross-shard tx
			for dest == source {
				dest = core.ShardID(rand.Intn(int(numShards)))
			}
			txs[i] = core.NewCrossShardInitTransaction(
				[]byte(fmt.Sprintf("Cross-Shard Data %d (S%d->S%d)", i, source, dest)),
				source,
				dest,
			)
			log.Printf("Created Cross-Shard Tx %x (Shard %d -> Shard %d)", txs[i].ID, source, dest)
		} else {
			txs[i] = core.NewTransaction([]byte(fmt.Sprintf("Regular Transaction Data %d", i)))
			log.Printf("Created Intra-Shard Tx %x", txs[i].ID)
		}

		err := bc.AddTransaction(txs[i])
		if err != nil {
			log.Printf("Warning: Failed to add transaction %x: %v", txs[i].ID, err)
		}
		time.Sleep(5 * time.Millisecond) // Small delay
	}

	// --- Simulate mining blocks across shards concurrently ---
	log.Println("\n--- Mining Blocks Across Shards ---")
	var wg sync.WaitGroup
	miningRounds := 3

	for round := 0; round < miningRounds; round++ {
		log.Printf("\n--- Mining Round %d ---", round+1)
		for i := uint(0); i < numShards; i++ {
			wg.Add(1)
			go func(shardID core.ShardID) {
				defer wg.Done()
				log.Printf("Starting mining for Shard %d...", shardID)
				_, err := bc.MineShardBlock(shardID)
				if err != nil {
					// Don't treat "no transactions" as fatal in demo if empty blocks are allowed/handled
					if err.Error() != "no transactions available for mining" { // Example error check
						log.Printf("Warning: Mining failed for Shard %d: %v", shardID, err)
					} else {
						log.Printf("Shard %d: Mining skipped/empty block (%v)", shardID, err)
					}
				}
			}(core.ShardID(i))
		}
		wg.Wait() // Wait for all shards in this round to finish mining attempt
		log.Printf("--- Mining Round %d Complete ---", round+1)
		time.Sleep(50 * time.Millisecond) // Pause between rounds
	}

	log.Println("\n--- Blockchain Content (Per Shard) ---")
	bc.Mu.RLock() // Lock for reading BlockChains map
	defer bc.Mu.RUnlock()
	for shardID, chain := range bc.BlockChains {
		fmt.Printf("\n======= Shard %d =======\n", shardID)
		for _, block := range chain {
			fmt.Printf("--- Block %d ---\n", block.Height)
			fmt.Printf("Timestamp:      %d\n", block.Timestamp)
			fmt.Printf("Previous Hash:  %x\n", block.PrevBlockHash)
			fmt.Printf("Merkle Root:    %x\n", block.MerkleRoot)
			// Display Bloom Filter Info (Optional)
			if block.BloomFilter != nil {
				bf := block.GetBloomFilter()
				bfInfo := "N/A"
				if bf != nil {
					// The willf/bloom library doesn't have EstimateOccupancy, so just show M and K
					bfInfo = fmt.Sprintf("M=%d, K=%d", block.BloomFilter.M, block.BloomFilter.K)
				}
				fmt.Printf("Bloom Filter:   %s\n", bfInfo)
			} else {
				fmt.Printf("Bloom Filter:   None\n")
			}
			fmt.Printf("Nonce:          %d\n", block.Nonce)
			fmt.Printf("Hash:           %x\n", block.Hash)
			pow := core.NewProofOfWork(block, bc.Difficulty)
			fmt.Printf("PoW Valid:      %s\n", strconv.FormatBool(pow.Validate()))
			fmt.Printf("Transactions (%d):\n", len(block.Transactions))
			for j, tx := range block.Transactions {
				txTypeStr := "IntraShard"
				if tx.Type == core.CrossShardTxInit {
					txTypeStr = "CrossShardInit"
				}
				if tx.Type == core.CrossShardTxFinalize {
					txTypeStr = "CrossShardFinalize"
				}
				fmt.Printf("  Tx %d: [%s] ID=%x, Data=%s", j, txTypeStr, tx.ID, string(tx.Data))
				if tx.SourceShard != nil && tx.DestinationShard != nil {
					fmt.Printf(" (S%d->S%d)", *tx.SourceShard, *tx.DestinationShard)
				}
				fmt.Println()
			}
			fmt.Println()

			// Bloom filter check example
			if len(block.Transactions) > 0 {
				checkTx := block.Transactions[0]
				maybePresent := block.CheckBloomFilter(checkTx.ID)
				fmt.Printf("Bloom Check (Tx %x exists?): %t\n", checkTx.ID[:4], maybePresent) // Check first tx ID
				maybePresent = block.CheckBloomFilter([]byte("non-existent tx"))
				fmt.Printf("Bloom Check ('non-existent tx' exists?): %t\n", maybePresent)
				fmt.Println()
			}
		}
	}

	log.Println("\n--- Validating All Shard Chains ---")
	isValid := bc.IsChainValid()
	log.Printf("Overall Blockchain valid: %t\n", isValid)

	// --- Demonstrate Pruning ---
	pruneHeight := 1 // Prune blocks below height 1 (i.e., prune genesis block)
	log.Printf("\n--- Attempting pruning below height %d ---", pruneHeight)
	bc.PruneChain(pruneHeight)
	log.Printf("--- Pruning complete ---")

	log.Println("\n--- Blockchain Content After Pruning ---")
	// Print chain content again or just check lengths
	bc.Mu.RLock()
	for shardID, chain := range bc.BlockChains {
		log.Printf("Shard %d length after pruning: %d", shardID, len(chain))
	}
	bc.Mu.RUnlock()
}
