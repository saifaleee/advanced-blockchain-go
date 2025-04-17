package core_test // Use _test package convention

import (
	"bytes"
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core" // Adjust import path
)

func TestTransaction(t *testing.T) {
	data := []byte("Test transaction data")
	tx := core.NewTransaction(data)

	if tx == nil {
		t.Fatal("NewTransaction returned nil")
	}
	if len(tx.ID) == 0 {
		t.Error("Transaction ID is empty")
	}
	if !bytes.Equal(tx.Data, data) {
		t.Errorf("Transaction data mismatch: expected %s, got %s", data, tx.Data)
	}

	// Test hashing consistency
	hash1 := tx.Hash()
	hash2 := tx.Hash()
	if !bytes.Equal(hash1, hash2) {
		t.Error("Transaction hashing is not deterministic")
	}
}

func TestTransactionSerialization(t *testing.T) {
	tx := core.NewTransaction([]byte("Serialize me"))
	serialized := tx.Serialize()
	if len(serialized) == 0 {
		t.Fatal("Serialization produced empty byte slice")
	}

	deserializedTx, err := core.DeserializeTransaction(serialized)
	if err != nil {
		t.Fatalf("Deserialization failed: %v", err)
	}

	if !bytes.Equal(tx.ID, deserializedTx.ID) {
		t.Errorf("Deserialized ID mismatch: expected %x, got %x", tx.ID, deserializedTx.ID)
	}
	if !bytes.Equal(tx.Data, deserializedTx.Data) {
		t.Errorf("Deserialized data mismatch: expected %s, got %s", tx.Data, deserializedTx.Data)
	}
}
