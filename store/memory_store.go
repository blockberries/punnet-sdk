package store

import (
	"bytes"
	"sort"
	"sync"
)

// MemoryStore is a simple in-memory backing store
// TODO: Replace with IAVL store for production use
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string][]byte),
	}
}

// Get retrieves raw bytes by key
func (ms *MemoryStore) Get(key []byte) ([]byte, error) {
	if ms == nil {
		return nil, ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return nil, err
	}

	ms.mu.RLock()
	defer ms.mu.RUnlock()

	value, ok := ms.data[string(key)]
	if !ok {
		return nil, ErrNotFound
	}

	// Return defensive copy
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Set stores raw bytes with the given key
func (ms *MemoryStore) Set(key []byte, value []byte) error {
	if ms == nil {
		return ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Store defensive copies
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	ms.data[string(keyCopy)] = valueCopy
	return nil
}

// Delete removes a key
func (ms *MemoryStore) Delete(key []byte) error {
	if ms == nil {
		return ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return err
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	delete(ms.data, string(key))
	return nil
}

// Has checks if a key exists
func (ms *MemoryStore) Has(key []byte) (bool, error) {
	if ms == nil {
		return false, ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return false, err
	}

	ms.mu.RLock()
	defer ms.mu.RUnlock()

	_, ok := ms.data[string(key)]
	return ok, nil
}

// Iterator returns an iterator over a range of keys
func (ms *MemoryStore) Iterator(start, end []byte) (RawIterator, error) {
	if ms == nil {
		return nil, ErrStoreNil
	}

	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.iterator(start, end, false)
}

// ReverseIterator returns a reverse iterator over a range of keys
func (ms *MemoryStore) ReverseIterator(start, end []byte) (RawIterator, error) {
	if ms == nil {
		return nil, ErrStoreNil
	}

	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.iterator(start, end, true)
}

// iterator creates an iterator (must be called with lock held)
func (ms *MemoryStore) iterator(start, end []byte, reverse bool) (RawIterator, error) {
	// Collect and sort keys
	keys := make([]string, 0, len(ms.data))
	for key := range ms.data {
		keyBytes := []byte(key)

		// Check if key is in range
		if start != nil && bytes.Compare(keyBytes, start) < 0 {
			continue
		}
		if end != nil && bytes.Compare(keyBytes, end) >= 0 {
			continue
		}

		keys = append(keys, key)
	}

	// Sort keys
	sort.Strings(keys)

	// Reverse if needed
	if reverse {
		for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
			keys[i], keys[j] = keys[j], keys[i]
		}
	}

	// Copy data for iterator
	items := make([]kvPair, len(keys))
	for i, key := range keys {
		value := ms.data[key]
		items[i] = kvPair{
			key:   []byte(key),
			value: value,
		}
	}

	return &memoryIterator{
		items: items,
		index: 0,
	}, nil
}

// Flush writes pending changes (no-op for memory store)
func (ms *MemoryStore) Flush() error {
	return nil
}

// Close releases resources (no-op for memory store)
func (ms *MemoryStore) Close() error {
	return nil
}

// kvPair is a key-value pair for the memory iterator
type kvPair struct {
	key   []byte
	value []byte
}

// memoryIterator implements RawIterator for MemoryStore
type memoryIterator struct {
	mu     sync.RWMutex
	items  []kvPair
	index  int
	closed bool
}

// Valid returns true if positioned at a valid entry
func (mi *memoryIterator) Valid() bool {
	if mi == nil {
		return false
	}

	mi.mu.RLock()
	defer mi.mu.RUnlock()

	if mi.closed {
		return false
	}

	return mi.index >= 0 && mi.index < len(mi.items)
}

// Next advances to the next entry
func (mi *memoryIterator) Next() {
	if mi == nil {
		return
	}

	mi.mu.Lock()
	defer mi.mu.Unlock()

	if mi.closed {
		return
	}

	mi.index++
}

// Key returns the current key
func (mi *memoryIterator) Key() []byte {
	if mi == nil {
		return nil
	}

	mi.mu.RLock()
	defer mi.mu.RUnlock()

	if mi.closed || !mi.isValidLocked() {
		return nil
	}

	// Return defensive copy
	key := mi.items[mi.index].key
	result := make([]byte, len(key))
	copy(result, key)
	return result
}

// Value returns the current value
func (mi *memoryIterator) Value() []byte {
	if mi == nil {
		return nil
	}

	mi.mu.RLock()
	defer mi.mu.RUnlock()

	if mi.closed || !mi.isValidLocked() {
		return nil
	}

	// Return defensive copy
	value := mi.items[mi.index].value
	result := make([]byte, len(value))
	copy(result, value)
	return result
}

// Error returns any error that occurred during iteration
func (mi *memoryIterator) Error() error {
	if mi == nil {
		return nil
	}

	mi.mu.RLock()
	defer mi.mu.RUnlock()

	if mi.closed {
		return ErrIteratorClosed
	}

	return nil
}

// Close releases iterator resources
func (mi *memoryIterator) Close() error {
	if mi == nil {
		return nil
	}

	mi.mu.Lock()
	defer mi.mu.Unlock()

	if mi.closed {
		return nil
	}

	mi.closed = true
	mi.items = nil
	return nil
}

// isValidLocked checks if index is valid (must be called with lock held)
func (mi *memoryIterator) isValidLocked() bool {
	return mi.index >= 0 && mi.index < len(mi.items)
}
