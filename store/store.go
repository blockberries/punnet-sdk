package store

import (
	"context"
	"errors"
)

var (
	// ErrNotFound is returned when a key is not found in the store
	ErrNotFound = errors.New("key not found")

	// ErrInvalidKey is returned when a key is invalid
	ErrInvalidKey = errors.New("invalid key")

	// ErrInvalidValue is returned when a value is invalid
	ErrInvalidValue = errors.New("invalid value")

	// ErrIteratorClosed is returned when an iterator is used after being closed
	ErrIteratorClosed = errors.New("iterator closed")

	// ErrStoreNil is returned when a store is nil
	ErrStoreNil = errors.New("store is nil")
)

// ObjectStore is a typed key-value store interface with caching support
// T is the type of objects stored in the store
type ObjectStore[T any] interface {
	// Get retrieves an object by key
	// Returns ErrNotFound if the key does not exist
	Get(ctx context.Context, key []byte) (T, error)

	// Set stores an object with the given key
	Set(ctx context.Context, key []byte, value T) error

	// Delete removes an object by key
	Delete(ctx context.Context, key []byte) error

	// Has checks if a key exists in the store
	Has(ctx context.Context, key []byte) (bool, error)

	// Iterator returns an iterator over a range of keys
	// Start and end are inclusive and exclusive respectively
	// If start is nil, iteration begins from the first key
	// If end is nil, iteration continues to the last key
	Iterator(ctx context.Context, start, end []byte) (Iterator[T], error)

	// ReverseIterator returns a reverse iterator over a range of keys
	ReverseIterator(ctx context.Context, start, end []byte) (Iterator[T], error)

	// GetBatch retrieves multiple objects by keys
	// Returns a map of key to value for keys that exist
	// Missing keys are not included in the result
	GetBatch(ctx context.Context, keys [][]byte) (map[string]T, error)

	// SetBatch stores multiple objects atomically
	// All operations succeed or all fail
	SetBatch(ctx context.Context, items map[string]T) error

	// DeleteBatch removes multiple objects atomically
	DeleteBatch(ctx context.Context, keys [][]byte) error

	// Flush writes any pending changes to the underlying storage
	Flush(ctx context.Context) error

	// Close releases any resources held by the store
	Close() error
}

// Iterator is an iterator over key-value pairs in a store
type Iterator[T any] interface {
	// Valid returns true if the iterator is positioned at a valid entry
	Valid() bool

	// Next advances the iterator to the next entry
	Next() error

	// Key returns the key at the current position
	// Returns ErrIteratorClosed if the iterator is closed
	Key() ([]byte, error)

	// Value returns the value at the current position
	// Returns ErrIteratorClosed if the iterator is closed
	Value() (T, error)

	// Close releases resources held by the iterator
	Close() error
}

// Serializer handles serialization and deserialization of objects
type Serializer[T any] interface {
	// Marshal converts an object to bytes
	Marshal(obj T) ([]byte, error)

	// Unmarshal converts bytes to an object
	Unmarshal(data []byte) (T, error)
}

// BackingStore is the underlying key-value storage interface
// This will be implemented by IAVL or other storage backends
type BackingStore interface {
	// Get retrieves raw bytes by key
	Get(key []byte) ([]byte, error)

	// Set stores raw bytes with the given key
	Set(key []byte, value []byte) error

	// Delete removes a key
	Delete(key []byte) error

	// Has checks if a key exists
	Has(key []byte) (bool, error)

	// Iterator returns an iterator over a range of keys
	Iterator(start, end []byte) (RawIterator, error)

	// ReverseIterator returns a reverse iterator over a range of keys
	ReverseIterator(start, end []byte) (RawIterator, error)

	// Flush writes pending changes
	Flush() error

	// Close releases resources
	Close() error
}

// RawIterator is an iterator over raw key-value pairs
type RawIterator interface {
	// Valid returns true if positioned at a valid entry
	Valid() bool

	// Next advances to the next entry
	Next()

	// Key returns the current key
	Key() []byte

	// Value returns the current value
	Value() []byte

	// Error returns any error that occurred during iteration
	Error() error

	// Close releases iterator resources
	Close() error
}

// validateKey checks if a key is valid
func validateKey(key []byte) error {
	if key == nil {
		return ErrInvalidKey
	}
	if len(key) == 0 {
		return ErrInvalidKey
	}
	return nil
}

// copyKey creates a defensive copy of a key
func copyKey(key []byte) []byte {
	if key == nil {
		return nil
	}
	result := make([]byte, len(key))
	copy(result, key)
	return result
}

// keyToString converts a key to a string for map indexing
// This creates a defensive copy to prevent external mutation
func keyToString(key []byte) string {
	return string(copyKey(key))
}
