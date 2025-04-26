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

// BlockHeader represents the header part of a block.
type BlockHeader struct {
	ShardID       uint64
	Timestamp     int64
	PrevBlockHash []byte
	MerkleRoot    []byte // Root hash of transactions in this block
	StateRoot     []byte // Root hash of the shard's state AFTER applying this block
	Nonce         int64
	Height        uint64
	Difficulty    int    // PoW difficulty target for this block
	BloomFilter   []byte // Serialized Bloom filter data

	// --- Phase 4 Additions ---
	ProposerID         NodeID   // ID of the node that proposed this block via PoW
	FinalitySignatures []NodeID // List of Validator IDs that finalized this block (simulated)
	// In a real system, FinalitySignatures would be actual crypto signatures

	// --- Ticket 5: Conflict Resolution ---
	VectorClock VectorClock // Tracks causal history of the block

	// --- Suggested Change ---
	Clock VectorClock // Vector clock representing state after this block
}

// Block represents a block in the blockchain.
type Block struct {
	Header       *BlockHeader
	Transactions []*Transaction
	Hash         []byte // Hash of the block header (result of PoW)
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

// Target returns a copy of the target value for the PoW.
func (pow *ProofOfWork) Target() *big.Int {
	// Return a copy to prevent external modification of the internal target
	targetCopy := new(big.Int)
	if pow.target != nil { // Add nil check for safety
		targetCopy.Set(pow.target)
	}
	return targetCopy
}

// prepareData prepares the data to be hashed for PoW.
// It includes fields from the header EXCEPT the Nonce, Hash, and FinalitySignatures.
// ProposerID IS included in the hash calculation.
// VectorClock IS included in the hash calculation.
func (pow *ProofOfWork) prepareData(nonce int64) []byte {
	header := pow.block.Header
	// Serialize VectorClock for hashing
	vcBytes, err := header.VectorClock.Serialize() // Assuming a Serialize method exists
	if err != nil {
		log.Printf("CRITICAL: Failed to serialize vector clock for hashing block %d: %v", header.Height, err)
		// Handle error appropriately - maybe return an error or use a placeholder?
		// Using empty bytes for now, but this is not ideal.
		vcBytes = []byte{}
	}

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
			[]byte(header.ProposerID),        // Include Proposer ID in hash
			vcBytes,                          // Include serialized Vector Clock
			[]byte(fmt.Sprintf("%d", nonce)), // Include the nonce being tried
			// DO NOT INCLUDE FinalitySignatures here
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

	// Use a reasonable upper bound for nonce
	maxNonce := int64(1 << 60) // Reduced max nonce slightly to prevent extreme loops

	for nonce < maxNonce {
		data := pow.prepareData(nonce)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(pow.target) == -1 { // -1 if hashInt < target
			// fmt.Printf("Found hash: %x (Nonce: %d)\n", hash, nonce)
			break
		}
		nonce++
		// Add occasional check to prevent infinite loop in case target is unreachable
		// if nonce % 1000000 == 0 {
		//  log.Printf("Shard %d still mining... Nonce: %d", pow.block.Header.ShardID, nonce)
		// }
	}
	duration := time.Since(startTime)

	if nonce >= maxNonce {
		log.Printf("Shard %d WARNING: Mining reached max nonce (%d) without finding solution. Took %s.", pow.block.Header.ShardID, maxNonce, duration)
		// Return something invalid? Or just the last attempt? Return last attempt for now.
		return nonce, hash[:]
	}

	// Log mining time only if successful
	log.Printf("Shard %d PoW found in %s. Hash: %x (Nonce: %d)", pow.block.Header.ShardID, duration, hash, nonce)

	return nonce, hash[:]
}

// Validate checks if the block's hash is valid according to PoW.
func (pow *ProofOfWork) Validate() bool {
	var hashInt big.Int
	data := pow.prepareData(pow.block.Header.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	isValid := hashInt.Cmp(pow.target) == -1
	// Add check if the calculated hash matches the stored hash
	if !bytes.Equal(hash[:], pow.block.Hash) {
		log.Printf("Block %x hash mismatch! Stored: %x, Calculated: %x", pow.block.Hash, pow.block.Hash, hash[:])
		return false
	}
	return isValid
}

// ProposeBlock creates a new block proposal via PoW, but doesn't finalize it.
// It performs PoW and sets the Nonce and Hash.
// Now also calculates and sets the block's VectorClock.
func ProposeBlock(shardID uint64, transactions []*Transaction, prevBlockHash []byte, height uint64, stateRoot []byte, difficulty int, proposerID NodeID, prevBlockVC VectorClock) (*Block, error) { // Added prevBlockVC
	if proposerID == "" {
		return nil, fmt.Errorf("proposer ID cannot be empty")
	}

	header := &BlockHeader{
		ShardID:       shardID,
		Timestamp:     time.Now().UnixNano(),
		PrevBlockHash: prevBlockHash,
		Height:        height,
		Difficulty:    difficulty,
		StateRoot:     stateRoot,
		ProposerID:    proposerID, // Set proposer ID
		// VectorClock will be calculated below
		// Nonce, BloomFilter, Hash, FinalitySignatures will be set below or later
		Clock: prevBlockVC.Copy(), // Initialize with a copy of previous block's clock
	}

	block := &Block{
		Header:       header,
		Transactions: transactions,
	}

	// --- Calculate Block Vector Clock --- (Ticket 5)
	// Start with a copy of the previous block's clock
	blockVC := prevBlockVC.Copy()
	// Merge clocks from all included transactions
	for _, tx := range transactions {
		// Merge assuming tx.VectorClock is now the correct map[uint64]uint64 type
		blockVC.Merge(tx.VectorClock)
	}
	// Increment the clock for the current shard
	blockVC[shardID]++
	block.Header.VectorClock = blockVC
	// -----------------------------------

	// Calculate Merkle Root
	txHashes := make([][]byte, len(transactions))
	for i, tx := range transactions {
		txHashes[i] = tx.Hash()
	}
	merkleTree, err := NewMerkleTree(txHashes)
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle tree: %w", err)
	}
	block.Header.MerkleRoot = merkleTree.GetMerkleRoot()

	// Create and Serialize Bloom Filter
	n := uint(1000) // Example: Expected items
	p := 0.01       // Example: False positive rate
	filter := bloom.NewWithEstimates(n, p)
	for _, tx := range transactions {
		filter.Add(tx.Hash())
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

	// Check if PoW actually succeeded (didn't hit max nonce)
	var hashInt big.Int
	hashInt.SetBytes(hash)
	if hashInt.Cmp(pow.target) != -1 {
		return nil, fmt.Errorf("proof of work failed for shard %d (max nonce reached or other issue)", shardID)
	}

	block.Header.Nonce = nonce
	block.Hash = hash // Store the hash resulting from PoW

	log.Printf("Shard %d: Proposed Block H:%d by %s. Hash: %x... VC: %v", shardID, height, proposerID, safeSlice(hash, 4), block.Header.VectorClock)

	return block, nil
}

// FinalizeBlock sets the finality signatures on a block header.
// This assumes the block hash (from PoW) is the primary identifier and doesn't change.
func (b *Block) Finalize(finalizingValidators []NodeID) {
	b.Header.FinalitySignatures = finalizingValidators
	log.Printf("Shard %d: Finalized Block H:%d Hash: %x... with %d signatures.",
		b.Header.ShardID, b.Header.Height, safeSlice(b.Hash, 4), len(finalizingValidators))
}

// NewGenesisBlock creates the first block for a specific shard.
// Genesis blocks are typically pre-finalized or don't require the same consensus.
func NewGenesisBlock(shardID uint64, coinbase *Transaction, difficulty int, genesisProposerID NodeID) *Block {
	if coinbase == nil {
		coinbase = NewTransaction([]byte(fmt.Sprintf("Genesis Block Coinbase Shard %d", shardID)), IntraShard, nil)
	}
	// Genesis block has height 0 and no previous hash.
	// State root is initially empty.
	emptyStateRoot := []byte{}

	// Genesis Vector Clock: Starts at 1 for its own shard, 0 otherwise
	genesisVC := make(VectorClock)
	genesisVC[shardID] = 1

	// Use ProposeBlock to get PoW done (even if difficulty is low for genesis)
	// It's simpler to reuse the logic. Genesis difficulty can be set low.
	genesisProposer := genesisProposerID
	if genesisProposer == "" {
		genesisProposer = "GENESIS" // Use a special ID
	}

	// Attempt to propose with potentially very low difficulty
	// Pass an empty VectorClock as the 'previous' clock for genesis
	block, err := ProposeBlock(shardID, []*Transaction{coinbase}, []byte{}, 0, emptyStateRoot, difficulty, genesisProposer, make(VectorClock))

	if err != nil {
		// Fallback if ProposeBlock fails (e.g., extreme difficulty setting)
		log.Printf("Warning: ProposeBlock failed for Genesis Shard %d: %v. Creating minimal Genesis.", shardID, err)
		header := &BlockHeader{
			ShardID:            shardID,
			Timestamp:          time.Now().UnixNano(),
			PrevBlockHash:      []byte{},
			MerkleRoot:         []byte{}, // Calculate simple root if needed
			StateRoot:          emptyStateRoot,
			Nonce:              0,
			Height:             0,
			Difficulty:         difficulty,
			BloomFilter:        []byte{},
			ProposerID:         genesisProposer,
			FinalitySignatures: []NodeID{"GENESIS"}, // Mark as finalized by "GENESIS"
			VectorClock:        genesisVC,           // Set fallback VC
			Clock:              genesisVC,           // Set fallback Clock
		}
		block = &Block{
			Header:       header,
			Transactions: []*Transaction{coinbase},
			Hash:         []byte("genesis_fallback_hash"), // Placeholder hash
		}
		// Recalculate Merkle root for the single tx
		if len(block.Transactions) > 0 {
			tree, _ := NewMerkleTree([][]byte{block.Transactions[0].Hash()})
			if tree != nil {
				block.Header.MerkleRoot = tree.GetMerkleRoot()
			}
		}
		// Recalculate Hash (basic hash of simplified header)
		pow := NewProofOfWork(block)    // Use PoW struct for hashing logic
		block.Hash = pow.prepareData(0) // Use data prep function for consistency (without nonce search)

	} else {
		// Mark the proposed genesis block as finalized immediately
		block.Finalize([]NodeID{genesisProposer}) // Finalized by its own proposer
		// Ensure the correct genesis VC is set even if ProposeBlock was used
		block.Header.VectorClock = genesisVC
		block.Header.Clock = genesisVC
	}

	log.Printf("Created Genesis Block for Shard %d. Hash: %x... VC: %v", shardID, safeSlice(block.Hash, 4), block.Header.VectorClock)
	return block
}

// Serialize encodes the block into bytes. (No changes needed)
func (b *Block) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(b)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize block: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeBlock decodes bytes into a block. (No changes needed)
func DeserializeBlock(data []byte) (*Block, error) {
	var block Block
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&block)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block: %w", err)
	}
	return &block, nil
}

// GetTransactionMerkleProof (No changes needed)
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
	proofHashes := [][]byte{merkleTree.RootNode.Data} // Placeholder proof

	return proofHashes, uint64(txIndex), nil
}

// Helper for logging/display
func safeSlice(data []byte, n int) []byte {
	if len(data) == 0 {
		return []byte("nil")
	}
	if len(data) < n {
		return data
	}
	return data[:n]
}
