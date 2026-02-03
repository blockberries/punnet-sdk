package crypto

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryKeyStore_NewMemoryKeyStore(t *testing.T) {
	store := NewMemoryKeyStore()
	require.NotNil(t, store)
	assert.Equal(t, 0, store.Len())

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

func TestMemoryKeyStore_NewMemoryKeyStoreWithCapacity(t *testing.T) {
	store := NewMemoryKeyStoreWithCapacity(100)
	require.NotNil(t, store)
	assert.Equal(t, 0, store.Len())

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

func TestMemoryKeyStore_Store(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	key := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key-bytes"),
		PrivKeyData: []byte("private-key-data"),
	}

	// Store should succeed on first call
	err := store.Store("test-key", key)
	require.NoError(t, err)
	assert.Equal(t, 1, store.Len())

	// Store should return ErrKeyStoreExists on duplicate
	err = store.Store("test-key", key)
	assert.ErrorIs(t, err, ErrKeyStoreExists)
}

func TestMemoryKeyStore_StoreValidation(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	t.Run("rejects empty key name", func(t *testing.T) {
		key := EncryptedKey{
			Name:      "",
			Algorithm: AlgorithmEd25519,
		}
		err := store.Store("", key)
		assert.ErrorIs(t, err, ErrInvalidKeyName)
	})

	t.Run("rejects path traversal in name", func(t *testing.T) {
		key := EncryptedKey{
			Name:      "../../etc/passwd",
			Algorithm: AlgorithmEd25519,
		}
		err := store.Store("../../etc/passwd", key)
		assert.ErrorIs(t, err, ErrInvalidKeyName)
	})

	t.Run("rejects name mismatch", func(t *testing.T) {
		key := EncryptedKey{
			Name:      "alice",
			Algorithm: AlgorithmEd25519,
		}
		err := store.Store("bob", key)
		assert.ErrorIs(t, err, ErrKeyNameMismatch)
	})

	t.Run("rejects invalid algorithm", func(t *testing.T) {
		key := EncryptedKey{
			Name:      "test",
			Algorithm: Algorithm("unknown"),
		}
		err := store.Store("test", key)
		assert.ErrorIs(t, err, ErrInvalidAlgorithm)
	})

	t.Run("rejects invalid encryption params", func(t *testing.T) {
		key := EncryptedKey{
			Name:      "test",
			Algorithm: AlgorithmEd25519,
			Salt:      []byte("short"), // Too short
			Nonce:     make([]byte, AESGCMNonceLength),
		}
		err := store.Store("test", key)
		assert.ErrorIs(t, err, ErrInvalidEncryptionParams)
	})
}

func TestMemoryKeyStore_Load(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Load non-existent key should return ErrKeyStoreNotFound
	_, err := store.Load("missing")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)

	// Store a key with valid encryption params
	original := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmSecp256k1,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
		Salt:        make([]byte, MinSaltLength),     // 16 bytes
		Nonce:       make([]byte, AESGCMNonceLength), // 12 bytes
	}
	require.NoError(t, store.Store("test-key", original))

	// Load should return the stored key
	loaded, err := store.Load("test-key")
	require.NoError(t, err)
	assert.Equal(t, original.Name, loaded.Name)
	assert.Equal(t, original.Algorithm, loaded.Algorithm)
	assert.Equal(t, original.PubKey, loaded.PubKey)
	assert.Equal(t, original.PrivKeyData, loaded.PrivKeyData)
	assert.Equal(t, original.Salt, loaded.Salt)
	assert.Equal(t, original.Nonce, loaded.Nonce)
}

func TestMemoryKeyStore_LoadReturnsCopy(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	original := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public"),
		PrivKeyData: []byte("private"),
	}
	require.NoError(t, store.Store("test-key", original))

	// Load and modify the returned key
	loaded, err := store.Load("test-key")
	require.NoError(t, err)
	loaded.PubKey[0] = 'X'
	loaded.PrivKeyData[0] = 'X'

	// Load again - original should be unchanged
	reloaded, err := store.Load("test-key")
	require.NoError(t, err)
	assert.Equal(t, byte('p'), reloaded.PubKey[0], "stored key should not be mutated")
	assert.Equal(t, byte('p'), reloaded.PrivKeyData[0], "stored key should not be mutated")
}

func TestMemoryKeyStore_Delete(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Delete non-existent key should return ErrKeyStoreNotFound
	err := store.Delete("missing")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)

	// Store then delete a key
	key := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	require.NoError(t, store.Store("test-key", key))
	assert.Equal(t, 1, store.Len())

	err = store.Delete("test-key")
	require.NoError(t, err)
	assert.Equal(t, 0, store.Len())

	// Should not be loadable after delete
	_, err = store.Load("test-key")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)
}

func TestMemoryKeyStore_List(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Empty store should return empty list
	names, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, names)

	// Store some keys
	for _, name := range []string{"key1", "key2", "key3"} {
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, store.Store(name, key))
	}

	// List should return all keys
	names, err = store.List()
	require.NoError(t, err)
	assert.Len(t, names, 3)
	assert.ElementsMatch(t, []string{"key1", "key2", "key3"}, names)
}

func TestMemoryKeyStore_Close(t *testing.T) {
	store := NewMemoryKeyStore()

	// Store a key
	key := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("secret-private-key"),
	}
	require.NoError(t, store.Store("test-key", key))

	// Close the store
	err := store.Close()
	require.NoError(t, err)

	// All operations should return ErrKeyStoreClosed
	_, err = store.Load("test-key")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	_, err = store.List()
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	err = store.Store("new-key", EncryptedKey{
		Name:      "new-key",
		Algorithm: AlgorithmEd25519,
	})
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	err = store.Delete("test-key")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)

	// Len should return 0 when closed
	assert.Equal(t, 0, store.Len())
}

func TestMemoryKeyStore_CloseIdempotent(t *testing.T) {
	store := NewMemoryKeyStore()

	// Multiple closes should not panic or error
	err := store.Close()
	require.NoError(t, err)

	err = store.Close()
	require.NoError(t, err)

	err = store.Close()
	require.NoError(t, err)
}

// TestMemoryKeyStore_ConcurrentAccess verifies thread safety when
// multiple goroutines access the same key.
func TestMemoryKeyStore_ConcurrentAccess(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	var wg sync.WaitGroup
	var successCount int32

	// Concurrent stores to same key - exactly one should succeed
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := EncryptedKey{
				Name:        "key",
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("priv"),
			}
			err := store.Store(key.Name, key)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	// Exactly one store should have succeeded
	assert.Equal(t, int32(1), successCount, "exactly one concurrent store should succeed")

	// Concurrent reads should all succeed
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Load("key")
			assert.NoError(t, err)
			_, err = store.List()
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// Only one key should exist
	names, err := store.List()
	require.NoError(t, err)
	assert.Len(t, names, 1)
}

// TestMemoryKeyStore_ConcurrentDifferentKeys verifies no false conflicts
// when accessing different keys concurrently.
func TestMemoryKeyStore_ConcurrentDifferentKeys(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	var wg sync.WaitGroup
	keyCount := 100

	// Concurrent stores to different keys - all should succeed
	var successCount int32
	for i := 0; i < keyCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "key-" + string(rune('a'+idx%26)) + string(rune('0'+idx/26))
			key := EncryptedKey{
				Name:        name,
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("priv"),
			}
			err := store.Store(name, key)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}
	wg.Wait()

	// All stores should have succeeded
	assert.Equal(t, int32(keyCount), successCount, "all concurrent stores to different keys should succeed")

	// Verify all keys exist
	names, err := store.List()
	require.NoError(t, err)
	assert.Len(t, names, keyCount)
}

// TestMemoryKeyStore_ConcurrentMixedOperations verifies thread safety
// with concurrent Store, Load, Delete, and List operations.
func TestMemoryKeyStore_ConcurrentMixedOperations(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate with some keys
	for i := 0; i < 10; i++ {
		name := "key-" + string(rune('0'+i))
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, store.Store(name, key))
	}

	var wg sync.WaitGroup

	// Run concurrent mixed operations
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			keyIdx := idx % 10
			name := "key-" + string(rune('0'+keyIdx))

			switch idx % 4 {
			case 0:
				// Load existing key
				_, _ = store.Load(name)
			case 1:
				// List all keys
				_, _ = store.List()
			case 2:
				// Delete and re-store (may race with other deleters)
				_ = store.Delete(name)
				key := EncryptedKey{
					Name:        name,
					Algorithm:   AlgorithmEd25519,
					PubKey:      []byte("pub"),
					PrivKeyData: []byte("priv"),
				}
				_ = store.Store(name, key)
			case 3:
				// Store new key with unique name (may already exist)
				newName := "new-" + string(rune('0'+idx))
				key := EncryptedKey{
					Name:        newName,
					Algorithm:   AlgorithmEd25519,
					PubKey:      []byte("pub"),
					PrivKeyData: []byte("priv"),
				}
				_ = store.Store(newName, key)
			}
		}(i)
	}
	wg.Wait()

	// Store should still be in consistent state (no panics, valid response)
	_, err := store.List()
	require.NoError(t, err)
}

// TestMemoryKeyStore_ConcurrentClose verifies safe concurrent close behavior.
func TestMemoryKeyStore_ConcurrentClose(t *testing.T) {
	store := NewMemoryKeyStore()

	// Pre-populate
	for i := 0; i < 5; i++ {
		name := "key-" + string(rune('0'+i))
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte("pub"),
			PrivKeyData: []byte("priv"),
		}
		require.NoError(t, store.Store(name, key))
	}

	var wg sync.WaitGroup

	// Concurrent operations during close
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			switch idx % 5 {
			case 0:
				_ = store.Close()
			case 1:
				_, _ = store.Load("key-0")
			case 2:
				_, _ = store.List()
			case 3:
				_ = store.Delete("key-1")
			case 4:
				key := EncryptedKey{
					Name:      "new",
					Algorithm: AlgorithmEd25519,
				}
				_ = store.Store("new", key)
			}
		}(i)
	}
	wg.Wait()

	// Store should be closed and all ops should return ErrKeyStoreClosed
	_, err := store.List()
	assert.ErrorIs(t, err, ErrKeyStoreClosed)
}

func TestMemoryKeyStore_StorePreventsMutation(t *testing.T) {
	store := NewMemoryKeyStore()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	original := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public"),
		PrivKeyData: []byte("private"),
	}

	require.NoError(t, store.Store("test-key", original))

	// Mutate the original after storing
	original.PubKey[0] = 'X'
	original.PrivKeyData[0] = 'X'

	// Load should return the original unchanged data
	loaded, err := store.Load("test-key")
	require.NoError(t, err)
	assert.Equal(t, byte('p'), loaded.PubKey[0], "stored key should not be mutated by external changes")
	assert.Equal(t, byte('p'), loaded.PrivKeyData[0], "stored key should not be mutated by external changes")
}

// BenchmarkMemoryKeyStore_Store measures Store performance.
func BenchmarkMemoryKeyStore_Store(b *testing.B) {
	store := NewMemoryKeyStoreWithCapacity(b.N)
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := "key-" + string(rune('a'+i%26)) + string(rune('0'+i/26%10))
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      make([]byte, 32),
			PrivKeyData: make([]byte, 64),
		}
		_ = store.Store(name, key)
	}
}

// BenchmarkMemoryKeyStore_Load measures Load performance.
func BenchmarkMemoryKeyStore_Load(b *testing.B) {
	store := NewMemoryKeyStore()
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate
	key := EncryptedKey{
		Name:        "benchmark-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      make([]byte, 32),
		PrivKeyData: make([]byte, 64),
	}
	_ = store.Store("benchmark-key", key)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load("benchmark-key")
	}
}

// BenchmarkMemoryKeyStore_List measures List performance.
func BenchmarkMemoryKeyStore_List(b *testing.B) {
	store := NewMemoryKeyStoreWithCapacity(100)
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	// Pre-populate with 100 keys
	for i := 0; i < 100; i++ {
		name := "key-" + string(rune('a'+i%26)) + string(rune('0'+i/26%10))
		key := EncryptedKey{
			Name:        name,
			Algorithm:   AlgorithmEd25519,
			PubKey:      make([]byte, 32),
			PrivKeyData: make([]byte, 64),
		}
		_ = store.Store(name, key)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.List()
	}
}
