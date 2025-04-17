package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

func main() {
	// Set PoW difficulty (e.g., number of leading zero bits required)
	// Higher number means significantly more difficult/slower mining.
	// Start with a low number like 10-16 for testing.
	difficulty := 16
	log.Printf("Starting blockchain with PoW difficulty: %d\n", difficulty)

	bc, err := core.NewBlockchain(difficulty)
	if err != nil {
		log.Fatalf("Failed to create blockchain: %v", err)
	}
	log.Println("Blockchain created successfully.")

	// Add some blocks with sample transactions
	log.Println("Adding Block 1...")
	tx1 := core.NewTransaction([]byte("Alice sends 10 to Bob"))
	tx2 := core.NewTransaction([]byte("Charlie sends 5 to Alice"))
	err = bc.AddBlock([]*core.Transaction{tx1, tx2})
	if err != nil {
		log.Fatalf("Failed to add block 1: %v", err)
	}

	log.Println("Adding Block 2...")
	tx3 := core.NewTransaction([]byte("Bob sends 2 to Dave"))
	err = bc.AddBlock([]*core.Transaction{tx3})
	if err != nil {
		log.Fatalf("Failed to add block 2: %v", err)
	}

	log.Println("\n--- Blockchain Content ---")
	for _, block := range bc.Blocks {
		fmt.Printf("--- Block %d ---\n", block.Height)
		fmt.Printf("Timestamp:      %d\n", block.Timestamp)
		fmt.Printf("Previous Hash:  %x\n", block.PrevBlockHash)
		fmt.Printf("Merkle Root:    %x\n", block.MerkleRoot)
		fmt.Printf("Nonce:          %d\n", block.Nonce)
		fmt.Printf("Hash:           %x\n", block.Hash)
		pow := core.NewProofOfWork(block, bc.Difficulty) // Recreate PoW context for validation check
		fmt.Printf("PoW Valid:      %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println("Transactions:")
		for j, tx := range block.Transactions {
			fmt.Printf("  Tx %d: ID=%x, Data=%s\n", j, tx.ID, string(tx.Data))
		}
		fmt.Println()
	}

	log.Println("\n--- Validating Chain Integrity ---")
	isValid := bc.IsChainValid()
	log.Printf("Blockchain valid: %t\n", isValid)
}
