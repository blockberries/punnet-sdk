package effects

import (
	"fmt"
)

// ReadEffect represents an effect that reads a value from storage
type ReadEffect[T any] struct {
	// Store is the store name
	Store string

	// Key is the storage key
	StoreKey []byte

	// Dest is where to store the result
	Dest *T
}

// Type returns the effect type
func (e ReadEffect[T]) Type() EffectType {
	return EffectTypeRead
}

// Validate performs validation
func (e ReadEffect[T]) Validate() error {
	if e.Store == "" {
		return fmt.Errorf("store name cannot be empty")
	}
	if len(e.StoreKey) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if e.Dest == nil {
		return fmt.Errorf("destination cannot be nil")
	}
	return nil
}

// Dependencies returns the dependencies
func (e ReadEffect[T]) Dependencies() []Dependency {
	return []Dependency{
		{
			Type:     DependencyTypeGeneric,
			Key:      e.fullKey(),
			ReadOnly: true,
		},
	}
}

// Key returns the primary key
func (e ReadEffect[T]) Key() []byte {
	return e.fullKey()
}

// fullKey returns the full key including store prefix
// Creates defensive copy to prevent slice aliasing
func (e ReadEffect[T]) fullKey() []byte {
	prefix := []byte(e.Store + "/")
	// Create new slice with exact capacity to prevent aliasing
	result := make([]byte, len(prefix)+len(e.StoreKey))
	copy(result, prefix)
	copy(result[len(prefix):], e.StoreKey)
	return result
}
