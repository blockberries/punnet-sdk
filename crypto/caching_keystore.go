package crypto

import (
	"container/list"
	"io"
	"sync"
)

// CachingKeyStore wraps an EncryptedKeyStore backend with an in-memory LRU cache.
// Provides read-through caching for high-frequency key access patterns.
//
// Performance characteristics:
//   - Cache hit:  O(1) lookup, zero disk I/O
//   - Cache miss: O(1) + backend latency (typically disk I/O)
//   - Store:      Write-through to both cache and backend
//   - Delete:     Removes from both cache and backend
//
// Thread-safe via RWMutex. Optimized for read-heavy workloads.
//
// Memory: ~200 bytes overhead per cached entry + key data size.
// For 10k entries with 64-byte keys: ~2.6 MB cache overhead.
type CachingKeyStore struct {
	backend  EncryptedKeyStore
	capacity int

	mu    sync.RWMutex
	cache map[string]*list.Element // name -> LRU list element
	lru   *list.List               // LRU eviction list (front = most recent)

	// Stats for monitoring cache efficiency
	hits   uint64
	misses uint64

	closed bool
}

// cacheEntry holds a cached key and its name for LRU tracking.
type cacheEntry struct {
	name string
	key  EncryptedKey
}

// NewCachingKeyStore wraps a backend store with an in-memory LRU cache.
//
// Parameters:
//   - backend: The underlying EncryptedKeyStore (e.g., FileKeyStore)
//   - capacity: Maximum number of keys to cache (must be > 0)
//
// Cache behavior:
//   - Load: Checks cache first, falls back to backend on miss
//   - Store: Writes to both cache and backend (write-through)
//   - Delete: Removes from both cache and backend
//   - List: Delegates to backend (cache may be partial)
//
// Complexity: O(1) for cache operations, backend operations vary.
func NewCachingKeyStore(backend EncryptedKeyStore, capacity int) *CachingKeyStore {
	if capacity <= 0 {
		capacity = 100 // Sensible default
	}
	return &CachingKeyStore{
		backend:  backend,
		capacity: capacity,
		cache:    make(map[string]*list.Element, capacity),
		lru:      list.New(),
	}
}

// Load retrieves a key, checking cache first before falling back to backend.
//
// Complexity:
//   - Cache hit: O(1), zero allocations on hot path
//   - Cache miss: O(1) + backend Load latency
//
// Returns ErrKeyStoreClosed if the store has been closed.
// Returns ErrKeyStoreNotFound if key doesn't exist in backend.
func (c *CachingKeyStore) Load(name string) (EncryptedKey, error) {
	// Fast path: check cache with read lock
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return EncryptedKey{}, ErrKeyStoreClosed
	}

	if elem, ok := c.cache[name]; ok {
		entry := elem.Value.(*cacheEntry)
		// Copy key data to prevent external mutation
		key := copyEncryptedKey(entry.key)
		c.mu.RUnlock()

		// Promote to front of LRU (requires write lock)
		c.mu.Lock()
		if !c.closed { // Re-check after lock upgrade
			c.lru.MoveToFront(elem)
			c.hits++
		}
		c.mu.Unlock()

		return key, nil
	}
	c.mu.RUnlock()

	// Cache miss: load from backend
	key, err := c.backend.Load(name)
	if err != nil {
		c.mu.Lock()
		if !c.closed {
			c.misses++
		}
		c.mu.Unlock()
		return EncryptedKey{}, err
	}

	// Add to cache
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return EncryptedKey{}, ErrKeyStoreClosed
	}

	c.misses++
	c.addToCache(name, key)

	return copyEncryptedKey(key), nil
}

// Store saves a key to both cache and backend (write-through).
//
// Complexity: O(1) + backend Store latency
//
// Returns ErrKeyStoreClosed if the store has been closed.
// Returns ErrKeyStoreExists if key already exists.
func (c *CachingKeyStore) Store(name string, key EncryptedKey) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrKeyStoreClosed
	}
	c.mu.Unlock()

	// Write to backend first (ensures durability before caching)
	if err := c.backend.Store(name, key); err != nil {
		return err
	}

	// Add to cache after successful backend write
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil // Backend write succeeded, cache miss is acceptable
	}

	c.addToCache(name, key)
	return nil
}

// Delete removes a key from both cache and backend.
//
// Complexity: O(1) + backend Delete latency
//
// Returns ErrKeyStoreClosed if the store has been closed.
// Returns ErrKeyStoreNotFound if key doesn't exist.
func (c *CachingKeyStore) Delete(name string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrKeyStoreClosed
	}

	// Remove from cache first
	c.removeFromCache(name)
	c.mu.Unlock()

	// Delete from backend
	return c.backend.Delete(name)
}

// List returns all key names from the backend.
// Note: Cache may contain only a subset of keys, so we delegate to backend.
//
// Complexity: Backend List complexity (typically O(n))
//
// Returns ErrKeyStoreClosed if the store has been closed.
func (c *CachingKeyStore) List() ([]string, error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrKeyStoreClosed
	}
	c.mu.RUnlock()

	return c.backend.List()
}

// Close releases cache resources and optionally closes the backend.
// After Close, all operations return ErrKeyStoreClosed.
// Safe to call multiple times (idempotent).
//
// If backend implements io.Closer, it will be closed as well.
func (c *CachingKeyStore) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	// Wipe all cached keys to minimize memory exposure
	for _, elem := range c.cache {
		entry := elem.Value.(*cacheEntry)
		entry.key.Wipe()
	}

	// Clear cache structures
	c.cache = nil
	c.lru = nil

	// Close backend if it implements io.Closer
	if closer, ok := c.backend.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

// Stats returns cache hit/miss statistics.
// Useful for monitoring cache efficiency.
//
// Returns (hits, misses, hitRate) where hitRate is in range [0.0, 1.0].
func (c *CachingKeyStore) Stats() (hits, misses uint64, hitRate float64) {
	c.mu.RLock()
	hits = c.hits
	misses = c.misses
	c.mu.RUnlock()

	total := hits + misses
	if total == 0 {
		return hits, misses, 0.0
	}
	return hits, misses, float64(hits) / float64(total)
}

// Len returns the current number of cached entries.
func (c *CachingKeyStore) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cache == nil {
		return 0
	}
	return len(c.cache)
}

// Capacity returns the maximum cache capacity.
func (c *CachingKeyStore) Capacity() int {
	return c.capacity
}

// Invalidate removes a specific key from the cache without touching the backend.
// Useful when you know the backend has changed externally.
func (c *CachingKeyStore) Invalidate(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeFromCache(name)
}

// InvalidateAll clears the entire cache without touching the backend.
// Useful for cache consistency after bulk backend operations.
func (c *CachingKeyStore) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Wipe all cached keys
	for _, elem := range c.cache {
		entry := elem.Value.(*cacheEntry)
		entry.key.Wipe()
	}

	c.cache = make(map[string]*list.Element, c.capacity)
	c.lru = list.New()
}

// addToCache adds a key to the cache, evicting LRU entry if at capacity.
// Must be called with write lock held.
func (c *CachingKeyStore) addToCache(name string, key EncryptedKey) {
	// Check if already cached (update existing)
	if elem, ok := c.cache[name]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.key.Wipe() // Clear old data
		entry.key = copyEncryptedKey(key)
		c.lru.MoveToFront(elem)
		return
	}

	// Evict LRU if at capacity
	if len(c.cache) >= c.capacity {
		c.evictLRU()
	}

	// Add new entry
	entry := &cacheEntry{
		name: name,
		key:  copyEncryptedKey(key),
	}
	elem := c.lru.PushFront(entry)
	c.cache[name] = elem
}

// removeFromCache removes an entry from the cache.
// Must be called with write lock held.
func (c *CachingKeyStore) removeFromCache(name string) {
	if elem, ok := c.cache[name]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.key.Wipe() // Zero sensitive data
		c.lru.Remove(elem)
		delete(c.cache, name)
	}
}

// evictLRU removes the least recently used entry from the cache.
// Must be called with write lock held.
func (c *CachingKeyStore) evictLRU() {
	back := c.lru.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*cacheEntry)
	entry.key.Wipe() // Zero sensitive data before eviction
	c.lru.Remove(back)
	delete(c.cache, entry.name)
}

// Verify CachingKeyStore implements EncryptedKeyStore interface.
var _ EncryptedKeyStore = (*CachingKeyStore)(nil)
