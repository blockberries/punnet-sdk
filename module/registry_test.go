package module

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	require.Equal(t, 0, r.Count())
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	// Create simple module
	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)
	require.Equal(t, 1, r.Count())
}

func TestRegistry_Register_Nil(t *testing.T) {
	r := NewRegistry()
	err := r.Register(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestRegistry_Register_Invalid(t *testing.T) {
	r := NewRegistry()

	// Create invalid module (empty name)
	mod := &baseModule{name: ""}

	err := r.Register(mod)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()

	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)

	// Try to register again
	err = r.Register(mod)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Register_MissingDependency(t *testing.T) {
	r := NewRegistry()

	// Create module with dependency
	mod, err := NewModuleBuilder("test").
		WithDependency("missing").
		Build()
	require.NoError(t, err)

	// Should fail because dependency is not registered
	err = r.Register(mod)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing dependency")
}

func TestRegistry_Register_WithDependency(t *testing.T) {
	r := NewRegistry()

	// Register dependency first
	dep, err := NewModuleBuilder("dep").Build()
	require.NoError(t, err)
	err = r.Register(dep)
	require.NoError(t, err)

	// Now register module that depends on it
	mod, err := NewModuleBuilder("test").
		WithDependency("dep").
		Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)
	require.Equal(t, 2, r.Count())
}

func TestRegistry_Build(t *testing.T) {
	r := NewRegistry()

	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)

	err = r.Build()
	require.NoError(t, err)
}

func TestRegistry_Build_Empty(t *testing.T) {
	r := NewRegistry()
	err := r.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no modules")
}

func TestRegistry_Build_Nil(t *testing.T) {
	var r *Registry
	err := r.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestRegistry_TopologicalSort_Simple(t *testing.T) {
	r := NewRegistry()

	// Register modules in dependency order
	modA, _ := NewModuleBuilder("a").Build()
	modB, _ := NewModuleBuilder("b").WithDependency("a").Build()
	modC, _ := NewModuleBuilder("c").WithDependency("b").Build()

	require.NoError(t, r.Register(modA))
	require.NoError(t, r.Register(modB))
	require.NoError(t, r.Register(modC))

	err := r.Build()
	require.NoError(t, err)

	names, err := r.ModuleNames()
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, names)
}

func TestRegistry_TopologicalSort_Diamond(t *testing.T) {
	r := NewRegistry()

	//     a
	//    / \
	//   b   c
	//    \ /
	//     d

	modA, _ := NewModuleBuilder("a").Build()
	modB, _ := NewModuleBuilder("b").WithDependency("a").Build()
	modC, _ := NewModuleBuilder("c").WithDependency("a").Build()
	modD, _ := NewModuleBuilder("d").WithDependencies("b", "c").Build()

	require.NoError(t, r.Register(modA))
	require.NoError(t, r.Register(modB))
	require.NoError(t, r.Register(modC))
	require.NoError(t, r.Register(modD))

	err := r.Build()
	require.NoError(t, err)

	names, err := r.ModuleNames()
	require.NoError(t, err)

	// Verify that 'a' comes before 'b' and 'c'
	// and 'b' and 'c' come before 'd'
	require.Len(t, names, 4)

	aIdx := indexOf(names, "a")
	bIdx := indexOf(names, "b")
	cIdx := indexOf(names, "c")
	dIdx := indexOf(names, "d")

	require.True(t, aIdx < bIdx, "a should come before b")
	require.True(t, aIdx < cIdx, "a should come before c")
	require.True(t, bIdx < dIdx, "b should come before d")
	require.True(t, cIdx < dIdx, "c should come before d")
}

func TestRegistry_TopologicalSort_Cycle(t *testing.T) {
	// We can't create a cycle with the Register method because it checks dependencies
	// However, we can test the cycle detection directly
	r := NewRegistry()

	// Manually create modules with cyclic dependencies
	// This bypasses the Register checks to test cycle detection
	r.modules = map[string]Module{
		"a": &baseModule{name: "a", dependencies: []string{"b"}},
		"b": &baseModule{name: "b", dependencies: []string{"c"}},
		"c": &baseModule{name: "c", dependencies: []string{"a"}},
	}

	err := r.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "cyclic dependency")
}

func TestRegistry_TopologicalSort_SelfCycle(t *testing.T) {
	r := NewRegistry()

	// Manually create module with self-dependency
	r.modules = map[string]Module{
		"a": &baseModule{name: "a", dependencies: []string{"a"}},
	}

	err := r.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "cyclic dependency")
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)

	retrieved, err := r.Get("test")
	require.NoError(t, err)
	require.Equal(t, mod.Name(), retrieved.Name())
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("missing")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestRegistry_Get_Nil(t *testing.T) {
	var r *Registry
	_, err := r.Get("test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestRegistry_Has(t *testing.T) {
	r := NewRegistry()

	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)

	require.True(t, r.Has("test"))
	require.False(t, r.Has("missing"))
}

func TestRegistry_Has_Nil(t *testing.T) {
	var r *Registry
	require.False(t, r.Has("test"))
}

func TestRegistry_Modules(t *testing.T) {
	r := NewRegistry()

	modA, _ := NewModuleBuilder("a").Build()
	modB, _ := NewModuleBuilder("b").WithDependency("a").Build()

	require.NoError(t, r.Register(modA))
	require.NoError(t, r.Register(modB))

	err := r.Build()
	require.NoError(t, err)

	modules, err := r.Modules()
	require.NoError(t, err)
	require.Len(t, modules, 2)
	require.Equal(t, "a", modules[0].Name())
	require.Equal(t, "b", modules[1].Name())
}

func TestRegistry_Modules_NotBuilt(t *testing.T) {
	r := NewRegistry()

	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)

	// Try to get modules before Build()
	_, err = r.Modules()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not built")
}

func TestRegistry_Modules_Nil(t *testing.T) {
	var r *Registry
	_, err := r.Modules()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestRegistry_ModuleNames(t *testing.T) {
	r := NewRegistry()

	modA, _ := NewModuleBuilder("a").Build()
	modB, _ := NewModuleBuilder("b").WithDependency("a").Build()

	require.NoError(t, r.Register(modA))
	require.NoError(t, r.Register(modB))

	err := r.Build()
	require.NoError(t, err)

	names, err := r.ModuleNames()
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, names)

	// Verify defensive copy
	names[0] = "modified"
	names2, _ := r.ModuleNames()
	require.Equal(t, "a", names2[0])
}

func TestRegistry_ModuleNames_NotBuilt(t *testing.T) {
	r := NewRegistry()

	mod, err := NewModuleBuilder("test").Build()
	require.NoError(t, err)

	err = r.Register(mod)
	require.NoError(t, err)

	// Try to get names before Build()
	_, err = r.ModuleNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not built")
}

func TestRegistry_ModuleNames_Nil(t *testing.T) {
	var r *Registry
	_, err := r.ModuleNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry()
	require.Equal(t, 0, r.Count())

	mod1, _ := NewModuleBuilder("a").Build()
	require.NoError(t, r.Register(mod1))
	require.Equal(t, 1, r.Count())

	mod2, _ := NewModuleBuilder("b").Build()
	require.NoError(t, r.Register(mod2))
	require.Equal(t, 2, r.Count())
}

func TestRegistry_Count_Nil(t *testing.T) {
	var r *Registry
	require.Equal(t, 0, r.Count())
}

func TestRegistry_ComplexDependencies(t *testing.T) {
	r := NewRegistry()

	// Build a complex dependency graph
	//      a   b
	//     / \ / \
	//    c   d   e
	//     \ / \ /
	//      f   g

	modA, _ := NewModuleBuilder("a").Build()
	modB, _ := NewModuleBuilder("b").Build()
	modC, _ := NewModuleBuilder("c").WithDependency("a").Build()
	modD, _ := NewModuleBuilder("d").WithDependencies("a", "b").Build()
	modE, _ := NewModuleBuilder("e").WithDependency("b").Build()
	modF, _ := NewModuleBuilder("f").WithDependencies("c", "d").Build()
	modG, _ := NewModuleBuilder("g").WithDependencies("d", "e").Build()

	require.NoError(t, r.Register(modA))
	require.NoError(t, r.Register(modB))
	require.NoError(t, r.Register(modC))
	require.NoError(t, r.Register(modD))
	require.NoError(t, r.Register(modE))
	require.NoError(t, r.Register(modF))
	require.NoError(t, r.Register(modG))

	err := r.Build()
	require.NoError(t, err)

	names, err := r.ModuleNames()
	require.NoError(t, err)
	require.Len(t, names, 7)

	// Verify dependency ordering constraints
	aIdx := indexOf(names, "a")
	bIdx := indexOf(names, "b")
	cIdx := indexOf(names, "c")
	dIdx := indexOf(names, "d")
	eIdx := indexOf(names, "e")
	fIdx := indexOf(names, "f")
	gIdx := indexOf(names, "g")

	// a comes before c, d
	require.True(t, aIdx < cIdx)
	require.True(t, aIdx < dIdx)

	// b comes before d, e
	require.True(t, bIdx < dIdx)
	require.True(t, bIdx < eIdx)

	// c, d come before f
	require.True(t, cIdx < fIdx)
	require.True(t, dIdx < fIdx)

	// d, e come before g
	require.True(t, dIdx < gIdx)
	require.True(t, eIdx < gIdx)
}

func TestRegistry_Deterministic(t *testing.T) {
	// Test that the topological sort is deterministic
	// by building the same graph multiple times

	buildRegistry := func() []string {
		r := NewRegistry()

		modA, _ := NewModuleBuilder("a").Build()
		modB, _ := NewModuleBuilder("b").Build()
		modC, _ := NewModuleBuilder("c").WithDependencies("a", "b").Build()

		r.Register(modA)
		r.Register(modB)
		r.Register(modC)

		r.Build()
		names, _ := r.ModuleNames()
		return names
	}

	// Build registry 10 times and verify same order
	expected := buildRegistry()
	for i := 0; i < 10; i++ {
		result := buildRegistry()
		require.Equal(t, expected, result, "iteration %d should have same order", i)
	}
}

// Helper function to find index of string in slice
func indexOf(slice []string, value string) int {
	for i, v := range slice {
		if v == value {
			return i
		}
	}
	return -1
}
