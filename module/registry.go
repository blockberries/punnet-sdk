package module

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	// ErrCyclicDependency is returned when a cyclic dependency is detected
	ErrCyclicDependency = errors.New("cyclic dependency detected")

	// ErrMissingDependency is returned when a required dependency is not registered
	ErrMissingDependency = errors.New("missing dependency")
)

// Registry manages module registration and initialization order
// It validates dependencies and computes the correct initialization order using topological sort
type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module // name -> module
	order   []string          // initialization order (topologically sorted)
}

// NewRegistry creates a new module registry
func NewRegistry() *Registry {
	return &Registry{
		modules: make(map[string]Module),
		order:   make([]string, 0),
	}
}

// Register registers a module
// Modules must be registered in dependency order or the Build() call will fail
func (r *Registry) Register(module Module) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}

	if module == nil {
		return ErrModuleNil
	}

	// Validate module
	if err := ValidateModule(module); err != nil {
		return fmt.Errorf("invalid module: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := module.Name()

	// Check for duplicate
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateModule, name)
	}

	// Verify all dependencies are already registered
	for _, dep := range module.Dependencies() {
		if _, exists := r.modules[dep]; !exists {
			return fmt.Errorf("%w: module %s requires %s", ErrMissingDependency, name, dep)
		}
	}

	r.modules[name] = module
	return nil
}

// Build finalizes the registry and computes the initialization order
// It performs topological sort and cycle detection on the dependency graph
func (r *Registry) Build() error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.modules) == 0 {
		return fmt.Errorf("no modules registered")
	}

	// Build dependency graph and perform topological sort
	order, err := r.topologicalSort()
	if err != nil {
		return err
	}

	r.order = order
	return nil
}

// topologicalSort performs topological sort using Kahn's algorithm
// Returns the modules in dependency order (dependencies before dependents)
func (r *Registry) topologicalSort() ([]string, error) {
	// Build adjacency list and in-degree map
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize all modules with in-degree 0
	for name := range r.modules {
		inDegree[name] = 0
		adjList[name] = make([]string, 0)
	}

	// Build the graph
	for name, module := range r.modules {
		for _, dep := range module.Dependencies() {
			// dep -> name (name depends on dep)
			adjList[dep] = append(adjList[dep], name)
			inDegree[name]++
		}
	}

	// Find all nodes with in-degree 0 (no dependencies)
	queue := make([]string, 0)
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Sort queue for deterministic ordering
	sort.Strings(queue)

	// Process nodes
	result := make([]string, 0, len(r.modules))

	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]

		result = append(result, current)

		// Process dependents
		dependents := adjList[current]

		// Sort dependents for deterministic ordering
		sort.Strings(dependents)

		for _, dependent := range dependents {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
				// Keep queue sorted
				sort.Strings(queue)
			}
		}
	}

	// Check for cycles
	if len(result) != len(r.modules) {
		// Find nodes not in result (part of cycle)
		missing := make([]string, 0)
		resultSet := make(map[string]bool)
		for _, name := range result {
			resultSet[name] = true
		}
		for name := range r.modules {
			if !resultSet[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing) // deterministic error message
		return nil, fmt.Errorf("%w: modules %v", ErrCyclicDependency, missing)
	}

	return result, nil
}

// Get retrieves a module by name
func (r *Registry) Get(name string) (Module, error) {
	if r == nil {
		return nil, fmt.Errorf("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	module, exists := r.modules[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotFound, name)
	}

	return module, nil
}

// Has checks if a module is registered
func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.modules[name]
	return exists
}

// Modules returns all registered modules in initialization order
// Returns an error if Build() has not been called
func (r *Registry) Modules() ([]Module, error) {
	if r == nil {
		return nil, fmt.Errorf("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.order) == 0 && len(r.modules) > 0 {
		return nil, fmt.Errorf("registry not built: call Build() first")
	}

	// Return modules in dependency order
	result := make([]Module, len(r.order))
	for i, name := range r.order {
		result[i] = r.modules[name]
	}

	return result, nil
}

// ModuleNames returns all registered module names in initialization order
// Returns an error if Build() has not been called
func (r *Registry) ModuleNames() ([]string, error) {
	if r == nil {
		return nil, fmt.Errorf("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.order) == 0 && len(r.modules) > 0 {
		return nil, fmt.Errorf("registry not built: call Build() first")
	}

	// Return defensive copy
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names, nil
}

// Count returns the number of registered modules
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.modules)
}
