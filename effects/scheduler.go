package effects

import (
	"fmt"
)

// Batch represents a batch of effects that can be executed in parallel
type Batch struct {
	// Effects are the effects in this batch
	Effects []Effect

	// Level is the dependency level (0 = no dependencies, 1 = depends on level 0, etc.)
	Level int
}

// Scheduler schedules effects for parallel execution
type Scheduler struct {
	// graph is the dependency graph
	graph *Graph
}

// NewScheduler creates a new scheduler from a dependency graph
func NewScheduler(graph *Graph) (*Scheduler, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	return &Scheduler{
		graph: graph,
	}, nil
}

// Schedule creates execution batches from the dependency graph
// Returns batches ordered by dependency level (all effects in a batch can execute in parallel)
func (s *Scheduler) Schedule() ([]Batch, error) {
	if s == nil {
		return nil, fmt.Errorf("scheduler is nil")
	}

	// Check for cycles first
	if s.graph.HasCycles() {
		return nil, fmt.Errorf("graph has cycles, cannot schedule")
	}

	// Track which nodes have been scheduled
	scheduled := make(map[int]bool)
	batches := make([]Batch, 0)

	level := 0
	for len(scheduled) < s.graph.Size() {
		// Find all nodes whose dependencies have been scheduled
		batch := s.findReadyNodes(scheduled)
		if len(batch) == 0 {
			// No more nodes can be scheduled - there might be a cycle or isolated component
			if len(scheduled) < s.graph.Size() {
				return nil, fmt.Errorf("unable to schedule all effects: %d/%d scheduled", len(scheduled), s.graph.Size())
			}
			break
		}

		// Create batch
		effects := make([]Effect, len(batch))
		for i, nodeIdx := range batch {
			effects[i] = s.graph.Nodes[nodeIdx].Effect
			scheduled[nodeIdx] = true
		}

		batches = append(batches, Batch{
			Effects: effects,
			Level:   level,
		})
		level++
	}

	return batches, nil
}

// findReadyNodes finds all nodes that are ready to execute
// A node is ready if all its dependencies have been scheduled
func (s *Scheduler) findReadyNodes(scheduled map[int]bool) []int {
	if s == nil || s.graph == nil {
		return nil
	}

	ready := make([]int, 0)

	for i, node := range s.graph.Nodes {
		// Skip already scheduled nodes
		if scheduled[i] {
			continue
		}

		// Check if all dependencies are scheduled
		allDepsScheduled := true
		for _, depIdx := range node.Dependencies {
			if !scheduled[depIdx] {
				allDepsScheduled = false
				break
			}
		}

		if allDepsScheduled {
			ready = append(ready, i)
		}
	}

	return ready
}

// ScheduleEffects is a convenience function that creates a graph and schedules it
func ScheduleEffects(effects []Effect) ([]Batch, error) {
	if effects == nil {
		return nil, fmt.Errorf("effects cannot be nil")
	}

	// Create dependency graph
	graph, err := NewGraph(effects)
	if err != nil {
		return nil, fmt.Errorf("failed to create graph: %w", err)
	}

	// Create scheduler
	scheduler, err := NewScheduler(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Schedule effects
	batches, err := scheduler.Schedule()
	if err != nil {
		return nil, fmt.Errorf("failed to schedule: %w", err)
	}

	return batches, nil
}

// ParallelismFactor returns the potential speedup from parallel execution
// Returns the ratio of total effects to number of batches
func (s *Scheduler) ParallelismFactor(batches []Batch) float64 {
	if s == nil || len(batches) == 0 {
		return 1.0
	}

	totalEffects := 0
	for _, batch := range batches {
		totalEffects += len(batch.Effects)
	}

	if totalEffects == 0 {
		return 1.0
	}

	return float64(totalEffects) / float64(len(batches))
}

// BatchStatistics returns statistics about the batches
type BatchStatistics struct {
	// TotalBatches is the number of batches
	TotalBatches int

	// TotalEffects is the total number of effects
	TotalEffects int

	// MinBatchSize is the size of the smallest batch
	MinBatchSize int

	// MaxBatchSize is the size of the largest batch
	MaxBatchSize int

	// AvgBatchSize is the average batch size
	AvgBatchSize float64

	// ParallelismFactor is the potential speedup
	ParallelismFactor float64
}

// GetStatistics computes statistics for a set of batches
func (s *Scheduler) GetStatistics(batches []Batch) BatchStatistics {
	stats := BatchStatistics{
		TotalBatches: len(batches),
		MinBatchSize: -1,
	}

	if len(batches) == 0 {
		return stats
	}

	for _, batch := range batches {
		size := len(batch.Effects)
		stats.TotalEffects += size

		if stats.MinBatchSize == -1 || size < stats.MinBatchSize {
			stats.MinBatchSize = size
		}
		if size > stats.MaxBatchSize {
			stats.MaxBatchSize = size
		}
	}

	if stats.TotalBatches > 0 {
		stats.AvgBatchSize = float64(stats.TotalEffects) / float64(stats.TotalBatches)
	}

	stats.ParallelismFactor = s.ParallelismFactor(batches)

	return stats
}

// OptimizeBatches attempts to optimize batch sizes by grouping small sequential batches
// This reduces scheduling overhead while maintaining correctness
func (s *Scheduler) OptimizeBatches(batches []Batch, maxBatchSize int) []Batch {
	if s == nil || len(batches) == 0 || maxBatchSize <= 0 {
		return batches
	}

	optimized := make([]Batch, 0, len(batches))
	currentBatch := Batch{
		Effects: make([]Effect, 0),
		Level:   0,
	}

	for i, batch := range batches {
		// If adding this batch would exceed max size, start a new batch
		if len(currentBatch.Effects)+len(batch.Effects) > maxBatchSize && len(currentBatch.Effects) > 0 {
			optimized = append(optimized, currentBatch)
			currentBatch = Batch{
				Effects: make([]Effect, 0),
				Level:   i,
			}
		}

		// Add effects to current batch
		currentBatch.Effects = append(currentBatch.Effects, batch.Effects...)
		if currentBatch.Level == 0 && len(currentBatch.Effects) > 0 {
			currentBatch.Level = batch.Level
		}
	}

	// Add final batch if not empty
	if len(currentBatch.Effects) > 0 {
		optimized = append(optimized, currentBatch)
	}

	return optimized
}

// ValidateBatches checks if batches are valid for parallel execution
// Returns error if any batch contains conflicting effects
func ValidateBatches(batches []Batch) error {
	for batchIdx, batch := range batches {
		// Check for conflicts within the batch
		for i := 0; i < len(batch.Effects); i++ {
			for j := i + 1; j < len(batch.Effects); j++ {
				if conflict := DetectConflict(batch.Effects[i], batch.Effects[j]); conflict != nil {
					return fmt.Errorf("batch %d: conflict detected: %v", batchIdx, conflict)
				}
			}
		}
	}
	return nil
}

// EffectCount returns the total number of effects across all batches
func EffectCount(batches []Batch) int {
	count := 0
	for _, batch := range batches {
		count += len(batch.Effects)
	}
	return count
}

// GetBatch returns the batch at the specified index
func GetBatch(batches []Batch, index int) (*Batch, error) {
	if index < 0 || index >= len(batches) {
		return nil, fmt.Errorf("batch index out of bounds: %d", index)
	}
	return &batches[index], nil
}

// ExtractEffects extracts all effects from batches into a flat list
func ExtractEffects(batches []Batch) []Effect {
	effects := make([]Effect, 0, EffectCount(batches))
	for _, batch := range batches {
		effects = append(effects, batch.Effects...)
	}
	return effects
}

// ConvertToExecutorFormat converts scheduler batches to executor format
func ConvertToExecutorFormat(batches []Batch) [][]Effect {
	result := make([][]Effect, len(batches))
	for i, batch := range batches {
		result[i] = make([]Effect, len(batch.Effects))
		copy(result[i], batch.Effects)
	}
	return result
}
