package crypto

import (
	"errors"
	"fmt"
	"sync"
)

// Key name constraints.
const (
	// MaxKeyNameLength is the maximum allowed length for a key name.
	// Prevents resource exhaustion attacks.
	MaxKeyNameLength = 256

	// MaxSignDataLength is the maximum allowed input length for Sign.
	// Ed25519 handles any length, but we cap for consistency with future backends.
	// 64MB should handle any reasonable signing use case.
	MaxSignDataLength = 64 * 1024 * 1024
)

// Keyring error types.
// Note: ErrInvalidPassword is defined in errors.go for shared use.
var (
	ErrKeyNotFound   = errors.New("key not found")
	ErrKeyExists     = errors.New("key already exists")
	ErrInvalidKey    = errors.New("invalid key data")
	ErrDataTooLarge  = errors.New("data exceeds maximum sign length")
	ErrKeyringClosed = errors.New("keyring is closed")
)

// validateKeyNameSimple validates a key name for security.
// Rejects empty names, overly long names, and names with dangerous characters.
// Complexity: O(n) where n is name length.
// Note: This is a simpler version used by Keyring; see file_keystore.go for
// the full validation used by FileKeyStore.
func validateKeyNameSimple(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidKeyName)
	}
	if len(name) > MaxKeyNameLength {
		return fmt.Errorf("%w: name too long (max %d characters)", ErrInvalidKeyName, MaxKeyNameLength)
	}
	// Reject path separators, control chars, null bytes.
	// This prevents path traversal in file-based backends.
	for _, r := range name {
		if r < 32 || r == '/' || r == '\\' || r == 0 {
			return fmt.Errorf("%w: name contains invalid characters", ErrInvalidKeyName)
		}
	}
	return nil
}

// Keyring manages multiple signing keys.
// All methods are thread-safe.
// Implements io.Closer for graceful shutdown with key zeroization.
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

	// Close releases all resources and zeroizes all cached private keys.
	// After Close is called, all other methods will return ErrKeyringClosed.
	//
	// Shutdown order:
	//   1. Zeroize all cached signers (private keys in memory)
	//   2. For each key in the store: zeroize the private key data, then delete
	//   3. Close the underlying key store (if it supports Close)
	//
	// If any store.Delete operations fail, errors are aggregated and returned.
	// The keyring is still marked as closed even if some deletions fail.
	// This ensures the keyring cannot be used again, but alerts the caller
	// that some key material may remain on disk.
	//
	// This method is safe to call multiple times; subsequent calls are no-ops.
	// Complexity: O(n) where n is total number of keys (cached + stored).
	Close() error
}

// defaultKeyring implements Keyring with a pluggable SimpleKeyStore backend.
//
// CACHE SEMANTICS: Uses an approximate LRU cache for hot keys. Recency is
// updated only on cache misses (when keys are loaded from store), not on
// hits. This trades true LRU semantics for reduced lock contention on the
// read path - cache hits require only a read lock, not a write lock.
//
// For typical workloads with a small working set of keys, this provides
// excellent performance while maintaining correctness.
type defaultKeyring struct {
	store SimpleKeyStore

	// mu protects the cache and closed flag
	mu sync.RWMutex
	// cache maps key names to signers for fast repeated access
	// Key insight: most signing operations use a small set of keys
	cache map[string]Signer
	// cacheOrder tracks access order for LRU eviction
	cacheOrder []string
	// maxCacheSize limits memory usage
	maxCacheSize int
	// closed indicates if the keyring has been closed
	closed bool
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
func NewKeyring(store SimpleKeyStore, opts ...KeyringOption) Keyring {
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
	kr.mu.RLock()
	if err := kr.checkClosed(); err != nil {
		kr.mu.RUnlock()
		return nil, err
	}
	kr.mu.RUnlock()

	// Validate key name (prevents path traversal, injection attacks)
	if err := validateKeyNameSimple(name); err != nil {
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
	kr.mu.RLock()
	if err := kr.checkClosed(); err != nil {
		kr.mu.RUnlock()
		return nil, err
	}
	kr.mu.RUnlock()

	// Validate key name (prevents path traversal, injection attacks)
	if err := validateKeyNameSimple(name); err != nil {
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
	kr.mu.RLock()
	if err := kr.checkClosed(); err != nil {
		kr.mu.RUnlock()
		return nil, err
	}
	kr.mu.RUnlock()

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
	if err := kr.checkClosed(); err != nil {
		kr.mu.RUnlock()
		return nil, err
	}
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
	kr.mu.RLock()
	if err := kr.checkClosed(); err != nil {
		kr.mu.RUnlock()
		return nil, err
	}
	kr.mu.RUnlock()

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
	if err := kr.checkClosed(); err != nil {
		kr.mu.Unlock()
		return err
	}

	// Remove from cache and zeroize if present
	if signer, ok := kr.cache[name]; ok {
		zeroizeSigner(signer)
		delete(kr.cache, name)
	}
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
//
// Thread safety: This method holds a read lock for the entire duration of the
// signing operation. This prevents Close() from zeroizing the signer's private
// key while signing is in progress.
func (kr *defaultKeyring) Sign(name string, data []byte) ([]byte, error) {
	// Bounds check for data length (future HSM backends may have limits)
	if len(data) > MaxSignDataLength {
		return nil, ErrDataTooLarge
	}

	kr.mu.RLock()
	defer kr.mu.RUnlock()

	if kr.closed {
		return nil, ErrKeyringClosed
	}

	// Check cache first (hot path, already holding lock)
	if signer, ok := kr.cache[name]; ok {
		return signer.Sign(data)
	}

	// Not in cache - need to load from store.
	// Note: We can't call addToCache here because it needs a write lock.
	// We load, sign, and let GetKey populate the cache on next access.
	entry, err := kr.store.Get(name)
	if err != nil {
		return nil, err
	}
	defer Zeroize(entry.PrivateKey)

	privKey, err := PrivateKeyFromBytes(entry.Algorithm, entry.PrivateKey)
	if err != nil {
		return nil, ErrInvalidKey
	}

	signer := NewSigner(privKey)
	sig, err := signer.Sign(data)

	// Zeroize the temporary signer's key since we can't cache it
	zeroizeSigner(signer)

	return sig, err
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

	// Evict if at capacity, zeroizing evicted keys
	for len(kr.cache) >= kr.maxCacheSize && len(kr.cacheOrder) > 0 {
		oldest := kr.cacheOrder[0]
		kr.cacheOrder = kr.cacheOrder[1:]
		if oldSigner, ok := kr.cache[oldest]; ok {
			zeroizeSigner(oldSigner)
		}
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

// Close releases all resources and zeroizes all private keys.
// Aggregates any errors from store delete operations and returns them.
// The keyring is marked as closed even if some deletions fail.
// Safe to call multiple times.
// Complexity: O(n) where n is total number of keys.
func (kr *defaultKeyring) Close() error {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	if kr.closed {
		return nil // Already closed, no-op
	}

	// Mark as closed first - even if we have errors below, the keyring
	// should not be usable again
	kr.closed = true

	// Step 1: Zeroize all cached signers
	for name, signer := range kr.cache {
		zeroizeSigner(signer)
		delete(kr.cache, name)
	}
	kr.cacheOrder = nil

	// Step 2: Zeroize and delete all keys in the store
	// This ensures no unzeroed key material remains on disk for file-backed stores
	var deleteErrors []error
	names, err := kr.store.List()
	if err != nil {
		// Can't list keys - store may be in bad state, but we're still closed
		deleteErrors = append(deleteErrors, fmt.Errorf("failed to list keys: %w", err))
	} else {
		for _, name := range names {
			entry, err := kr.store.Get(name)
			if err != nil {
				// Key may have been deleted concurrently, skip
				continue
			}
			// Zeroize the private key data in the entry
			Zeroize(entry.PrivateKey)

			// Delete from store
			if err := kr.store.Delete(name); err != nil {
				deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete %s: %w", name, err))
			}
		}
	}

	// Step 3: Close the underlying store if it supports Close
	if closer, ok := kr.store.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to close store: %w", err))
		}
	}

	// Return aggregated errors
	if len(deleteErrors) > 0 {
		return fmt.Errorf("close completed with %d error(s): %v", len(deleteErrors), deleteErrors)
	}
	return nil
}

// zeroizeSigner attempts to zeroize the private key within a signer.
// Works with BasicSigner which wraps a PrivateKey.
func zeroizeSigner(s Signer) {
	// Type assert to access the underlying PrivateKey
	if bs, ok := s.(*BasicSigner); ok {
		if bs.privateKey != nil {
			bs.privateKey.Zeroize()
		}
	}
}

// checkClosed returns ErrKeyringClosed if the keyring is closed.
// Must be called with at least a read lock held.
func (kr *defaultKeyring) checkClosed() error {
	if kr.closed {
		return ErrKeyringClosed
	}
	return nil
}
