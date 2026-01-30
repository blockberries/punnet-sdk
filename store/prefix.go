package store

import (
	"bytes"
	"fmt"
	"sync"
)

// PrefixStore wraps a BackingStore and prefixes all keys
// This provides namespace isolation for modules
type PrefixStore struct {
	mu      sync.RWMutex
	parent  BackingStore
	prefix  []byte
	closed  bool
}

// NewPrefixStore creates a new prefix store
func NewPrefixStore(parent BackingStore, prefix []byte) *PrefixStore {
	if parent == nil {
		panic("parent store cannot be nil")
	}
	if len(prefix) == 0 {
		panic("prefix cannot be empty")
	}

	// Create defensive copy of prefix
	prefixCopy := make([]byte, len(prefix))
	copy(prefixCopy, prefix)

	return &PrefixStore{
		parent: parent,
		prefix: prefixCopy,
		closed: false,
	}
}

// prefixKey adds the prefix to a key
func (ps *PrefixStore) prefixKey(key []byte) []byte {
	if key == nil {
		return nil
	}

	prefixed := make([]byte, len(ps.prefix)+len(key))
	copy(prefixed, ps.prefix)
	copy(prefixed[len(ps.prefix):], key)
	return prefixed
}

// Get retrieves raw bytes by key
func (ps *PrefixStore) Get(key []byte) ([]byte, error) {
	if ps == nil {
		return nil, ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return nil, fmt.Errorf("store is closed")
	}

	if err := validateKey(key); err != nil {
		return nil, err
	}

	return ps.parent.Get(ps.prefixKey(key))
}

// Set stores raw bytes with the given key
func (ps *PrefixStore) Set(key []byte, value []byte) error {
	if ps == nil {
		return ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return fmt.Errorf("store is closed")
	}

	if err := validateKey(key); err != nil {
		return err
	}

	return ps.parent.Set(ps.prefixKey(key), value)
}

// Delete removes a key
func (ps *PrefixStore) Delete(key []byte) error {
	if ps == nil {
		return ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return fmt.Errorf("store is closed")
	}

	if err := validateKey(key); err != nil {
		return err
	}

	return ps.parent.Delete(ps.prefixKey(key))
}

// Has checks if a key exists
func (ps *PrefixStore) Has(key []byte) (bool, error) {
	if ps == nil {
		return false, ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return false, fmt.Errorf("store is closed")
	}

	if err := validateKey(key); err != nil {
		return false, err
	}

	return ps.parent.Has(ps.prefixKey(key))
}

// Iterator returns an iterator over a range of keys
func (ps *PrefixStore) Iterator(start, end []byte) (RawIterator, error) {
	if ps == nil {
		return nil, ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Prefix the start and end keys
	var prefixedStart, prefixedEnd []byte
	if start != nil {
		prefixedStart = ps.prefixKey(start)
	} else {
		// Start from prefix
		prefixedStart = ps.prefix
	}

	if end != nil {
		prefixedEnd = ps.prefixKey(end)
	} else {
		// End at prefix boundary
		prefixedEnd = prefixBound(ps.prefix)
	}

	iter, err := ps.parent.Iterator(prefixedStart, prefixedEnd)
	if err != nil {
		return nil, err
	}

	return newPrefixIterator(iter, ps.prefix), nil
}

// ReverseIterator returns a reverse iterator over a range of keys
func (ps *PrefixStore) ReverseIterator(start, end []byte) (RawIterator, error) {
	if ps == nil {
		return nil, ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Prefix the start and end keys
	var prefixedStart, prefixedEnd []byte
	if start != nil {
		prefixedStart = ps.prefixKey(start)
	} else {
		prefixedStart = ps.prefix
	}

	if end != nil {
		prefixedEnd = ps.prefixKey(end)
	} else {
		prefixedEnd = prefixBound(ps.prefix)
	}

	iter, err := ps.parent.ReverseIterator(prefixedStart, prefixedEnd)
	if err != nil {
		return nil, err
	}

	return newPrefixIterator(iter, ps.prefix), nil
}

// Flush writes pending changes
func (ps *PrefixStore) Flush() error {
	if ps == nil {
		return ErrStoreNil
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.closed {
		return fmt.Errorf("store is closed")
	}

	return ps.parent.Flush()
}

// Close releases resources
func (ps *PrefixStore) Close() error {
	if ps == nil {
		return ErrStoreNil
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.closed {
		return nil
	}

	ps.closed = true
	// Don't close parent - it may be shared
	return nil
}

// prefixBound returns the end boundary for a prefix
// This is used to iterate over all keys with a given prefix
func prefixBound(prefix []byte) []byte {
	if prefix == nil {
		return nil
	}

	// Find the last non-0xFF byte
	bound := make([]byte, len(prefix))
	copy(bound, prefix)

	for i := len(bound) - 1; i >= 0; i-- {
		if bound[i] < 0xFF {
			bound[i]++
			return bound[:i+1]
		}
	}

	// All bytes are 0xFF, no upper bound
	return nil
}

// prefixIterator wraps a RawIterator and strips the prefix from keys
type prefixIterator struct {
	mu     sync.RWMutex
	parent RawIterator
	prefix []byte
	closed bool
}

// newPrefixIterator creates a new prefix iterator
// This is an internal constructor - it panics if inputs are invalid
func newPrefixIterator(parent RawIterator, prefix []byte) *prefixIterator {
	if parent == nil {
		panic("newPrefixIterator: parent cannot be nil")
	}

	// Note: prefix can be nil/empty for iteration over entire store
	return &prefixIterator{
		parent: parent,
		prefix: prefix,
		closed: false,
	}
}

// Valid returns true if positioned at a valid entry
func (pi *prefixIterator) Valid() bool {
	if pi == nil {
		return false
	}

	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if pi.closed {
		return false
	}

	if !pi.parent.Valid() {
		return false
	}

	// Check if key still has our prefix
	key := pi.parent.Key()
	return bytes.HasPrefix(key, pi.prefix)
}

// Next advances to the next entry
func (pi *prefixIterator) Next() {
	if pi == nil {
		return
	}

	pi.mu.Lock()
	defer pi.mu.Unlock()

	if pi.closed {
		return
	}

	pi.parent.Next()
}

// Key returns the current key with prefix stripped
func (pi *prefixIterator) Key() []byte {
	if pi == nil {
		return nil
	}

	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if pi.closed {
		return nil
	}

	if !pi.parent.Valid() {
		return nil
	}

	key := pi.parent.Key()
	if !bytes.HasPrefix(key, pi.prefix) {
		return nil
	}

	// Return unprefixed key (defensive copy)
	unprefixed := key[len(pi.prefix):]
	result := make([]byte, len(unprefixed))
	copy(result, unprefixed)
	return result
}

// Value returns the current value
func (pi *prefixIterator) Value() []byte {
	if pi == nil {
		return nil
	}

	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if pi.closed {
		return nil
	}

	return pi.parent.Value()
}

// Error returns any error that occurred during iteration
func (pi *prefixIterator) Error() error {
	if pi == nil {
		return nil
	}

	pi.mu.RLock()
	defer pi.mu.RUnlock()

	if pi.closed {
		return ErrIteratorClosed
	}

	return pi.parent.Error()
}

// Close releases iterator resources
func (pi *prefixIterator) Close() error {
	if pi == nil {
		return nil
	}

	pi.mu.Lock()
	defer pi.mu.Unlock()

	if pi.closed {
		return nil
	}

	pi.closed = true
	return pi.parent.Close()
}
