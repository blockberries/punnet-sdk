package effects

import (
	"fmt"
)

// DeleteEffect represents an effect that deletes a value from storage
type DeleteEffect[T any] struct {
	// Store is the store name (e.g., "account", "balance")
	Store string

	// Key is the storage key
	StoreKey []byte
}

// Type returns the effect type
func (e DeleteEffect[T]) Type() EffectType {
	return EffectTypeDelete
}

// Validate performs validation
func (e DeleteEffect[T]) Validate() error {
	if e.Store == "" {
		return fmt.Errorf("store name cannot be empty")
	}
	if len(e.StoreKey) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	return nil
}

// Dependencies returns the dependencies
func (e DeleteEffect[T]) Dependencies() []Dependency {
	return []Dependency{
		{
			Type:     DependencyTypeGeneric,
			Key:      e.fullKey(),
			ReadOnly: false,
		},
	}
}

// Key returns the primary key
func (e DeleteEffect[T]) Key() []byte {
	return e.fullKey()
}

// fullKey returns the full key including store prefix
// Creates defensive copy to prevent slice aliasing
func (e DeleteEffect[T]) fullKey() []byte {
	prefix := []byte(e.Store + "/")
	// Create new slice with exact capacity to prevent aliasing
	result := make([]byte, len(prefix)+len(e.StoreKey))
	copy(result, prefix)
	copy(result[len(prefix):], e.StoreKey)
	return result
}
