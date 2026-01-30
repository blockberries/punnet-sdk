package effects

import (
	"testing"
)

func TestNewScheduler(t *testing.T) {
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

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}
	if scheduler == nil {
		t.Fatal("NewScheduler returned nil scheduler")
	}
}

func TestNewScheduler_NilGraph(t *testing.T) {
	scheduler, err := NewScheduler(nil)
	if err == nil {
		t.Error("NewScheduler should fail with nil graph")
	}
	if scheduler != nil {
		t.Error("NewScheduler should return nil scheduler with nil graph")
	}
}

func TestScheduler_Schedule_Empty(t *testing.T) {
	graph, err := NewGraph([]Effect{})
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	batches, err := scheduler.Schedule()
	if err != nil {
		t.Errorf("Schedule failed: %v", err)
	}
	if len(batches) != 0 {
		t.Errorf("Expected 0 batches, got %d", len(batches))
	}
}

func TestScheduler_Schedule_NilScheduler(t *testing.T) {
	var scheduler *Scheduler
	_, err := scheduler.Schedule()
	if err == nil {
		t.Error("Schedule on nil scheduler should fail")
	}
}

func TestScheduler_Schedule_IndependentEffects(t *testing.T) {
	// All effects are independent - should be in one batch
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

	batches, err := ScheduleEffects(effects)
	if err != nil {
		t.Fatalf("ScheduleEffects failed: %v", err)
	}

	// All independent effects should be in one batch
	if len(batches) != 1 {
		t.Errorf("Expected 1 batch, got %d", len(batches))
	}

	if len(batches) > 0 && len(batches[0].Effects) != 3 {
		t.Errorf("Expected batch with 3 effects, got %d", len(batches[0].Effects))
	}
}

func TestScheduler_Schedule_SequentialDependencies(t *testing.T) {
	// Effects with sequential dependencies: W1 -> R1 -> W2
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

	batches, err := ScheduleEffects(effects)
	if err != nil {
		t.Fatalf("ScheduleEffects failed: %v", err)
	}

	// Should have 3 batches (one for each level)
	if len(batches) < 2 {
		t.Errorf("Expected at least 2 batches for sequential dependencies, got %d", len(batches))
	}

	// Verify ordering
	if len(batches) > 0 {
		// First batch should have the first write
		if batches[0].Level != 0 {
			t.Errorf("First batch level = %d, want 0", batches[0].Level)
		}
	}
}

func TestScheduler_Schedule_MixedDependencies(t *testing.T) {
	// Mix of independent and dependent effects
	effects := []Effect{
		WriteEffect[string]{ // 0: independent
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "v1",
		},
		WriteEffect[string]{ // 1: independent
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "v2",
		},
		ReadEffect[string]{ // 2: depends on 0
			Store:    "test",
			StoreKey: []byte("key1"),
			Dest:     new(string),
		},
		ReadEffect[string]{ // 3: depends on 1
			Store:    "test",
			StoreKey: []byte("key2"),
			Dest:     new(string),
		},
		WriteEffect[string]{ // 4: depends on 0, 2
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "v3",
		},
	}

	batches, err := ScheduleEffects(effects)
	if err != nil {
		t.Fatalf("ScheduleEffects failed: %v", err)
	}

	// Should have at least 3 levels
	if len(batches) < 3 {
		t.Errorf("Expected at least 3 batches, got %d", len(batches))
	}

	// Verify total effect count
	totalEffects := EffectCount(batches)
	if totalEffects != 5 {
		t.Errorf("Total effects = %d, want 5", totalEffects)
	}
}

func TestScheduleEffects_Nil(t *testing.T) {
	_, err := ScheduleEffects(nil)
	if err == nil {
		t.Error("ScheduleEffects should fail with nil effects")
	}
}

func TestScheduleEffects_InvalidEffect(t *testing.T) {
	effects := []Effect{
		WriteEffect[string]{
			Store:    "", // Invalid
			StoreKey: []byte("key"),
			Value:    "value",
		},
	}

	_, err := ScheduleEffects(effects)
	if err == nil {
		t.Error("ScheduleEffects should fail with invalid effect")
	}
}

func TestScheduler_ParallelismFactor(t *testing.T) {
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

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	batches, err := scheduler.Schedule()
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	factor := scheduler.ParallelismFactor(batches)
	// 3 independent effects in 1 batch = factor of 3.0
	if factor != 3.0 {
		t.Errorf("ParallelismFactor = %.2f, want 3.00", factor)
	}
}

func TestScheduler_ParallelismFactor_NilScheduler(t *testing.T) {
	var scheduler *Scheduler
	factor := scheduler.ParallelismFactor(nil)
	if factor != 1.0 {
		t.Errorf("ParallelismFactor for nil scheduler = %.2f, want 1.00", factor)
	}
}

func TestScheduler_ParallelismFactor_EmptyBatches(t *testing.T) {
	graph, err := NewGraph([]Effect{})
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	factor := scheduler.ParallelismFactor([]Batch{})
	if factor != 1.0 {
		t.Errorf("ParallelismFactor for empty batches = %.2f, want 1.00", factor)
	}
}

func TestScheduler_GetStatistics(t *testing.T) {
	effects := []Effect{
		WriteEffect[string]{Store: "test", StoreKey: []byte("key1"), Value: "v1"},
		WriteEffect[string]{Store: "test", StoreKey: []byte("key2"), Value: "v2"},
		WriteEffect[string]{Store: "test", StoreKey: []byte("key3"), Value: "v3"},
		WriteEffect[string]{Store: "test", StoreKey: []byte("key4"), Value: "v4"},
		WriteEffect[string]{Store: "test", StoreKey: []byte("key5"), Value: "v5"},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	batches, err := scheduler.Schedule()
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	stats := scheduler.GetStatistics(batches)

	if stats.TotalEffects != 5 {
		t.Errorf("TotalEffects = %d, want 5", stats.TotalEffects)
	}
	if stats.TotalBatches != len(batches) {
		t.Errorf("TotalBatches = %d, want %d", stats.TotalBatches, len(batches))
	}
	if stats.ParallelismFactor <= 0 {
		t.Errorf("ParallelismFactor = %.2f, want > 0", stats.ParallelismFactor)
	}
}

func TestScheduler_GetStatistics_EmptyBatches(t *testing.T) {
	graph, err := NewGraph([]Effect{})
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	stats := scheduler.GetStatistics([]Batch{})

	if stats.TotalBatches != 0 {
		t.Errorf("TotalBatches = %d, want 0", stats.TotalBatches)
	}
	if stats.TotalEffects != 0 {
		t.Errorf("TotalEffects = %d, want 0", stats.TotalEffects)
	}
	if stats.MinBatchSize != -1 {
		t.Errorf("MinBatchSize = %d, want -1 for empty batches", stats.MinBatchSize)
	}
}

func TestScheduler_OptimizeBatches(t *testing.T) {
	graph, err := NewGraph([]Effect{})
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	// Create batches with small sizes
	batches := []Batch{
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k1"), Value: "v1"}}, Level: 0},
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k2"), Value: "v2"}}, Level: 1},
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k3"), Value: "v3"}}, Level: 2},
	}

	optimized := scheduler.OptimizeBatches(batches, 10)

	// Should be combined into fewer batches
	if len(optimized) == 0 {
		t.Error("OptimizeBatches returned empty result")
	}

	// Total effects should be preserved
	if EffectCount(optimized) != 3 {
		t.Errorf("Effect count after optimization = %d, want 3", EffectCount(optimized))
	}
}

func TestScheduler_OptimizeBatches_NilScheduler(t *testing.T) {
	var scheduler *Scheduler
	batches := []Batch{
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k"), Value: "v"}}, Level: 0},
	}

	optimized := scheduler.OptimizeBatches(batches, 10)
	if len(optimized) != 1 {
		t.Error("OptimizeBatches on nil scheduler should return original batches")
	}
}

func TestScheduler_OptimizeBatches_EmptyBatches(t *testing.T) {
	graph, err := NewGraph([]Effect{})
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	optimized := scheduler.OptimizeBatches([]Batch{}, 10)
	if len(optimized) != 0 {
		t.Errorf("OptimizeBatches on empty batches should return empty, got %d", len(optimized))
	}
}

func TestScheduler_OptimizeBatches_MaxSize(t *testing.T) {
	graph, err := NewGraph([]Effect{})
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	// Create batches that would exceed max size
	batches := []Batch{
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k1"), Value: "v1"},
			WriteEffect[string]{Store: "t", StoreKey: []byte("k2"), Value: "v2"},
		}, Level: 0},
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k3"), Value: "v3"},
			WriteEffect[string]{Store: "t", StoreKey: []byte("k4"), Value: "v4"},
		}, Level: 1},
	}

	// Max size of 2 should prevent combining
	optimized := scheduler.OptimizeBatches(batches, 2)

	if len(optimized) < 2 {
		t.Errorf("OptimizeBatches should respect max size, got %d batches", len(optimized))
	}
}

func TestValidateBatches(t *testing.T) {
	// Valid batches - no conflicts
	batches := []Batch{
		{Effects: []Effect{
			WriteEffect[string]{Store: "test", StoreKey: []byte("key1"), Value: "v1"},
			WriteEffect[string]{Store: "test", StoreKey: []byte("key2"), Value: "v2"},
		}},
	}

	err := ValidateBatches(batches)
	if err != nil {
		t.Errorf("ValidateBatches failed on valid batches: %v", err)
	}
}

func TestValidateBatches_Conflict(t *testing.T) {
	// Invalid batches - write-write conflict
	batches := []Batch{
		{Effects: []Effect{
			WriteEffect[string]{Store: "test", StoreKey: []byte("key"), Value: "v1"},
			WriteEffect[string]{Store: "test", StoreKey: []byte("key"), Value: "v2"},
		}},
	}

	err := ValidateBatches(batches)
	if err == nil {
		t.Error("ValidateBatches should detect conflict")
	}
}

func TestEffectCount(t *testing.T) {
	batches := []Batch{
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k1"), Value: "v1"},
			WriteEffect[string]{Store: "t", StoreKey: []byte("k2"), Value: "v2"},
		}},
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k3"), Value: "v3"},
		}},
	}

	count := EffectCount(batches)
	if count != 3 {
		t.Errorf("EffectCount = %d, want 3", count)
	}
}

func TestEffectCount_Empty(t *testing.T) {
	count := EffectCount([]Batch{})
	if count != 0 {
		t.Errorf("EffectCount = %d, want 0", count)
	}
}

func TestGetBatch(t *testing.T) {
	batches := []Batch{
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k1"), Value: "v1"}}, Level: 0},
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k2"), Value: "v2"}}, Level: 1},
	}

	batch, err := GetBatch(batches, 0)
	if err != nil {
		t.Errorf("GetBatch failed: %v", err)
	}
	if batch == nil {
		t.Fatal("GetBatch returned nil batch")
	}
	if batch.Level != 0 {
		t.Errorf("Batch level = %d, want 0", batch.Level)
	}
}

func TestGetBatch_OutOfBounds(t *testing.T) {
	batches := []Batch{
		{Effects: []Effect{WriteEffect[string]{Store: "t", StoreKey: []byte("k"), Value: "v"}}, Level: 0},
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
			_, err := GetBatch(batches, tt.index)
			if err == nil {
				t.Errorf("GetBatch(%d) should return error", tt.index)
			}
		})
	}
}

func TestExtractEffects(t *testing.T) {
	batches := []Batch{
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k1"), Value: "v1"},
			WriteEffect[string]{Store: "t", StoreKey: []byte("k2"), Value: "v2"},
		}},
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k3"), Value: "v3"},
		}},
	}

	effects := ExtractEffects(batches)
	if len(effects) != 3 {
		t.Errorf("ExtractEffects returned %d effects, want 3", len(effects))
	}
}

func TestExtractEffects_Empty(t *testing.T) {
	effects := ExtractEffects([]Batch{})
	if len(effects) != 0 {
		t.Errorf("ExtractEffects on empty batches returned %d effects, want 0", len(effects))
	}
}

func TestConvertToExecutorFormat(t *testing.T) {
	batches := []Batch{
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k1"), Value: "v1"},
		}},
		{Effects: []Effect{
			WriteEffect[string]{Store: "t", StoreKey: []byte("k2"), Value: "v2"},
		}},
	}

	execFormat := ConvertToExecutorFormat(batches)
	if len(execFormat) != 2 {
		t.Errorf("ConvertToExecutorFormat returned %d batches, want 2", len(execFormat))
	}
	if len(execFormat[0]) != 1 {
		t.Errorf("First batch has %d effects, want 1", len(execFormat[0]))
	}
}

func TestConvertToExecutorFormat_Empty(t *testing.T) {
	execFormat := ConvertToExecutorFormat([]Batch{})
	if len(execFormat) != 0 {
		t.Errorf("ConvertToExecutorFormat on empty batches returned %d, want 0", len(execFormat))
	}
}

func TestScheduler_ComplexScenario(t *testing.T) {
	// Create a complex scenario with multiple dependency chains
	effects := []Effect{
		// Chain 1: key1
		WriteEffect[string]{Store: "test", StoreKey: []byte("key1"), Value: "v1"}, // 0
		ReadEffect[string]{Store: "test", StoreKey: []byte("key1"), Dest: new(string)}, // 1
		WriteEffect[string]{Store: "test", StoreKey: []byte("key1"), Value: "v2"}, // 2

		// Chain 2: key2 (parallel to chain 1)
		WriteEffect[string]{Store: "test", StoreKey: []byte("key2"), Value: "v3"}, // 3
		ReadEffect[string]{Store: "test", StoreKey: []byte("key2"), Dest: new(string)}, // 4

		// Chain 3: key3 (independent)
		WriteEffect[string]{Store: "test", StoreKey: []byte("key3"), Value: "v4"}, // 5

		// Events (independent)
		EventEffect{EventType: "event1", Attributes: map[string][]byte{}}, // 6
		EventEffect{EventType: "event2", Attributes: map[string][]byte{}}, // 7
	}

	batches, err := ScheduleEffects(effects)
	if err != nil {
		t.Fatalf("ScheduleEffects failed: %v", err)
	}

	// Verify batch structure
	if len(batches) == 0 {
		t.Fatal("No batches created")
	}

	// Verify all effects are included
	totalEffects := EffectCount(batches)
	if totalEffects != 8 {
		t.Errorf("Total effects = %d, want 8", totalEffects)
	}

	// Validate no conflicts within batches
	if err := ValidateBatches(batches); err != nil {
		t.Errorf("Batches contain conflicts: %v", err)
	}

	// Get statistics
	graph, _ := NewGraph(effects)
	scheduler, _ := NewScheduler(graph)
	stats := scheduler.GetStatistics(batches)

	t.Logf("Statistics: Batches=%d, Effects=%d, AvgSize=%.2f, Parallelism=%.2f",
		stats.TotalBatches, stats.TotalEffects, stats.AvgBatchSize, stats.ParallelismFactor)

	// Parallelism factor should be > 1 (some parallel execution)
	if stats.ParallelismFactor <= 1.0 {
		t.Errorf("ParallelismFactor = %.2f, expected > 1.0 for this scenario", stats.ParallelismFactor)
	}
}

func TestScheduler_EventsOnly(t *testing.T) {
	// Events should all be in one batch (all independent)
	effects := []Effect{
		EventEffect{EventType: "event1", Attributes: map[string][]byte{"id": []byte("1")}},
		EventEffect{EventType: "event2", Attributes: map[string][]byte{"id": []byte("2")}},
		EventEffect{EventType: "event3", Attributes: map[string][]byte{"id": []byte("3")}},
		EventEffect{EventType: "event4", Attributes: map[string][]byte{"id": []byte("4")}},
	}

	batches, err := ScheduleEffects(effects)
	if err != nil {
		t.Fatalf("ScheduleEffects failed: %v", err)
	}

	// All events should be in one batch
	if len(batches) != 1 {
		t.Errorf("Expected 1 batch for events, got %d", len(batches))
	}

	if len(batches) > 0 && len(batches[0].Effects) != 4 {
		t.Errorf("Expected 4 effects in batch, got %d", len(batches[0].Effects))
	}
}

func TestScheduler_Concurrent(t *testing.T) {
	// Test concurrent scheduling operations
	effects := []Effect{
		WriteEffect[string]{Store: "test", StoreKey: []byte("key1"), Value: "v1"},
		WriteEffect[string]{Store: "test", StoreKey: []byte("key2"), Value: "v2"},
		WriteEffect[string]{Store: "test", StoreKey: []byte("key3"), Value: "v3"},
	}

	graph, err := NewGraph(effects)
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	scheduler, err := NewScheduler(graph)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	const numGoroutines = 100
	done := make(chan bool, numGoroutines)
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()

			batches, err := scheduler.Schedule()
			if err != nil {
				errChan <- err
				return
			}

			_ = scheduler.GetStatistics(batches)
			_ = scheduler.ParallelismFactor(batches)
			_ = EffectCount(batches)
			_ = ExtractEffects(batches)
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for errors
	close(errChan)
	for err := range errChan {
		t.Errorf("Concurrent error: %v", err)
	}
}
