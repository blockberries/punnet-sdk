// Package di provides a dependency injection container for punnet-sdk.
// It uses interface markers for automatic dependency injection into modules.
package di

import (
	"fmt"
	"reflect"
	"sync"
)

// Container is the dependency injection container.
// It manages service instances and factories for lazy instantiation.
type Container struct {
	mu        sync.RWMutex
	services  map[reflect.Type]any
	factories map[reflect.Type]Factory
}

// Factory is a function that creates a service instance.
type Factory func(*Container) (any, error)

// New creates a new dependency injection container.
func New() *Container {
	return &Container{
		services:  make(map[reflect.Type]any),
		factories: make(map[reflect.Type]Factory),
	}
}

// Register registers a service instance directly.
// The service will be available for injection immediately.
func (c *Container) Register(service any) error {
	if service == nil {
		return fmt.Errorf("cannot register nil service")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	t := reflect.TypeOf(service)
	c.services[t] = service
	return nil
}

// RegisterInterface registers a service under an interface type.
// This allows injecting by interface rather than concrete type.
func (c *Container) RegisterInterface(ifacePtr any, service any) error {
	if ifacePtr == nil {
		return fmt.Errorf("interface pointer cannot be nil")
	}
	if service == nil {
		return fmt.Errorf("service cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Get the interface type from the pointer
	ptrType := reflect.TypeOf(ifacePtr)
	if ptrType.Kind() != reflect.Ptr {
		return fmt.Errorf("ifacePtr must be a pointer to interface")
	}
	ifaceType := ptrType.Elem()

	// Verify service implements the interface
	serviceType := reflect.TypeOf(service)
	if !serviceType.Implements(ifaceType) && !reflect.PointerTo(serviceType).Implements(ifaceType) {
		return fmt.Errorf("service of type %v does not implement interface %v", serviceType, ifaceType)
	}

	c.services[ifaceType] = service
	return nil
}

// RegisterFactory registers a factory function for lazy instantiation.
// The factory will be called once when the service is first resolved.
func (c *Container) RegisterFactory(serviceType reflect.Type, factory Factory) error {
	if serviceType == nil {
		return fmt.Errorf("service type cannot be nil")
	}
	if factory == nil {
		return fmt.Errorf("factory cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.factories[serviceType] = factory
	return nil
}

// RegisterFactoryFor registers a factory using a type parameter.
// This is a convenience method that infers the type from T.
func RegisterFactoryFor[T any](c *Container, factory func(*Container) (T, error)) error {
	var zero T
	t := reflect.TypeOf(&zero).Elem()
	return c.RegisterFactory(t, func(c *Container) (any, error) {
		return factory(c)
	})
}

// Resolve resolves a dependency by type.
// The target must be a pointer to the type to resolve.
func (c *Container) Resolve(target any) error {
	if target == nil {
		return fmt.Errorf("target cannot be nil")
	}

	targetVal := reflect.ValueOf(target)
	if targetVal.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}

	targetType := targetVal.Elem().Type()

	c.mu.RLock()
	service, exists := c.services[targetType]
	c.mu.RUnlock()

	if exists {
		targetVal.Elem().Set(reflect.ValueOf(service))
		return nil
	}

	// Check for factory
	c.mu.RLock()
	factory, hasFactory := c.factories[targetType]
	c.mu.RUnlock()

	if hasFactory {
		// Create the service using the factory
		service, err := factory(c)
		if err != nil {
			return fmt.Errorf("factory for %v failed: %w", targetType, err)
		}

		// Cache the created service
		c.mu.Lock()
		c.services[targetType] = service
		c.mu.Unlock()

		targetVal.Elem().Set(reflect.ValueOf(service))
		return nil
	}

	return fmt.Errorf("no service or factory registered for type %v", targetType)
}

// MustResolve resolves a dependency or panics.
func (c *Container) MustResolve(target any) {
	if err := c.Resolve(target); err != nil {
		panic(err)
	}
}

// ResolveNew creates a new instance using the factory without caching.
// Returns an error if no factory is registered for the type.
func (c *Container) ResolveNew(target any) error {
	if target == nil {
		return fmt.Errorf("target cannot be nil")
	}

	targetVal := reflect.ValueOf(target)
	if targetVal.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}

	targetType := targetVal.Elem().Type()

	c.mu.RLock()
	factory, hasFactory := c.factories[targetType]
	c.mu.RUnlock()

	if !hasFactory {
		return fmt.Errorf("no factory registered for type %v", targetType)
	}

	service, err := factory(c)
	if err != nil {
		return fmt.Errorf("factory for %v failed: %w", targetType, err)
	}

	targetVal.Elem().Set(reflect.ValueOf(service))
	return nil
}

// Has checks if a service or factory is registered for the given type.
func (c *Container) Has(serviceType reflect.Type) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, hasService := c.services[serviceType]
	_, hasFactory := c.factories[serviceType]
	return hasService || hasFactory
}

// HasFor checks if a service or factory is registered for type T.
func HasFor[T any](c *Container) bool {
	var zero T
	t := reflect.TypeOf(&zero).Elem()
	return c.Has(t)
}

// Get retrieves a service of type T from the container.
func Get[T any](c *Container) (T, error) {
	var result T
	if err := c.Resolve(&result); err != nil {
		return result, err
	}
	return result, nil
}

// MustGet retrieves a service of type T or panics.
func MustGet[T any](c *Container) T {
	result, err := Get[T](c)
	if err != nil {
		panic(err)
	}
	return result
}

// InjectDependencies injects dependencies into a target that implements
// dependency marker interfaces.
func (c *Container) InjectDependencies(target any) error {
	if target == nil {
		return nil
	}

	// Check each marker interface and inject if implemented
	if m, ok := target.(NeedsStateStore); ok {
		var store StateStoreProvider
		if err := c.Resolve(&store); err != nil {
			return fmt.Errorf("resolve StateStore: %w", err)
		}
		m.SetStateStore(store.StateStore())
	}

	if m, ok := target.(NeedsAccountStore); ok {
		var store AccountStoreProvider
		if err := c.Resolve(&store); err != nil {
			return fmt.Errorf("resolve AccountStore: %w", err)
		}
		m.SetAccountStore(store.AccountStore())
	}

	if m, ok := target.(NeedsBalanceStore); ok {
		var store BalanceStoreProvider
		if err := c.Resolve(&store); err != nil {
			return fmt.Errorf("resolve BalanceStore: %w", err)
		}
		m.SetBalanceStore(store.BalanceStore())
	}

	if m, ok := target.(NeedsValidatorStore); ok {
		var store ValidatorStoreProvider
		if err := c.Resolve(&store); err != nil {
			return fmt.Errorf("resolve ValidatorStore: %w", err)
		}
		m.SetValidatorStore(store.ValidatorStore())
	}

	if m, ok := target.(NeedsParamsStore); ok {
		var store ParamsStoreProvider
		if err := c.Resolve(&store); err != nil {
			return fmt.Errorf("resolve ParamsStore: %w", err)
		}
		m.SetParamsStore(store.ParamsStore())
	}

	return nil
}

// Clear removes all registered services and factories.
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services = make(map[reflect.Type]any)
	c.factories = make(map[reflect.Type]Factory)
}
