package core

import (
	"crypto/sha256"
	"errors"
	"sort"
	"sync"
)

// StateDB defines the interface for shard state storage.
type StateDB interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(key string) error
	GetKeys() ([]string, error) // Return keys as strings
	Clear() error
	Size() int                     // Add Size method
	GetStateRoot() ([]byte, error) // Add GetStateRoot method
}

// InMemoryStateDB provides a simple in-memory implementation of StateDB.
type InMemoryStateDB struct {
	data map[string][]byte
	mu   sync.RWMutex
}

// NewInMemoryStateDB creates a new in-memory state database.
func NewInMemoryStateDB() *InMemoryStateDB {
	return &InMemoryStateDB{
		data: make(map[string][]byte),
	}
}

// Get retrieves a value by key.
func (db *InMemoryStateDB) Get(key string) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	value, ok := db.data[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	// Return a copy to prevent external modification
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	return valueCopy, nil
}

// Put stores a key-value pair.
func (db *InMemoryStateDB) Put(key string, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	// Store a copy to prevent external modification
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	db.data[key] = valueCopy
	return nil
}

// Delete removes a key-value pair.
func (db *InMemoryStateDB) Delete(key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, ok := db.data[key]; !ok {
		return errors.New("key not found")
	}
	delete(db.data, key)
	return nil
}

// GetKeys returns all keys in the database.
func (db *InMemoryStateDB) GetKeys() ([]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	keys := make([]string, 0, len(db.data))
	for k := range db.data {
		keys = append(keys, k)
	}
	sort.Strings(keys) // Ensure consistent order for state root calculation
	return keys, nil
}

// Clear removes all entries from the database.
func (db *InMemoryStateDB) Clear() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.data = make(map[string][]byte) // Reinitialize the map
	return nil
}

// Size returns the number of key-value pairs in the database.
func (db *InMemoryStateDB) Size() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.data)
}

// GetStateRoot calculates a simple hash of all key-value pairs as the state root.
// NOTE: This is a basic implementation. A real blockchain would use a Merkle tree or similar structure.
func (db *InMemoryStateDB) GetStateRoot() ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if len(db.data) == 0 {
		// Return a predefined hash for an empty state or nil
		return sha256.New().Sum(nil), nil
	}

	// Get keys and sort them for deterministic hashing
	keys, _ := db.GetKeys() // Already sorted by GetKeys

	hasher := sha256.New()
	for _, key := range keys {
		value := db.data[key] // Access directly as we hold the lock
		hasher.Write([]byte(key))
		hasher.Write(value)
	}

	return hasher.Sum(nil), nil
}
