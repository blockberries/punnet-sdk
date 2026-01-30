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
func (e DeleteEffect[T]) fullKey() []byte {
	return append([]byte(e.Store+"/"), e.StoreKey...)
}
