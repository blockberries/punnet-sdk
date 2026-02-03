package crypto

import (
	"fmt"
	"os"
	"testing"
)

// BenchmarkCachingKeyStore_CacheHit measures cache hit latency.
// Target: <100ns per operation.
func BenchmarkCachingKeyStore_CacheHit(b *testing.B) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 1000)
	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate cache
	key := EncryptedKey{
		Name:        "bench-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      make([]byte, 32),
		PrivKeyData: make([]byte, 64),
	}
	if err := cache.Store("bench-key", key); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.Load("bench-key")
	}

	b.StopTimer()
	hits, _, hitRate := cache.Stats()
	b.ReportMetric(hitRate, "hit_rate")
	b.Logf("Cache hits: %d, hit rate: %.2f%%", hits, hitRate*100)
}

// BenchmarkCachingKeyStore_CacheMiss measures cache miss latency with in-memory backend.
func BenchmarkCachingKeyStore_CacheMiss(b *testing.B) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10) // Small cache to force misses

	// Pre-populate backend with many keys
	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("key-%d", i)
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      make([]byte, 32),
			PrivKeyData: make([]byte, 64),
		}
		if err := backend.Store(name, key); err != nil {
			b.Fatal(err)
		}
	}

	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("key-%d", i%1000)
		_, _ = cache.Load(name)
	}

	b.StopTimer()
	hits, misses, hitRate := cache.Stats()
	b.ReportMetric(hitRate, "hit_rate")
	b.Logf("Cache hits: %d, misses: %d, hit rate: %.2f%%", hits, misses, hitRate*100)
}

// BenchmarkCachingKeyStore_MixedWorkload simulates realistic read-heavy workload.
// 90% reads, 10% writes with Zipf distribution (some keys accessed more often).
func BenchmarkCachingKeyStore_MixedWorkload(b *testing.B) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 100)
	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("key-%d", i)
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      make([]byte, 32),
			PrivKeyData: make([]byte, 64),
		}
		if err := cache.Store(name, key); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Zipf-like: lower indices accessed more frequently
		idx := (i * i) % 50
		name := fmt.Sprintf("key-%d", idx)

		if i%10 == 0 {
			// 10% writes
			key := EncryptedKey{
				Name:        name,
				Algorithm:   AlgorithmEd25519,
				PubKey:      make([]byte, 32),
				PrivKeyData: make([]byte, 64),
			}
			_ = cache.Store(name, key)
		} else {
			// 90% reads
			_, _ = cache.Load(name)
		}
	}

	b.StopTimer()
	hits, misses, hitRate := cache.Stats()
	b.ReportMetric(hitRate, "hit_rate")
	b.Logf("Cache hits: %d, misses: %d, hit rate: %.2f%%", hits, misses, hitRate*100)
}

// BenchmarkCachingKeyStore_LRUEviction measures eviction overhead.
func BenchmarkCachingKeyStore_LRUEviction(b *testing.B) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10) // Tiny cache to force evictions
	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("evict-key-%d", i)
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      make([]byte, 32),
			PrivKeyData: make([]byte, 64),
		}
		_ = cache.Store(name, key)
	}
}

// BenchmarkCachingKeyStore_ParallelReads measures concurrent read performance.
func BenchmarkCachingKeyStore_ParallelReads(b *testing.B) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 1000)
	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("parallel-key-%d", i)
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      make([]byte, 32),
			PrivKeyData: make([]byte, 64),
		}
		if err := cache.Store(name, key); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			name := fmt.Sprintf("parallel-key-%d", i%100)
			_, _ = cache.Load(name)
			i++
		}
	})

	b.StopTimer()
	hits, misses, hitRate := cache.Stats()
	b.ReportMetric(hitRate, "hit_rate")
	b.Logf("Cache hits: %d, misses: %d, hit rate: %.2f%%", hits, misses, hitRate*100)
}

// BenchmarkFileKeyStore_Load measures FileKeyStore load latency for comparison.
// This demonstrates the speedup from caching.
func BenchmarkFileKeyStore_Load(b *testing.B) {
	// Create temp directory for file store
	dir, err := os.MkdirTemp("", "keystore-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewFileKeyStore(dir, "benchmark-password")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if closer, ok := store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	// Store a key
	key := EncryptedKey{
		Name:        "bench-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      make([]byte, 32),
		PrivKeyData: make([]byte, 64),
	}
	if err := store.Store("bench-key", key); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = store.Load("bench-key")
	}
}

// BenchmarkCachingKeyStore_WithFileBackend measures cached FileKeyStore performance.
// Demonstrates real-world speedup from caching disk I/O.
func BenchmarkCachingKeyStore_WithFileBackend(b *testing.B) {
	// Create temp directory for file store
	dir, err := os.MkdirTemp("", "keystore-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fileStore, err := NewFileKeyStore(dir, "benchmark-password")
	if err != nil {
		b.Fatal(err)
	}

	cache := NewCachingKeyStore(fileStore, 100)
	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Store a key
	key := EncryptedKey{
		Name:        "bench-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      make([]byte, 32),
		PrivKeyData: make([]byte, 64),
	}
	if err := cache.Store("bench-key", key); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.Load("bench-key")
	}

	b.StopTimer()
	hits, _, hitRate := cache.Stats()
	b.ReportMetric(hitRate, "hit_rate")
	b.Logf("Cache hits: %d, hit rate: %.2f%%", hits, hitRate*100)
}

// BenchmarkDirectVsCached compares direct backend access vs cached access.
func BenchmarkDirectVsCached(b *testing.B) {
	// Create temp directory for file store
	dir, err := os.MkdirTemp("", "keystore-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fileStore, err := NewFileKeyStore(dir, "benchmark-password")
	if err != nil {
		b.Fatal(err)
	}

	cache := NewCachingKeyStore(fileStore, 100)
	b.Cleanup(func() {
		if err := cache.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Store a key through cache (populates both)
	key := EncryptedKey{
		Name:        "compare-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      make([]byte, 32),
		PrivKeyData: make([]byte, 64),
	}
	if err := cache.Store("compare-key", key); err != nil {
		b.Fatal(err)
	}

	b.Run("Direct_FileKeyStore", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = fileStore.Load("compare-key")
		}
	})

	b.Run("Cached_FileKeyStore", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = cache.Load("compare-key")
		}
	})
}
