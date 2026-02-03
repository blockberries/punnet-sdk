package crypto

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachingKeyStore_BasicOperations tests store/load/delete with caching.
func TestCachingKeyStore_BasicOperations(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	key := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key-bytes"),
		PrivKeyData: []byte("private-key-data"),
	}

	// Store should succeed
	err := cache.Store("test-key", key)
	require.NoError(t, err)

	// Load should return cached value
	loaded, err := cache.Load("test-key")
	require.NoError(t, err)
	assert.Equal(t, key.Name, loaded.Name)
	assert.Equal(t, key.Algorithm, loaded.Algorithm)
	assert.Equal(t, key.PubKey, loaded.PubKey)
	assert.Equal(t, key.PrivKeyData, loaded.PrivKeyData)

	// Delete should work
	err = cache.Delete("test-key")
	require.NoError(t, err)

	// Load after delete should fail
	_, err = cache.Load("test-key")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)
}

// TestCachingKeyStore_CacheHit verifies cache hits don't touch backend.
func TestCachingKeyStore_CacheHit(t *testing.T) {
	backend := newCountingKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	key := EncryptedKey{
		Name:        "cached-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}

	// Store key (1 backend store)
	require.NoError(t, cache.Store("cached-key", key))
	assert.Equal(t, int64(1), backend.storeCount.Load())
	assert.Equal(t, int64(0), backend.loadCount.Load())

	// First load populates cache (0 backend loads - already cached from Store)
	_, err := cache.Load("cached-key")
	require.NoError(t, err)
	assert.Equal(t, int64(0), backend.loadCount.Load()) // Cache hit!

	// Subsequent loads should hit cache
	for i := 0; i < 100; i++ {
		_, err := cache.Load("cached-key")
		require.NoError(t, err)
	}
	assert.Equal(t, int64(0), backend.loadCount.Load()) // Still no backend loads

	// Verify stats
	hits, misses, hitRate := cache.Stats()
	assert.Equal(t, uint64(101), hits) // 101 cache hits
	assert.Equal(t, uint64(0), misses)
	assert.Equal(t, 1.0, hitRate)
}

// TestCachingKeyStore_CacheMiss verifies cache misses load from backend.
func TestCachingKeyStore_CacheMiss(t *testing.T) {
	backend := newCountingKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Store directly in backend (bypassing cache)
	key := EncryptedKey{
		Name:        "backend-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	require.NoError(t, backend.Store("backend-key", key))

	// First load should miss cache and hit backend
	loaded, err := cache.Load("backend-key")
	require.NoError(t, err)
	assert.Equal(t, key.Name, loaded.Name)
	assert.Equal(t, int64(1), backend.loadCount.Load())

	// Second load should hit cache
	_, err = cache.Load("backend-key")
	require.NoError(t, err)
	assert.Equal(t, int64(1), backend.loadCount.Load()) // No additional backend load

	// Verify stats
	hits, misses, _ := cache.Stats()
	assert.Equal(t, uint64(1), hits)
	assert.Equal(t, uint64(1), misses)
}

// TestCachingKeyStore_LRUEviction verifies LRU eviction behavior.
func TestCachingKeyStore_LRUEviction(t *testing.T) {
	backend := newCountingKeyStore()
	cache := NewCachingKeyStore(backend, 3) // Small capacity for testing
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Store 3 keys (fills cache)
	for i := 0; i < 3; i++ {
		name := string(rune('a' + i)) // "a", "b", "c"
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, cache.Store(name, key))
	}
	assert.Equal(t, 3, cache.Len())

	// Access "a" to make it most recently used
	_, err := cache.Load("a")
	require.NoError(t, err)

	// Store 4th key - should evict "b" (least recently used after "a" access)
	key := EncryptedKey{
		Name:        "d",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	require.NoError(t, cache.Store("d", key))
	assert.Equal(t, 3, cache.Len())

	// "a" should still be cached (was accessed)
	backend.loadCount.Store(0)
	_, err = cache.Load("a")
	require.NoError(t, err)
	assert.Equal(t, int64(0), backend.loadCount.Load()) // Cache hit

	// "b" should have been evicted
	_, err = cache.Load("b")
	require.NoError(t, err)
	assert.Equal(t, int64(1), backend.loadCount.Load()) // Cache miss, backend load
}

// TestCachingKeyStore_WriteThrough verifies writes go to backend.
func TestCachingKeyStore_WriteThrough(t *testing.T) {
	backend := newCountingKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	key := EncryptedKey{
		Name:        "write-test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}

	require.NoError(t, cache.Store("write-test", key))

	// Verify backend received the write
	assert.Equal(t, int64(1), backend.storeCount.Load())

	// Verify backend has the key
	loaded, err := backend.Load("write-test")
	require.NoError(t, err)
	assert.Equal(t, key.Name, loaded.Name)
}

// TestCachingKeyStore_DeleteInvalidatesCache verifies delete removes from cache.
func TestCachingKeyStore_DeleteInvalidatesCache(t *testing.T) {
	backend := newCountingKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	key := EncryptedKey{
		Name:        "delete-test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}

	require.NoError(t, cache.Store("delete-test", key))
	assert.Equal(t, 1, cache.Len())

	require.NoError(t, cache.Delete("delete-test"))
	assert.Equal(t, 0, cache.Len())

	// Verify backend delete was called
	assert.Equal(t, int64(1), backend.deleteCount.Load())
}

// TestCachingKeyStore_Invalidate verifies manual cache invalidation.
func TestCachingKeyStore_Invalidate(t *testing.T) {
	backend := newCountingKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	key := EncryptedKey{
		Name:        "invalidate-test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}

	require.NoError(t, cache.Store("invalidate-test", key))
	assert.Equal(t, 1, cache.Len())

	// Invalidate without touching backend
	cache.Invalidate("invalidate-test")
	assert.Equal(t, 0, cache.Len())
	assert.Equal(t, int64(0), backend.deleteCount.Load()) // Backend not touched

	// Load should now hit backend
	_, err := cache.Load("invalidate-test")
	require.NoError(t, err)
	assert.Equal(t, int64(1), backend.loadCount.Load())
}

// TestCachingKeyStore_InvalidateAll verifies bulk cache invalidation.
func TestCachingKeyStore_InvalidateAll(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Store several keys
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, cache.Store(name, key))
	}
	assert.Equal(t, 5, cache.Len())

	cache.InvalidateAll()
	assert.Equal(t, 0, cache.Len())

	// Stats should be preserved
	hits, _, _ := cache.Stats()
	assert.Equal(t, uint64(0), hits) // No loads yet
}

// TestCachingKeyStore_List verifies List delegates to backend.
func TestCachingKeyStore_List(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Store keys through cache
	for _, name := range []string{"key1", "key2", "key3"} {
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, cache.Store(name, key))
	}

	// Store key directly in backend (not in cache)
	directKey := EncryptedKey{
		Name:        "direct-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	require.NoError(t, backend.Store("direct-key", directKey))

	// List should return all keys from backend
	names, err := cache.List()
	require.NoError(t, err)
	assert.Len(t, names, 4)
	assert.ElementsMatch(t, []string{"key1", "key2", "key3", "direct-key"}, names)
}

// TestCachingKeyStore_Close verifies Close behavior.
func TestCachingKeyStore_Close(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10)

	key := EncryptedKey{
		Name:        "close-test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	require.NoError(t, cache.Store("close-test", key))

	// Close should succeed
	require.NoError(t, cache.Close())

	// All operations should fail after close
	_, err := cache.Load("close-test")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	err = cache.Store("new-key", key)
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	err = cache.Delete("close-test")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	_, err = cache.List()
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	// Second close should be no-op
	require.NoError(t, cache.Close())
}

// TestCachingKeyStore_ConcurrentAccess verifies thread safety.
func TestCachingKeyStore_ConcurrentAccess(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 100)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate with some keys
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, cache.Store(name, key))
	}

	var wg sync.WaitGroup
	const goroutines = 50
	const opsPerGoroutine = 100

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				name := string(rune('a' + (id+j)%10))
				_, _ = cache.Load(name)
			}
		}(i)
	}

	// Concurrent writes (to different keys)
	for i := 0; i < goroutines/5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine/10; j++ {
				name := string(rune('A' + id))
				key := EncryptedKey{
					Name:        name,
					Algorithm:   AlgorithmEd25519,
					PubKey:      []byte("pub"),
					PrivKeyData: []byte("priv"),
				}
				_ = cache.Store(name, key)
			}
		}(i)
	}

	wg.Wait()

	// Cache should be in consistent state
	hits, misses, _ := cache.Stats()
	t.Logf("Cache stats: hits=%d, misses=%d, size=%d", hits, misses, cache.Len())
}

// TestCachingKeyStore_ZeroCapacity verifies default capacity behavior.
func TestCachingKeyStore_ZeroCapacity(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 0) // Should default to 100
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	assert.Equal(t, 100, cache.Capacity())
}

// TestCachingKeyStore_KeyIsolation verifies cached keys are isolated from mutations.
func TestCachingKeyStore_KeyIsolation(t *testing.T) {
	backend := newMockKeyStore()
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	original := EncryptedKey{
		Name:        "isolation-test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("original-pub"),
		PrivKeyData: []byte("original-priv"),
	}
	require.NoError(t, cache.Store("isolation-test", original))

	// Load and mutate
	loaded1, err := cache.Load("isolation-test")
	require.NoError(t, err)
	loaded1.PubKey[0] = 'X' // Mutate

	// Load again - should have original value
	loaded2, err := cache.Load("isolation-test")
	require.NoError(t, err)
	assert.Equal(t, []byte("original-pub"), loaded2.PubKey)
}

// TestCachingKeyStore_BackendError verifies error propagation.
func TestCachingKeyStore_BackendError(t *testing.T) {
	backend := &failingKeyStore{
		storeErr: errors.New("store failed"),
		loadErr:  errors.New("load failed"),
	}
	cache := NewCachingKeyStore(backend, 10)
	t.Cleanup(func() {
		_ = cache.Close()
	})

	key := EncryptedKey{
		Name:        "error-test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}

	// Store error should propagate
	err := cache.Store("error-test", key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store failed")

	// Load error should propagate
	_, err = cache.Load("missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load failed")
}

// countingKeyStore wraps mockKeyStore with operation counters.
type countingKeyStore struct {
	*mockKeyStore
	storeCount  atomic.Int64
	loadCount   atomic.Int64
	deleteCount atomic.Int64
}

func newCountingKeyStore() *countingKeyStore {
	return &countingKeyStore{
		mockKeyStore: newMockKeyStore(),
	}
}

func (c *countingKeyStore) Store(name string, key EncryptedKey) error {
	c.storeCount.Add(1)
	return c.mockKeyStore.Store(name, key)
}

func (c *countingKeyStore) Load(name string) (EncryptedKey, error) {
	c.loadCount.Add(1)
	return c.mockKeyStore.Load(name)
}

func (c *countingKeyStore) Delete(name string) error {
	c.deleteCount.Add(1)
	return c.mockKeyStore.Delete(name)
}

// failingKeyStore returns errors for all operations.
type failingKeyStore struct {
	storeErr  error
	loadErr   error
	deleteErr error
	listErr   error
}

func (f *failingKeyStore) Store(name string, key EncryptedKey) error {
	if f.storeErr != nil {
		return f.storeErr
	}
	return nil
}

func (f *failingKeyStore) Load(name string) (EncryptedKey, error) {
	if f.loadErr != nil {
		return EncryptedKey{}, f.loadErr
	}
	return EncryptedKey{}, nil
}

func (f *failingKeyStore) Delete(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return nil
}

func (f *failingKeyStore) List() ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return nil, nil
}
