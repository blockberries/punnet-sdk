package crypto

import (
	"errors"
	"fmt"
	"sync"
)

// Key signing constraints.
const (
	// MaxSignDataLength is the maximum allowed input length for Sign.
	// Ed25519 handles any length, but we cap for consistency with future backends.
	// 64MB should handle any reasonable signing use case.
	MaxSignDataLength = 64 * 1024 * 1024
)

// Keyring error types.
var (
	ErrKeyNotFound     = errors.New("key not found")
	ErrKeyExists       = errors.New("key already exists")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidKey      = errors.New("invalid key data")
	ErrDataTooLarge    = errors.New("data exceeds maximum sign length")
)

// validateKeyName validates a key name for security.
// Uses the shared ValidateKeyName function from keystore.go.
// Complexity: O(n) where n is name length.
func validateKeyName(name string) error {
	return ValidateKeyName(name)
}

// Keyring manages multiple signing keys.
// All methods are thread-safe.
type Keyring interface {
	// NewKey generates a new key with the given name and algorithm.
	// Returns ErrKeyExists if a key with this name already exists.
	// Complexity: O(1) for key generation + O(store.Put).
	NewKey(name string, algo Algorithm) (Signer, error)

	// ImportKey imports an existing private key.
	// Returns ErrKeyExists if a key with this name already exists.
	// Returns ErrInvalidKey if the key data is malformed.
	// Complexity: O(n) for key validation + O(store.Put).
	ImportKey(name string, privKey []byte, algo Algorithm) (Signer, error)

	// ExportKey exports a private key (may require password).
	// Returns ErrKeyNotFound if key doesn't exist.
	// Returns ErrInvalidPassword if password is incorrect (for encrypted keys).
	// Complexity: O(store.Get) + O(decryption if encrypted).
	ExportKey(name string, password string) ([]byte, error)

	// GetKey retrieves a signer by name.
	// Returns ErrKeyNotFound if key doesn't exist.
	// Complexity: O(store.Get) or O(1) if cached.
	GetKey(name string) (Signer, error)

	// ListKeys returns all key names.
	// Complexity: O(n) where n is number of keys.
	ListKeys() ([]string, error)

	// DeleteKey removes a key.
	// Returns ErrKeyNotFound if key doesn't exist.
	// Complexity: O(store.Delete).
	DeleteKey(name string) error

	// Sign signs data with the named key.
	// Returns ErrKeyNotFound if key doesn't exist.
	// Complexity: O(GetKey) + O(n) where n is data length.
	Sign(name string, data []byte) ([]byte, error)
}

// defaultKeyring implements Keyring with a pluggable KeyStore backend.
//
// CACHE SEMANTICS: Uses an approximate LRU cache for hot keys. Recency is
// updated only on cache misses (when keys are loaded from store), not on
// hits. This trades true LRU semantics for reduced lock contention on the
// read path - cache hits require only a read lock, not a write lock.
//
// For typical workloads with a small working set of keys, this provides
// excellent performance while maintaining correctness.
type defaultKeyring struct {
	store KeyStore

	// mu protects the cache
	mu sync.RWMutex
	// cache maps key names to signers for fast repeated access
	// Key insight: most signing operations use a small set of keys
	cache map[string]Signer
	// cacheOrder tracks access order for LRU eviction
	cacheOrder []string
	// maxCacheSize limits memory usage
	maxCacheSize int
}

// KeyringOption configures a Keyring.
type KeyringOption func(*defaultKeyring)

// WithCacheSize sets the maximum number of keys to cache.
// Default is 100. Set to 0 to disable caching.
func WithCacheSize(size int) KeyringOption {
	return func(k *defaultKeyring) {
		k.maxCacheSize = size
	}
}

// NewKeyring creates a new keyring with the given storage backend.
// Complexity: O(1).
func NewKeyring(store KeyStore, opts ...KeyringOption) Keyring {
	kr := &defaultKeyring{
		store:        store,
		cache:        make(map[string]Signer),
		cacheOrder:   make([]string, 0, 100),
		maxCacheSize: 100,
	}
	for _, opt := range opts {
		opt(kr)
	}
	return kr
}

// NewKey generates a new key.
func (kr *defaultKeyring) NewKey(name string, algo Algorithm) (Signer, error) {
	// Validate key name (prevents path traversal, injection attacks)
	if err := validateKeyName(name); err != nil {
		return nil, err
	}

	// Check if key already exists
	exists, err := kr.store.Has(name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrKeyExists
	}

	// Generate private key
	privKey, err := GeneratePrivateKey(algo)
	if err != nil {
		return nil, err
	}

	// Create entry
	entry := &KeyEntry{
		Name:       name,
		Algorithm:  algo,
		PrivateKey: privKey.Bytes(),
		PublicKey:  privKey.PublicKey().Bytes(),
		Encrypted:  false,
	}

	// Store
	if err := kr.store.Put(entry, false); err != nil {
		return nil, err
	}

	// Create signer and cache
	signer := NewSigner(privKey)
	kr.addToCache(name, signer)

	return signer, nil
}

// ImportKey imports an existing private key.
func (kr *defaultKeyring) ImportKey(name string, privKeyBytes []byte, algo Algorithm) (Signer, error) {
	// Validate key name (prevents path traversal, injection attacks)
	if err := validateKeyName(name); err != nil {
		return nil, err
	}

	// Fail fast for unimplemented algorithms
	if algo != AlgorithmEd25519 {
		return nil, fmt.Errorf("algorithm %s not yet implemented", algo)
	}

	// Check if key already exists
	exists, err := kr.store.Has(name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrKeyExists
	}

	// Parse and validate private key
	privKey, err := PrivateKeyFromBytes(algo, privKeyBytes)
	if err != nil {
		return nil, ErrInvalidKey
	}

	// Create entry
	entry := &KeyEntry{
		Name:       name,
		Algorithm:  algo,
		PrivateKey: privKey.Bytes(),
		PublicKey:  privKey.PublicKey().Bytes(),
		Encrypted:  false,
	}

	// Store
	if err := kr.store.Put(entry, false); err != nil {
		return nil, err
	}

	// Create signer and cache
	signer := NewSigner(privKey)
	kr.addToCache(name, signer)

	return signer, nil
}

// ExportKey exports a private key.
// Password is reserved for future encrypted keystore support.
// Note: Caller should zero the returned bytes when done with them.
func (kr *defaultKeyring) ExportKey(name string, password string) ([]byte, error) {
	entry, err := kr.store.Get(name)
	if err != nil {
		return nil, err
	}
	// Zero the entry's private key bytes after we've copied them
	defer Zeroize(entry.PrivateKey)

	// TODO: Implement decryption when encrypted keystores are supported
	if entry.Encrypted {
		return nil, errors.New("encrypted keys not yet supported")
	}

	// Return a copy to prevent external mutation
	result := make([]byte, len(entry.PrivateKey))
	copy(result, entry.PrivateKey)
	return result, nil
}

// GetKey retrieves a signer by name.
// Note: Between releasing the read lock and acquiring the write lock in addToCache,
// another goroutine may add the same key. This is handled correctly in addToCache
// (becomes move-to-front), so correctness is preserved. This is a deliberate
// trade-off: avoiding a single write lock on the hot path reduces lock contention.
func (kr *defaultKeyring) GetKey(name string) (Signer, error) {
	// Check cache first (hot path)
	kr.mu.RLock()
	if signer, ok := kr.cache[name]; ok {
		kr.mu.RUnlock()
		return signer, nil
	}
	kr.mu.RUnlock()

	// Load from store (potential duplicate work if racing, but correctness preserved)
	entry, err := kr.store.Get(name)
	if err != nil {
		return nil, err
	}
	// Zero the entry's private key bytes after we're done with them
	defer Zeroize(entry.PrivateKey)

	// Reconstruct signer
	privKey, err := PrivateKeyFromBytes(entry.Algorithm, entry.PrivateKey)
	if err != nil {
		return nil, ErrInvalidKey
	}

	signer := NewSigner(privKey)
	kr.addToCache(name, signer)

	return signer, nil
}

// ListKeys returns all key names.
func (kr *defaultKeyring) ListKeys() ([]string, error) {
	return kr.store.List()
}

// DeleteKey removes a key.
//
// CONSISTENCY: Cache is invalidated before store deletion. If store.Delete
// fails (e.g., network error on remote store), the key remains in the store.
// GetKey will reload it on next access, restoring consistency (eventual
// consistency). This is a deliberate choice: cache-first invalidation prevents
// serving stale data, and store self-healing handles transient failures.
// ASSUMPTION: Store failures are transient; permanent store corruption
// requires out-of-band recovery.
func (kr *defaultKeyring) DeleteKey(name string) error {
	// Remove from cache first (prevents serving stale data on store failure)
	kr.mu.Lock()
	delete(kr.cache, name)
	// Remove from order slice
	for i, n := range kr.cacheOrder {
		if n == name {
			kr.cacheOrder = append(kr.cacheOrder[:i], kr.cacheOrder[i+1:]...)
			break
		}
	}
	kr.mu.Unlock()

	return kr.store.Delete(name)
}

// Sign signs data with the named key.
// Validates data length to ensure compatibility with all backends.
// Complexity: O(GetKey) + O(n) where n is data length.
func (kr *defaultKeyring) Sign(name string, data []byte) ([]byte, error) {
	// Bounds check for data length (future HSM backends may have limits)
	if len(data) > MaxSignDataLength {
		return nil, ErrDataTooLarge
	}

	signer, err := kr.GetKey(name)
	if err != nil {
		return nil, err
	}
	return signer.Sign(data)
}

// addToCache adds a signer to the cache with LRU eviction.
// Complexity: O(1) amortized.
func (kr *defaultKeyring) addToCache(name string, signer Signer) {
	if kr.maxCacheSize <= 0 {
		return
	}

	kr.mu.Lock()
	defer kr.mu.Unlock()

	// Already in cache? Move to front
	if _, ok := kr.cache[name]; ok {
		kr.moveToFront(name)
		return
	}

	// Evict if at capacity
	for len(kr.cache) >= kr.maxCacheSize && len(kr.cacheOrder) > 0 {
		oldest := kr.cacheOrder[0]
		kr.cacheOrder = kr.cacheOrder[1:]
		delete(kr.cache, oldest)
	}

	// Add to cache
	kr.cache[name] = signer
	kr.cacheOrder = append(kr.cacheOrder, name)
}

// moveToFront marks a key as recently used by moving it to the end of cacheOrder.
// cacheOrder is maintained oldest-first: eviction pops from index 0, newest
// items are appended to the end. The name "moveToFront" refers to moving to
// the front of the recency queue (most recently used), not the slice position.
// Complexity: O(n) where n is cache size. With default maxCacheSize=100, this
// is acceptable; for larger caches, consider container/list for O(1) operations.
func (kr *defaultKeyring) moveToFront(name string) {
	for i, n := range kr.cacheOrder {
		if n == name {
			kr.cacheOrder = append(kr.cacheOrder[:i], kr.cacheOrder[i+1:]...)
			kr.cacheOrder = append(kr.cacheOrder, name)
			return
		}
	}
}
