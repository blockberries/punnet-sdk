package crypto

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// skipIfNoKeychain skips the test if the OS keychain is unavailable.
// This allows tests to run on CI environments without a keychain.
func skipIfNoKeychain(t *testing.T) {
	t.Helper()
	// Try a test operation to check keychain availability
	testService := "punnet-sdk-test-probe"
	testKey := "_availability_check"

	_, err := keyring.Get(testService, testKey)
	if err != nil && err != keyring.ErrNotFound {
		t.Skipf("keychain unavailable: %v", err)
	}
	// Clean up probe
	_ = keyring.Delete(testService, testKey)
}

// testServiceName generates a unique service name for test isolation.
// Uses nanosecond timestamp to ensure uniqueness across parallel runs
// and -count=N iterations.
func testServiceName(t *testing.T) string {
	return fmt.Sprintf("punnet-sdk-test-%s-%d", t.Name(), time.Now().UnixNano())
}

// cleanupKeychain removes all test keys from the keychain.
// It cleans up both BEFORE and AFTER the test to handle stale data
// from previous runs (especially when using -count=N).
func cleanupKeychain(t *testing.T, serviceName string) {
	t.Helper()

	// Helper to perform the actual cleanup
	doCleanup := func() {
		// Get list and delete all keys
		listStr, err := keyring.Get(serviceName, keychainListKey)
		if err == nil && listStr != "" {
			// Delete individual keys
			ks, _ := NewKeychainStore(serviceName)
			if ks != nil {
				names, _ := ks.List()
				for _, name := range names {
					_ = ks.Delete(name)
				}
			}
		}
		// Delete the list key itself
		_ = keyring.Delete(serviceName, keychainListKey)
	}

	// Clean BEFORE the test to handle leftover keys from previously failed runs
	doCleanup()

	// Also clean AFTER the test
	t.Cleanup(doCleanup)
}

// cleanupKeychainWithKnownKeys performs thorough cleanup by deleting both
// the indexed keys and specific known key patterns. This ensures cleanup
// succeeds even if the key list index is corrupt or incomplete (orphaned keys).
func cleanupKeychainWithKnownKeys(t *testing.T, serviceName string, knownKeyPatterns []string) {
	t.Helper()

	doCleanup := func() {
		// First, try cleaning via the index (standard cleanup)
		listStr, err := keyring.Get(serviceName, keychainListKey)
		if err == nil && listStr != "" {
			ks, _ := NewKeychainStore(serviceName)
			if ks != nil {
				names, _ := ks.List()
				for _, name := range names {
					_ = ks.Delete(name)
				}
			}
		}

		// Then, directly delete known key patterns that may be orphaned
		// (in keychain but not in the index)
		for _, keyName := range knownKeyPatterns {
			_ = keyring.Delete(serviceName, keychainKeyPrefix+keyName)
		}

		// Delete the list key itself
		_ = keyring.Delete(serviceName, keychainListKey)
	}

	// Clean BEFORE and AFTER
	doCleanup()
	t.Cleanup(doCleanup)
}

func TestNewKeychainStore(t *testing.T) {
	skipIfNoKeychain(t)

	t.Run("creates store with valid service name", func(t *testing.T) {
		serviceName := testServiceName(t)
		cleanupKeychain(t, serviceName)

		ks, err := NewKeychainStore(serviceName)
		require.NoError(t, err)
		require.NotNil(t, ks)
	})

	t.Run("empty service name error", func(t *testing.T) {
		_, err := NewKeychainStore("")
		assert.ErrorIs(t, err, ErrKeyStoreIO)
	})
}

func TestKeychainStore_StoreLoad(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
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

		// Keychain handles encryption, so Salt and Nonce should be empty
		assert.Empty(t, loaded.Salt)
		assert.Empty(t, loaded.Nonce)
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

func TestKeychainStore_Delete(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
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

func TestKeychainStore_List(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
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
}

func TestKeychainStore_KeyNameValidation(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
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

func TestKeychainStore_AllAlgorithms(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
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

func TestKeychainStore_LargeKey(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)

	// Test with a reasonably sized private key data.
	// Note: OS keychains have size limits that vary by platform:
	// - macOS Keychain: ~2KB limit for password items
	// - Windows Credential Store: 2560 bytes (CRED_MAX_CREDENTIAL_BLOB_SIZE)
	// - Linux Secret Service: varies by implementation
	// For typical Ed25519/secp256k1 keys (32-64 bytes), this is not an issue.
	// Use 512 bytes which is well within limits for all platforms.
	largePrivKey := make([]byte, 512)
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

func TestKeychainStore_Close(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
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
	kcs := ks.(*KeychainStore)
	err = kcs.Close()
	require.NoError(t, err)

	// All operations should return ErrKeyStoreClosed
	t.Run("Store after close", func(t *testing.T) {
		err := kcs.Store("new-key", testKey)
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})

	t.Run("Load after close", func(t *testing.T) {
		_, err := kcs.Load("test-key")
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})

	t.Run("Delete after close", func(t *testing.T) {
		err := kcs.Delete("test-key")
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})

	t.Run("List after close", func(t *testing.T) {
		_, err := kcs.List()
		assert.ErrorIs(t, err, ErrKeyStoreClosed)
	})
}

func TestKeychainStore_CloseIdempotent(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)

	kcs := ks.(*KeychainStore)

	// First close
	err = kcs.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = kcs.Close()
	require.NoError(t, err)

	// Third close should also be no-op
	err = kcs.Close()
	require.NoError(t, err)
}

func TestKeychainStore_ConcurrentAccess(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychainWithKnownKeys(t, serviceName, []string{"concurrent"})

	ks, err := NewKeychainStore(serviceName)
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
			for j := 0; j < 10; j++ {
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

func TestKeychainStore_ConcurrentStoreLoad(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)

	const numKeys = 10
	const numReaders = 3
	const readsPerReader = 10

	// Build list of all key names this test uses
	knownKeys := make([]string, 0, numKeys)
	for i := 0; i < numKeys/2; i++ {
		knownKeys = append(knownKeys, fmt.Sprintf("preload-%d", i))
	}
	for i := numKeys / 2; i < numKeys; i++ {
		knownKeys = append(knownKeys, fmt.Sprintf("concurrent-%d", i))
	}
	cleanupKeychainWithKnownKeys(t, serviceName, knownKeys)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)

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

func TestKeychainStore_ConcurrentStoreDelete(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)

	const numIterations = 10
	const numDeleters = 2

	// Build list of all key names this test uses
	knownKeys := make([]string, 0, numIterations*2)
	for i := 0; i < numIterations; i++ {
		knownKeys = append(knownKeys, fmt.Sprintf("delete-me-%d", i))
		knownKeys = append(knownKeys, fmt.Sprintf("new-key-%d", i))
	}
	cleanupKeychainWithKnownKeys(t, serviceName, knownKeys)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)

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

	// Verify: all new keys exist
	names, err := ks.List()
	require.NoError(t, err)
	assert.Len(t, names, numIterations, "expected only new keys to remain")
}

func TestKeychainStore_ConcurrentClose(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)

	const numKeys = 5

	// Build list of all key names this test uses
	knownKeys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		knownKeys[i] = fmt.Sprintf("key-%d", i)
	}
	cleanupKeychainWithKnownKeys(t, serviceName, knownKeys)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)

	kcs := ks.(*KeychainStore)

	// Pre-store some keys
	for i := 0; i < numKeys; i++ {
		key := EncryptedKey{
			Name:        fmt.Sprintf("key-%d", i),
			Algorithm:   AlgorithmEd25519,
			PubKey:      []byte(fmt.Sprintf("pub-%d", i)),
			PrivKeyData: []byte(fmt.Sprintf("priv-%d", i)),
		}
		err := kcs.Store(key.Name, key)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Start concurrent operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_, err := kcs.Load(fmt.Sprintf("key-%d", id%5))
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
		kcs.Close()
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
	_, err = kcs.Load("key-0")
	assert.ErrorIs(t, err, ErrKeyStoreClosed)
}

// TestKeychainStore_UnavailableKeychain tests the error handling when keychain
// is unavailable. This is typically skipped unless running in an environment
// without a keychain.
func TestKeychainStore_UnavailableKeychain(t *testing.T) {
	// This test is mostly documentation - it can't easily be run in most
	// environments since a keychain is usually available.
	// The actual behavior is tested by running on a headless Linux system
	// without D-Bus or a secret service.
	t.Skip("requires environment without keychain access")

	_, err := NewKeychainStore("punnet-sdk")
	assert.ErrorIs(t, err, ErrKeychainUnavailable)
}

// TestKeychainStore_ServiceIsolation verifies that different service names
// create isolated key namespaces.
func TestKeychainStore_ServiceIsolation(t *testing.T) {
	skipIfNoKeychain(t)

	serviceName1 := testServiceName(t) + "-1"
	serviceName2 := testServiceName(t) + "-2"
	cleanupKeychain(t, serviceName1)
	cleanupKeychain(t, serviceName2)

	ks1, err := NewKeychainStore(serviceName1)
	require.NoError(t, err)

	ks2, err := NewKeychainStore(serviceName2)
	require.NoError(t, err)

	// Store same key name in both services
	key := EncryptedKey{
		Name:        "shared-name",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub1"),
		PrivKeyData: []byte("priv1"),
	}

	err = ks1.Store("shared-name", key)
	require.NoError(t, err)

	key.PubKey = []byte("pub2")
	key.PrivKeyData = []byte("priv2")
	err = ks2.Store("shared-name", key)
	require.NoError(t, err)

	// Verify they are isolated
	loaded1, err := ks1.Load("shared-name")
	require.NoError(t, err)
	assert.Equal(t, []byte("pub1"), loaded1.PubKey)

	loaded2, err := ks2.Load("shared-name")
	require.NoError(t, err)
	assert.Equal(t, []byte("pub2"), loaded2.PubKey)
}

// skipCI skips the test in CI environments (for slower keychain tests).
func skipCI(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping slow test in CI")
	}
}

func TestKeychainStore_RepairIndex(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)
	kcs := ks.(*KeychainStore)

	t.Run("empty store repair", func(t *testing.T) {
		report, err := kcs.RepairIndex(nil)
		require.NoError(t, err)
		assert.Empty(t, report.StaleEntriesRemoved)
		assert.Empty(t, report.OrphanedKeysFound)
		assert.Equal(t, 0, report.KeysVerified)
	})

	t.Run("no repair needed", func(t *testing.T) {
		// Store some keys
		keys := []EncryptedKey{
			{Name: "repair-key1", Algorithm: AlgorithmEd25519, PubKey: []byte("pub1"), PrivKeyData: []byte("priv1")},
			{Name: "repair-key2", Algorithm: AlgorithmEd25519, PubKey: []byte("pub2"), PrivKeyData: []byte("priv2")},
		}
		for _, k := range keys {
			err := kcs.Store(k.Name, k)
			require.NoError(t, err)
		}

		report, err := kcs.RepairIndex(nil)
		require.NoError(t, err)
		assert.Empty(t, report.StaleEntriesRemoved)
		assert.Empty(t, report.OrphanedKeysFound)
		assert.Equal(t, 2, report.KeysVerified)

		// Cleanup
		_ = kcs.Delete("repair-key1")
		_ = kcs.Delete("repair-key2")
	})
}

func TestKeychainStore_RepairIndex_StaleEntries(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)
	kcs := ks.(*KeychainStore)

	// Store a key normally
	key := EncryptedKey{
		Name:        "real-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	err = kcs.Store("real-key", key)
	require.NoError(t, err)

	// Manually corrupt the index by adding a fake entry directly
	// Get current index and add a stale entry
	listStr, err := keyring.Get(serviceName, keychainListKey)
	require.NoError(t, err)
	corruptedList := listStr + ",stale-key-never-existed"
	err = keyring.Set(serviceName, keychainListKey, corruptedList)
	require.NoError(t, err)

	// Verify the corruption is visible in List()
	names, err := kcs.List()
	require.NoError(t, err)
	assert.Contains(t, names, "stale-key-never-existed")

	// Run repair
	report, err := kcs.RepairIndex(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"stale-key-never-existed"}, report.StaleEntriesRemoved)
	assert.Empty(t, report.OrphanedKeysFound)
	assert.Equal(t, 1, report.KeysVerified)

	// Verify the stale entry is gone
	names, err = kcs.List()
	require.NoError(t, err)
	assert.NotContains(t, names, "stale-key-never-existed")
	assert.Contains(t, names, "real-key")

	// Cleanup
	_ = kcs.Delete("real-key")
}

func TestKeychainStore_RepairIndex_OrphanedKeys(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)
	kcs := ks.(*KeychainStore)

	// Store a key normally
	key := EncryptedKey{
		Name:        "indexed-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
	}
	err = kcs.Store("indexed-key", key)
	require.NoError(t, err)

	// Create an orphaned key by storing directly in keychain without updating index
	orphanData := `{"name":"orphan-key","algorithm":"ed25519","pub_key":"b3JwaGFu","priv_key_data":"b3JwaGFucHJpdg=="}`
	err = keyring.Set(serviceName, keychainKeyPrefix+"orphan-key", orphanData)
	require.NoError(t, err)

	// Verify the orphan is not in the index
	names, err := kcs.List()
	require.NoError(t, err)
	assert.NotContains(t, names, "orphan-key")

	// Run repair with probe list
	probeKeys := []string{"orphan-key", "nonexistent-key", "another-missing"}
	report, err := kcs.RepairIndex(probeKeys)
	require.NoError(t, err)
	assert.Empty(t, report.StaleEntriesRemoved)
	assert.Equal(t, []string{"orphan-key"}, report.OrphanedKeysFound)
	assert.Equal(t, 1, report.KeysVerified) // indexed-key was verified

	// Verify the orphan is now in the index
	names, err = kcs.List()
	require.NoError(t, err)
	assert.Contains(t, names, "orphan-key")
	assert.Contains(t, names, "indexed-key")

	// Cleanup
	_ = kcs.Delete("indexed-key")
	_ = kcs.Delete("orphan-key")
}

func TestKeychainStore_RepairIndex_Combined(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)
	kcs := ks.(*KeychainStore)

	// Store some valid keys
	validKeys := []EncryptedKey{
		{Name: "valid1", Algorithm: AlgorithmEd25519, PubKey: []byte("pub1"), PrivKeyData: []byte("priv1")},
		{Name: "valid2", Algorithm: AlgorithmEd25519, PubKey: []byte("pub2"), PrivKeyData: []byte("priv2")},
	}
	for _, k := range validKeys {
		err := kcs.Store(k.Name, k)
		require.NoError(t, err)
	}

	// Corrupt the index with stale entries
	listStr, err := keyring.Get(serviceName, keychainListKey)
	require.NoError(t, err)
	corruptedList := listStr + ",stale1,stale2"
	err = keyring.Set(serviceName, keychainListKey, corruptedList)
	require.NoError(t, err)

	// Create orphaned keys
	orphanData := `{"name":"orphan1","algorithm":"ed25519","pub_key":"cHVi","priv_key_data":"cHJpdg=="}`
	err = keyring.Set(serviceName, keychainKeyPrefix+"orphan1", orphanData)
	require.NoError(t, err)

	// Run repair with probe list
	report, err := kcs.RepairIndex([]string{"orphan1", "orphan2"})
	require.NoError(t, err)

	// Verify results
	assert.Len(t, report.StaleEntriesRemoved, 2)
	assert.Contains(t, report.StaleEntriesRemoved, "stale1")
	assert.Contains(t, report.StaleEntriesRemoved, "stale2")
	assert.Equal(t, []string{"orphan1"}, report.OrphanedKeysFound)
	assert.Equal(t, 2, report.KeysVerified) // valid1 and valid2

	// Verify the final state
	names, err := kcs.List()
	require.NoError(t, err)
	assert.Len(t, names, 3) // valid1, valid2, orphan1
	assert.Contains(t, names, "valid1")
	assert.Contains(t, names, "valid2")
	assert.Contains(t, names, "orphan1")
	assert.NotContains(t, names, "stale1")
	assert.NotContains(t, names, "stale2")

	// Cleanup
	_ = kcs.Delete("valid1")
	_ = kcs.Delete("valid2")
	_ = kcs.Delete("orphan1")
}

func TestKeychainStore_RepairIndex_ClosedStore(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)
	kcs := ks.(*KeychainStore)

	// Close the store
	err = kcs.Close()
	require.NoError(t, err)

	// RepairIndex should return ErrKeyStoreClosed
	_, err = kcs.RepairIndex(nil)
	assert.ErrorIs(t, err, ErrKeyStoreClosed)
}

func TestKeychainStore_RepairIndex_InvalidProbeKeys(t *testing.T) {
	skipIfNoKeychain(t)
	serviceName := testServiceName(t)
	cleanupKeychain(t, serviceName)

	ks, err := NewKeychainStore(serviceName)
	require.NoError(t, err)
	kcs := ks.(*KeychainStore)

	// Create a valid orphan
	orphanData := `{"name":"valid-orphan","algorithm":"ed25519","pub_key":"cHVi","priv_key_data":"cHJpdg=="}`
	err = keyring.Set(serviceName, keychainKeyPrefix+"valid-orphan", orphanData)
	require.NoError(t, err)

	// Run repair with mix of valid and invalid probe keys
	probeKeys := []string{
		"valid-orphan",
		"",               // empty - invalid
		"../path/attack", // path traversal - invalid
		".hidden",        // starts with dot - invalid
	}

	report, err := kcs.RepairIndex(probeKeys)
	require.NoError(t, err)

	// Should only find the valid orphan, invalid names are skipped
	assert.Equal(t, []string{"valid-orphan"}, report.OrphanedKeysFound)

	// Cleanup
	_ = kcs.Delete("valid-orphan")
}
