package crypto

import (
	"os"
	"path/filepath"
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
