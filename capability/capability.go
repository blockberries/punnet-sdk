package capability

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/blockberries/punnet-sdk/store"
)

var (
	// ErrCapabilityNil is returned when a capability is nil
	ErrCapabilityNil = errors.New("capability is nil")

	// ErrModuleNotFound is returned when a module is not registered
	ErrModuleNotFound = errors.New("module not found")

	// ErrDuplicateModule is returned when a module is already registered
	ErrDuplicateModule = errors.New("module already registered")

	// ErrStoreNil is returned when a store is nil
	ErrStoreNil = errors.New("store is nil")
)

// Capability represents controlled access to state operations
// T is the type of objects that can be accessed through this capability
type Capability[T any] interface {
	// ModuleName returns the name of the module this capability is scoped to
	ModuleName() string

	// Get retrieves an object by key
	Get(ctx context.Context, key []byte) (T, error)

	// Set stores an object with the given key
	Set(ctx context.Context, key []byte, value T) error

	// Delete removes an object by key
	Delete(ctx context.Context, key []byte) error

	// Has checks if a key exists
	Has(ctx context.Context, key []byte) (bool, error)

	// Iterator returns an iterator over a range of keys
	Iterator(ctx context.Context, start, end []byte) (store.Iterator[T], error)
}

// CapabilityManager manages capability grants to modules
// It creates scoped stores with module-specific prefixes
type CapabilityManager struct {
	mu      sync.RWMutex
	modules map[string]bool // tracks registered modules
	backing store.BackingStore
}

// NewCapabilityManager creates a new capability manager
func NewCapabilityManager(backing store.BackingStore) *CapabilityManager {
	if backing == nil {
		panic("backing store cannot be nil")
	}

	return &CapabilityManager{
		modules: make(map[string]bool),
		backing: backing,
	}
}

// RegisterModule registers a module name
// This must be called before granting capabilities to a module
func (cm *CapabilityManager) RegisterModule(moduleName string) error {
	if cm == nil {
		return ErrCapabilityNil
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if moduleName == "" {
		return fmt.Errorf("module name cannot be empty")
	}

	if cm.modules[moduleName] {
		return fmt.Errorf("%w: %s", ErrDuplicateModule, moduleName)
	}

	cm.modules[moduleName] = true
	return nil
}

// IsModuleRegistered checks if a module is registered
func (cm *CapabilityManager) IsModuleRegistered(moduleName string) bool {
	if cm == nil {
		return false
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.modules[moduleName]
}

// createPrefixedStore creates a store with a module-specific prefix
// This provides namespace isolation for modules
func (cm *CapabilityManager) createPrefixedStore(moduleName string) (store.BackingStore, error) {
	if cm == nil {
		return nil, ErrCapabilityNil
	}

	if !cm.IsModuleRegistered(moduleName) {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, moduleName)
	}

	// Create prefix: "module/<moduleName>/"
	prefix := []byte(fmt.Sprintf("module/%s/", moduleName))
	return store.NewPrefixStore(cm.backing, prefix), nil
}

// GrantAccountCapability grants account access capability to a module
func (cm *CapabilityManager) GrantAccountCapability(moduleName string) (AccountCapability, error) {
	if cm == nil {
		return nil, ErrCapabilityNil
	}

	prefixedStore, err := cm.createPrefixedStore(moduleName)
	if err != nil {
		return nil, err
	}

	// Create account store with the prefixed backing store
	accountStore := store.NewAccountStore(prefixedStore)

	return &accountCapability{
		moduleName: moduleName,
		store:      accountStore,
	}, nil
}

// GrantBalanceCapability grants balance access capability to a module
func (cm *CapabilityManager) GrantBalanceCapability(moduleName string) (BalanceCapability, error) {
	if cm == nil {
		return nil, ErrCapabilityNil
	}

	prefixedStore, err := cm.createPrefixedStore(moduleName)
	if err != nil {
		return nil, err
	}

	// Create balance store with the prefixed backing store
	balanceStore := store.NewBalanceStore(prefixedStore)

	return &balanceCapability{
		moduleName: moduleName,
		store:      balanceStore,
	}, nil
}

// GrantValidatorCapability grants validator access capability to a module
func (cm *CapabilityManager) GrantValidatorCapability(moduleName string) (ValidatorCapability, error) {
	if cm == nil {
		return nil, ErrCapabilityNil
	}

	prefixedStore, err := cm.createPrefixedStore(moduleName)
	if err != nil {
		return nil, err
	}

	// Create validator and delegation stores with the prefixed backing store
	validatorStore := store.NewValidatorStore(prefixedStore)
	delegationStore := store.NewDelegationStore(prefixedStore)

	return &validatorCapability{
		moduleName:      moduleName,
		validatorStore:  validatorStore,
		delegationStore: delegationStore,
	}, nil
}

// Flush flushes all pending changes to the underlying storage
func (cm *CapabilityManager) Flush(ctx context.Context) error {
	if cm == nil {
		return ErrCapabilityNil
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.backing == nil {
		return ErrStoreNil
	}

	return cm.backing.Flush()
}

// Close closes the capability manager and releases resources
func (cm *CapabilityManager) Close() error {
	if cm == nil {
		return ErrCapabilityNil
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.backing == nil {
		return ErrStoreNil
	}

	return cm.backing.Close()
}
