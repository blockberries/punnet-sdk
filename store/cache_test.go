package store

import (
	"sync"
	"testing"
)

func TestCache_Basic(t *testing.T) {
	cache := NewCache[string](10)

	// Test Set and Get
	entry := CacheEntry[string]{Value: "value1", Dirty: false, Deleted: false}
	cache.Set("key1", entry)

	got, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if got.Value != "value1" {
		t.Errorf("expected value1, got %s", got.Value)
	}
}

func TestCache_NotFound(t *testing.T) {
	cache := NewCache[string](10)

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent key")
	}
}

func TestCache_Delete(t *testing.T) {
	cache := NewCache[string](10)

	entry := CacheEntry[string]{Value: "value1", Dirty: false, Deleted: false}
	cache.Set("key1", entry)

	cache.Delete("key1")

	if cache.Has("key1") {
		t.Error("expected key1 to be deleted")
	}
}

func TestCache_LRUEviction(t *testing.T) {
	cache := NewCache[string](3)

	// Fill cache to capacity
	for i := 0; i < 3; i++ {
		key := string(rune('a' + i))
		entry := CacheEntry[string]{Value: key, Dirty: false, Deleted: false}
		cache.Set(key, entry)
	}

	// Add one more - should evict LRU (first one)
	entry := CacheEntry[string]{Value: "d", Dirty: false, Deleted: false}
	cache.Set("d", entry)

	if cache.Has("a") {
		t.Error("expected 'a' to be evicted")
	}
	if !cache.Has("b") {
		t.Error("expected 'b' to still be in cache")
	}
	if !cache.Has("c") {
		t.Error("expected 'c' to still be in cache")
	}
	if !cache.Has("d") {
		t.Error("expected 'd' to be in cache")
	}
}

func TestCache_LRUUpdate(t *testing.T) {
	cache := NewCache[string](3)

	// Fill cache
	for i := 0; i < 3; i++ {
		key := string(rune('a' + i))
		entry := CacheEntry[string]{Value: key, Dirty: false, Deleted: false}
		cache.Set(key, entry)
	}

	// Access 'a' to make it most recently used
	cache.Get("a")

	// Add one more - should evict 'b' (now LRU)
	entry := CacheEntry[string]{Value: "d", Dirty: false, Deleted: false}
	cache.Set("d", entry)

	if !cache.Has("a") {
		t.Error("expected 'a' to still be in cache (was accessed)")
	}
	if cache.Has("b") {
		t.Error("expected 'b' to be evicted")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache[string](10)

	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		entry := CacheEntry[string]{Value: key, Dirty: false, Deleted: false}
		cache.Set(key, entry)
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected cache to be empty, got len=%d", cache.Len())
	}
}

func TestCache_Stats(t *testing.T) {
	cache := NewCache[string](10)

	entry := CacheEntry[string]{Value: "value", Dirty: false, Deleted: false}
	cache.Set("key1", entry)

	// Hit
	cache.Get("key1")

	// Miss
	cache.Get("key2")

	hits, misses := cache.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
}

func TestCache_Concurrent(t *testing.T) {
	cache := NewCache[int](100)

	var wg sync.WaitGroup
	concurrency := 10
	operations := 100

	// Concurrent writes
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				key := string(rune('a' + (id*operations+j)%26))
				entry := CacheEntry[int]{Value: id*operations + j, Dirty: true, Deleted: false}
				cache.Set(key, entry)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				key := string(rune('a' + j%26))
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()
}

func TestCache_DirtyEntries(t *testing.T) {
	cache := NewCache[string](10)

	// Add clean entry
	cache.Set("key1", CacheEntry[string]{Value: "value1", Dirty: false, Deleted: false})

	// Add dirty entry
	cache.Set("key2", CacheEntry[string]{Value: "value2", Dirty: true, Deleted: false})

	// Add deleted entry
	cache.Set("key3", CacheEntry[string]{Value: "value3", Dirty: true, Deleted: true})

	dirty := cache.GetDirtyEntries()

	if len(dirty) != 2 {
		t.Errorf("expected 2 dirty entries, got %d", len(dirty))
	}

	if !dirty["key2"].Dirty {
		t.Error("expected key2 to be dirty")
	}

	if !dirty["key3"].Deleted {
		t.Error("expected key3 to be deleted")
	}
}

func TestCache_ClearDirtyFlag(t *testing.T) {
	cache := NewCache[string](10)

	cache.Set("key1", CacheEntry[string]{Value: "value1", Dirty: true, Deleted: false})

	dirty := cache.GetDirtyEntries()
	if len(dirty) != 1 {
		t.Fatal("expected 1 dirty entry")
	}

	cache.ClearDirtyFlag("key1")

	dirty = cache.GetDirtyEntries()
	if len(dirty) != 0 {
		t.Error("expected no dirty entries after clearing flag")
	}
}

func TestCache_NilCache(t *testing.T) {
	var cache *Cache[string]

	// All operations should handle nil gracefully
	_, ok := cache.Get("key")
	if ok {
		t.Error("expected Get on nil cache to return false")
	}

	cache.Set("key", CacheEntry[string]{})
	cache.Delete("key")
	cache.Clear()

	if cache.Has("key") {
		t.Error("expected Has on nil cache to return false")
	}

	if cache.Len() != 0 {
		t.Error("expected Len on nil cache to return 0")
	}

	hits, misses := cache.Stats()
	if hits != 0 || misses != 0 {
		t.Error("expected Stats on nil cache to return zeros")
	}
}

func TestMultiLevelCache_Basic(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	// Set in L1
	entry := CacheEntry[string]{Value: "value1", Dirty: false, Deleted: false}
	mc.Set("key1", entry)

	// Get from L1
	got, level, ok := mc.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if level != L1Cache {
		t.Errorf("expected L1Cache, got %v", level)
	}
	if got.Value != "value1" {
		t.Errorf("expected value1, got %s", got.Value)
	}
}

func TestMultiLevelCache_Promotion(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	// Set in L2 directly (simulating cache hierarchy)
	entry := CacheEntry[string]{Value: "value1", Dirty: false, Deleted: false}
	mc.l2.Set("key1", entry)

	// First get - should promote from L2 to L1
	got, level, ok := mc.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if level != L2Cache {
		t.Errorf("expected L2Cache on first get, got %v", level)
	}

	// Second get - should now be in L1
	got, level, ok = mc.Get("key1")
	if !ok {
		t.Fatal("expected to find key1 in L1")
	}
	if level != L1Cache {
		t.Errorf("expected L1Cache after promotion, got %v", level)
	}
	if got.Value != "value1" {
		t.Errorf("expected value1, got %s", got.Value)
	}
}

func TestMultiLevelCache_Delete(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	// Set in both levels
	entry := CacheEntry[string]{Value: "value1", Dirty: false, Deleted: false}
	mc.l1.Set("key1", entry)
	mc.l2.Set("key1", entry)

	// Delete
	mc.Delete("key1")

	// Should be gone from both
	if mc.l1.Has("key1") {
		t.Error("expected key1 to be deleted from L1")
	}
	if mc.l2.Has("key1") {
		t.Error("expected key1 to be deleted from L2")
	}
}

func TestMultiLevelCache_Clear(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	// Add to both levels
	entry := CacheEntry[string]{Value: "value", Dirty: false, Deleted: false}
	mc.l1.Set("key1", entry)
	mc.l2.Set("key2", entry)

	mc.Clear()

	if mc.l1.Len() != 0 {
		t.Error("expected L1 to be empty")
	}
	if mc.l2.Len() != 0 {
		t.Error("expected L2 to be empty")
	}
}

func TestMultiLevelCache_DirtyEntries(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	// Add dirty to L1
	mc.l1.Set("key1", CacheEntry[string]{Value: "value1", Dirty: true, Deleted: false})

	// Add dirty to L2
	mc.l2.Set("key2", CacheEntry[string]{Value: "value2", Dirty: true, Deleted: false})

	dirty := mc.GetDirtyEntries()

	if len(dirty) != 2 {
		t.Errorf("expected 2 dirty entries, got %d", len(dirty))
	}

	if !dirty["key1"].Dirty {
		t.Error("expected key1 to be dirty")
	}
	if !dirty["key2"].Dirty {
		t.Error("expected key2 to be dirty")
	}
}

func TestMultiLevelCache_ClearDirtyFlags(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	// Add dirty entries
	mc.l1.Set("key1", CacheEntry[string]{Value: "value1", Dirty: true, Deleted: false})
	mc.l2.Set("key2", CacheEntry[string]{Value: "value2", Dirty: true, Deleted: false})

	// Clear flags
	mc.ClearDirtyFlags([]string{"key1", "key2"})

	dirty := mc.GetDirtyEntries()
	if len(dirty) != 0 {
		t.Errorf("expected no dirty entries, got %d", len(dirty))
	}
}

func TestMultiLevelCache_Stats(t *testing.T) {
	mc := NewMultiLevelCache[string](10, 100)

	entry := CacheEntry[string]{Value: "value", Dirty: false, Deleted: false}
	mc.Set("key1", entry)
	mc.l2.Set("key2", entry)

	// L1 hit
	mc.Get("key1")

	// L2 hit (miss in L1), promotes to L1
	mc.Get("key2")

	// Second get of key2 should now hit L1
	mc.Get("key2")

	// Complete miss
	mc.Get("key3")

	l1Hits, _, l2Hits, _ := mc.Stats()

	// key1: 1 L1 hit
	// key2: 1 L1 miss + 1 L2 hit (on first get), then 1 L1 hit (on second get)
	// key2 second: 1 L1 hit
	// Total: 2 L1 hits, 1 L2 hit
	if l1Hits != 2 {
		t.Errorf("expected 2 L1 hits, got %d", l1Hits)
	}
	if l2Hits != 1 {
		t.Errorf("expected 1 L2 hit, got %d", l2Hits)
	}
}

func TestMultiLevelCache_NilCache(t *testing.T) {
	var mc *MultiLevelCache[string]

	// All operations should handle nil gracefully
	_, _, ok := mc.Get("key")
	if ok {
		t.Error("expected Get on nil cache to return false")
	}

	mc.Set("key", CacheEntry[string]{})
	mc.Delete("key")
	mc.Clear()

	dirty := mc.GetDirtyEntries()
	if len(dirty) != 0 {
		t.Error("expected GetDirtyEntries on nil cache to return empty map")
	}

	mc.ClearDirtyFlags([]string{"key"})

	l1Hits, _, l2Hits, _ := mc.Stats()
	if l1Hits != 0 || l2Hits != 0 {
		t.Error("expected Stats on nil cache to return zeros")
	}
}

func BenchmarkCache_Set(b *testing.B) {
	cache := NewCache[int](10000)
	entry := CacheEntry[int]{Value: 42, Dirty: false, Deleted: false}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		cache.Set(key, entry)
	}
}

func BenchmarkCache_Get(b *testing.B) {
	cache := NewCache[int](10000)
	entry := CacheEntry[int]{Value: 42, Dirty: false, Deleted: false}

	// Pre-populate
	for i := 0; i < 26; i++ {
		key := string(rune('a' + i))
		cache.Set(key, entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune('a' + i%26))
		cache.Get(key)
	}
}

func BenchmarkCache_Concurrent(b *testing.B) {
	cache := NewCache[int](10000)
	entry := CacheEntry[int]{Value: 42, Dirty: false, Deleted: false}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := string(rune('a' + i%26))
			if i%2 == 0 {
				cache.Set(key, entry)
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}
