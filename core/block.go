package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"log"
	"math/big"
	"time"

	// Assuming merkle is here
	"github.com/willf/bloom" // Import Bloom filter
)

const powTargetBits = 16 // Proof-of-work difficulty (adjust as needed)

// BlockHeader represents the header part of a block.
type BlockHeader struct {
	ShardID       uint64
	Timestamp     int64
	PrevBlockHash []byte
	MerkleRoot    []byte // Root hash of transactions in this block
	StateRoot     []byte // Root hash of the shard's state AFTER applying this block (placeholder for SMT)
	Nonce         int64
	Height        uint64
	Difficulty    int    // PoW difficulty target for this block
	BloomFilter   []byte // Serialized Bloom filter data
}

// Block represents a block in the blockchain.
type Block struct {
	Header       *BlockHeader
	Transactions []*Transaction
	Hash         []byte // Hash of the block header
}

// ProofOfWork holds the target for PoW and links to the block.
type ProofOfWork struct {
	block  *Block
	target *big.Int // Target threshold for the hash
}

// NewProofOfWork creates a new ProofOfWork instance.
func NewProofOfWork(b *Block) *ProofOfWork {
	target := big.NewInt(1)
	// Lsh: target << (256 - difficulty_bits)
	target.Lsh(target, uint(256-b.Header.Difficulty))
	pow := &ProofOfWork{b, target}
	return pow
}

// prepareData prepares the data to be hashed for PoW.
// It includes fields from the header EXCEPT the Nonce and Hash itself.
func (pow *ProofOfWork) prepareData(nonce int64) []byte {
	header := pow.block.Header
	data := bytes.Join(
		[][]byte{
			header.PrevBlockHash,
			header.MerkleRoot,
			header.StateRoot, // Include StateRoot in PoW
			[]byte(fmt.Sprintf("%d", header.Timestamp)),
			[]byte(fmt.Sprintf("%d", header.ShardID)),
			[]byte(fmt.Sprintf("%d", header.Height)),
			[]byte(fmt.Sprintf("%d", header.Difficulty)),
			header.BloomFilter,               // Include Bloom filter data
			[]byte(fmt.Sprintf("%d", nonce)), // Include the nonce being tried
		},
		[]byte{},
	)
	return data
}

// Run performs the proof-of-work calculation.
func (pow *ProofOfWork) Run() (int64, []byte) {
	var hashInt big.Int
	var hash [32]byte
	var nonce int64 = 0

	// fmt.Printf("Mining block for shard %d with target %x\n", pow.block.Header.ShardID, pow.target.Bytes())
	startTime := time.Now()

	for nonce < 1<<62 { // Avoid overflow, add max iterations or timeout?
		data := pow.prepareData(nonce)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(pow.target) == -1 { // -1 if hashInt < target
			// fmt.Printf("Found hash: %x (Nonce: %d)\n", hash, nonce)
			break
		}
		nonce++
	}
	duration := time.Since(startTime)
	fmt.Printf("Shard %d mining took %s. Hash: %x (Nonce: %d)\n", pow.block.Header.ShardID, duration, hash, nonce)

	return nonce, hash[:]
}

// Validate checks if the block's hash is valid according to PoW.
func (pow *ProofOfWork) Validate() bool {
	var hashInt big.Int
	data := pow.prepareData(pow.block.Header.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	return hashInt.Cmp(pow.target) == -1
}

// NewBlock creates a new block.
func NewBlock(shardID uint64, transactions []*Transaction, prevBlockHash []byte, height uint64, stateRoot []byte, difficulty int) (*Block, error) {
	header := &BlockHeader{
		ShardID:       shardID,
		Timestamp:     time.Now().UnixNano(),
		PrevBlockHash: prevBlockHash,
		Height:        height,
		Difficulty:    difficulty,
		StateRoot:     stateRoot, // Pass the calculated state root
		// Nonce and BloomFilter will be set later
	}

	block := &Block{
		Header:       header,
		Transactions: transactions,
	}

	// Calculate Merkle Root
	txHashes := make([][]byte, len(transactions))
	for i, tx := range transactions {
		txHashes[i] = tx.Hash() // Use pre-calculated hash/ID
	}
	merkleTree, err := NewMerkleTree(txHashes)
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle tree: %w", err)
	}
	block.Header.MerkleRoot = merkleTree.GetMerkleRoot()

	// Create and Serialize Bloom Filter
	// Choose parameters (e.g., expected items 'n', false positive rate 'p')
	// These should likely be configurable or dynamically adjusted.
	n := uint(1000) // Example: Expected items
	p := 0.01       // Example: False positive rate
	filter := bloom.NewWithEstimates(n, p)
	for _, tx := range transactions {
		filter.Add(tx.Hash())
		// Potentially add other relevant data (involved addresses?)
	}
	var buf bytes.Buffer
	_, err = filter.WriteTo(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize bloom filter: %w", err)
	}
	block.Header.BloomFilter = buf.Bytes()

	// Perform Proof of Work
	pow := NewProofOfWork(block)
	nonce, hash := pow.Run()

	block.Header.Nonce = nonce
	block.Hash = hash // Store the hash of the header

	return block, nil
}

// NewGenesisBlock creates the first block for a specific shard.
func NewGenesisBlock(shardID uint64, coinbase *Transaction, difficulty int) *Block {
	if coinbase == nil {
		coinbase = NewTransaction([]byte(fmt.Sprintf("Genesis Block Coinbase Shard %d", shardID)), IntraShard, nil)
	}
	// Genesis block has height 0 and no previous hash within its shard chain
	// For genesis block, we use empty state root (initial state)
	emptyStateRoot := []byte{}
	block, err := NewBlock(shardID, []*Transaction{coinbase}, []byte{}, 0, emptyStateRoot, difficulty)
	if err != nil {
		// This should not happen for a genesis block, but handle gracefully
		log.Printf("Error creating genesis block: %v", err)
		// Create a minimal block directly as fallback
		block = &Block{
			Header: &BlockHeader{
				ShardID:    shardID,
				Timestamp:  time.Now().UnixNano(),
				Difficulty: difficulty,
			},
			Transactions: []*Transaction{coinbase},
		}
	}
	return block
}

// Serialize encodes the block into bytes.
func (b *Block) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(b)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize block: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeBlock decodes bytes into a block.
func DeserializeBlock(data []byte) (*Block, error) {
	var block Block
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&block)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block: %w", err)
	}
	return &block, nil
}

// GetTransactionMerkleProof finds a transaction and generates its Merkle proof.
func (b *Block) GetTransactionMerkleProof(txID []byte) ([][]byte, uint64, error) {
	txHashes := make([][]byte, len(b.Transactions))
	txIndex := -1
	for i, tx := range b.Transactions {
		txHashes[i] = tx.Hash()
		if bytes.Equal(tx.Hash(), txID) {
			txIndex = i
		}
	}

	if txIndex == -1 {
		return nil, 0, fmt.Errorf("transaction %x not found in block %x", txID, b.Hash)
	}

	merkleTree, err := NewMerkleTree(txHashes)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reconstruct merkle tree for proof: %w", err)
	}

	// Simplified Merkle proof - just return the hashes needed for verification
	// This is a placeholder; real implementation would generate proper inclusion path
	proofHashes := [][]byte{merkleTree.RootNode.Data}

	return proofHashes, uint64(txIndex), nil
}
