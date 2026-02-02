package crypto

import (
	"sync"
)

// MemoryStore implements KeyStore with in-memory storage.
// Thread-safe via RWMutex. Optimized for read-heavy workloads.
//
// Performance characteristics:
// - Get: O(1) average, O(n) worst case (hash collision)
// - Put: O(1) average
// - Delete: O(1) average
// - List: O(n) where n is number of keys
// - Has: O(1) average
//
// Memory: ~128 bytes overhead per key + key data size.
type MemoryStore struct {
	mu   sync.RWMutex
	keys map[string]*KeyEntry
}

// NewMemoryStore creates a new in-memory key store.
// Pre-allocates map for expected capacity to reduce reallocations.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		keys: make(map[string]*KeyEntry, 16), // Pre-allocate for typical use
	}
}

// NewMemoryStoreWithCapacity creates a store with specified initial capacity.
// Use this when you know the approximate number of keys to avoid rehashing.
func NewMemoryStoreWithCapacity(capacity int) *MemoryStore {
	return &MemoryStore{
		keys: make(map[string]*KeyEntry, capacity),
	}
}

// Get retrieves a key entry by name.
// Returns a clone to prevent external mutation.
func (s *MemoryStore) Get(name string) (*KeyEntry, error) {
	s.mu.RLock()
	entry, ok := s.keys[name]
	s.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}
	return entry.Clone(), nil
}

// Put stores a key entry.
func (s *MemoryStore) Put(entry *KeyEntry, overwrite bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !overwrite {
		if _, exists := s.keys[entry.Name]; exists {
			return ErrKeyExists
		}
	}

	// Store a clone to prevent external mutation
	s.keys[entry.Name] = entry.Clone()
	return nil
}

// Delete removes a key entry.
// Zeros private key material before removal for security.
func (s *MemoryStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.keys[name]
	if !exists {
		return ErrKeyNotFound
	}

	// Zero private key bytes before deletion to minimize memory exposure
	Zeroize(entry.PrivateKey)
	delete(s.keys, name)
	return nil
}

// List returns all key names.
// Pre-allocates result slice to avoid reallocations.
func (s *MemoryStore) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.keys))
	for name := range s.keys {
		names = append(names, name)
	}
	return names, nil
}

// Has returns true if a key exists.
// More efficient than Get when you don't need the key data.
func (s *MemoryStore) Has(name string) (bool, error) {
	s.mu.RLock()
	_, exists := s.keys[name]
	s.mu.RUnlock()
	return exists, nil
}

// Len returns the number of keys.
// Useful for monitoring and testing.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	n := len(s.keys)
	s.mu.RUnlock()
	return n
}

// Clear removes all keys.
// Zeros private key material before removal for security.
// Useful for testing.
func (s *MemoryStore) Clear() {
	s.mu.Lock()
	// Zero all private keys before clearing
	for _, entry := range s.keys {
		Zeroize(entry.PrivateKey)
	}
	s.keys = make(map[string]*KeyEntry, 16)
	s.mu.Unlock()
}
