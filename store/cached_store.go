package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// CachedObjectStore implements ObjectStore with multi-level caching
type CachedObjectStore[T any] struct {
	mu         sync.RWMutex
	cache      *MultiLevelCache[T]
	backing    BackingStore
	serializer Serializer[T]
	closed     bool
}

// NewCachedObjectStore creates a new cached object store
func NewCachedObjectStore[T any](
	backing BackingStore,
	serializer Serializer[T],
	l1Size, l2Size int,
) *CachedObjectStore[T] {
	if backing == nil {
		panic("backing store cannot be nil")
	}
	if serializer == nil {
		panic("serializer cannot be nil")
	}

	return &CachedObjectStore[T]{
		cache:      NewMultiLevelCache[T](l1Size, l2Size),
		backing:    backing,
		serializer: serializer,
		closed:     false,
	}
}

// Get retrieves an object by key
func (s *CachedObjectStore[T]) Get(ctx context.Context, key []byte) (T, error) {
	var zero T

	if s == nil {
		return zero, ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return zero, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if err := validateKey(key); err != nil {
		return zero, err
	}

	// Check cache
	keyStr := keyToString(key)
	if entry, _, ok := s.cache.Get(keyStr); ok {
		if entry.Deleted {
			return zero, ErrNotFound
		}
		return entry.Value, nil
	}

	// Cache miss - load from backing store
	data, err := s.backing.Get(key)
	if err != nil {
		return zero, ErrNotFound
	}

	// Deserialize
	obj, err := s.serializer.Unmarshal(data)
	if err != nil {
		return zero, fmt.Errorf("failed to unmarshal: %w", err)
	}

	// Store in cache
	s.cache.Set(keyStr, CacheEntry[T]{
		Value:   obj,
		Dirty:   false,
		Deleted: false,
	})

	return obj, nil
}

// Set stores an object with the given key
func (s *CachedObjectStore[T]) Set(ctx context.Context, key []byte, value T) error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if err := validateKey(key); err != nil {
		return err
	}

	// Store in cache with dirty flag
	keyStr := keyToString(key)
	s.cache.Set(keyStr, CacheEntry[T]{
		Value:   value,
		Dirty:   true,
		Deleted: false,
	})

	return nil
}

// Delete removes an object by key
func (s *CachedObjectStore[T]) Delete(ctx context.Context, key []byte) error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if err := validateKey(key); err != nil {
		return err
	}

	// Mark as deleted in cache
	keyStr := keyToString(key)
	var zero T
	s.cache.Set(keyStr, CacheEntry[T]{
		Value:   zero,
		Dirty:   true,
		Deleted: true,
	})

	return nil
}

// Has checks if a key exists in the store
func (s *CachedObjectStore[T]) Has(ctx context.Context, key []byte) (bool, error) {
	if s == nil {
		return false, ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return false, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if err := validateKey(key); err != nil {
		return false, err
	}

	// Check cache first
	keyStr := keyToString(key)
	if entry, _, ok := s.cache.Get(keyStr); ok {
		return !entry.Deleted, nil
	}

	// Check backing store
	return s.backing.Has(key)
}

// Iterator returns an iterator over a range of keys
func (s *CachedObjectStore[T]) Iterator(ctx context.Context, start, end []byte) (Iterator[T], error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	rawIter, err := s.backing.Iterator(start, end)
	if err != nil {
		return nil, err
	}

	return newCachedIterator(rawIter, s.serializer, false), nil
}

// ReverseIterator returns a reverse iterator over a range of keys
func (s *CachedObjectStore[T]) ReverseIterator(ctx context.Context, start, end []byte) (Iterator[T], error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	rawIter, err := s.backing.ReverseIterator(start, end)
	if err != nil {
		return nil, err
	}

	return newCachedIterator(rawIter, s.serializer, true), nil
}

// GetBatch retrieves multiple objects by keys
func (s *CachedObjectStore[T]) GetBatch(ctx context.Context, keys [][]byte) (map[string]T, error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if keys == nil {
		return make(map[string]T), nil
	}

	result := make(map[string]T)

	for _, key := range keys {
		if err := validateKey(key); err != nil {
			continue
		}

		obj, err := s.Get(ctx, key)
		if err == nil {
			result[keyToString(key)] = obj
		}
	}

	return result, nil
}

// SetBatch stores multiple objects atomically
func (s *CachedObjectStore[T]) SetBatch(ctx context.Context, items map[string]T) error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if items == nil {
		return nil
	}

	// Store all items in cache
	for keyStr, value := range items {
		s.cache.Set(keyStr, CacheEntry[T]{
			Value:   value,
			Dirty:   true,
			Deleted: false,
		})
	}

	return nil
}

// DeleteBatch removes multiple objects atomically
func (s *CachedObjectStore[T]) DeleteBatch(ctx context.Context, keys [][]byte) error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("store is closed")
	}
	s.mu.RUnlock()

	if keys == nil {
		return nil
	}

	// Mark all as deleted in cache
	var zero T
	for _, key := range keys {
		if err := validateKey(key); err != nil {
			continue
		}

		keyStr := keyToString(key)
		s.cache.Set(keyStr, CacheEntry[T]{
			Value:   zero,
			Dirty:   true,
			Deleted: true,
		})
	}

	return nil
}

// Flush writes any pending changes to the underlying storage
func (s *CachedObjectStore[T]) Flush(ctx context.Context) error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Get all dirty entries
	dirty := s.cache.GetDirtyEntries()

	// Sort keys for deterministic iteration (required for blockchain consensus)
	sortedKeys := make([]string, 0, len(dirty))
	for key := range dirty {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	// Write to backing store in deterministic order
	flushedKeys := make([]string, 0, len(dirty))
	for _, keyStr := range sortedKeys {
		entry := dirty[keyStr]
		key := []byte(keyStr)

		if entry.Deleted {
			// Delete from backing store
			if err := s.backing.Delete(key); err != nil {
				return fmt.Errorf("failed to delete key: %w", err)
			}
		} else {
			// Serialize and write
			data, err := s.serializer.Marshal(entry.Value)
			if err != nil {
				return fmt.Errorf("failed to marshal: %w", err)
			}

			if err := s.backing.Set(key, data); err != nil {
				return fmt.Errorf("failed to set key: %w", err)
			}
		}

		flushedKeys = append(flushedKeys, keyStr)
	}

	// Flush backing store
	if err := s.backing.Flush(); err != nil {
		return fmt.Errorf("failed to flush backing store: %w", err)
	}

	// Clear dirty flags
	s.cache.ClearDirtyFlags(flushedKeys)

	return nil
}

// Close releases any resources held by the store
func (s *CachedObjectStore[T]) Close() error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.cache.Clear()

	return s.backing.Close()
}

// cachedIterator wraps a raw iterator and deserializes values
type cachedIterator[T any] struct {
	mu         sync.RWMutex
	rawIter    RawIterator
	serializer Serializer[T]
	reverse    bool
	closed     bool
}

// newCachedIterator creates a new cached iterator
// This is an internal constructor - it panics if inputs are invalid
func newCachedIterator[T any](rawIter RawIterator, serializer Serializer[T], reverse bool) *cachedIterator[T] {
	if rawIter == nil {
		panic("newCachedIterator: rawIter cannot be nil")
	}
	if serializer == nil {
		panic("newCachedIterator: serializer cannot be nil")
	}

	return &cachedIterator[T]{
		rawIter:    rawIter,
		serializer: serializer,
		reverse:    reverse,
		closed:     false,
	}
}

// Valid returns true if the iterator is positioned at a valid entry
func (it *cachedIterator[T]) Valid() bool {
	if it == nil {
		return false
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed {
		return false
	}

	return it.rawIter.Valid()
}

// Next advances the iterator to the next entry
func (it *cachedIterator[T]) Next() error {
	if it == nil {
		return ErrIteratorClosed
	}

	it.mu.Lock()
	defer it.mu.Unlock()

	if it.closed {
		return ErrIteratorClosed
	}

	it.rawIter.Next()
	return nil
}

// Key returns the key at the current position
func (it *cachedIterator[T]) Key() ([]byte, error) {
	if it == nil {
		return nil, ErrIteratorClosed
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed {
		return nil, ErrIteratorClosed
	}

	if !it.rawIter.Valid() {
		return nil, fmt.Errorf("iterator not valid")
	}

	return copyKey(it.rawIter.Key()), nil
}

// Value returns the value at the current position
func (it *cachedIterator[T]) Value() (T, error) {
	var zero T

	if it == nil {
		return zero, ErrIteratorClosed
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed {
		return zero, ErrIteratorClosed
	}

	if !it.rawIter.Valid() {
		return zero, fmt.Errorf("iterator not valid")
	}

	data := it.rawIter.Value()
	return it.serializer.Unmarshal(data)
}

// Close releases resources held by the iterator
func (it *cachedIterator[T]) Close() error {
	if it == nil {
		return nil
	}

	it.mu.Lock()
	defer it.mu.Unlock()

	if it.closed {
		return nil
	}

	it.closed = true
	return it.rawIter.Close()
}
