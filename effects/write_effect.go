package effects

import (
	"fmt"
)

// WriteEffect represents an effect that writes a value to storage
type WriteEffect[T any] struct {
	// Store is the store name (e.g., "account", "balance")
	Store string

	// Key is the storage key
	StoreKey []byte

	// Value is the value to write
	Value T
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

// fullKey returns the full key including store prefix
func (e WriteEffect[T]) fullKey() []byte {
	return append([]byte(e.Store+"/"), e.StoreKey...)
}
