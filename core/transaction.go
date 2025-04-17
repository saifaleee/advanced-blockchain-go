package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
)

// Transaction represents a basic transaction structure.
// In a real system, this would be more complex (inputs, outputs, signatures).
type Transaction struct {
	ID   []byte
	Data []byte // Simple data payload for now
}

// NewTransaction creates a basic transaction.
// The ID is simply the hash of the data for this basic example.
func NewTransaction(data []byte) *Transaction {
	tx := &Transaction{Data: data}
	tx.ID = tx.Hash()
	return tx
}

// Hash calculates the SHA256 hash of the transaction data.
func (tx *Transaction) Hash() []byte {
	hash := sha256.Sum256(tx.Data)
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
