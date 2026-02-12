package effects

import (
	"fmt"

	"github.com/blockberries/cramberry/pkg/cramberry"
)

// WriteEffect represents an effect that writes a value to storage.
// The value is serialized using cramberry when SerializedValue() is called.
type WriteEffect[T any] struct {
	// Store is the store name (e.g., "account", "balance")
	Store string

	// Key is the storage key
	StoreKey []byte

	// Value is the value to write
	Value T

	// serializedCache caches the serialized value to avoid re-serialization
	serializedCache []byte
}

// Type returns the effect type
func (e WriteEffect[T]) Type() EffectType {
	return EffectTypeWrite
}

// Validate performs validation
func (e WriteEffect[T]) Validate() error {
	if e.Store == "" {
		return fmt.Errorf("store name cannot be empty")
	}
	if len(e.StoreKey) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	// Validate that value can be serialized
	if _, err := e.SerializedValue(); err != nil {
		return fmt.Errorf("value serialization failed: %w", err)
	}
	return nil
}

// Dependencies returns the dependencies
func (e WriteEffect[T]) Dependencies() []Dependency {
	return []Dependency{
		{
			Type:     DependencyTypeGeneric,
			Key:      e.fullKey(),
			ReadOnly: false,
		},
	}
}

// Key returns the primary key
func (e WriteEffect[T]) Key() []byte {
	return e.fullKey()
}

// SerializedValue returns the cramberry-serialized value.
// The serialization is cached for efficiency.
func (e *WriteEffect[T]) SerializedValue() ([]byte, error) {
	if e.serializedCache != nil {
		return e.serializedCache, nil
	}

	data, err := cramberry.Marshal(e.Value)
	if err != nil {
		return nil, fmt.Errorf("cramberry marshal: %w", err)
	}

	e.serializedCache = data
	return data, nil
}

// MustSerializedValue returns the serialized value or panics on error.
// Use only when the value is known to be serializable.
func (e *WriteEffect[T]) MustSerializedValue() []byte {
	data, err := e.SerializedValue()
	if err != nil {
		panic(fmt.Sprintf("failed to serialize write effect value: %v", err))
	}
	return data
}

// fullKey returns the full key including store prefix
// Creates defensive copy to prevent slice aliasing
func (e WriteEffect[T]) fullKey() []byte {
	prefix := []byte(e.Store + "/")
	// Create new slice with exact capacity to prevent aliasing
	result := make([]byte, len(prefix)+len(e.StoreKey))
	copy(result, prefix)
	copy(result[len(prefix):], e.StoreKey)
	return result
}

// WriteEffectSerializer is an interface for effects that can provide serialized values.
// This allows the executor to handle serialization uniformly.
type WriteEffectSerializer interface {
	Effect
	SerializedValue() ([]byte, error)
}
