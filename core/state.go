package core

import (
	"errors"
	"sync"
	"unsafe" // For approximate size calculation
)

// StateDB defines the interface for accessing blockchain state.
type StateDB interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Delete(key []byte) error
	Size() int64 // Added method to get approximate size
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
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	return valueCopy, nil
}

// Put stores a key-value pair.
func (db *InMemoryStateDB) Put(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
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
		return ErrNotFound
	}
	delete(db.store, keyString)
	return nil
}

// Size returns an *approximate* size of the data stored in the in-memory map in bytes.
// This is a rough estimate and doesn't account for map overhead perfectly.
func (db *InMemoryStateDB) Size() int64 {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var totalSize int64 = 0
	for k, v := range db.store {
		// Size of key string header + key string data
		totalSize += int64(unsafe.Sizeof(k)) + int64(len(k))
		// Size of value slice header + value slice data
		totalSize += int64(unsafe.Sizeof(v)) + int64(len(v))
	}
	// Add approximate overhead for the map structure itself (highly variable)
	// This is just a guess; real measurement is complex.
	mapOverhead := int64(unsafe.Sizeof(db.store)) + int64(len(db.store)*16) // Guess 16 bytes overhead per entry
	totalSize += mapOverhead

	return totalSize
}

