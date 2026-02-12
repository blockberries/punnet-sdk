package di

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test types
type TestService struct {
	Name string
}

type TestInterface interface {
	GetName() string
}

func (t *TestService) GetName() string {
	return t.Name
}

func TestContainer_Register(t *testing.T) {
	c := New()

	svc := &TestService{Name: "test"}
	err := c.Register(svc)
	require.NoError(t, err)

	var resolved *TestService
	err = c.Resolve(&resolved)
	require.NoError(t, err)
	assert.Equal(t, "test", resolved.Name)
}

func TestContainer_RegisterNil(t *testing.T) {
	c := New()

	err := c.Register(nil)
	assert.Error(t, err)
}

func TestContainer_RegisterInterface(t *testing.T) {
	c := New()

	svc := &TestService{Name: "test"}
	err := c.RegisterInterface((*TestInterface)(nil), svc)
	require.NoError(t, err)

	var resolved TestInterface
	err = c.Resolve(&resolved)
	require.NoError(t, err)
	assert.Equal(t, "test", resolved.GetName())
}

func TestContainer_RegisterFactory(t *testing.T) {
	c := New()

	callCount := 0
	err := RegisterFactoryFor(c, func(c *Container) (*TestService, error) {
		callCount++
		return &TestService{Name: "factory"}, nil
	})
	require.NoError(t, err)

	// First resolve - factory should be called
	var svc1 *TestService
	err = c.Resolve(&svc1)
	require.NoError(t, err)
	assert.Equal(t, "factory", svc1.Name)
	assert.Equal(t, 1, callCount)

	// Second resolve - should return cached instance
	var svc2 *TestService
	err = c.Resolve(&svc2)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // Factory should not be called again
	assert.Same(t, svc1, svc2)    // Should be the same instance
}

func TestContainer_ResolveNew(t *testing.T) {
	c := New()

	callCount := 0
	err := RegisterFactoryFor(c, func(c *Container) (*TestService, error) {
		callCount++
		return &TestService{Name: "factory"}, nil
	})
	require.NoError(t, err)

	// Each ResolveNew should call the factory
	var svc1 *TestService
	err = c.ResolveNew(&svc1)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	var svc2 *TestService
	err = c.ResolveNew(&svc2)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)

	// Should be different instances
	assert.NotSame(t, svc1, svc2)
}

func TestContainer_ResolveNotFound(t *testing.T) {
	c := New()

	var svc *TestService
	err := c.Resolve(&svc)
	assert.Error(t, err)
}

func TestContainer_MustResolve_Panics(t *testing.T) {
	c := New()

	assert.Panics(t, func() {
		var svc *TestService
		c.MustResolve(&svc)
	})
}

func TestContainer_Has(t *testing.T) {
	c := New()

	assert.False(t, HasFor[*TestService](c))

	c.Register(&TestService{Name: "test"})
	assert.True(t, HasFor[*TestService](c))
}

func TestContainer_Get(t *testing.T) {
	c := New()

	c.Register(&TestService{Name: "test"})

	svc, err := Get[*TestService](c)
	require.NoError(t, err)
	assert.Equal(t, "test", svc.Name)
}

func TestContainer_MustGet(t *testing.T) {
	c := New()

	c.Register(&TestService{Name: "test"})

	svc := MustGet[*TestService](c)
	assert.Equal(t, "test", svc.Name)
}

func TestContainer_MustGet_Panics(t *testing.T) {
	c := New()

	assert.Panics(t, func() {
		MustGet[*TestService](c)
	})
}

func TestContainer_Clear(t *testing.T) {
	c := New()

	c.Register(&TestService{Name: "test"})
	assert.True(t, HasFor[*TestService](c))

	c.Clear()
	assert.False(t, HasFor[*TestService](c))
}

// Test dependency injection via markers
type MockAccountStore struct{}

type ModuleWithDeps struct {
	accountStore any
}

func (m *ModuleWithDeps) SetAccountStore(store any) {
	m.accountStore = store
}

func TestContainer_InjectDependencies(t *testing.T) {
	c := New()

	// Create a mock provider that satisfies AccountStoreProvider
	mockStore := &MockAccountStore{}

	// Register a provider
	provider := &mockAccountProvider{store: mockStore}
	c.Register(provider)
	c.RegisterInterface((*AccountStoreProvider)(nil), provider)

	// Create module
	module := &ModuleWithDeps{}

	// Inject dependencies
	err := c.InjectDependencies(module)
	require.NoError(t, err)
	assert.Equal(t, mockStore, module.accountStore)
}

type mockAccountProvider struct {
	store any
}

func (m *mockAccountProvider) AccountStore() any {
	return m.store
}
