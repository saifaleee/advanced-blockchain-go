package core

import (
	"errors"
	"sync"
)

// StateDB defines the interface for accessing blockchain state.
// In a sharded system, each shard would typically have its own StateDB instance.
type StateDB interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Delete(key []byte) error
	// In a real DB, you'd have methods for iteration, batching, snapshots, etc.
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
