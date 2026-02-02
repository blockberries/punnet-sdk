package crypto

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

const testServiceName = "punnet-sdk-test"

// cleanupTestKeys removes all test keys from the keychain.
// Call this at the start of each test to ensure a clean state.
func cleanupTestKeys(t *testing.T, serviceName string) {
	t.Helper()

	// Try to delete the key list and any known test keys
	_ = keyring.Delete(serviceName, keychainListKey)
	_ = keyring.Delete(serviceName, "test-key")
	_ = keyring.Delete(serviceName, "alice")
	_ = keyring.Delete(serviceName, "bob")
	_ = keyring.Delete(serviceName, "charlie")
	_ = keyring.Delete(serviceName, "concurrent-key")
	for i := 0; i < 20; i++ {
		_ = keyring.Delete(serviceName, "concurrent-key-"+string(rune('a'+i)))
	}
}

func TestNewKeychainStore(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid service name",
			serviceName: testServiceName,
			wantErr:     false,
		},
		{
			name:        "empty service name",
			serviceName: "",
			wantErr:     true,
			errContains: "service name cannot be empty",
		},
		{
			name:        "service name with spaces",
			serviceName: "test service",
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:        "service name too long",
			serviceName: strings.Repeat("a", 101),
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "valid service name with dots",
			serviceName: "com.example.punnet-sdk",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewKeychainStore(tt.serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, store)
			} else {
				// Skip if keychain is not available (e.g., in CI)
				if err != nil && strings.Contains(err.Error(), "keychain unavailable") {
					t.Skip("Keychain not available in this environment")
				}
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}
}

func TestKeychainStore_StoreAndLoad(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	testKey := EncryptedKey{
		Name:        "alice",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("test-public-key-32-bytes-long!!"),
		PrivKeyData: []byte("test-private-key-64-bytes-long-for-ed25519!!!!!!!!!!!!!!!!!!!!!!"),
	}

	// Store the key
	err = store.Store("alice", testKey)
	require.NoError(t, err)

	// Load the key back
	loaded, err := store.Load("alice")
	require.NoError(t, err)

	assert.Equal(t, testKey.Name, loaded.Name)
	assert.Equal(t, testKey.Algorithm, loaded.Algorithm)
	assert.Equal(t, testKey.PubKey, loaded.PubKey)
	assert.Equal(t, testKey.PrivKeyData, loaded.PrivKeyData)

	// Salt and Nonce should be nil (keychain handles encryption)
	assert.Nil(t, loaded.Salt)
	assert.Nil(t, loaded.Nonce)

	// Cleanup
	err = store.Delete("alice")
	require.NoError(t, err)
}

func TestKeychainStore_StoreExisting(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	testKey := EncryptedKey{
		Name:        "bob",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("test-public-key-32-bytes-long!!"),
		PrivKeyData: []byte("test-private-key"),
	}

	// Store the key first time
	err = store.Store("bob", testKey)
	require.NoError(t, err)

	// Try to store again - should fail
	err = store.Store("bob", testKey)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyStoreExists)

	// Cleanup
	_ = store.Delete("bob")
}

func TestKeychainStore_LoadNotFound(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	_, err = store.Load("nonexistent-key")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)
}

func TestKeychainStore_Delete(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	testKey := EncryptedKey{
		Name:        "charlie",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("test-public-key-32-bytes-long!!"),
		PrivKeyData: []byte("test-private-key"),
	}

	// Store and then delete
	err = store.Store("charlie", testKey)
	require.NoError(t, err)

	err = store.Delete("charlie")
	require.NoError(t, err)

	// Should not be found after delete
	_, err = store.Load("charlie")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)
}

func TestKeychainStore_DeleteNotFound(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store.Delete("nonexistent-key")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)
}

func TestKeychainStore_List(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	// Initially empty
	names, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, names)

	// Add some keys
	testKey := EncryptedKey{
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("test-public-key-32-bytes-long!!"),
		PrivKeyData: []byte("test-private-key"),
	}

	err = store.Store("alice", testKey)
	require.NoError(t, err)

	testKey.Name = "bob"
	err = store.Store("bob", testKey)
	require.NoError(t, err)

	// List should now contain both
	names, err = store.List()
	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "alice")
	assert.Contains(t, names, "bob")

	// Delete one and verify list updates
	err = store.Delete("alice")
	require.NoError(t, err)

	names, err = store.List()
	require.NoError(t, err)
	assert.Len(t, names, 1)
	assert.Contains(t, names, "bob")
	assert.NotContains(t, names, "alice")

	// Cleanup
	_ = store.Delete("bob")
}

func TestKeychainStore_InvalidKeyNames(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	testKey := EncryptedKey{
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("test-public-key-32-bytes-long!!"),
		PrivKeyData: []byte("test-private-key"),
	}

	tests := []struct {
		name        string
		keyName     string
		errContains string
	}{
		{
			name:        "empty name",
			keyName:     "",
			errContains: "cannot be empty",
		},
		{
			name:        "reserved name",
			keyName:     keychainListKey,
			errContains: "reserved",
		},
		{
			name:        "path traversal forward slash",
			keyName:     "../etc/passwd",
			errContains: "path separator",
		},
		{
			name:        "path traversal backslash",
			keyName:     "..\\windows\\system32",
			errContains: "path separator",
		},
		{
			name:        "double dots",
			keyName:     "test..key",
			errContains: "..",
		},
		{
			name:        "hidden file prefix",
			keyName:     ".hidden",
			errContains: "cannot start with",
		},
		{
			name:        "name too long",
			keyName:     strings.Repeat("a", maxKeychainKeyNameLen+1),
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Store
			err := store.Store(tt.keyName, testKey)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)

			// Test Load
			_, err = store.Load(tt.keyName)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)

			// Test Delete
			err = store.Delete(tt.keyName)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestKeychainStore_AllAlgorithms(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	algorithms := []Algorithm{
		AlgorithmEd25519,
		AlgorithmSecp256k1,
		AlgorithmSecp256r1,
	}

	for _, alg := range algorithms {
		t.Run(string(alg), func(t *testing.T) {
			keyName := "test-" + string(alg)
			testKey := EncryptedKey{
				Name:        keyName,
				Algorithm:   alg,
				PubKey:      []byte("test-public-key-32-bytes-long!!"),
				PrivKeyData: []byte("test-private-key-data"),
			}

			err := store.Store(keyName, testKey)
			require.NoError(t, err)

			loaded, err := store.Load(keyName)
			require.NoError(t, err)
			assert.Equal(t, alg, loaded.Algorithm)

			// Cleanup
			_ = store.Delete(keyName)
		})
	}
}

func TestKeychainStore_ConcurrentAccess(t *testing.T) {
	cleanupTestKeys(t, testServiceName)

	store, err := NewKeychainStore(testServiceName)
	if err != nil {
		if strings.Contains(err.Error(), "keychain unavailable") {
			t.Skip("Keychain not available in this environment")
		}
		t.Fatalf("Failed to create store: %v", err)
	}

	testKey := EncryptedKey{
		Name:        "concurrent-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("test-public-key-32-bytes-long!!"),
		PrivKeyData: []byte("test-private-key"),
	}

	// Store a key for reading
	err = store.Store("concurrent-key", testKey)
	require.NoError(t, err)

	// Concurrent reads and writes
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Multiple concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_, err := store.Load("concurrent-key")
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	// Multiple concurrent writers (different keys)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		keyName := "concurrent-key-" + string(rune('a'+i))
		go func(name string) {
			defer wg.Done()
			key := EncryptedKey{
				Name:        name,
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte("test-public-key-32-bytes-long!!"),
				PrivKeyData: []byte("test-private-key"),
			}
			if err := store.Store(name, key); err != nil {
				errors <- err
			}
		}(keyName)
	}

	// Concurrent list operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				_, err := store.List()
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Cleanup
	_ = store.Delete("concurrent-key")
	for i := 0; i < 5; i++ {
		_ = store.Delete("concurrent-key-" + string(rune('a'+i)))
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		wantError bool
	}{
		{"valid simple", "myapp", false},
		{"valid with dots", "com.example.app", false},
		{"valid with hyphens", "my-app-name", false},
		{"valid with underscores", "my_app_name", false},
		{"valid mixed", "com.example.my-app_v2", false},
		{"empty", "", true},
		{"with space", "my app", true},
		{"with special char", "my@app", true},
		{"too long", strings.Repeat("a", 101), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServiceName(tt.service)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateKeychainKeyName(t *testing.T) {
	tests := []struct {
		name      string
		keyName   string
		wantError bool
	}{
		{"valid simple", "mykey", false},
		{"valid with hyphens", "my-key", false},
		{"valid with underscores", "my_key", false},
		{"empty", "", true},
		{"reserved", keychainListKey, true},
		{"path traversal", "../foo", true},
		{"hidden", ".hidden", true},
		{"double dots", "a..b", true},
		{"too long", strings.Repeat("a", maxKeychainKeyNameLen+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeychainKeyName(tt.keyName)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		isNotFound bool
	}{
		{"nil error", nil, false},
		{"go-keyring ErrNotFound", keyring.ErrNotFound, true},
		{"generic not found", assert.AnError, false}, // This doesn't contain "not found"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			assert.Equal(t, tt.isNotFound, result)
		})
	}
}

func TestClearKeychainKeyData(t *testing.T) {
	// Create key data with sensitive information
	data := &keychainKeyData{
		Name:        "test-key",
		Algorithm:   "ed25519",
		PubKey:      []byte("public-key-data-here"),
		PrivKeyData: []byte("sensitive-private-key-data"),
	}

	// Clear the data
	clearKeychainKeyData(data)

	// Verify all fields are cleared
	assert.Empty(t, data.Name)
	assert.Empty(t, data.Algorithm)

	// Verify byte slices are zeroed (not nil, but zeroed)
	for _, b := range data.PubKey {
		assert.Equal(t, byte(0), b, "PubKey should be zeroed")
	}
	for _, b := range data.PrivKeyData {
		assert.Equal(t, byte(0), b, "PrivKeyData should be zeroed")
	}

	// Test nil safety
	clearKeychainKeyData(nil) // Should not panic
}
