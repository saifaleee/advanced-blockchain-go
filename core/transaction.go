package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"time"
)

// TransactionType defines the type of transaction.
type TransactionType int

const (
	// Standard transaction within a single shard.
	IntraShardTx TransactionType = iota
	// CrossShardInitiateTx initiates a transaction spanning multiple shards.
	CrossShardInitiateTx
	// CrossShardFinalizeTx finalizes a transaction initiated in another shard.
	CrossShardFinalizeTx
)

// Transaction represents a state change request in the blockchain.
type Transaction struct {
	ID        []byte          // Transaction hash serves as its ID
	Type      TransactionType // Type of transaction
	Timestamp int64           // Time the transaction was created
	Data      []byte          // Payload data (e.g., transfer details, state changes)
	FromShard uint32          // Source shard ID (relevant for cross-shard)
	ToShard   uint32          // Destination shard ID (relevant for cross-shard)

	// --- Fields for Enhanced Cross-Shard Sync (Phase 6 - Ticket 3) ---
	// Example fields for a potential Two-Phase Commit (2PC) like mechanism:
	SequenceNumber uint64 // Sequence number for ordering related cross-shard steps
	CommitProof    []byte // Proof of commitment from the initiating phase (e.g., Merkle proof of initiate TX)
	Status         string // e.g., "Prepared", "Committed", "Aborted" (could be managed externally or within TX data)
}

// NewTransaction creates a new transaction.
func NewTransaction(txType TransactionType, data []byte, fromShard, toShard uint32) *Transaction {
	tx := &Transaction{
		Type:      txType,
		Timestamp: time.Now().UnixNano(), // Use nanoseconds for better granularity
		Data:      data,
		FromShard: fromShard,
		ToShard:   toShard,
		// Initialize new fields
		SequenceNumber: 0, // Needs proper assignment logic
		CommitProof:    []byte{},
		Status:         "New",
	}
	tx.ID = tx.CalculateHash()
	return tx
}

// CalculateHash computes the SHA256 hash of the transaction content.
func (tx *Transaction) CalculateHash() []byte {
	// Ensure deterministic serialization for hashing
	// Include relevant fields in the hash
	record := fmt.Sprintf("%d%d%d%d%s%d", // Added SequenceNumber
		tx.Timestamp,
		tx.Type,
		tx.FromShard,
		tx.ToShard,
		string(tx.Data),   // Consider a more robust way if data isn't always string-safe
		tx.SequenceNumber, // Include sequence number in hash
		// CommitProof and Status are typically metadata *about* the TX, not part of its intrinsic ID hash.
	)
	hash := sha256.Sum256([]byte(record))
	return hash[:]
}

// IsCrossShard checks if the transaction involves multiple shards.
func (tx *Transaction) IsCrossShard() bool {
	return tx.FromShard != tx.ToShard
}

// Serialize encodes the transaction into a byte slice using gob.
func (tx *Transaction) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeTransaction decodes a transaction from a byte slice using gob.
func DeserializeTransaction(data []byte) (*Transaction, error) {
	var tx Transaction
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&tx)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}
	return &tx, nil
}

// MarkPrepared marks the transaction as prepared.
func (tx *Transaction) MarkPrepared() {
	tx.Status = "Prepared"
}

// MarkCommitted marks the transaction as committed.
func (tx *Transaction) MarkCommitted() {
	tx.Status = "Committed"
}

// MarkAborted marks the transaction as aborted.
func (tx *Transaction) MarkAborted() {
	tx.Status = "Aborted"
}

// String provides a human-readable representation of the transaction.
func (tx *Transaction) String() string {
	typeStr := "IntraShard"
	crossShardInfo := ""
	if tx.Type == CrossShardInitiateTx {
		typeStr = "CrossShardInitiate"
		crossShardInfo = fmt.Sprintf(" (%d -> %d)", tx.FromShard, tx.ToShard)
	} else if tx.Type == CrossShardFinalizeTx {
		typeStr = "CrossShardFinalize"
		crossShardInfo = fmt.Sprintf(" (%d -> %d)", tx.FromShard, tx.ToShard)
	}
	// Include new fields in string representation if helpful
	return fmt.Sprintf("Transaction{ID: %s..., Type: %s%s, Time: %d, Data: %s, Seq: %d, Status: %s}",
		hex.EncodeToString(tx.ID[:8]), typeStr, crossShardInfo, tx.Timestamp, string(tx.Data), tx.SequenceNumber, tx.Status)
}
