package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"log"
	"math"
	"math/big"
	"time"
)

// Block represents a single unit in the blockchain.
type Block struct {
	Timestamp     int64
	Transactions  []*Transaction // Holds the actual transactions
	PrevBlockHash []byte
	Hash          []byte
	MerkleRoot    []byte // Root hash of the Merkle tree of transactions
	Nonce         int64  // Counter for Proof-of-Work
	Height        int    // Block height in the chain
}

// NewBlock creates and returns a new block.
// It calculates the Merkle root and performs Proof-of-Work.
func NewBlock(transactions []*Transaction, prevBlockHash []byte, height int, difficulty int) *Block {
	block := &Block{
		Timestamp:     time.Now().Unix(),
		Transactions:  transactions,
		PrevBlockHash: prevBlockHash,
		Hash:          []byte{},
		Nonce:         0,
		Height:        height,
	}
	block.MerkleRoot = block.CalculateMerkleRoot()
	pow := NewProofOfWork(block, difficulty)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// NewGenesisBlock creates the first block in the chain.
func NewGenesisBlock(coinbase *Transaction, difficulty int) *Block {
	// Typically, the genesis block contains a "coinbase" transaction.
	// We use a simple transaction here for demonstration.
	if coinbase == nil {
		coinbase = NewTransaction([]byte("Genesis Block Coinbase"))
	}
	return NewBlock([]*Transaction{coinbase}, []byte{}, 0, difficulty) // Height 0, no previous hash
}

// CalculateMerkleRoot computes the Merkle root for the block's transactions.
func (b *Block) CalculateMerkleRoot() []byte {
	txIDs := GetTransactionIDs(b.Transactions)
	if len(txIDs) == 0 {
		// Handle blocks with no transactions (e.g., potentially valid in some chains)
		// Return a hash of an empty string or some placeholder
		emptyHash := sha256.Sum256([]byte{})
		return emptyHash[:]
	}
	tree := NewMerkleTree(txIDs)
	return tree.RootNode.Data
}

// Serialize serializes the block using gob encoding.
func (b *Block) Serialize() []byte {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)
	err := encoder.Encode(b)
	if err != nil {
		log.Panicf("Failed to encode block: %v", err) // Panic on serialization error
	}
	return result.Bytes()
}

// DeserializeBlock deserializes a block from bytes.
func DeserializeBlock(d []byte) (*Block, error) {
	var block Block
	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)
	if err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}
	return &block, nil
}

// --- Proof of Work ---

const maxNonce = math.MaxInt64 // Maximum value for nonce

// ProofOfWork represents the PoW computation context.
type ProofOfWork struct {
	block  *Block
	target *big.Int // Target threshold (hash must be less than this)
}

// NewProofOfWork creates a new ProofOfWork instance.
func NewProofOfWork(b *Block, difficulty int) *ProofOfWork {
	target := big.NewInt(1)
	// Left-shift 1 by (256 - difficulty) bits.
	// The higher the difficulty, the smaller the target number.
	target.Lsh(target, uint(256-difficulty))

	pow := &ProofOfWork{b, target}
	return pow
}

// prepareData concatenates block fields for hashing in PoW.
func (pow *ProofOfWork) prepareData(nonce int64) []byte {
	data := bytes.Join(
		[][]byte{
			pow.block.PrevBlockHash,
			pow.block.MerkleRoot, // Use MerkleRoot instead of hashing all tx data directly
			[]byte(fmt.Sprintf("%d", pow.block.Timestamp)),
			[]byte(fmt.Sprintf("%d", pow.block.Height)),
			[]byte(fmt.Sprintf("%d", nonce)),               // Include nonce
			[]byte(fmt.Sprintf("%d", pow.target.BitLen())), // Include target difficulty bits representation
		},
		[]byte{},
	)
	return data
}

// Run performs the Proof-of-Work algorithm.
// It finds a nonce such that the block hash is less than the target.
func (pow *ProofOfWork) Run() (int64, []byte) {
	var hashInt big.Int
	var hash [32]byte
	var nonce int64 = 0

	fmt.Printf("Mining block with %d transactions...\n", len(pow.block.Transactions))
	startTime := time.Now()

	for nonce < maxNonce {
		data := pow.prepareData(nonce)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:]) // Convert hash bytes to big.Int

		// Compare the hash integer with the target integer
		// Cmp returns -1 if hashInt < target, 0 if equal, 1 if hashInt > target
		if hashInt.Cmp(pow.target) == -1 {
			duration := time.Since(startTime)
			fmt.Printf("Block mined! Hash: %x, Nonce: %d, Duration: %s\n", hash, nonce, duration)
			break // Found a valid nonce
		} else {
			nonce++
		}

		// Optional: Print progress periodically
		// if nonce%100000 == 0 {
		//  fmt.Printf("\rMining... Nonce: %d, Hash: %x", nonce, hash[:])
		// }
	}
	fmt.Println() // Newline after mining completes or loop finishes

	if nonce == maxNonce {
		log.Println("Warning: Proof-of-Work failed to find a solution within maxNonce iterations.")
		// Handle this case appropriately - maybe return an error or a special value
		return -1, []byte{}
	}

	return nonce, hash[:]
}

// Validate checks if the Proof-of-Work is valid for the block.
func (pow *ProofOfWork) Validate() bool {
	var hashInt big.Int
	data := pow.prepareData(pow.block.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	isValid := hashInt.Cmp(pow.target) == -1
	return isValid
}
