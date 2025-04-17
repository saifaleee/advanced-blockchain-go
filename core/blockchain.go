package core

import (
	"bytes"
	"errors"
	"fmt"
	"log"
)

// Blockchain keeps a sequence of Blocks.
// For simplicity, this is an in-memory structure.
// A real implementation would use a database (e.g., LevelDB).
type Blockchain struct {
	Blocks     []*Block
	Difficulty int // PoW difficulty
}

// NewBlockchain creates a new blockchain with a genesis block.
func NewBlockchain(difficulty int) (*Blockchain, error) {
	if difficulty < 1 {
		return nil, errors.New("difficulty must be at least 1")
	}
	genesis := NewGenesisBlock(nil, difficulty) // Use default genesis transaction
	return &Blockchain{
		Blocks:     []*Block{genesis},
		Difficulty: difficulty,
	}, nil
}

// AddBlock adds a new block to the blockchain.
func (bc *Blockchain) AddBlock(transactions []*Transaction) error {
	if len(transactions) == 0 {
		return errors.New("cannot add block with no transactions")
	}

	prevBlock := bc.GetLatestBlock()
	if prevBlock == nil {
		return errors.New("blockchain has no blocks to build upon") // Should not happen after NewBlockchain
	}

	newHeight := prevBlock.Height + 1
	newBlock := NewBlock(transactions, prevBlock.Hash, newHeight, bc.Difficulty)

	// Basic validation before adding
	pow := NewProofOfWork(newBlock, bc.Difficulty)
	if !pow.Validate() {
		return fmt.Errorf("new block failed proof-of-work validation")
	}
	if !ValidateBlockIntegrity(newBlock, prevBlock) {
		return fmt.Errorf("new block failed integrity validation against previous block")
	}

	bc.Blocks = append(bc.Blocks, newBlock)
	log.Printf("Added Block %d\n", newBlock.Height)
	return nil
}

// GetLatestBlock returns the most recent block in the chain.
func (bc *Blockchain) GetLatestBlock() *Block {
	if len(bc.Blocks) == 0 {
		return nil
	}
	return bc.Blocks[len(bc.Blocks)-1]
}

// ValidateBlockIntegrity performs basic checks on a new block relative to the previous one.
func ValidateBlockIntegrity(newBlock, prevBlock *Block) bool {
	if newBlock == nil || prevBlock == nil {
		log.Println("Error: Cannot validate nil blocks.")
		return false
	}
	// Check previous hash link
	if !bytes.Equal(newBlock.PrevBlockHash, prevBlock.Hash) {
		log.Printf("Validation Error: Block %d PrevBlockHash (%x) does not match Block %d Hash (%x)\n",
			newBlock.Height, newBlock.PrevBlockHash, prevBlock.Height, prevBlock.Hash)
		return false
	}
	// Check height
	if newBlock.Height != prevBlock.Height+1 {
		log.Printf("Validation Error: Block %d Height is not sequential to previous block height %d\n",
			newBlock.Height, prevBlock.Height)
		return false
	}

	// Re-calculate hash to ensure block data wasn't tampered with after PoW
	// Note: This requires the *same* difficulty used during mining. If difficulty changes,
	// this simple check needs adjustment. PoW validation handles hash correctness vs target.
	// pow := NewProofOfWork(newBlock, difficulty) // Need difficulty context here...
	// _, expectedHash := pow.Run() // Re-running PoW is inefficient; Validate is better
	// We rely on pow.Validate() called before adding the block instead.

	log.Printf("Block %d integrity validated successfully against Block %d.\n", newBlock.Height, prevBlock.Height)
	return true
}

// IsChainValid iterates through the blockchain and validates each block's hash and links.
func (bc *Blockchain) IsChainValid() bool {
	if len(bc.Blocks) == 0 {
		log.Println("Cannot validate an empty chain.")
		return false // Or true, depending on definition
	}
	if len(bc.Blocks) == 1 {
		// Validate Genesis Block PoW
		genesis := bc.Blocks[0]
		powGenesis := NewProofOfWork(genesis, bc.Difficulty)
		if !powGenesis.Validate() {
			log.Printf("Genesis block PoW validation failed.")
			return false
		}
		return true
	}

	for i := 1; i < len(bc.Blocks); i++ {
		currentBlock := bc.Blocks[i]
		prevBlock := bc.Blocks[i-1]

		// Validate current block's PoW
		pow := NewProofOfWork(currentBlock, bc.Difficulty)
		if !pow.Validate() {
			log.Printf("PoW validation failed for Block %d (Hash: %x)\n", currentBlock.Height, currentBlock.Hash)
			return false
		}

		// Validate integrity link with previous block
		if !ValidateBlockIntegrity(currentBlock, prevBlock) {
			log.Printf("Integrity validation failed between Block %d and Block %d\n", currentBlock.Height, prevBlock.Height)
			return false
		}
	}
	log.Println("Blockchain validation successful.")
	return true
}
