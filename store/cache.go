package store

import (
	"container/list"
	"sync"
)

// CacheLevel represents a cache level (L1, L2, or L3)
type CacheLevel int

const (
	// L1Cache is the smallest, fastest cache (10k entries)
	L1Cache CacheLevel = iota
	// L2Cache is the medium cache (100k entries)
	L2Cache
	// L3Cache is the backing store (disk/IAVL)
	L3Cache
)

// CacheEntry represents a cached value with metadata
type CacheEntry[T any] struct {
	// Value is the cached object
	Value T

	// Dirty indicates if the value has been modified
	Dirty bool

	// Deleted indicates if the value has been deleted
	Deleted bool
}

// Cache is a thread-safe LRU cache
type Cache[T any] struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element
	lru      *list.List
	hits     uint64
	misses   uint64
}

// cacheItem is an internal type for LRU list elements
type cacheItem[T any] struct {
	key   string
	entry CacheEntry[T]
}

// NewCache creates a new LRU cache with the given capacity
func NewCache[T any](capacity int) *Cache[T] {
	if capacity <= 0 {
		capacity = 1
	}
	return &Cache[T]{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// Get retrieves a value from the cache
// Returns the entry and a boolean indicating if it was found
func (c *Cache[T]) Get(key string) (CacheEntry[T], bool) {
	if c == nil {
		var zero CacheEntry[T]
		return zero, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.misses++
		var zero CacheEntry[T]
		return zero, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	c.hits++

	item := elem.Value.(*cacheItem[T])
	return item.entry, true
}

// Set stores a value in the cache
func (c *Cache[T]) Set(key string, entry CacheEntry[T]) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if elem, ok := c.items[key]; ok {
		// Update existing entry
		c.lru.MoveToFront(elem)
		item := elem.Value.(*cacheItem[T])
		item.entry = entry
		return
	}

	// Add new entry
	item := &cacheItem[T]{
		key:   key,
		entry: entry,
	}
	elem := c.lru.PushFront(item)
	c.items[key] = elem

	// Evict if over capacity
	if c.lru.Len() > c.capacity {
		c.evictLRU()
	}
}

// Delete removes a value from the cache
func (c *Cache[T]) Delete(key string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.lru.Remove(elem)
		delete(c.items, key)
	}
}

// Has checks if a key exists in the cache
func (c *Cache[T]) Has(key string) bool {
	if c == nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.items[key]
	return ok
}

// Clear removes all entries from the cache
func (c *Cache[T]) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lru.Init()
}

// Len returns the number of entries in the cache
func (c *Cache[T]) Len() int {
	if c == nil {
		return 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lru.Len()
}

// Stats returns cache statistics
func (c *Cache[T]) Stats() (hits, misses uint64) {
	if c == nil {
		return 0, 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.hits, c.misses
}

// evictLRU removes the least recently used entry
// Must be called with lock held
func (c *Cache[T]) evictLRU() {
	elem := c.lru.Back()
	if elem != nil {
		c.lru.Remove(elem)
		item := elem.Value.(*cacheItem[T])
		delete(c.items, item.key)
	}
}

// GetDirtyEntries returns all dirty entries in the cache
// This is used for flushing changes to the backing store
func (c *Cache[T]) GetDirtyEntries() map[string]CacheEntry[T] {
	if c == nil {
		return make(map[string]CacheEntry[T])
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	dirty := make(map[string]CacheEntry[T])
	for key, elem := range c.items {
		item := elem.Value.(*cacheItem[T])
		if item.entry.Dirty {
			// NOTE: This creates a shallow copy of CacheEntry[T].
			// If T contains slices, maps, or pointers, the caller must not mutate the Value field.
			// Typed stores (AccountStore, BalanceStore, ValidatorStore) are responsible for
			// implementing proper defensive copying for their specific types.
			dirty[key] = CacheEntry[T]{
				Value:   item.entry.Value,
				Dirty:   item.entry.Dirty,
				Deleted: item.entry.Deleted,
			}
		}
	}
	return dirty
}

// ClearDirtyFlag clears the dirty flag for a key
func (c *Cache[T]) ClearDirtyFlag(key string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		item := elem.Value.(*cacheItem[T])
		item.entry.Dirty = false
	}
}

// MultiLevelCache implements a three-level cache hierarchy
// L1: Fast, small cache (10k entries)
// L2: Medium cache (100k entries)
// L3: Backing store (IAVL/disk)
type MultiLevelCache[T any] struct {
	l1 *Cache[T]
	l2 *Cache[T]
}

// NewMultiLevelCache creates a new multi-level cache
func NewMultiLevelCache[T any](l1Size, l2Size int) *MultiLevelCache[T] {
	return &MultiLevelCache[T]{
		l1: NewCache[T](l1Size),
		l2: NewCache[T](l2Size),
	}
}

// Get retrieves a value from the cache hierarchy
// Checks L1, then L2, promoting values up the hierarchy
func (mc *MultiLevelCache[T]) Get(key string) (CacheEntry[T], CacheLevel, bool) {
	if mc == nil {
		var zero CacheEntry[T]
		return zero, L3Cache, false
	}

	// Try L1 first
	if entry, ok := mc.l1.Get(key); ok {
		return entry, L1Cache, true
	}

	// Try L2
	if entry, ok := mc.l2.Get(key); ok {
		// Promote to L1
		mc.l1.Set(key, entry)
		return entry, L2Cache, true
	}

	// Not found in cache
	var zero CacheEntry[T]
	return zero, L3Cache, false
}

// Set stores a value in the L1 cache
func (mc *MultiLevelCache[T]) Set(key string, entry CacheEntry[T]) {
	if mc == nil {
		return
	}

	mc.l1.Set(key, entry)
}

// Delete removes a value from all cache levels
func (mc *MultiLevelCache[T]) Delete(key string) {
	if mc == nil {
		return
	}

	mc.l1.Delete(key)
	mc.l2.Delete(key)
}

// Clear removes all entries from all cache levels
func (mc *MultiLevelCache[T]) Clear() {
	if mc == nil {
		return
	}

	mc.l1.Clear()
	mc.l2.Clear()
}

// GetDirtyEntries returns all dirty entries from both cache levels
func (mc *MultiLevelCache[T]) GetDirtyEntries() map[string]CacheEntry[T] {
	if mc == nil {
		return make(map[string]CacheEntry[T])
	}

	dirty := make(map[string]CacheEntry[T])

	// Get dirty entries from L1
	for key, entry := range mc.l1.GetDirtyEntries() {
		dirty[key] = entry
	}

	// Get dirty entries from L2 (if not already in L1)
	for key, entry := range mc.l2.GetDirtyEntries() {
		if _, exists := dirty[key]; !exists {
			dirty[key] = entry
		}
	}

	return dirty
}

// ClearDirtyFlags clears dirty flags for all specified keys
func (mc *MultiLevelCache[T]) ClearDirtyFlags(keys []string) {
	if mc == nil {
		return
	}

	for _, key := range keys {
		mc.l1.ClearDirtyFlag(key)
		mc.l2.ClearDirtyFlag(key)
	}
}

// Stats returns combined cache statistics
func (mc *MultiLevelCache[T]) Stats() (l1Hits, l1Misses, l2Hits, l2Misses uint64) {
	if mc == nil {
		return 0, 0, 0, 0
	}

	l1Hits, l1Misses = mc.l1.Stats()
	l2Hits, l2Misses = mc.l2.Stats()
	return
}
