package core_test

import (
	"bytes"
	"testing"

	"github.com/saifaleee/advanced-blockchain-go/core"
)

func TestInMemoryStateDB_PutGet(t *testing.T) {
	db := core.NewInMemoryStateDB()
	key := []byte("testKey")
	value := []byte("testValue")

	err := db.Put(key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	retrievedValue, err := db.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(value, retrievedValue) {
		t.Errorf("Value mismatch: expected %s, got %s", value, retrievedValue)
	}

	// Test getting non-existent key
	_, err = db.Get([]byte("nonExistentKey"))
	if err == nil {
		t.Error("Expected error when getting non-existent key, but got nil")
	}
	if err != core.ErrNotFound {
		t.Errorf("Expected ErrNotFound, but got: %v", err)
	}
}

func TestInMemoryStateDB_Delete(t *testing.T) {
	db := core.NewInMemoryStateDB()
	key := []byte("testKey")
	value := []byte("testValue")

	db.Put(key, value) // Ensure key exists

	err := db.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify key is deleted
	_, err = db.Get(key)
	if err == nil || err != core.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, but got: %v", err)
	}

	// Test deleting non-existent key
	err = db.Delete([]byte("nonExistentKeyAgain"))
	// Depending on desired behavior: check for specific error or nil
	if err != core.ErrNotFound { // Assuming delete should error if key doesn't exist
		t.Errorf("Expected ErrNotFound when deleting non-existent key, got: %v", err)
	}
}

func TestInMemoryStateDB_Overwrite(t *testing.T) {
	db := core.NewInMemoryStateDB()
	key := []byte("testKey")
	value1 := []byte("value1")
	value2 := []byte("value2")

	db.Put(key, value1)
	retrievedValue1, _ := db.Get(key)
	if !bytes.Equal(value1, retrievedValue1) {
		t.Fatalf("Initial Put/Get failed")
	}

	db.Put(key, value2) // Overwrite
	retrievedValue2, err := db.Get(key)
	if err != nil {
		t.Fatalf("Get after overwrite failed: %v", err)
	}
	if !bytes.Equal(value2, retrievedValue2) {
		t.Errorf("Value mismatch after overwrite: expected %s, got %s", value2, retrievedValue2)
	}
}
