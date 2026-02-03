package crypto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileKeyStore(t *testing.T) {
	t.Run("creates directory if not exists", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "subdir", "keys")

		ks, err := NewFileKeyStore(subdir, "password123")
		require.NoError(t, err)
		require.NotNil(t, ks)

		// Directory should exist
		info, err := os.Stat(subdir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("empty directory error", func(t *testing.T) {
		_, err := NewFileKeyStore("", "password123")
		assert.ErrorIs(t, err, ErrKeyStoreIO)
	})

	t.Run("empty password error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := NewFileKeyStore(dir, "")
		assert.ErrorIs(t, err, ErrKeyStoreIO)
	})
}

func TestFileKeyStore_StoreLoad(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	testKey := EncryptedKey{
		Name:        "alice",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key-bytes-32-bytes-long!!"),
		PrivKeyData: []byte("private-key-bytes-64-bytes-long-secret-material!!!!!!!!!!!!!!!!"),
	}

	t.Run("store and load roundtrip", func(t *testing.T) {
		err := ks.Store("alice", testKey)
		require.NoError(t, err)

		loaded, err := ks.Load("alice")
		require.NoError(t, err)

		assert.Equal(t, testKey.Name, loaded.Name)
		assert.Equal(t, testKey.Algorithm, loaded.Algorithm)
		assert.Equal(t, testKey.PubKey, loaded.PubKey)
		assert.Equal(t, testKey.PrivKeyData, loaded.PrivKeyData)

		// Salt and nonce should be populated (from encryption)
		assert.NotEmpty(t, loaded.Salt)
		assert.NotEmpty(t, loaded.Nonce)
	})

	t.Run("file has correct permissions", func(t *testing.T) {
		filePath := filepath.Join(dir, "alice.key")
		info, err := os.Stat(filePath)
		require.NoError(t, err)

		// Should be 0600 (owner read/write only)
		perm := info.Mode().Perm()
		assert.Equal(t, os.FileMode(0600), perm, "expected 0600, got %o", perm)
	})

	t.Run("store duplicate key error", func(t *testing.T) {
		err := ks.Store("alice", testKey)
		assert.ErrorIs(t, err, ErrKeyStoreExists)
	})

	t.Run("load non-existent key error", func(t *testing.T) {
		_, err := ks.Load("nonexistent")
		assert.ErrorIs(t, err, ErrKeyStoreNotFound)
	})
}

func TestFileKeyStore_WrongPassword(t *testing.T) {
	dir := t.TempDir()

	// Store with one password
	ks1, err := NewFileKeyStore(dir, "correct-password")
	require.NoError(t, err)

	testKey := EncryptedKey{
		Name:        "bob",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key-bytes-32-bytes-long!!"),
		PrivKeyData: []byte("secret-private-key-data-that-should-be-encrypted"),
	}

	err = ks1.Store("bob", testKey)
	require.NoError(t, err)

	// Try to load with wrong password
	ks2, err := NewFileKeyStore(dir, "wrong-password")
	require.NoError(t, err)

	_, err = ks2.Load("bob")
	assert.ErrorIs(t, err, ErrInvalidPassword)
}

func TestFileKeyStore_Delete(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	testKey := EncryptedKey{
		Name:        "charlie",
		Algorithm:   AlgorithmSecp256k1,
		PubKey:      []byte("public-key-bytes-33-bytes-long!!!"),
		PrivKeyData: []byte("private-key-data"),
	}

	t.Run("delete existing key", func(t *testing.T) {
		err := ks.Store("charlie", testKey)
		require.NoError(t, err)

		err = ks.Delete("charlie")
		require.NoError(t, err)

		// Should no longer exist
		_, err = ks.Load("charlie")
		assert.ErrorIs(t, err, ErrKeyStoreNotFound)
	})

	t.Run("delete non-existent key error", func(t *testing.T) {
		err := ks.Delete("nonexistent")
		assert.ErrorIs(t, err, ErrKeyStoreNotFound)
	})
}

func TestFileKeyStore_List(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	t.Run("empty list", func(t *testing.T) {
		names, err := ks.List()
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("list multiple keys", func(t *testing.T) {
		keys := []EncryptedKey{
			{Name: "key1", Algorithm: AlgorithmEd25519, PubKey: []byte("pub1"), PrivKeyData: []byte("priv1")},
			{Name: "key2", Algorithm: AlgorithmEd25519, PubKey: []byte("pub2"), PrivKeyData: []byte("priv2")},
			{Name: "key3", Algorithm: AlgorithmEd25519, PubKey: []byte("pub3"), PrivKeyData: []byte("priv3")},
		}

		for _, k := range keys {
			err := ks.Store(k.Name, k)
			require.NoError(t, err)
		}

		names, err := ks.List()
		require.NoError(t, err)
		assert.Len(t, names, 3)
		assert.Contains(t, names, "key1")
		assert.Contains(t, names, "key2")
		assert.Contains(t, names, "key3")
	})

	t.Run("ignores non-key files", func(t *testing.T) {
		// Create a non-.key file
		err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0644)
		require.NoError(t, err)

		names, err := ks.List()
		require.NoError(t, err)
		assert.NotContains(t, names, "notes")
		assert.NotContains(t, names, "notes.txt")
	})
}

func TestFileKeyStore_KeyNameValidation(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	testKey := EncryptedKey{
		Name:        "test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}

	tests := []struct {
		name        string
		keyName     string
		shouldError bool
		errorMsg    string
	}{
		{"valid name", "valid-key-name", false, ""},
		{"valid with numbers", "key123", false, ""},
		{"empty name", "", true, "cannot be empty"},
		{"path traversal forward slash", "../../etc/passwd", true, "path separators"},
		{"path traversal backslash", "..\\..\\etc\\passwd", true, "path separators"},
		{"parent directory", "..", true, "'..'"},
		{"hidden file", ".hidden", true, "start with '.'"},
		{"forward slash only", "foo/bar", true, "path separators"},
		{"backslash only", "foo\\bar", true, "path separators"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ks.Store(tt.keyName, testKey)
			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// Clean up successful stores
				if err == nil {
					_ = ks.Delete(tt.keyName)
				}
			}
		})
	}
}

func TestFileKeyStore_UniqueSaltAndNonce(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	// Store two keys with identical content
	key1 := EncryptedKey{
		Name:        "key1",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("identical-public-key-bytes-32!!!"),
		PrivKeyData: []byte("identical-private-key-bytes-that-are-exactly-the-same"),
	}
	key2 := EncryptedKey{
		Name:        "key2",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("identical-public-key-bytes-32!!!"),
		PrivKeyData: []byte("identical-private-key-bytes-that-are-exactly-the-same"),
	}

	err = ks.Store("key1", key1)
	require.NoError(t, err)

	err = ks.Store("key2", key2)
	require.NoError(t, err)

	// Load both and verify salt/nonce are different
	loaded1, err := ks.Load("key1")
	require.NoError(t, err)

	loaded2, err := ks.Load("key2")
	require.NoError(t, err)

	assert.NotEqual(t, loaded1.Salt, loaded2.Salt, "salts should be unique per key")
	assert.NotEqual(t, loaded1.Nonce, loaded2.Nonce, "nonces should be unique per key")
}

func TestFileKeyStore_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	// Store initial key
	initialKey := EncryptedKey{
		Name:        "concurrent",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	err = ks.Store("concurrent", initialKey)
	require.NoError(t, err)

	// Run concurrent loads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, err := ks.Load("concurrent")
				assert.NoError(t, err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestFileKeyStore_ConcurrentStoreLoad tests concurrent Store and Load operations.
// This validates thread safety when multiple goroutines are simultaneously
// writing new keys and reading existing keys.
func TestFileKeyStore_ConcurrentStoreLoad(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	const numKeys = 20
	const numReaders = 5
	const readsPerReader = 50

	// Pre-store some keys that readers will access
	for i := 0; i < numKeys/2; i++ {
		key := EncryptedKey{
			Name:        fmt.Sprintf("preload-%d", i),
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte(fmt.Sprintf("pub-preload-%d", i)),
			PrivKeyData: []byte(fmt.Sprintf("priv-preload-%d", i)),
		}
		err := ks.Store(key.Name, key)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, numKeys+numReaders*readsPerReader)

	// Writers: store new keys concurrently
	for i := numKeys / 2; i < numKeys; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := EncryptedKey{
				Name:        fmt.Sprintf("concurrent-%d", idx),
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte(fmt.Sprintf("pub-%d", idx)),
				PrivKeyData: []byte(fmt.Sprintf("priv-%d", idx)),
			}
			if err := ks.Store(key.Name, key); err != nil {
				errors <- fmt.Errorf("store key %d failed: %w", idx, err)
			}
		}(i)
	}

	// Readers: load pre-existing keys concurrently with the writes
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < readsPerReader; j++ {
				keyIdx := j % (numKeys / 2) // Only read preloaded keys
				keyName := fmt.Sprintf("preload-%d", keyIdx)
				loaded, err := ks.Load(keyName)
				if err != nil {
					errors <- fmt.Errorf("reader %d load %s failed: %w", readerID, keyName, err)
					continue
				}
				// Verify the loaded data is correct
				expectedPub := fmt.Sprintf("pub-preload-%d", keyIdx)
				if string(loaded.PubKey) != expectedPub {
					errors <- fmt.Errorf("reader %d: key %s has wrong pubkey: got %s, want %s",
						readerID, keyName, loaded.PubKey, expectedPub)
				}
			}
		}(r)
	}

	wg.Wait()
	close(errors)

	// Collect and report all errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	require.Empty(t, errList, "concurrent store+load errors: %v", errList)

	// Verify all keys were stored correctly
	names, err := ks.List()
	require.NoError(t, err)
	assert.Len(t, names, numKeys)
}

// TestFileKeyStore_ConcurrentStoreDelete tests concurrent Store and Delete operations.
// This validates thread safety when keys are being added and removed simultaneously.
func TestFileKeyStore_ConcurrentStoreDelete(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	const numIterations = 50
	const numDeleters = 3

	// Pre-store keys that will be deleted
	keysToDelete := make([]string, numIterations)
	for i := 0; i < numIterations; i++ {
		keyName := fmt.Sprintf("delete-me-%d", i)
		keysToDelete[i] = keyName
		key := EncryptedKey{
			Name:        keyName,
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte(fmt.Sprintf("pub-delete-%d", i)),
			PrivKeyData: []byte(fmt.Sprintf("priv-delete-%d", i)),
		}
		err := ks.Store(keyName, key)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, numIterations*2)
	deleteIdx := int32(0)

	// Writers: store new keys concurrently
	for i := 0; i < numIterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := EncryptedKey{
				Name:        fmt.Sprintf("new-key-%d", idx),
				Algorithm:   AlgorithmSecp256k1,
				PubKey:      []byte(fmt.Sprintf("pub-new-%d", idx)),
				PrivKeyData: []byte(fmt.Sprintf("priv-new-%d", idx)),
			}
			if err := ks.Store(key.Name, key); err != nil {
				errors <- fmt.Errorf("store new key %d failed: %w", idx, err)
			}
		}(i)
	}

	// Deleters: delete pre-existing keys concurrently with the writes
	for d := 0; d < numDeleters; d++ {
		wg.Add(1)
		go func(deleterID int) {
			defer wg.Done()
			for {
				idx := int(atomic.AddInt32(&deleteIdx, 1) - 1)
				if idx >= numIterations {
					return
				}
				keyName := keysToDelete[idx]
				err := ks.Delete(keyName)
				if err != nil && err != ErrKeyStoreNotFound {
					// ErrKeyStoreNotFound is acceptable if another deleter got there first
					errors <- fmt.Errorf("deleter %d delete %s failed: %w", deleterID, keyName, err)
				}
			}
		}(d)
	}

	wg.Wait()
	close(errors)

	// Collect and report all errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	require.Empty(t, errList, "concurrent store+delete errors: %v", errList)

	// Verify: all new keys exist, all deleted keys are gone
	names, err := ks.List()
	require.NoError(t, err)

	for _, name := range names {
		assert.True(t, strings.HasPrefix(name, "new-key-"),
			"unexpected key remaining: %s", name)
	}
	assert.Len(t, names, numIterations, "expected only new keys to remain")
}

// TestFileKeyStore_ConcurrentDeleteList tests concurrent Delete and List operations.
// This validates thread safety when listing keys while deletions are happening.
func TestFileKeyStore_ConcurrentDeleteList(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	const numKeys = 30
	const numListers = 5
	const listsPerLister = 20

	// Pre-store keys
	for i := 0; i < numKeys; i++ {
		key := EncryptedKey{
			Name:        fmt.Sprintf("list-key-%d", i),
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte(fmt.Sprintf("pub-%d", i)),
			PrivKeyData: []byte(fmt.Sprintf("priv-%d", i)),
		}
		err := ks.Store(key.Name, key)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, numKeys+numListers*listsPerLister)
	deleteIdx := int32(0)

	// Deleters: delete keys one by one
	for d := 0; d < 3; d++ {
		wg.Add(1)
		go func(deleterID int) {
			defer wg.Done()
			for {
				idx := int(atomic.AddInt32(&deleteIdx, 1) - 1)
				if idx >= numKeys {
					return
				}
				keyName := fmt.Sprintf("list-key-%d", idx)
				err := ks.Delete(keyName)
				if err != nil && err != ErrKeyStoreNotFound {
					errors <- fmt.Errorf("deleter %d delete %s failed: %w", deleterID, keyName, err)
				}
			}
		}(d)
	}

	// Listers: list keys concurrently with deletions
	for l := 0; l < numListers; l++ {
		wg.Add(1)
		go func(listerID int) {
			defer wg.Done()
			for j := 0; j < listsPerLister; j++ {
				names, err := ks.List()
				if err != nil {
					errors <- fmt.Errorf("lister %d iteration %d failed: %w", listerID, j, err)
					continue
				}
				// List should return a consistent snapshot (no duplicates, valid names)
				seen := make(map[string]bool)
				for _, name := range names {
					if seen[name] {
						errors <- fmt.Errorf("lister %d: duplicate key in list: %s", listerID, name)
					}
					seen[name] = true
					if !strings.HasPrefix(name, "list-key-") {
						errors <- fmt.Errorf("lister %d: unexpected key name: %s", listerID, name)
					}
				}
			}
		}(l)
	}

	wg.Wait()
	close(errors)

	// Collect and report all errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	require.Empty(t, errList, "concurrent delete+list errors: %v", errList)

	// Final state: all keys should be deleted
	names, err := ks.List()
	require.NoError(t, err)
	assert.Empty(t, names, "all keys should have been deleted")
}

// TestFileKeyStore_ConcurrentMixedOperations performs a stress test with all
// operations (Store, Load, Delete, List) running concurrently.
// This is the most adversarial test for thread safety.
func TestFileKeyStore_ConcurrentMixedOperations(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	const numOps = 100
	const numWorkers = 10

	// Pre-store some keys for operations to work with
	for i := 0; i < 10; i++ {
		key := EncryptedKey{
			Name:        fmt.Sprintf("mixed-init-%d", i),
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte(fmt.Sprintf("pub-%d", i)),
			PrivKeyData: []byte(fmt.Sprintf("priv-%d", i)),
		}
		err := ks.Store(key.Name, key)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	storeCount := int32(0)
	errors := make(chan error, numWorkers*numOps)

	// Mixed operation workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < numOps; i++ {
				op := i % 4
				switch op {
				case 0: // Store
					idx := atomic.AddInt32(&storeCount, 1)
					key := EncryptedKey{
						Name:        fmt.Sprintf("mixed-new-%d", idx),
						Algorithm:   AlgorithmEd25519,
						PubKey:      []byte(fmt.Sprintf("pub-new-%d", idx)),
						PrivKeyData: []byte(fmt.Sprintf("priv-new-%d", idx)),
					}
					if err := ks.Store(key.Name, key); err != nil && err != ErrKeyStoreExists {
						errors <- fmt.Errorf("worker %d store failed: %w", workerID, err)
					}
				case 1: // Load (initial keys)
					keyIdx := i % 10
					keyName := fmt.Sprintf("mixed-init-%d", keyIdx)
					if _, err := ks.Load(keyName); err != nil && err != ErrKeyStoreNotFound {
						errors <- fmt.Errorf("worker %d load %s failed: %w", workerID, keyName, err)
					}
				case 2: // Delete (may race with other operations)
					keyIdx := i % 10
					keyName := fmt.Sprintf("mixed-init-%d", keyIdx)
					if err := ks.Delete(keyName); err != nil && err != ErrKeyStoreNotFound {
						// ErrKeyStoreNotFound is acceptable - another worker may have deleted it
						errors <- fmt.Errorf("worker %d delete %s failed: %w", workerID, keyName, err)
					}
				case 3: // List
					if _, err := ks.List(); err != nil {
						errors <- fmt.Errorf("worker %d list failed: %w", workerID, err)
					}
				}
			}
		}(w)
	}

	wg.Wait()
	close(errors)

	// Collect and report all errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	require.Empty(t, errList, "concurrent mixed operations errors: %v", errList)
}

func TestFileKeyStore_AllAlgorithms(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	algorithms := []Algorithm{
		AlgorithmEd25519,
		AlgorithmSecp256k1,
		AlgorithmSecp256r1,
	}

	for _, alg := range algorithms {
		t.Run(string(alg), func(t *testing.T) {
			key := EncryptedKey{
				Name:        "test-" + string(alg),
				Algorithm:   alg,
				PubKey:      []byte("public-key-" + string(alg)),
				PrivKeyData: []byte("private-key-" + string(alg)),
			}

			err := ks.Store(key.Name, key)
			require.NoError(t, err)

			loaded, err := ks.Load(key.Name)
			require.NoError(t, err)

			assert.Equal(t, alg, loaded.Algorithm)
		})
	}
}

func TestFileKeyStore_LargeKey(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	// Test with a reasonably large private key data (could be aggregate signature material)
	largePrivKey := make([]byte, 64*1024) // 64KB
	for i := range largePrivKey {
		largePrivKey[i] = byte(i % 256)
	}

	key := EncryptedKey{
		Name:        "large-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: largePrivKey,
	}

	err = ks.Store("large-key", key)
	require.NoError(t, err)

	loaded, err := ks.Load("large-key")
	require.NoError(t, err)

	assert.Equal(t, largePrivKey, loaded.PrivKeyData)
}

func TestEncryptionAEAD_AdditionalData(t *testing.T) {
	// This tests that the key name is bound to the ciphertext as additional data.
	// If someone renames a key file, decryption should fail.
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	key := EncryptedKey{
		Name:        "original",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("secret-data"),
	}

	err = ks.Store("original", key)
	require.NoError(t, err)

	// Manually rename the file (simulating tampering)
	oldPath := filepath.Join(dir, "original.key")
	newPath := filepath.Join(dir, "renamed.key")
	err = os.Rename(oldPath, newPath)
	require.NoError(t, err)

	// Loading with the new name should fail (because AAD includes the name)
	_, err = ks.Load("renamed")
	assert.ErrorIs(t, err, ErrInvalidPassword, "renamed key should fail AAD check")
}

// TestFileKeyStore_Close tests the Close method and ErrKeyStoreClosed behavior.
// Verifies consistent error handling with MemoryStore pattern.
func TestFileKeyStore_Close(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	// Store a key before closing
	testKey := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key-bytes-32-bytes-long!!"),
		PrivKeyData: []byte("private-key-bytes-64-bytes-long-secret-material!!!!!!!!!!!!!!!!"),
	}
	err = ks.Store("test-key", testKey)
	require.NoError(t, err)

	// Close should succeed
	fks := ks.(*FileKeyStore)
	err = fks.Close()
	require.NoError(t, err)

	// All operations should return ErrKeyStoreClosed
	t.Run("Store after close", func(t *testing.T) {
		err := fks.Store("new-key", testKey)
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})

	t.Run("Load after close", func(t *testing.T) {
		_, err := fks.Load("test-key")
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})

	t.Run("Delete after close", func(t *testing.T) {
		err := fks.Delete("test-key")
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})

	t.Run("List after close", func(t *testing.T) {
		_, err := fks.List()
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})
}

// TestFileKeyStore_CloseIdempotent verifies Close can be called multiple times.
func TestFileKeyStore_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	fks := ks.(*FileKeyStore)

	// First close
	err = fks.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = fks.Close()
	require.NoError(t, err)

	// Third close should also be no-op
	err = fks.Close()
	require.NoError(t, err)
}

// TestFileKeyStore_CloseConsistentWithMemoryStore documents consistent behavior
// between FileKeyStore and the expected MemoryStore pattern.
func TestFileKeyStore_CloseConsistentWithMemoryStore(t *testing.T) {
	// This test verifies FileKeyStore returns ErrKeyStoreClosed after Close(),
	// making its behavior consistent with the expected pattern for all stores.
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	fks := ks.(*FileKeyStore)
	err = fks.Close()
	require.NoError(t, err)

	// Verify the error message is clear and consistent
	_, err = fks.Load("any-key")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)
	assert.Contains(t, err.Error(), "closed")
}

// TestFileKeyStore_ConcurrentClose tests thread-safety of Close with concurrent operations.
func TestFileKeyStore_ConcurrentClose(t *testing.T) {
	dir := t.TempDir()
	ks, err := NewFileKeyStore(dir, "test-password")
	require.NoError(t, err)

	fks := ks.(*FileKeyStore)

	// Pre-store some keys
	for i := 0; i < 10; i++ {
		key := EncryptedKey{
			Name:        fmt.Sprintf("key-%d", i),
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte(fmt.Sprintf("pub-%d", i)),
			PrivKeyData: []byte(fmt.Sprintf("priv-%d", i)),
		}
		err := fks.Store(key.Name, key)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Start concurrent operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := fks.Load(fmt.Sprintf("key-%d", id%10))
				// Either success or ErrKeyStoreClosed is acceptable
				if err != nil && err != ErrKeyStoreClosed && err != ErrKeyStoreNotFound {
					errors <- fmt.Errorf("unexpected error: %w", err)
				}
			}
		}(i)
	}

	// Close concurrently with operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		fks.Close()
	}()

	wg.Wait()
	close(errors)

	// Check for unexpected errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	require.Empty(t, errList, "unexpected errors during concurrent close: %v", errList)

	// Verify store is definitely closed now
	_, err = fks.Load("key-0")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)
}
