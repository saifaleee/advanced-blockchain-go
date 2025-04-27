package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"log"
	"math/big"
	"time"

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
	VectorClock VectorClock // Tracks causal history of the block (Renamed from Clock)

	// --- Placeholders for Advanced Features (Phase 6 - Tickets 2, 9) ---
	AccumulatorState []byte // Placeholder for cryptographic accumulator state (e.g., RSA accumulator)
	ProofMetadata    []byte // Placeholder for metadata related to advanced/compressed proofs
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
	vcBytes, err := header.VectorClock.Serialize() // Use the renamed field VectorClock
	if err != nil {
		log.Printf("CRITICAL: Failed to serialize vector clock for hashing block %d: %v", header.Height, err)
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
			header.AccumulatorState,          // Include accumulator state in hash
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

	startTime := time.Now()

	// Use a reasonable upper bound for nonce
	maxNonce := int64(1 << 60) // Reduced max nonce slightly to prevent extreme loops

	for nonce < maxNonce {
		data := pow.prepareData(nonce)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(pow.target) == -1 { // -1 if hashInt < target
			break
		}
		nonce++
	}
	duration := time.Since(startTime)

	if nonce >= maxNonce {
		log.Printf("Shard %d WARNING: Mining reached max nonce (%d) without finding solution. Took %s.", pow.block.Header.ShardID, maxNonce, duration)
		return nonce, hash[:]
	}

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
	if !bytes.Equal(hash[:], pow.block.Hash) {
		log.Printf("Block %x hash mismatch! Stored: %x, Calculated: %x", pow.block.Hash, pow.block.Hash, hash[:])
		return false
	}
	return isValid
}

// ProposeBlock creates a new block proposal via PoW, but doesn't finalize it.
// It performs PoW and sets the Nonce and Hash.
// Now also calculates and sets the block's VectorClock.
func ProposeBlock(shardID uint64, transactions []*Transaction, prevBlockHash []byte, height uint64, stateRoot []byte, difficulty int, proposerID NodeID, prevBlockVC VectorClock) (*Block, error) {
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
		ProposerID:    proposerID,
	}

	block := &Block{
		Header:       header,
		Transactions: transactions,
	}

	// Calculate the block's vector clock.
	// Start by copying the previous block's vector clock.
	blockVC := prevBlockVC.Copy()
	// The original code attempted to merge vector clocks from transactions,
	// but the Transaction type doesn't have a VectorClock field.
	// Removing this loop as transactions don't carry vector clocks in the current definition.
	// for _, tx := range transactions {
	// 	blockVC.Merge(tx.VectorClock) // This line caused the compile error
	// }
	// Increment the clock for the current shard where the block is being proposed.
	blockVC[shardID]++
	block.Header.VectorClock = blockVC

	txHashes := make([][]byte, len(transactions))
	for i, tx := range transactions {
		txHashes[i] = tx.ID // Replace tx.Hash() with tx.ID
	}
	merkleTree, err := NewMerkleTree(txHashes)
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle tree: %w", err)
	}
	block.Header.MerkleRoot = merkleTree.GetMerkleRoot()

	// Correct Bloom filter estimation
	n := uint(len(transactions))
	if n == 0 {
		n = 1 // Avoid creating a filter with 0 items if there are no transactions
	}
	p := 0.01 // Standard false positive rate
	filter := bloom.NewWithEstimates(n, p)
	for _, tx := range transactions {
		filter.Add(tx.ID) // Replace tx.Hash() with tx.ID
	}
	var buf bytes.Buffer
	_, err = filter.WriteTo(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize bloom filter: %w", err)
	}
	block.Header.BloomFilter = buf.Bytes()

	// Initialize Accumulator state for the new block (e.g., from previous block or empty)
	// For simplicity, let's assume it starts empty or inherits. Here, start empty.
	block.Header.AccumulatorState = []byte("genesis_accumulator_state") // Or fetch from prev block header
	// Update accumulator with transactions in this block
	txDataForAccumulator := make([][]byte, len(transactions))
	for i, tx := range transactions {
		txDataForAccumulator[i] = tx.ID // Replace tx.Hash() with tx.ID
	}
	err = block.Header.UpdateAccumulator(txDataForAccumulator)
	if err != nil {
		log.Printf("Warning: Failed to update accumulator during block proposal: %v", err)
		// Decide if this is critical. For PoC, maybe just log.
	}

	pow := NewProofOfWork(block)
	nonce, hash := pow.Run()

	var hashInt big.Int
	hashInt.SetBytes(hash)
	if hashInt.Cmp(pow.target) != -1 {
		return nil, fmt.Errorf("proof of work failed for shard %d (max nonce reached or other issue)", shardID)
	}

	block.Header.Nonce = nonce
	block.Hash = hash

	log.Printf("Shard %d: Proposed Block H:%d by %s. Hash: %x... VC: %v", shardID, height, proposerID, safeSlice(hash, 4), block.Header.VectorClock)

	return block, nil
}

// FinalizeBlock sets the finality signatures on a block header.
func (b *Block) Finalize(finalizingValidators []NodeID) {
	b.Header.FinalitySignatures = finalizingValidators
	log.Printf("Shard %d: Finalized Block H:%d Hash: %x... with %d signatures.",
		b.Header.ShardID, b.Header.Height, safeSlice(b.Hash, 4), len(finalizingValidators))
}

// NewGenesisBlock creates the first block for a specific shard.
func NewGenesisBlock(shardID uint64, coinbase *Transaction, difficulty int, genesisProposerID NodeID) *Block {
	if coinbase == nil {
		coinbase = NewTransaction(IntraShardTx, []byte(fmt.Sprintf("Genesis Block Coinbase Shard %d", shardID)), uint32(shardID), uint32(shardID))
	}
	emptyStateRoot := []byte{}

	genesisVC := make(VectorClock)
	genesisVC[shardID] = 1

	genesisProposer := genesisProposerID
	if genesisProposer == "" {
		genesisProposer = "GENESIS"
	}

	block, err := ProposeBlock(shardID, []*Transaction{coinbase}, []byte{}, 0, emptyStateRoot, difficulty, genesisProposer, make(VectorClock))

	if err != nil {
		log.Printf("Warning: ProposeBlock failed for Genesis Shard %d: %v. Creating minimal Genesis.", shardID, err)
		header := &BlockHeader{
			ShardID:            shardID,
			Timestamp:          time.Now().UnixNano(),
			PrevBlockHash:      []byte{},
			MerkleRoot:         []byte{},
			StateRoot:          emptyStateRoot,
			Nonce:              0,
			Height:             0,
			Difficulty:         difficulty,
			BloomFilter:        []byte{},
			ProposerID:         genesisProposer,
			FinalitySignatures: []NodeID{"GENESIS"},
			VectorClock:        genesisVC,
		}
		block = &Block{
			Header:       header,
			Transactions: []*Transaction{coinbase},
			Hash:         []byte("genesis_fallback_hash"),
		}
		if len(block.Transactions) > 0 {
			tree, _ := NewMerkleTree([][]byte{block.Transactions[0].ID}) // Replace `Hash()` with `ID`
			if tree != nil {
				block.Header.MerkleRoot = tree.GetMerkleRoot()
			}
		}
		pow := NewProofOfWork(block)
		block.Hash = pow.prepareData(0)

	} else {
		block.Finalize([]NodeID{genesisProposer})
		block.Header.VectorClock = genesisVC
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
		txHashes[i] = tx.ID           // Replace tx.Hash() with tx.ID
		if bytes.Equal(tx.ID, txID) { // Replace tx.Hash() with tx.ID
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

	proofHashes := [][]byte{merkleTree.RootNode.Data}

	return proofHashes, uint64(txIndex), nil
}

// UpdateAccumulator updates the block's accumulator state with new data (placeholder - simple hash chain).
// H_new = SHA256(H_old || SHA256(data1) || SHA256(data2) || ...)
func (bh *BlockHeader) UpdateAccumulator(newData [][]byte) error {
	hasher := sha256.New()
	hasher.Write(bh.AccumulatorState) // Start with the current state

	for _, item := range newData {
		itemHash := sha256.Sum256(item)
		hasher.Write(itemHash[:]) // Append hash of new item
	}

	bh.AccumulatorState = hasher.Sum(nil)
	// log.Printf("[BlockHeader H:%d] Updated accumulator state to %x", bh.Height, safeSlice(bh.AccumulatorState, 8)) // Reduce log noise
	return nil
}

// GenerateProof generates an advanced proof (placeholder - simple accumulator proof).
// For a simple hash chain H(H_old || H(d1) || H(d2)), proof for d1 could be [H_old, H(d2)].
func (bh *BlockHeader) GenerateProof(dataID []byte) ([]byte, error) {
	log.Printf("[BlockHeader H:%d] Generating accumulator proof for data %x (placeholder)", bh.Height, safeSlice(dataID, 4))
	// This requires knowing the order/content of items added. Complex for this PoC.
	// Return a placeholder indicating the data ID and the final accumulator state.
	proof := bytes.Join([][]byte{
		[]byte("simple_accumulator_proof_for:"),
		dataID,
		[]byte("final_state:"),
		bh.AccumulatorState,
	}, []byte(" "))
	return proof, nil
}

// VerifyProof verifies an advanced proof (placeholder - simple accumulator).
// This would require recomputing the accumulator state with the proof elements.
func VerifyProof(blockHeader *BlockHeader, proof []byte, dataID []byte) bool {
	if blockHeader == nil {
		log.Printf("[VerifyProof] Error: Block header is nil")
		return false
	}
	log.Printf("[VerifyProof H:%d] Verifying accumulator proof for data %x (placeholder)", blockHeader.Height, safeSlice(dataID, 4))
	// Placeholder: Check if the proof contains the expected final state.
	// A real verification would involve cryptographic checks.
	parts := bytes.Split(proof, []byte(" "))
	if len(parts) == 4 && bytes.Equal(parts[0], []byte("simple_accumulator_proof_for:")) && bytes.Equal(parts[2], []byte("final_state:")) {
		// Check if the dataID matches (optional, depends on proof format)
		// Check if the final state in the proof matches the header's state
		if bytes.Equal(parts[3], blockHeader.AccumulatorState) {
			// log.Printf("[VerifyProof H:%d] Placeholder verification successful for %x", blockHeader.Height, safeSlice(dataID, 4)) // Reduce log noise
			return true
		}
		log.Printf("[VerifyProof H:%d] Failed: Accumulator state mismatch. Expected %x, Got %x", blockHeader.Height, safeSlice(blockHeader.AccumulatorState, 8), safeSlice(parts[3], 8))
		return false
	}
	log.Printf("[VerifyProof H:%d] Failed: Invalid proof format.", blockHeader.Height)
	return false // Placeholder failure
}
