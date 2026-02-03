package crypto

import (
	"strings"
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
	// Validate key name
	if err := ValidateKeyName(name); err != nil {
		return err
	}

	// Validate name matches key.Name
	if name != key.Name {
		return ErrKeyNameMismatch
	}

	// Validate algorithm
	if !key.Algorithm.IsValid() {
		return ErrInvalidAlgorithm
	}

	// Validate encryption params
	if err := key.ValidateEncryptionParams(); err != nil {
		return err
	}

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

	key, exists := m.keys[name]
	if !exists {
		return ErrKeyStoreNotFound
	}

	// Secure wipe before deletion
	key.Wipe()
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

// Verify mockKeyStore implements EncryptedKeyStore interface.
var _ EncryptedKeyStore = (*mockKeyStore)(nil)

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

	// Store a key with valid encryption params
	original := EncryptedKey{
		Name:        "test-key",
		Algorithm:   AlgorithmSecp256k1,
		PubKey:      []byte("pub"),
		PrivKeyData: []byte("priv"),
		Salt:        make([]byte, MinSaltLength),   // 16 bytes
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
			name: "encrypted key (valid salt and nonce lengths)",
			key: EncryptedKey{
				Name:        "encrypted",
				Algorithm:   AlgorithmSecp256k1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("encrypted-data"),
				Salt:        make([]byte, MinSaltLength),
				Nonce:       make([]byte, AESGCMNonceLength),
			},
			expected: true,
		},
		{
			name: "invalid: salt too short",
			key: EncryptedKey{
				Name:        "short-salt",
				Algorithm:   AlgorithmSecp256r1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("data"),
				Salt:        []byte("short"),
				Nonce:       make([]byte, AESGCMNonceLength),
			},
			expected: false,
		},
		{
			name: "invalid: nonce wrong length",
			key: EncryptedKey{
				Name:        "wrong-nonce",
				Algorithm:   AlgorithmSecp256r1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("data"),
				Salt:        make([]byte, MinSaltLength),
				Nonce:       []byte("wrong"),
			},
			expected: false,
		},
		{
			name: "partial encryption (only salt)",
			key: EncryptedKey{
				Name:        "partial",
				Algorithm:   AlgorithmSecp256r1,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("data"),
				Salt:        make([]byte, MinSaltLength),
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
				Nonce:       make([]byte, AESGCMNonceLength),
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

func TestEncryptedKey_ValidateEncryptionParams(t *testing.T) {
	tests := []struct {
		name    string
		key     EncryptedKey
		wantErr error
	}{
		{
			name: "valid plaintext (no salt, no nonce)",
			key: EncryptedKey{
				Name:      "plaintext",
				Algorithm: AlgorithmEd25519,
			},
			wantErr: nil,
		},
		{
			name: "valid encrypted (correct lengths)",
			key: EncryptedKey{
				Name:      "encrypted",
				Algorithm: AlgorithmEd25519,
				Salt:      make([]byte, MinSaltLength),
				Nonce:     make([]byte, AESGCMNonceLength),
			},
			wantErr: nil,
		},
		{
			name: "valid encrypted (longer salt is ok)",
			key: EncryptedKey{
				Name:      "encrypted",
				Algorithm: AlgorithmEd25519,
				Salt:      make([]byte, 32), // > MinSaltLength
				Nonce:     make([]byte, AESGCMNonceLength),
			},
			wantErr: nil,
		},
		{
			name: "invalid: salt too short",
			key: EncryptedKey{
				Name:      "invalid",
				Algorithm: AlgorithmEd25519,
				Salt:      make([]byte, MinSaltLength-1),
				Nonce:     make([]byte, AESGCMNonceLength),
			},
			wantErr: ErrInvalidEncryptionParams,
		},
		{
			name: "invalid: nonce too short",
			key: EncryptedKey{
				Name:      "invalid",
				Algorithm: AlgorithmEd25519,
				Salt:      make([]byte, MinSaltLength),
				Nonce:     make([]byte, AESGCMNonceLength-1),
			},
			wantErr: ErrInvalidEncryptionParams,
		},
		{
			name: "invalid: nonce too long",
			key: EncryptedKey{
				Name:      "invalid",
				Algorithm: AlgorithmEd25519,
				Salt:      make([]byte, MinSaltLength),
				Nonce:     make([]byte, AESGCMNonceLength+1),
			},
			wantErr: ErrInvalidEncryptionParams,
		},
		{
			name: "invalid: partial (salt only)",
			key: EncryptedKey{
				Name:      "partial",
				Algorithm: AlgorithmEd25519,
				Salt:      make([]byte, MinSaltLength),
			},
			wantErr: ErrInvalidEncryptionParams,
		},
		{
			name: "invalid: partial (nonce only)",
			key: EncryptedKey{
				Name:      "partial",
				Algorithm: AlgorithmEd25519,
				Nonce:     make([]byte, AESGCMNonceLength),
			},
			wantErr: ErrInvalidEncryptionParams,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.key.ValidateEncryptionParams()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEncryptedKey_Validate(t *testing.T) {
	tests := []struct {
		name    string
		key     EncryptedKey
		wantErr error
	}{
		{
			name: "valid plaintext key",
			key: EncryptedKey{
				Name:      "test",
				Algorithm: AlgorithmEd25519,
			},
			wantErr: nil,
		},
		{
			name: "valid encrypted key",
			key: EncryptedKey{
				Name:      "test",
				Algorithm: AlgorithmSecp256k1,
				Salt:      make([]byte, MinSaltLength),
				Nonce:     make([]byte, AESGCMNonceLength),
			},
			wantErr: nil,
		},
		{
			name: "invalid algorithm",
			key: EncryptedKey{
				Name:      "test",
				Algorithm: Algorithm("invalid"),
			},
			wantErr: ErrInvalidAlgorithm,
		},
		{
			name: "invalid encryption params",
			key: EncryptedKey{
				Name:      "test",
				Algorithm: AlgorithmEd25519,
				Salt:      []byte("short"), // Too short
				Nonce:     make([]byte, AESGCMNonceLength),
			},
			wantErr: ErrInvalidEncryptionParams,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.key.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEncryptedKey_Wipe(t *testing.T) {
	key := EncryptedKey{
		Name:        "test",
		Algorithm:   AlgorithmEd25519,
		PubKey:      []byte("public-key"),
		PrivKeyData: []byte("secret-private-key-data"),
		Salt:        []byte("random-salt-bytes"),
		Nonce:       []byte("random-nonce"),
	}

	// Store original lengths
	privLen := len(key.PrivKeyData)
	saltLen := len(key.Salt)
	nonceLen := len(key.Nonce)

	// Wipe the key
	key.Wipe()

	// Verify all bytes are zeroed (lengths preserved)
	assert.Len(t, key.PrivKeyData, privLen)
	assert.Len(t, key.Salt, saltLen)
	assert.Len(t, key.Nonce, nonceLen)

	for i, b := range key.PrivKeyData {
		assert.Equal(t, byte(0), b, "PrivKeyData[%d] not zeroed", i)
	}
	for i, b := range key.Salt {
		assert.Equal(t, byte(0), b, "Salt[%d] not zeroed", i)
	}
	for i, b := range key.Nonce {
		assert.Equal(t, byte(0), b, "Nonce[%d] not zeroed", i)
	}
}

func TestEncryptedKey_SafeString(t *testing.T) {
	plaintextKey := EncryptedKey{
		Name:        "my-key",
		Algorithm:   AlgorithmEd25519,
		PrivKeyData: []byte("secret"),
	}

	encryptedKey := EncryptedKey{
		Name:        "secure-key",
		Algorithm:   AlgorithmSecp256k1,
		PrivKeyData: []byte("encrypted-secret"),
		Salt:        make([]byte, MinSaltLength),
		Nonce:       make([]byte, AESGCMNonceLength),
	}

	// Verify plaintext representation
	ptStr := plaintextKey.SafeString()
	assert.Contains(t, ptStr, "my-key")
	assert.Contains(t, ptStr, "ed25519")
	assert.Contains(t, ptStr, "plaintext")
	assert.NotContains(t, ptStr, "secret")

	// Verify encrypted representation
	encStr := encryptedKey.SafeString()
	assert.Contains(t, encStr, "secure-key")
	assert.Contains(t, encStr, "secp256k1")
	assert.Contains(t, encStr, "encrypted")
	assert.NotContains(t, encStr, "secret")
}

func TestValidateKeyName(t *testing.T) {
	tests := []struct {
		name    string
		keyName string
		wantErr error
	}{
		{
			name:    "valid simple name",
			keyName: "my-key",
			wantErr: nil,
		},
		{
			name:    "valid name with underscores",
			keyName: "my_key_123",
			wantErr: nil,
		},
		{
			name:    "valid unicode name",
			keyName: "密钥",
			wantErr: nil,
		},
		{
			name:    "invalid: empty name",
			keyName: "",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: path traversal ..",
			keyName: "../etc/passwd",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: forward slash",
			keyName: "path/to/key",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: backslash",
			keyName: "path\\to\\key",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: colon",
			keyName: "key:name",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: asterisk",
			keyName: "key*name",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: question mark",
			keyName: "key?name",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: double quote",
			keyName: "key\"name",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: angle brackets",
			keyName: "key<name>",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: pipe",
			keyName: "key|name",
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "invalid: too long",
			keyName: strings.Repeat("a", MaxKeyNameLength+1),
			wantErr: ErrInvalidKeyName,
		},
		{
			name:    "valid: max length",
			keyName: strings.Repeat("a", MaxKeyNameLength),
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeyName(tt.keyName)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestKeyStore_StoreValidation(t *testing.T) {
	store := newMockKeyStore()

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

	// Concurrent stores to same key
	successCount := 0
	var mu sync.Mutex
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
			err := store.Store(key.Name, key)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// Exactly one store should have succeeded
	assert.Equal(t, 1, successCount, "exactly one concurrent store should succeed")

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

// TestKeyStore_ConcurrentAccessDifferentKeys verifies thread safety
// when accessing different keys concurrently (no false conflicts).
func TestKeyStore_ConcurrentAccessDifferentKeys(t *testing.T) {
	store := newMockKeyStore()
	var wg sync.WaitGroup
	keyCount := 100

	// Concurrent stores to different keys - all should succeed
	errors := make([]error, keyCount)
	for i := 0; i < keyCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := strings.Repeat("k", idx+1) // Unique names: "k", "kk", "kkk", etc.
			if len(name) > MaxKeyNameLength {
				name = name[:MaxKeyNameLength]
			}
			key := EncryptedKey{
				Name:        name,
				Algorithm:   AlgorithmEd25519,
				PubKey:      []byte("pub"),
				PrivKeyData: []byte("priv"),
			}
			errors[idx] = store.Store(name, key)
		}(i)
	}
	wg.Wait()

	// Count successes (some may fail due to name collisions at MaxKeyNameLength)
	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}

	// At least many keys should have been stored successfully
	names, err := store.List()
	require.NoError(t, err)
	assert.Equal(t, successCount, len(names), "stored key count should match success count")
	assert.Greater(t, successCount, keyCount/2, "most concurrent stores to different keys should succeed")

	// Concurrent mixed operations on different keys
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := strings.Repeat("k", (idx%50)+1)
			if len(name) > MaxKeyNameLength {
				name = name[:MaxKeyNameLength]
			}
			switch idx % 3 {
			case 0:
				_, _ = store.Load(name)
			case 1:
				_, _ = store.List()
			case 2:
				_ = store.Delete(name)
			}
		}(i)
	}
	wg.Wait()

	// Store should still be in consistent state
	_, err = store.List()
	require.NoError(t, err)
}
