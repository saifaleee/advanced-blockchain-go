package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
)

// TransactionType distinguishes different kinds of transactions.
type TransactionType uint8

const (
	// IntraShardTx occurs within a single shard.
	IntraShardTx TransactionType = iota
	// CrossShardTxInit initiates a transaction spanning multiple shards.
	CrossShardTxInit
	// CrossShardTxFinalize finalizes a transaction based on a receipt from another shard.
	CrossShardTxFinalize
)

// Transaction represents a transaction structure.
type Transaction struct {
	ID   []byte
	Type TransactionType
	Data []byte // Payload specific to the transaction type

	// Optional fields for cross-shard transfers:
	SourceShard      *ShardID // Pointer allows nil for non-cross-shard or unknown source
	DestinationShard *ShardID // Pointer allows nil for non-cross-shard
}

// NewTransaction creates a basic intra-shard transaction.
func NewTransaction(data []byte) *Transaction {
	tx := &Transaction{
		Type: IntraShardTx,
		Data: data,
	}
	tx.ID = tx.Hash() // Use Hash() method which considers Type now
	return tx
}

// NewCrossShardInitTransaction creates the initiating part of a cross-shard transaction.
func NewCrossShardInitTransaction(data []byte, source ShardID, dest ShardID) *Transaction {
	tx := &Transaction{
		Type:             CrossShardTxInit,
		Data:             data,
		SourceShard:      &source,
		DestinationShard: &dest,
	}
	tx.ID = tx.Hash()
	return tx
}

// Hash calculates the SHA256 hash of the transaction (including type).
func (tx *Transaction) Hash() []byte {
	var encoded bytes.Buffer
	enc := gob.NewEncoder(&encoded)
	// Encode relevant fields for hashing
	err := enc.Encode(struct {
		Type TransactionType
		Data []byte
		Src  *ShardID
		Dst  *ShardID
	}{
		Type: tx.Type,
		Data: tx.Data,
		Src:  tx.SourceShard,
		Dst:  tx.DestinationShard,
	})
	if err != nil {
		log.Panicf("Failed to encode transaction for hashing: %v", err)
	}
	hash := sha256.Sum256(encoded.Bytes())
	return hash[:]
}

// Serialize serializes the transaction using gob.
func (tx *Transaction) Serialize() []byte {
	var encoded bytes.Buffer
	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panicf("Failed to encode transaction: %v", err)
	}
	return encoded.Bytes()
}

// DeserializeTransaction deserializes a transaction.
func DeserializeTransaction(data []byte) (*Transaction, error) {
	var transaction Transaction
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(&transaction)
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}
