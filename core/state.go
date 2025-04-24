package core

import (
	"errors"
	"fmt"
	"sync"
)

// StateDB defines the interface for accessing blockchain state.
// In a sharded system, each shard would typically have its own StateDB instance.
type StateDB interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Delete(key []byte) error
	// GetKeys returns all keys in the state DB. Used for migration.
	// In a real DB (e.g., BadgerDB, LevelDB), this would use iterators.
	GetKeys() ([][]byte, error)
	// GetStateRoot calculates a root hash representing the state.
	// Placeholder for SMT/Patricia Trie integration.
	GetStateRoot() ([]byte, error)
	// Size returns the approximate size or number of items.
	Size() int
	// Clear deletes all entries (used in merge/cleanup).
	Clear() error
}

// ErrNotFound indicates that a key was not found in the state database.
var ErrNotFound = errors.New("key not found")

// InMemoryStateDB is a simple thread-safe in-memory implementation of StateDB.
type InMemoryStateDB struct {
	mu    sync.RWMutex
	store map[string][]byte // Use string keys for map efficiency
}

// NewInMemoryStateDB creates a new in-memory state database.
func NewInMemoryStateDB() *InMemoryStateDB {
	return &InMemoryStateDB{
		store: make(map[string][]byte),
	}
}

// Get retrieves a value by key.
func (db *InMemoryStateDB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	value, ok := db.store[string(key)]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a copy to prevent modification of the underlying map slice
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	return valueCopy, nil
}

// Put stores a key-value pair.
func (db *InMemoryStateDB) Put(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	// Store copies to ensure immutability outside the DB
	keyCopy := string(key)
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	db.store[keyCopy] = valueCopy
	return nil
}

// Delete removes a key-value pair.
func (db *InMemoryStateDB) Delete(key []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	keyString := string(key)
	if _, ok := db.store[keyString]; !ok {
		return ErrNotFound // Or return nil if deleting non-existent key is okay
	}
	delete(db.store, keyString)
	return nil
}

// GetKeys returns all keys in the state DB.
func (db *InMemoryStateDB) GetKeys() ([][]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	keys := make([][]byte, 0, len(db.store))
	for k := range db.store {
		keyCopy := []byte(k)
		keys = append(keys, keyCopy)
	}
	return keys, nil
}

// GetStateRoot provides a placeholder state root calculation.
// TODO: Replace with actual SMT or Patricia Merkle Trie root calculation.
func (db *InMemoryStateDB) GetStateRoot() ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	// Extremely basic placeholder: hash of concatenated key-value pairs (sorted)
	// This is NOT cryptographically secure or efficient like a proper Merkle Trie/SMT.
	// Consider using a library like go-ethereum's trie package or a dedicated SMT lib.
	if len(db.store) == 0 {
		return EmptyMerkleRoot(), nil // Use the same empty root as transactions
	}
	// For a placeholder, maybe just return a hash of the number of items.
	// This is NOT a real state root.
	sizeBytes := []byte(fmt.Sprintf("%d", len(db.store)))
	return SimpleHash(sizeBytes), nil
}

func (db *InMemoryStateDB) Size() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.store)
}

func (db *InMemoryStateDB) Clear() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.store = make(map[string][]byte) // Re-initialize the map
	return nil
}
