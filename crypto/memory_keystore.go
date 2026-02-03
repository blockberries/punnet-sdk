package crypto

import (
	"sync"
)

// MemoryKeyStore implements EncryptedKeyStore with in-memory storage.
// Thread-safe via RWMutex. Optimized for read-heavy workloads.
// Keys are stored in plaintext (no encryption) - suitable for testing
// and ephemeral use cases only.
//
// Performance characteristics:
// - Store: O(1) average, O(n) worst case (hash collision)
// - Load: O(1) average, O(n) worst case
// - Delete: O(1) average, O(n) worst case
// - List: O(n) where n is number of keys
//
// Memory: ~128 bytes overhead per key + key data size.
//
// Implements io.Closer for graceful shutdown per Crypto-Resource-Lifecycle pattern.
type MemoryKeyStore struct {
	mu     sync.RWMutex
	keys   map[string]EncryptedKey
	closed bool
}

// NewMemoryKeyStore creates a new in-memory key store.
// Pre-allocates map for expected capacity to reduce reallocations.
// Complexity: O(1).
func NewMemoryKeyStore() *MemoryKeyStore {
	return &MemoryKeyStore{
		keys: make(map[string]EncryptedKey, 16), // Pre-allocate for typical use
	}
}

// NewMemoryKeyStoreWithCapacity creates a store with specified initial capacity.
// Use this when you know the approximate number of keys to avoid rehashing.
// Negative capacity is treated as zero (no pre-allocation).
// Complexity: O(1).
func NewMemoryKeyStoreWithCapacity(capacity int) *MemoryKeyStore {
	if capacity < 0 {
		capacity = 0
	}
	return &MemoryKeyStore{
		keys: make(map[string]EncryptedKey, capacity),
	}
}

// Store saves a key to the store.
//
// REQUIREMENTS:
//   - name MUST be non-empty and pass ValidateKeyName()
//   - name MUST equal key.Name (prevents lookup/storage mismatch)
//   - key.Algorithm MUST be valid (pass Algorithm.IsValid())
//   - If key has encryption params, they MUST pass ValidateEncryptionParams()
//
// Returns ErrKeyStoreClosed if the store has been closed.
// Returns ErrInvalidKeyName if name fails validation.
// Returns ErrKeyNameMismatch if name != key.Name.
// Returns ErrInvalidAlgorithm if algorithm is not recognized.
// Returns ErrInvalidEncryptionParams if encryption metadata is malformed.
// Returns ErrKeyStoreExists if a key with the same name already exists.
//
// Complexity: O(1) average.
// Memory: Allocates copy of key data.
func (m *MemoryKeyStore) Store(name string, key EncryptedKey) error {
	// Validate key name
	if err := ValidateKeyName(name); err != nil {
		return err
	}

	// Validate name matches key.Name
	if name != key.Name {
		return ErrKeyNameMismatch
	}

	// Validate algorithm
	if !key.Algorithm.IsValid() {
		return ErrInvalidAlgorithm
	}

	// Validate encryption params
	if err := key.ValidateEncryptionParams(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrKeyStoreClosed
	}

	if _, exists := m.keys[name]; exists {
		return ErrKeyStoreExists
	}

	// Store a deep copy to prevent external mutation
	m.keys[name] = copyEncryptedKey(key)
	return nil
}

// Load retrieves a key from the store.
// Returns a copy to prevent external mutation.
//
// Returns ErrKeyStoreClosed if the store has been closed.
// Returns ErrKeyStoreNotFound if no key exists with the given name.
//
// SECURITY: Caller should call Wipe() on the returned key when done
// to zero sensitive data from memory.
//
// Complexity: O(1) average.
// Memory: Allocates copy of key data.
func (m *MemoryKeyStore) Load(name string) (EncryptedKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return EncryptedKey{}, ErrKeyStoreClosed
	}

	key, exists := m.keys[name]
	if !exists {
		return EncryptedKey{}, ErrKeyStoreNotFound
	}

	// Return a copy to prevent external mutation
	return copyEncryptedKey(key), nil
}

// Delete removes a key from the store.
// Securely wipes key material before removal.
//
// Returns ErrKeyStoreClosed if the store has been closed.
// Returns ErrKeyStoreNotFound if no key exists with the given name.
//
// Complexity: O(1) average.
func (m *MemoryKeyStore) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrKeyStoreClosed
	}

	key, exists := m.keys[name]
	if !exists {
		return ErrKeyStoreNotFound
	}

	// Secure wipe before deletion to minimize memory exposure.
	// Note: key is a copy of the map value (Go range semantics), but Wipe()
	// zeros the underlying byte slice arrays which are shared with the map entry.
	key.Wipe()
	delete(m.keys, name)
	return nil
}

// List returns all key names in the store.
// Pre-allocates result slice to avoid reallocations.
//
// Returns ErrKeyStoreClosed if the store has been closed.
//
// The returned slice is not guaranteed to be in any particular order.
//
// Complexity: O(n) where n is number of keys.
// Memory: Allocates slice of n strings.
func (m *MemoryKeyStore) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, ErrKeyStoreClosed
	}

	names := make([]string, 0, len(m.keys))
	for name := range m.keys {
		names = append(names, name)
	}
	return names, nil
}

// Close marks the store as closed and wipes all stored keys.
// After Close is called, all operations will return ErrKeyStoreClosed.
// Safe to call multiple times; subsequent calls are no-ops.
//
// Complexity: O(n) where n is number of keys.
func (m *MemoryKeyStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil // Already closed, no-op
	}

	m.closed = true

	// Wipe all keys before clearing the map.
	// Note: key is a copy of the map value (Go range semantics), but Wipe()
	// zeros the underlying byte slice arrays which are shared with the map entry.
	for _, key := range m.keys {
		key.Wipe()
	}
	m.keys = nil

	return nil
}

// Has returns true if a key exists in the store.
// More efficient than Load when you don't need the key data.
//
// Returns ErrKeyStoreClosed if the store has been closed.
//
// Complexity: O(1) average.
func (m *MemoryKeyStore) Has(name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return false, ErrKeyStoreClosed
	}

	_, exists := m.keys[name]
	return exists, nil
}

// Len returns the number of keys in the store.
// Useful for monitoring and testing.
//
// Returns 0 if the store is closed.
//
// Complexity: O(1).
func (m *MemoryKeyStore) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return 0
	}
	return len(m.keys)
}

// Clear removes all keys from the store.
// Securely wipes all key material before removal.
// Useful for testing. The store remains open after Clear().
//
// Returns ErrKeyStoreClosed if the store has been closed.
//
// Complexity: O(n) where n is number of keys.
func (m *MemoryKeyStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrKeyStoreClosed
	}

	// Wipe all keys before clearing the map.
	// Note: key is a copy of the map value (Go range semantics), but Wipe()
	// zeros the underlying byte slice arrays which are shared with the map entry.
	for _, key := range m.keys {
		key.Wipe()
	}
	// Reinitialize with default capacity (matches NewMemoryKeyStore)
	m.keys = make(map[string]EncryptedKey, 16)

	return nil
}

// copyEncryptedKey creates a deep copy of an EncryptedKey.
// Prevents external mutation of stored keys.
// Complexity: O(n) where n is total byte size.
func copyEncryptedKey(key EncryptedKey) EncryptedKey {
	cp := EncryptedKey{
		Name:      key.Name,
		Algorithm: key.Algorithm,
	}

	if key.PubKey != nil {
		cp.PubKey = make([]byte, len(key.PubKey))
		copy(cp.PubKey, key.PubKey)
	}

	if key.PrivKeyData != nil {
		cp.PrivKeyData = make([]byte, len(key.PrivKeyData))
		copy(cp.PrivKeyData, key.PrivKeyData)
	}

	if key.Salt != nil {
		cp.Salt = make([]byte, len(key.Salt))
		copy(cp.Salt, key.Salt)
	}

	if key.Nonce != nil {
		cp.Nonce = make([]byte, len(key.Nonce))
		copy(cp.Nonce, key.Nonce)
	}

	return cp
}

// Verify MemoryKeyStore implements EncryptedKeyStore interface.
var _ EncryptedKeyStore = (*MemoryKeyStore)(nil)
