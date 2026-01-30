package effects

import (
	"testing"
)

func TestNewGraph_Nil(t *testing.T) {
	graph, err := NewGraph(nil)
	if err == nil {
		t.Error("NewGraph(nil) should return error")
	}
	if graph != nil {
		t.Error("NewGraph(nil) should return nil graph")
	}
}

func TestNewGraph_Empty(t *testing.T) {
	effects := []Effect{}
	graph, err := NewGraph(effects)
	if err != nil {
		t.Errorf("NewGraph(empty) failed: %v", err)
	}
	if graph == nil {
		t.Fatal("NewGraph(empty) returned nil graph")
	}
	if graph.Size() != 0 {
		t.Errorf("Empty graph size = %d, want 0", graph.Size())
	}
}

func TestNewGraph_InvalidEffect(t *testing.T) {
	effects := []Effect{
		WriteEffect[string]{
			Store:    "", // Invalid: empty store
			StoreKey: []byte("key"),
			Value:    "value",
		},
	}

	graph, err := NewGraph(effects)
	if err == nil {
		t.Error("NewGraph should fail with invalid effect")
	}
	if graph != nil {
		t.Error("NewGraph should return nil graph for invalid effects")
	}
}

func TestNewGraph_SingleEffect(t *testing.T) {
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}
	if graph == nil {
		t.Fatal("NewGraph returned nil graph")
	}
	if graph.Size() != 1 {
		t.Errorf("Graph size = %d, want 1", graph.Size())
	}

	node, err := graph.GetNode(0)
	if err != nil {
		t.Fatalf("GetNode(0) failed: %v", err)
	}
	if node.Index != 0 {
		t.Errorf("Node index = %d, want 0", node.Index)
	}
	if len(node.Dependencies) != 0 {
		t.Errorf("Node should have no dependencies, got %d", len(node.Dependencies))
	}
}

func TestGraph_ReadWriteDependency(t *testing.T) {
	// Write then read - read should depend on write
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value",
		},
		ReadEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Dest:     new(string),
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// Read node (index 1) should depend on write node (index 0)
	readNode := graph.Nodes[1]
	if len(readNode.Dependencies) != 1 {
		t.Errorf("Read node dependencies = %d, want 1", len(readNode.Dependencies))
	}
	if len(readNode.Dependencies) > 0 && readNode.Dependencies[0] != 0 {
		t.Errorf("Read node should depend on write node (index 0), got %d", readNode.Dependencies[0])
	}

	// Write node should have read as dependent
	writeNode := graph.Nodes[0]
	if len(writeNode.Dependents) != 1 {
		t.Errorf("Write node dependents = %d, want 1", len(writeNode.Dependents))
	}
}

func TestGraph_WriteWriteDependency(t *testing.T) {
	// Two writes to same key - second should depend on first
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value2",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// Second write should depend on first
	write2Node := graph.Nodes[1]
	if len(write2Node.Dependencies) != 1 {
		t.Errorf("Second write dependencies = %d, want 1", len(write2Node.Dependencies))
	}
	if len(write2Node.Dependencies) > 0 && write2Node.Dependencies[0] != 0 {
		t.Errorf("Second write should depend on first write (index 0), got %d", write2Node.Dependencies[0])
	}
}

func TestGraph_IndependentEffects(t *testing.T) {
	// Effects on different keys should be independent
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "value1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "value2",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key3"),
			Value:    "value3",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// All nodes should be independent
	independent := graph.IndependentNodes()
	if len(independent) != 3 {
		t.Errorf("IndependentNodes = %d, want 3", len(independent))
	}

	// No node should have dependencies
	for i, node := range graph.Nodes {
		if len(node.Dependencies) != 0 {
			t.Errorf("Node %d should have no dependencies, got %d", i, len(node.Dependencies))
		}
	}
}

func TestGraph_DetectConflicts_WriteWrite(t *testing.T) {
	// Two writes to same key should conflict
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value2",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	conflicts := graph.DetectConflicts()
	if len(conflicts) != 1 {
		t.Errorf("DetectConflicts found %d conflicts, want 1", len(conflicts))
	}
	if len(conflicts) > 0 && conflicts[0].Type != ConflictTypeWriteWrite {
		t.Errorf("Conflict type = %v, want %v", conflicts[0].Type, ConflictTypeWriteWrite)
	}
}

func TestGraph_DetectConflicts_NoConflict(t *testing.T) {
	// Different keys should not conflict
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "value1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "value2",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	conflicts := graph.DetectConflicts()
	if len(conflicts) != 0 {
		t.Errorf("DetectConflicts found %d conflicts, want 0", len(conflicts))
	}
}

func TestGraph_HasCycles_NoCycle(t *testing.T) {
	// Linear dependency chain: no cycles
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value1",
		},
		ReadEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Dest:     new(string),
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value2",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	if graph.HasCycles() {
		t.Error("Graph should not have cycles")
	}
}

func TestGraph_TopologicalSort(t *testing.T) {
	// Create a graph with dependencies
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "value1",
		},
		ReadEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Dest:     new(string),
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "value2",
		},
		ReadEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Dest:     new(string),
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	sorted, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	if len(sorted) != 4 {
		t.Errorf("Sorted length = %d, want 4", len(sorted))
	}

	// Verify ordering: write1 before read1, write2 before read2
	indices := make(map[int]int)
	for i, node := range sorted {
		indices[node.Index] = i
	}

	if indices[0] >= indices[1] {
		t.Error("Write1 (index 0) should come before Read1 (index 1) in sorted order")
	}
	if indices[2] >= indices[3] {
		t.Error("Write2 (index 2) should come before Read2 (index 3) in sorted order")
	}
}

func TestGraph_TopologicalSort_NilGraph(t *testing.T) {
	var graph *Graph
	_, err := graph.TopologicalSort()
	if err == nil {
		t.Error("TopologicalSort on nil graph should return error")
	}
}

func TestGraph_IndependentNodes_Multiple(t *testing.T) {
	// Create graph with some independent and some dependent nodes
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "value1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "value2",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "value3",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key3"),
			Value:    "value4",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	independent := graph.IndependentNodes()
	// Nodes 0, 2, and 3 should be independent (different keys or first on key)
	if len(independent) != 3 {
		t.Errorf("IndependentNodes = %d, want 3", len(independent))
	}

	// Node 1 should have a dependency on node 0
	node1 := graph.Nodes[1]
	if len(node1.Dependencies) == 0 {
		t.Error("Node 1 should have dependencies")
	}
}

func TestGraph_GetNode_OutOfBounds(t *testing.T) {
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	tests := []struct {
		name  string
		index int
	}{
		{"negative index", -1},
		{"index too large", 1},
		{"index far too large", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := graph.GetNode(tt.index)
			if err == nil {
				t.Errorf("GetNode(%d) should return error", tt.index)
			}
		})
	}
}

func TestGraph_GetNode_NilGraph(t *testing.T) {
	var graph *Graph
	_, err := graph.GetNode(0)
	if err == nil {
		t.Error("GetNode on nil graph should return error")
	}
}

func TestGraph_Size_NilGraph(t *testing.T) {
	var graph *Graph
	if size := graph.Size(); size != 0 {
		t.Errorf("Nil graph size = %d, want 0", size)
	}
}

func TestGraph_IndependentNodes_NilGraph(t *testing.T) {
	var graph *Graph
	nodes := graph.IndependentNodes()
	if nodes != nil {
		t.Error("IndependentNodes on nil graph should return nil")
	}
}

func TestGraph_DetectConflicts_NilGraph(t *testing.T) {
	var graph *Graph
	conflicts := graph.DetectConflicts()
	if conflicts != nil {
		t.Error("DetectConflicts on nil graph should return nil")
	}
}

func TestGraph_HasCycles_NilGraph(t *testing.T) {
	var graph *Graph
	if graph.HasCycles() {
		t.Error("Nil graph should not have cycles")
	}
}

func TestGraph_ComplexDependencies(t *testing.T) {
	// Create a more complex dependency graph
	// key1: W1 -> R1 -> W2
	// key2: W3 -> R2
	// key3: W4 (independent)
	effects := []Effect{
		WriteEffect[string]{ // 0: W1
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "v1",
		},
		ReadEffect[string]{ // 1: R1 (depends on 0)
			Store:    "test",
			StoreKey: []byte("key1"),
			Dest:     new(string),
		},
		WriteEffect[string]{ // 2: W2 (depends on 0, 1)
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "v2",
		},
		WriteEffect[string]{ // 3: W3 (independent)
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "v3",
		},
		ReadEffect[string]{ // 4: R2 (depends on 3)
			Store:    "test",
			StoreKey: []byte("key2"),
			Dest:     new(string),
		},
		WriteEffect[string]{ // 5: W4 (independent)
			Store:    "test",
			StoreKey: []byte("key3"),
			Value:    "v4",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// Check node 1 dependencies (should depend on 0)
	node1 := graph.Nodes[1]
	if len(node1.Dependencies) != 1 || node1.Dependencies[0] != 0 {
		t.Errorf("Node 1 dependencies = %v, want [0]", node1.Dependencies)
	}

	// Check node 2 dependencies (should depend on 0 and 1)
	node2 := graph.Nodes[2]
	expectedDeps := map[int]bool{0: true, 1: true}
	if len(node2.Dependencies) != 2 {
		t.Errorf("Node 2 should have 2 dependencies, got %d", len(node2.Dependencies))
	}
	for _, dep := range node2.Dependencies {
		if !expectedDeps[dep] {
			t.Errorf("Node 2 unexpected dependency: %d", dep)
		}
	}

	// Check node 4 dependencies (should depend on 3)
	node4 := graph.Nodes[4]
	if len(node4.Dependencies) != 1 || node4.Dependencies[0] != 3 {
		t.Errorf("Node 4 dependencies = %v, want [3]", node4.Dependencies)
	}

	// Check independent nodes (should be 0, 3, 5)
	independent := graph.IndependentNodes()
	if len(independent) != 3 {
		t.Errorf("IndependentNodes = %d, want 3", len(independent))
	}

	// Verify topological sort
	sorted, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// Create position map
	positions := make(map[int]int)
	for i, node := range sorted {
		positions[node.Index] = i
	}

	// Verify dependencies are satisfied in sort order
	for _, node := range graph.Nodes {
		for _, dep := range node.Dependencies {
			if positions[dep] >= positions[node.Index] {
				t.Errorf("Dependency %d of node %d appears after it in topological sort", dep, node.Index)
			}
		}
	}
}

func TestGraph_Concurrent(t *testing.T) {
	// Test concurrent access to graph methods
	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "v1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "v2",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key3"),
			Value:    "v3",
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_ = graph.Size()
			_ = graph.IndependentNodes()
			_ = graph.DetectConflicts()
			_ = graph.HasCycles()
			_, _ = graph.TopologicalSort()
			_, _ = graph.GetNode(0)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

func TestGraph_EventEffects(t *testing.T) {
	// Event effects should not create dependencies
	effects := []Effect{
		EventEffect{
			EventType:  "test1",
			Attributes: map[string][]byte{"key": []byte("value")},
		},
		EventEffect{
			EventType:  "test2",
			Attributes: map[string][]byte{"key": []byte("value")},
		},
		EventEffect{
			EventType:  "test3",
			Attributes: map[string][]byte{"key": []byte("value")},
		},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// All event effects should be independent
	independent := graph.IndependentNodes()
	if len(independent) != 3 {
		t.Errorf("IndependentNodes = %d, want 3", len(independent))
	}

	// No conflicts should be detected
	conflicts := graph.DetectConflicts()
	if len(conflicts) != 0 {
		t.Errorf("DetectConflicts found %d conflicts, want 0", len(conflicts))
	}
}
