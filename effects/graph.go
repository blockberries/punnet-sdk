package effects

import (
	"fmt"
)

// Node represents a node in the dependency graph
type Node struct {
	// Index is the position of the effect in the original list
	Index int

	// Effect is the actual effect
	Effect Effect

	// Dependencies are the indices of nodes this node depends on
	Dependencies []int

	// Dependents are the indices of nodes that depend on this node
	Dependents []int
}

// Graph represents a dependency graph of effects
type Graph struct {
	// Nodes are the graph nodes
	Nodes []*Node

	// keyToNodes maps keys to node indices for conflict detection
	keyToNodes map[string][]int

	// readKeys tracks which keys are read by which nodes
	readKeys map[string][]int

	// writeKeys tracks which keys are written by which nodes
	writeKeys map[string][]int
}

// NewGraph creates a new dependency graph from a list of effects
func NewGraph(effects []Effect) (*Graph, error) {
	if effects == nil {
		return nil, fmt.Errorf("effects list cannot be nil")
	}

	// Validate all effects first
	if err := ValidateEffects(effects); err != nil {
		return nil, fmt.Errorf("invalid effects: %w", err)
	}

	g := &Graph{
		Nodes:      make([]*Node, len(effects)),
		keyToNodes: make(map[string][]int),
		readKeys:   make(map[string][]int),
		writeKeys:  make(map[string][]int),
	}

	// Create nodes
	for i, effect := range effects {
		g.Nodes[i] = &Node{
			Index:        i,
			Effect:       effect,
			Dependencies: make([]int, 0),
			Dependents:   make([]int, 0),
		}
	}

	// Build key maps for dependency tracking
	for i, node := range g.Nodes {
		key := KeyString(node.Effect.Key())
		g.keyToNodes[key] = append(g.keyToNodes[key], i)

		// Track read/write operations
		deps := node.Effect.Dependencies()
		for _, dep := range deps {
			depKey := KeyString(dep.Key)
			if dep.ReadOnly {
				g.readKeys[depKey] = append(g.readKeys[depKey], i)
			} else {
				g.writeKeys[depKey] = append(g.writeKeys[depKey], i)
			}
		}
	}

	// Build dependencies based on read-write conflicts
	if err := g.buildDependencies(); err != nil {
		return nil, fmt.Errorf("failed to build dependencies: %w", err)
	}

	return g, nil
}

// buildDependencies constructs the dependency edges between nodes
func (g *Graph) buildDependencies() error {
	// For each node, find dependencies based on conflicts
	for i := 0; i < len(g.Nodes); i++ {
		node := g.Nodes[i]
		deps := node.Effect.Dependencies()

		for _, dep := range deps {
			depKey := KeyString(dep.Key)

			if dep.ReadOnly {
				// Read depends on previous writes to the same key
				if writers, ok := g.writeKeys[depKey]; ok {
					for _, writerIdx := range writers {
						// Only depend on earlier effects
						if writerIdx < i {
							g.addDependency(i, writerIdx)
						}
					}
				}
			} else {
				// Write depends on previous reads and writes to the same key
				if readers, ok := g.readKeys[depKey]; ok {
					for _, readerIdx := range readers {
						if readerIdx < i {
							g.addDependency(i, readerIdx)
						}
					}
				}
				if writers, ok := g.writeKeys[depKey]; ok {
					for _, writerIdx := range writers {
						if writerIdx < i {
							g.addDependency(i, writerIdx)
						}
					}
				}
			}
		}
	}

	return nil
}

// addDependency adds a dependency edge from dependent to dependency
func (g *Graph) addDependency(dependent, dependency int) {
	// Avoid duplicate dependencies
	for _, dep := range g.Nodes[dependent].Dependencies {
		if dep == dependency {
			return
		}
	}

	g.Nodes[dependent].Dependencies = append(g.Nodes[dependent].Dependencies, dependency)
	g.Nodes[dependency].Dependents = append(g.Nodes[dependency].Dependents, dependent)
}

// DetectConflicts finds all conflicts in the graph
func (g *Graph) DetectConflicts() []*Conflict {
	if g == nil {
		return nil
	}

	conflicts := make([]*Conflict, 0)

	// Check for conflicts between effects on the same key
	for _, indices := range g.keyToNodes {
		if len(indices) < 2 {
			continue
		}

		// Check all pairs of effects on this key
		for i := 0; i < len(indices); i++ {
			for j := i + 1; j < len(indices); j++ {
				idx1, idx2 := indices[i], indices[j]
				effect1 := g.Nodes[idx1].Effect
				effect2 := g.Nodes[idx2].Effect

				if conflict := DetectConflict(effect1, effect2); conflict != nil {
					// Only report if it's a real conflict (not just dependency)
					// We allow read-after-write and write-after-read if they're sequential
					// But we don't allow concurrent writes or reads during writes
					if conflict.Type == ConflictTypeWriteWrite {
						conflicts = append(conflicts, conflict)
					}
				}
			}
		}
	}

	return conflicts
}

// HasCycles detects if the graph has any cycles
func (g *Graph) HasCycles() bool {
	if g == nil {
		return false
	}

	visited := make(map[int]bool)
	recStack := make(map[int]bool)

	for i := range g.Nodes {
		if g.hasCycleUtil(i, visited, recStack) {
			return true
		}
	}

	return false
}

// hasCycleUtil is a helper for cycle detection using DFS
func (g *Graph) hasCycleUtil(nodeIdx int, visited, recStack map[int]bool) bool {
	if recStack[nodeIdx] {
		return true
	}
	if visited[nodeIdx] {
		return false
	}

	visited[nodeIdx] = true
	recStack[nodeIdx] = true

	for _, dep := range g.Nodes[nodeIdx].Dependencies {
		if g.hasCycleUtil(dep, visited, recStack) {
			return true
		}
	}

	recStack[nodeIdx] = false
	return false
}

// TopologicalSort returns the nodes in topological order
// Returns error if graph has cycles
func (g *Graph) TopologicalSort() ([]*Node, error) {
	if g == nil {
		return nil, fmt.Errorf("graph is nil")
	}

	if g.HasCycles() {
		return nil, fmt.Errorf("graph has cycles")
	}

	visited := make(map[int]bool)
	stack := make([]*Node, 0, len(g.Nodes))

	for i := range g.Nodes {
		if !visited[i] {
			g.topologicalSortUtil(i, visited, &stack)
		}
	}

	// Stack is already in correct order (dependencies before dependents)
	return stack, nil
}

// topologicalSortUtil is a helper for topological sort using DFS
func (g *Graph) topologicalSortUtil(nodeIdx int, visited map[int]bool, stack *[]*Node) {
	visited[nodeIdx] = true

	// Visit all dependencies first
	for _, dep := range g.Nodes[nodeIdx].Dependencies {
		if !visited[dep] {
			g.topologicalSortUtil(dep, visited, stack)
		}
	}

	// Add to stack after visiting dependencies
	*stack = append(*stack, g.Nodes[nodeIdx])
}

// IndependentNodes returns all nodes that have no dependencies
func (g *Graph) IndependentNodes() []*Node {
	if g == nil {
		return nil
	}

	independent := make([]*Node, 0)
	for _, node := range g.Nodes {
		if len(node.Dependencies) == 0 {
			independent = append(independent, node)
		}
	}
	return independent
}

// Size returns the number of nodes in the graph
func (g *Graph) Size() int {
	if g == nil {
		return 0
	}
	return len(g.Nodes)
}

// GetNode returns the node at the given index
func (g *Graph) GetNode(index int) (*Node, error) {
	if g == nil {
		return nil, fmt.Errorf("graph is nil")
	}
	if index < 0 || index >= len(g.Nodes) {
		return nil, fmt.Errorf("index out of bounds: %d", index)
	}
	return g.Nodes[index], nil
}
