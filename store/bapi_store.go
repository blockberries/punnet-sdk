// Package store provides typed storage implementations backed by blockberry's StateStore.
// This file contains the new BAPI-compatible store implementations.
package store

import (
	"context"
	"fmt"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/cramberry/pkg/cramberry"
)

// Codec handles serialization and deserialization of objects.
type Codec[T any] interface {
	Encode(value T) ([]byte, error)
	Decode(data []byte) (T, error)
}

// CramberryCodec uses cramberry for deterministic binary serialization.
type CramberryCodec[T any] struct{}

// Encode serializes a value to bytes using cramberry.
func (c CramberryCodec[T]) Encode(value T) ([]byte, error) {
	return cramberry.Marshal(value)
}

// Decode deserializes bytes to a value using cramberry.
func (c CramberryCodec[T]) Decode(data []byte) (T, error) {
	var result T
	if err := cramberry.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("cramberry unmarshal: %w", err)
	}
	return result, nil
}

// TypedStore provides typed access to state storage with a key prefix.
// It wraps blockberry's StateStore directly without additional caching layers.
type TypedStore[T any] struct {
	store  statestore.StateStore
	prefix string
	codec  Codec[T]
}

// NewTypedStore creates a new typed store with the given prefix.
func NewTypedStore[T any](store statestore.StateStore, prefix string) *TypedStore[T] {
	return &TypedStore[T]{
		store:  store,
		prefix: prefix,
		codec:  CramberryCodec[T]{},
	}
}

// NewTypedStoreWithCodec creates a new typed store with a custom codec.
func NewTypedStoreWithCodec[T any](store statestore.StateStore, prefix string, codec Codec[T]) *TypedStore[T] {
	return &TypedStore[T]{
		store:  store,
		prefix: prefix,
		codec:  codec,
	}
}

// prefixKey creates a prefixed key.
func (s *TypedStore[T]) prefixKey(key string) []byte {
	return []byte(s.prefix + key)
}

// Get retrieves a value by key.
func (s *TypedStore[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	if s.store == nil {
		return zero, ErrStoreNil
	}

	data, err := s.store.Get(s.prefixKey(key))
	if err != nil {
		return zero, fmt.Errorf("get key %s: %w", key, err)
	}
	if data == nil {
		return zero, ErrNotFound
	}

	return s.codec.Decode(data)
}

// Set stores a value with the given key.
func (s *TypedStore[T]) Set(ctx context.Context, key string, value T) error {
	if s.store == nil {
		return ErrStoreNil
	}

	data, err := s.codec.Encode(value)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}

	return s.store.Set(s.prefixKey(key), data)
}

// Delete removes a value by key.
func (s *TypedStore[T]) Delete(ctx context.Context, key string) error {
	if s.store == nil {
		return ErrStoreNil
	}

	return s.store.Delete(s.prefixKey(key))
}

// Has checks if a key exists.
func (s *TypedStore[T]) Has(ctx context.Context, key string) (bool, error) {
	if s.store == nil {
		return false, ErrStoreNil
	}

	return s.store.Has(s.prefixKey(key))
}

// GetWithProof retrieves a value with a Merkle proof (for key-value queries only).
func (s *TypedStore[T]) GetWithProof(ctx context.Context, key string) (T, *statestore.Proof, error) {
	var zero T
	if s.store == nil {
		return zero, nil, ErrStoreNil
	}

	proof, err := s.store.GetProof(s.prefixKey(key))
	if err != nil {
		return zero, nil, fmt.Errorf("get proof: %w", err)
	}

	if !proof.Exists {
		return zero, proof, ErrNotFound
	}

	value, err := s.codec.Decode(proof.Value)
	if err != nil {
		return zero, nil, fmt.Errorf("decode value: %w", err)
	}

	return value, proof, nil
}

// GetAtHeight retrieves a value at a specific block height (for historical queries).
// Note: This temporarily loads the version and restores it after. For production use,
// consider using a separate StateStore instance for historical queries.
func (s *TypedStore[T]) GetAtHeight(ctx context.Context, key string, height int64) (T, error) {
	var zero T
	if s.store == nil {
		return zero, ErrStoreNil
	}

	currentVersion := s.store.Version()

	// Load the requested version
	if err := s.store.LoadVersion(height); err != nil {
		return zero, fmt.Errorf("load version %d: %w", height, err)
	}

	// Get the value at that version
	value, err := s.Get(ctx, key)

	// Always restore the current version, regardless of error
	restoreErr := s.store.LoadVersion(currentVersion)

	if err != nil {
		return zero, err
	}
	if restoreErr != nil {
		return zero, fmt.Errorf("restore version: %w", restoreErr)
	}

	return value, nil
}

// Prefix returns the store's key prefix.
func (s *TypedStore[T]) Prefix() string {
	return s.prefix
}

// StateStore returns the underlying state store.
func (s *TypedStore[T]) StateStore() statestore.StateStore {
	return s.store
}
