package crypto

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKeyStore is a simple in-memory implementation for testing the interface contract.
// Complexity: O(1) average for all operations (hash map backed).
type mockKeyStore struct {
	mu   sync.RWMutex
	keys map[string]EncryptedKey
}

func newMockKeyStore() *mockKeyStore {
	return &mockKeyStore{
		keys: make(map[string]EncryptedKey),
	}
}

func (m *mockKeyStore) Store(name string, key EncryptedKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[name]; exists {
		return ErrKeyStoreExists
	}
	m.keys[name] = key
	return nil
}

func (m *mockKeyStore) Load(name string) (EncryptedKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key, exists := m.keys[name]
	if !exists {
		return EncryptedKey{}, ErrKeyStoreNotFound
	}
	return key, nil
}

func (m *mockKeyStore) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[name]; !exists {
		return ErrKeyStoreNotFound
	}
	delete(m.keys, name)
	return nil
}

func (m *mockKeyStore) List() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.keys))
	for name := range m.keys {
		names = append(names, name)
	}
	return names, nil
}

// Verify mockKeyStore implements KeyStore interface.
var _ KeyStore = (*mockKeyStore)(nil)

func TestKeyStore_Store(t *testing.T) {
	store := newMockKeyStore()

	key := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key-bytes"),
		PrivKeyData: []byte("private-key-data"),
	}

	// Store should succeed on first call
	err := store.Store("test-key", key)
	require.NoError(t, err)

	// Store should return ErrKeyStoreExists on duplicate
	err = store.Store("test-key", key)
	assert.ErrorIs(t, err, ErrKeyStoreExists)
}

func TestKeyStore_Load(t *testing.T) {
	store := newMockKeyStore()

	// Load non-existent key should return ErrKeyStoreNotFound
	_, err := store.Load("missing")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)

	// Store a key
	original := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmSecp256k1,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
		Salt:        []byte("salt"),
		Nonce:       []byte("nonce"),
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

func TestKeyStore_Delete(t *testing.T) {
	store := newMockKeyStore()

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

	err = store.Delete("test-key")
	require.NoError(t, err)

	// Should not be loadable after delete
	_, err = store.Load("test-key")
	assert.ErrorIs(t, err, ErrKeyStoreNotFound)
}

func TestKeyStore_List(t *testing.T) {
	store := newMockKeyStore()

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

func TestEncryptedKey_IsEncrypted(t *testing.T) {
	tests := []struct {
		name     string
		key      EncryptedKey
		expected bool
	}{
		{
			name: "plaintext key (no salt or nonce)",
			key: EncryptedKey{
				Name:        "plaintext",
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("priv"),
			},
			expected: false,
		},
		{
			name: "encrypted key (has salt and nonce)",
			key: EncryptedKey{
				Name:        "encrypted",
				Algorithm:   AlgorithmSecp256k1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("encrypted-data"),
				Salt:        []byte("salt-bytes"),
				Nonce:       []byte("nonce-bytes"),
			},
			expected: true,
		},
		{
			name: "partial encryption (only salt)",
			key: EncryptedKey{
				Name:        "partial",
				Algorithm:   AlgorithmSecp256r1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("data"),
				Salt:        []byte("salt"),
			},
			expected: false,
		},
		{
			name: "partial encryption (only nonce)",
			key: EncryptedKey{
				Name:        "partial",
				Algorithm:   AlgorithmSecp256r1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("data"),
				Nonce:       []byte("nonce"),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.IsEncrypted()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAlgorithm_IsValid(t *testing.T) {
	tests := []struct {
		algo  Algorithm
		valid bool
	}{
		{AlgorithmEd25519, true},
		{AlgorithmSecp256k1, true},
		{AlgorithmSecp256r1, true},
		{Algorithm("unknown"), false},
		{Algorithm(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.algo), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.algo.IsValid())
		})
	}
}

func TestAlgorithm_String(t *testing.T) {
	assert.Equal(t, "ed25519", AlgorithmEd25519.String())
	assert.Equal(t, "secp256k1", AlgorithmSecp256k1.String())
	assert.Equal(t, "secp256r1", AlgorithmSecp256r1.String())
}

// TestKeyStore_ConcurrentAccess verifies thread safety of KeyStore implementations.
func TestKeyStore_ConcurrentAccess(t *testing.T) {
	store := newMockKeyStore()
	var wg sync.WaitGroup

	// Concurrent stores
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := EncryptedKey{
				Name:        "key",
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("priv"),
			}
			_ = store.Store(key.Name, key)
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = store.Load("key")
			_, _ = store.List()
		}()
	}
	wg.Wait()

	// Only one store should have succeeded
	names, err := store.List()
	require.NoError(t, err)
	assert.Len(t, names, 1)
}
