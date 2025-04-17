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

	"github.com/willf/bloom" // Import Bloom filter library
)

// Block represents a single unit in the blockchain, now associated with a shard.
type Block struct {
	ShardID       ShardID // ID of the shard this block belongs to
	Timestamp     int64
	Transactions  []*Transaction // Holds the actual transactions
	PrevBlockHash []byte         // Hash of the previous block *in the same shard*
	Hash          []byte
	MerkleRoot    []byte           // Root hash of the Merkle tree of transactions
	Nonce         int64            // Counter for Proof-of-Work
	Height        int              // Block height *within its shard's chain*
	BloomFilter   *BloomFilterData // Bloom filter for transaction lookup (AMQ)
}

// BloomFilterData holds the serialized data of a Bloom filter for gob encoding.
type BloomFilterData struct {
	M      uint
	K      uint
	BitSet []byte // Raw bitset data
}

// NewBlock creates and returns a new block for a specific shard.
// It calculates the Merkle root, builds the Bloom filter, and performs Proof-of-Work.
func NewBlock(shardID ShardID, transactions []*Transaction, prevBlockHash []byte, height int, difficulty int) *Block {
	block := &Block{
		ShardID:       shardID,
		Timestamp:     time.Now().Unix(),
		Transactions:  transactions,
		PrevBlockHash: prevBlockHash,
		Hash:          []byte{},
		Nonce:         0,
		Height:        height,
	}
	block.MerkleRoot = block.CalculateMerkleRoot()
	block.BloomFilter = block.BuildBloomFilter() // Build Bloom filter

	// PoW remains largely the same, but might consider shard-specific difficulty later
	pow := NewProofOfWork(block, difficulty)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// NewGenesisBlock creates the first block for a specific shard.
func NewGenesisBlock(shardID ShardID, coinbase *Transaction, difficulty int) *Block {
	if coinbase == nil {
		coinbase = NewTransaction([]byte(fmt.Sprintf("Genesis Block Coinbase Shard %d", shardID)))
	}
	// Genesis block has height 0 and no previous hash within its shard chain
	return NewBlock(shardID, []*Transaction{coinbase}, []byte{}, 0, difficulty)
}

// CalculateMerkleRoot computes the Merkle root for the block's transactions.
func (b *Block) CalculateMerkleRoot() []byte {
	txIDs := GetTransactionIDs(b.Transactions)
	if len(txIDs) == 0 {
		emptyHash := sha256.Sum256([]byte{})
		return emptyHash[:]
	}
	tree := NewMerkleTree(txIDs)
	return tree.RootNode.Data
}

// --- Bloom Filter Integration (Ticket 2) ---

// BuildBloomFilter creates a Bloom filter containing IDs of transactions in the block.
// Parameters m and k determine size and hash functions, impacting accuracy and size.
// These should be chosen based on expected number of items and desired false positive rate.
func (b *Block) BuildBloomFilter() *BloomFilterData {
	// Estimate parameters (example values, tune based on requirements)
	// Estimate n = max items (e.g., max transactions per block)
	n := uint(1000) // Example capacity
	// Desired false positive probability (e.g., 1%)
	fpRate := 0.01

	// Calculate optimal m (size in bits) and k (number of hash functions)
	m, k := bloom.EstimateParameters(n, fpRate)

	filter := bloom.New(m, k)

	for _, tx := range b.Transactions {
		filter.Add(tx.ID) // Add transaction ID to the filter
	}

	// Prepare for serialization
	bitSetBytes, err := filter.GobEncode()
	if err != nil {
		log.Printf("Warning: Failed to encode bloom filter: %v. Filter will be empty.", err)
		return &BloomFilterData{M: m, K: k, BitSet: []byte{}}
	}

	return &BloomFilterData{
		M:      m,
		K:      k,
		BitSet: bitSetBytes,
	}
}

// GetBloomFilter reconstructs the Bloom filter from serialized data.
// Returns nil if the serialized data is invalid or missing.
func (b *Block) GetBloomFilter() *bloom.BloomFilter {
	if b.BloomFilter == nil || len(b.BloomFilter.BitSet) == 0 {
		// log.Println("Block has no Bloom filter data.")
		return nil // No filter available or it was empty
	}

	filter := bloom.New(b.BloomFilter.M, b.BloomFilter.K)
	err := filter.GobDecode(b.BloomFilter.BitSet)
	if err != nil {
		log.Printf("Error unmarshalling Bloom filter: %v", err)
		return nil // Invalid filter data
	}
	return filter
}

// CheckBloomFilter checks if an item might be in the block using the Bloom filter.
// Returns true if the item *might* be present (could be a false positive).
// Returns false if the item is *definitely not* present.
func (b *Block) CheckBloomFilter(item []byte) bool {
	filter := b.GetBloomFilter()
	if filter == nil {
		// If no filter, cannot perform check reliably. Depending on policy,
		// return true (assume it might exist) or false (assume check failed).
		// Let's assume it might exist if filter is missing/invalid.
		log.Println("Bloom filter check skipped: Filter unavailable.")
		return true
	}
	return filter.Test(item)
}

// --- Serialization ---

// Serialize serializes the block using gob encoding.
func (b *Block) Serialize() []byte {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)
	// Encode the entire block struct, including the new BloomFilterData
	err := encoder.Encode(b)
	if err != nil {
		log.Panicf("Failed to encode block: %v", err)
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

// --- Proof of Work (Update data preparation if needed) ---

// prepareData now includes ShardID and Bloom filter representation
func (pow *ProofOfWork) prepareData(nonce int64) []byte {
	bloomBytes := []byte{}
	if pow.block.BloomFilter != nil && len(pow.block.BloomFilter.BitSet) > 0 {
		// Use a stable representation of the filter data for hashing
		// We can just use the serialized bitset. M and K are implicitly part of the hash
		// if the deserialization relies on them being correct.
		bloomBytes = pow.block.BloomFilter.BitSet
		// Alternatively, serialize M, K, and BitSet explicitly here if needed
	}

	data := bytes.Join(
		[][]byte{
			pow.block.PrevBlockHash,
			pow.block.MerkleRoot,
			bloomBytes, // Include Bloom filter data
			[]byte(fmt.Sprintf("%d", pow.block.ShardID)), // Include ShardID
			[]byte(fmt.Sprintf("%d", pow.block.Timestamp)),
			[]byte(fmt.Sprintf("%d", pow.block.Height)),
			[]byte(fmt.Sprintf("%d", nonce)),
			[]byte(fmt.Sprintf("%d", pow.target.BitLen())),
		},
		[]byte{},
	)
	return data
}

// ProofOfWork struct and NewProofOfWork remain the same structurally
const maxNonce = math.MaxInt64

type ProofOfWork struct {
	block  *Block
	target *big.Int
}

func NewProofOfWork(b *Block, difficulty int) *ProofOfWork {
	target := big.NewInt(1)
	target.Lsh(target, uint(256-difficulty))
	pow := &ProofOfWork{b, target}
	return pow
}

// Run and Validate methods remain the same, using the updated prepareData implicitly.
func (pow *ProofOfWork) Run() (int64, []byte) { /* ... unchanged ... */
	var hashInt big.Int
	var hash [32]byte
	var nonce int64 = 0
	startTime := time.Now()
	for nonce < maxNonce {
		data := pow.prepareData(nonce)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:])
		if hashInt.Cmp(pow.target) == -1 {
			duration := time.Since(startTime)
			if len(pow.block.Transactions) > 0 { // Avoid printing for empty blocks if any
				fmt.Printf("Shard %d: Mined block %d! Hash: %x, Nonce: %d, Duration: %s\n", pow.block.ShardID, pow.block.Height, hash, nonce, duration)
			}
			break
		} else {
			nonce++
		}
	}
	fmt.Println()
	if nonce == maxNonce {
		log.Printf("Warning: Shard %d PoW failed for block %d.\n", pow.block.ShardID, pow.block.Height)
		return -1, []byte{}
	}
	return nonce, hash[:]
}
func (pow *ProofOfWork) Validate() bool { /* ... unchanged ... */
	var hashInt big.Int
	data := pow.prepareData(pow.block.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])
	isValid := hashInt.Cmp(pow.target) == -1
	return isValid
}
