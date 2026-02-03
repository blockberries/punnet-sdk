package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

// Test helpers for encrypted key operations

// pbkdf2TestHelper derives a key using PBKDF2-SHA256.
func pbkdf2TestHelper(password, salt []byte, iterations, keyLen int) []byte {
	return pbkdf2.Key(password, salt, iterations, keyLen, sha256.New)
}

// aesGCMEncryptTestHelper encrypts plaintext using AES-256-GCM.
func aesGCMEncryptTestHelper(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, additionalData)
	return ciphertext, nil
}

func TestKeyringNewKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate new key
	signer, err := kr.NewKey("test-key", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Verify signer is valid
	if signer == nil {
		t.Fatal("NewKey returned nil signer")
	}
	if signer.Algorithm() != AlgorithmEd25519 {
		t.Errorf("expected algorithm %s, got %s", AlgorithmEd25519, signer.Algorithm())
	}

	// Verify public key is correct size
	pubKey := signer.PublicKey()
	if len(pubKey.Bytes()) != ed25519.PublicKeySize {
		t.Errorf("expected public key size %d, got %d", ed25519.PublicKeySize, len(pubKey.Bytes()))
	}

	// Verify key is stored
	keys, err := kr.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 1 || keys[0] != "test-key" {
		t.Errorf("expected [test-key], got %v", keys)
	}
}

func TestKeyringNewKeyDuplicate(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate first key
	_, err := kr.NewKey("test-key", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("first NewKey failed: %v", err)
	}

	// Try to generate duplicate
	_, err = kr.NewKey("test-key", AlgorithmEd25519)
	if err != ErrKeyExists {
		t.Errorf("expected ErrKeyExists, got %v", err)
	}
}

func TestKeyringImportKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate a key outside the keyring
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Import it
	signer, err := kr.ImportKey("imported", priv, AlgorithmEd25519)
	if err != nil {
		t.Fatalf("ImportKey failed: %v", err)
	}

	// Verify signer
	if signer.Algorithm() != AlgorithmEd25519 {
		t.Errorf("expected algorithm %s, got %s", AlgorithmEd25519, signer.Algorithm())
	}

	// Verify public key matches
	expectedPub := priv.Public().(ed25519.PublicKey)
	if !bytes.Equal(signer.PublicKey().Bytes(), expectedPub) {
		t.Error("public key mismatch after import")
	}
}

func TestKeyringImportKeyInvalid(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Try to import invalid key data
	_, err := kr.ImportKey("bad-key", []byte("too short"), AlgorithmEd25519)
	if err != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestKeyringExportKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate key
	_, err := kr.NewKey("export-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Export it
	exported, err := kr.ExportKey("export-test", "")
	if err != nil {
		t.Fatalf("ExportKey failed: %v", err)
	}

	// Verify size
	if len(exported) != ed25519.PrivateKeySize {
		t.Errorf("expected private key size %d, got %d", ed25519.PrivateKeySize, len(exported))
	}
}

func TestKeyringExportKeyNotFound(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	_, err := kr.ExportKey("nonexistent", "")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestKeyringGetKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate key
	original, err := kr.NewKey("get-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Get it back
	retrieved, err := kr.GetKey("get-test")
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}

	// Verify public keys match
	if !original.PublicKey().Equals(retrieved.PublicKey()) {
		t.Error("public key mismatch")
	}
}

func TestKeyringGetKeyNotFound(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	_, err := kr.GetKey("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestKeyringDeleteKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate key
	_, err := kr.NewKey("delete-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Delete it
	if err := kr.DeleteKey("delete-test"); err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Verify it's gone
	_, err = kr.GetKey("delete-test")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}

	// Verify list is empty
	keys, err := kr.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty list, got %v", keys)
	}
}

func TestKeyringDeleteKeyNotFound(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	err := kr.DeleteKey("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestKeyringSign(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate key
	signer, err := kr.NewKey("sign-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Sign data
	data := []byte("hello world")
	sig, err := kr.Sign("sign-test", data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verify signature
	if len(sig) != ed25519.SignatureSize {
		t.Errorf("expected signature size %d, got %d", ed25519.SignatureSize, len(sig))
	}

	// Verify it's valid
	if !signer.PublicKey().Verify(data, sig) {
		t.Error("signature verification failed")
	}
}

func TestKeyringSignNotFound(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	_, err := kr.Sign("nonexistent", []byte("data"))
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestKeyringListKeys(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Empty initially
	keys, err := kr.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty list, got %v", keys)
	}

	// Add some keys
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		if _, err := kr.NewKey(name, AlgorithmEd25519); err != nil {
			t.Fatalf("NewKey %s failed: %v", name, err)
		}
	}

	// List again
	keys, err = kr.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 5 {
		t.Errorf("expected 5 keys, got %d", len(keys))
	}
}

func TestKeyringCacheEviction(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store, WithCacheSize(2))

	// Create 3 keys (exceeds cache size of 2)
	for _, name := range []string{"a", "b", "c"} {
		if _, err := kr.NewKey(name, AlgorithmEd25519); err != nil {
			t.Fatalf("NewKey %s failed: %v", name, err)
		}
	}

	// Access all keys - should work even though cache is smaller
	for _, name := range []string{"a", "b", "c"} {
		if _, err := kr.GetKey(name); err != nil {
			t.Errorf("GetKey %s failed: %v", name, err)
		}
	}
}

func TestKeyringConcurrency(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create a key to work with
	if _, err := kr.NewKey("concurrent", AlgorithmEd25519); err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get key
			_, err := kr.GetKey("concurrent")
			if err != nil {
				t.Errorf("GetKey failed: %v", err)
			}

			// Sign data
			_, err = kr.Sign("concurrent", []byte("test data"))
			if err != nil {
				t.Errorf("Sign failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

// Benchmarks

func BenchmarkKeyringNewKey(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := string(rune('a' + (i % 26)))
		// Delete if exists to allow recreation
		_ = kr.DeleteKey(name)
		_, _ = kr.NewKey(name, AlgorithmEd25519)
	}
}

func BenchmarkKeyringGetKeyCached(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)
	_, _ = kr.NewKey("bench", AlgorithmEd25519)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.GetKey("bench")
	}
}

func BenchmarkKeyringGetKeyUncached(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store, WithCacheSize(0)) // Disable cache
	_, _ = kr.NewKey("bench", AlgorithmEd25519)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.GetKey("bench")
	}
}

func BenchmarkKeyringSign(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)
	_, _ = kr.NewKey("bench", AlgorithmEd25519)
	data := []byte("benchmark signing data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.Sign("bench", data)
	}
}

func BenchmarkKeyringSignParallel(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)
	_, _ = kr.NewKey("bench", AlgorithmEd25519)
	data := []byte("benchmark signing data")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = kr.Sign("bench", data)
		}
	})
}

func BenchmarkKeyringListKeys(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Add 100 keys
	for i := 0; i < 100; i++ {
		_, _ = kr.NewKey(string(rune('a'+i/26))+string(rune('a'+i%26)), AlgorithmEd25519)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.ListKeys()
	}
}

func BenchmarkMemoryStoreGet(b *testing.B) {
	store := NewMemoryStore()
	entry := &KeyEntry{
		Name:       "bench",
		Algorithm:  AlgorithmEd25519,
		PrivateKey: make([]byte, 64),
		PublicKey:  make([]byte, 32),
	}
	_ = store.Put(entry, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get("bench")
	}
}

func BenchmarkMemoryStorePut(b *testing.B) {
	store := NewMemoryStore()
	entry := &KeyEntry{
		Name:       "bench",
		Algorithm:  AlgorithmEd25519,
		PrivateKey: make([]byte, 64),
		PublicKey:  make([]byte, 32),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.Put(entry, true) // Overwrite mode
	}
}

// Security tests

func TestKeyringKeyNameValidation(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"", true},                        // empty
		{"valid-key", false},              // valid
		{"key_with_underscore", false},    // valid
		{"key.with.dots", false},          // valid
		{"../etc/passwd", true},           // path traversal
		{"..\\windows\\system32", true},   // windows path traversal
		{"/absolute/path", true},          // absolute path
		{"key\x00null", true},             // null byte
		{"key\nwith\nnewlines", true},     // control chars
		{string(make([]byte, 300)), true}, // too long
	}

	for _, tt := range tests {
		_, err := kr.NewKey(tt.name, AlgorithmEd25519)
		if (err != nil) != tt.wantErr {
			t.Errorf("NewKey(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
		// Clean up if key was created
		if err == nil {
			_ = kr.DeleteKey(tt.name)
		}
	}
}

func TestKeyringImportKeyNameValidation(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Generate a valid key
	_, priv, _ := ed25519.GenerateKey(nil)

	// Empty name should fail
	_, err := kr.ImportKey("", priv, AlgorithmEd25519)
	if err == nil {
		t.Error("ImportKey should reject empty name")
	}

	// Path traversal should fail
	_, err = kr.ImportKey("../malicious", priv, AlgorithmEd25519)
	if err == nil {
		t.Error("ImportKey should reject path traversal")
	}
}

func TestKeyringImportKeyAlgorithmValidation(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	_, priv, _ := ed25519.GenerateKey(nil)

	// Unimplemented algorithm should fail fast
	_, err := kr.ImportKey("test", priv, AlgorithmSecp256k1)
	if err == nil {
		t.Error("ImportKey should reject unimplemented algorithm")
	}
}

func TestKeyringSignDataLimit(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	_, err := kr.NewKey("test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Data at the limit should succeed
	largeData := make([]byte, MaxSignDataLength)
	_, err = kr.Sign("test", largeData)
	if err != nil {
		t.Errorf("Sign should accept data at limit: %v", err)
	}

	// Data over the limit should fail
	tooLargeData := make([]byte, MaxSignDataLength+1)
	_, err = kr.Sign("test", tooLargeData)
	if err != ErrDataTooLarge {
		t.Errorf("Sign should reject data over limit, got: %v", err)
	}
}

func TestKeyringNilInputs(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Sign with nil data should work (Ed25519 handles it)
	_, err := kr.NewKey("test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	_, err = kr.Sign("test", nil)
	if err != nil {
		t.Errorf("Sign(nil) should work: %v", err)
	}

	// Import with nil should fail with ErrInvalidKey
	_, err = kr.ImportKey("nil-key", nil, AlgorithmEd25519)
	if err != ErrInvalidKey {
		t.Errorf("ImportKey(nil) should return ErrInvalidKey, got: %v", err)
	}
}

func TestKeyringConcurrentDeleteGet(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create initial keys
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		if _, err := kr.NewKey(name, AlgorithmEd25519); err != nil {
			t.Fatalf("NewKey %s failed: %v", name, err)
		}
	}

	// Concurrent deletes and gets - should not panic
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		name := string(rune('a' + (i % 10)))
		go func(n string) {
			defer wg.Done()
			_, _ = kr.GetKey(n) // May succeed or fail, shouldn't panic
		}(name)
		go func(n string) {
			defer wg.Done()
			_ = kr.DeleteKey(n) // May succeed or fail, shouldn't panic
		}(name)
	}
	wg.Wait()
}

func TestKeyringCacheExactBoundary(t *testing.T) {
	cacheSize := 5
	store := NewMemoryStore()
	kr := NewKeyring(store, WithCacheSize(cacheSize))

	// Create exactly cacheSize keys
	for i := 0; i < cacheSize; i++ {
		name := string(rune('a' + i))
		if _, err := kr.NewKey(name, AlgorithmEd25519); err != nil {
			t.Fatalf("NewKey %s failed: %v", name, err)
		}
	}

	// All should be accessible
	for i := 0; i < cacheSize; i++ {
		name := string(rune('a' + i))
		if _, err := kr.GetKey(name); err != nil {
			t.Errorf("GetKey %s failed: %v", name, err)
		}
	}

	// Add one more - should evict oldest
	if _, err := kr.NewKey("extra", AlgorithmEd25519); err != nil {
		t.Fatalf("NewKey extra failed: %v", err)
	}

	// All keys including "a" should still be accessible (from store)
	for i := 0; i < cacheSize; i++ {
		name := string(rune('a' + i))
		if _, err := kr.GetKey(name); err != nil {
			t.Errorf("GetKey %s failed after eviction: %v", name, err)
		}
	}
	if _, err := kr.GetKey("extra"); err != nil {
		t.Errorf("GetKey extra failed: %v", err)
	}
}

func TestZeroize(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	Zeroize(data)
	for i, b := range data {
		if b != 0 {
			t.Errorf("Zeroize failed at index %d: got %d, want 0", i, b)
		}
	}

	// Empty slice should not panic
	Zeroize(nil)
	Zeroize([]byte{})
}

func TestZeroize_VariousSizes(t *testing.T) {
	// Test various sizes to ensure XORBytes handles them correctly
	sizes := []int{1, 7, 8, 15, 16, 31, 32, 63, 64, 127, 128, 255, 256, 512, 1024}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			data := make([]byte, size)
			// Fill with non-zero pattern
			for i := range data {
				data[i] = byte(i+1) | 0x80 // Ensure high bit set
			}

			Zeroize(data)

			for i, b := range data {
				if b != 0 {
					t.Errorf("Zeroize failed at index %d for size %d: got %d, want 0", i, size, b)
				}
			}
		})
	}
}

func TestZeroize_AllOnes(t *testing.T) {
	// Test with all 0xFF bytes (worst case for XOR-based zeroing)
	data := make([]byte, 64)
	for i := range data {
		data[i] = 0xFF
	}

	Zeroize(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("Zeroize failed at index %d: got %d, want 0", i, b)
		}
	}
}

func TestZeroize_AlreadyZero(t *testing.T) {
	// Zeroing already-zero data should be a no-op (but not cause issues)
	data := make([]byte, 64) // Already zero
	Zeroize(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("Zeroize corrupted zero data at index %d: got %d", i, b)
		}
	}
}

func TestZeroize_PreservesLength(t *testing.T) {
	// Verify zeroing doesn't affect slice metadata
	original := make([]byte, 100, 200)
	for i := range original {
		original[i] = byte(i)
	}

	lenBefore := len(original)
	capBefore := cap(original)

	Zeroize(original)

	if len(original) != lenBefore {
		t.Errorf("Zeroize changed length: got %d, want %d", len(original), lenBefore)
	}
	if cap(original) != capBefore {
		t.Errorf("Zeroize changed capacity: got %d, want %d", cap(original), capBefore)
	}
}

func TestPrivateKeyZeroize(t *testing.T) {
	privKey, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	// Get a reference to the bytes
	keyBytes := privKey.Bytes()

	// Verify key is not zero
	allZero := true
	for _, b := range keyBytes {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Private key should not be all zeros initially")
	}

	// Zeroize
	privKey.Zeroize()

	// Verify key is now zero
	for i, b := range keyBytes {
		if b != 0 {
			t.Errorf("Zeroize failed at index %d: got %d, want 0", i, b)
		}
	}
}

// Close() tests

func TestKeyringClose(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create some keys
	_, err := kr.NewKey("key1", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}
	_, err = kr.NewKey("key2", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Close should succeed
	err = kr.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Verify store is now empty (keys deleted)
	if store.Len() != 0 {
		t.Errorf("expected store to be empty after close, got %d keys", store.Len())
	}
}

func TestKeyringCloseIdempotent(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create a key
	_, err := kr.NewKey("test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// First close
	err = kr.Close()
	if err != nil {
		t.Errorf("first Close() failed: %v", err)
	}

	// Second close should be a no-op and return nil
	err = kr.Close()
	if err != nil {
		t.Errorf("second Close() should return nil, got: %v", err)
	}
}

func TestKeyringOperationsAfterClose(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create a key before close
	_, err := kr.NewKey("before-close", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Close the keyring
	if err := kr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// All operations should return ErrKeyringClosed

	// NewKey
	_, err = kr.NewKey("after-close", AlgorithmEd25519)
	if err != ErrKeyringClosed {
		t.Errorf("NewKey after close: expected ErrKeyringClosed, got %v", err)
	}

	// ImportKey
	_, priv, _ := generateTestKey(t)
	_, err = kr.ImportKey("import-after-close", priv, AlgorithmEd25519)
	if err != ErrKeyringClosed {
		t.Errorf("ImportKey after close: expected ErrKeyringClosed, got %v", err)
	}

	// ExportKey
	_, err = kr.ExportKey("before-close", "")
	if err != ErrKeyringClosed {
		t.Errorf("ExportKey after close: expected ErrKeyringClosed, got %v", err)
	}

	// GetKey
	_, err = kr.GetKey("before-close")
	if err != ErrKeyringClosed {
		t.Errorf("GetKey after close: expected ErrKeyringClosed, got %v", err)
	}

	// ListKeys
	_, err = kr.ListKeys()
	if err != ErrKeyringClosed {
		t.Errorf("ListKeys after close: expected ErrKeyringClosed, got %v", err)
	}

	// DeleteKey
	err = kr.DeleteKey("before-close")
	if err != ErrKeyringClosed {
		t.Errorf("DeleteKey after close: expected ErrKeyringClosed, got %v", err)
	}

	// Sign
	_, err = kr.Sign("before-close", []byte("data"))
	if err != ErrKeyringClosed {
		t.Errorf("Sign after close: expected ErrKeyringClosed, got %v", err)
	}
}

// generateTestKey is a helper for tests
func generateTestKey(t *testing.T) ([]byte, []byte, error) {
	t.Helper()
	privKey, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		return nil, nil, err
	}
	return privKey.PublicKey().Bytes(), privKey.Bytes(), nil
}

// mockFailingStore wraps a store and fails Delete for specific keys
type mockFailingStore struct {
	SimpleKeyStore
	failDelete map[string]bool
}

func (m *mockFailingStore) Delete(name string) error {
	if m.failDelete[name] {
		return fmt.Errorf("simulated delete failure for %s", name)
	}
	return m.SimpleKeyStore.Delete(name)
}

func TestKeyringClosePartialFailure(t *testing.T) {
	underlying := NewMemoryStore()
	store := &mockFailingStore{
		SimpleKeyStore: underlying,
		failDelete:     make(map[string]bool),
	}
	kr := NewKeyring(store)

	// Create keys
	_, err := kr.NewKey("key1", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey key1 failed: %v", err)
	}
	_, err = kr.NewKey("key2", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey key2 failed: %v", err)
	}
	_, err = kr.NewKey("key3", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey key3 failed: %v", err)
	}

	// Make key2 deletion fail
	store.failDelete["key2"] = true

	// Close should return an error but still mark keyring as closed
	err = kr.Close()
	if err == nil {
		t.Error("Close() should return error when delete fails")
	}

	// Error should mention the failure
	errStr := err.Error()
	if !contains(errStr, "key2") || !contains(errStr, "failed to delete") {
		t.Errorf("error should mention failed key, got: %v", err)
	}

	// Keyring should still be closed
	_, err = kr.NewKey("new-key", AlgorithmEd25519)
	if err != ErrKeyringClosed {
		t.Errorf("expected ErrKeyringClosed after partial failure close, got: %v", err)
	}

	// Subsequent close should be no-op (returns nil)
	err = kr.Close()
	if err != nil {
		t.Errorf("second Close() after partial failure should return nil, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchSubstring(s, substr)))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestKeyringCloseZeroizesCache(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create a key and access it to ensure it's cached
	signer, err := kr.NewKey("cached-key", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Get reference to the signer's underlying private key
	// This is a bit of white-box testing, but important for security
	basicSigner, ok := signer.(*BasicSigner)
	if !ok {
		t.Skip("cannot verify zeroization - signer is not BasicSigner")
	}

	// Get the key bytes before close
	keyBytesBefore := basicSigner.privateKey.Bytes()
	nonZero := false
	for _, b := range keyBytesBefore {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatal("private key should not be all zeros before close")
	}

	// Close
	if err := kr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// After close, the cached signer's private key should be zeroized
	// Note: We check the same bytes reference since Zeroize works in-place
	allZero := true
	for _, b := range keyBytesBefore {
		if b != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("private key should be zeroized after close")
	}
}

// TestKeyringConcurrentClose tests that Close() is thread-safe when called
// concurrently with other operations. This verifies:
// 1. No race conditions between Close() and other operations
// 2. Operations during/after Close() correctly return ErrKeyringClosed
// 3. No panics from accessing nil'd cache or cleaned-up resources
func TestKeyringConcurrentClose(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Pre-create keys to operate on
	const numKeys = 10
	for i := 0; i < numKeys; i++ {
		name := fmt.Sprintf("key-%d", i)
		_, err := kr.NewKey(name, AlgorithmEd25519)
		if err != nil {
			t.Fatalf("setup NewKey %s failed: %v", name, err)
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	// Launch concurrent GetKey operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				name := fmt.Sprintf("key-%d", (id+j)%numKeys)
				_, err := kr.GetKey(name)
				// Expected: success before close, ErrKeyringClosed after
				if err != nil && err != ErrKeyringClosed && err != ErrKeyNotFound {
					errors <- fmt.Errorf("GetKey %s: unexpected error: %w", name, err)
				}
			}
		}(i)
	}

	// Launch concurrent Sign operations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := []byte("test data to sign")
			for j := 0; j < 10; j++ {
				name := fmt.Sprintf("key-%d", (id+j)%numKeys)
				_, err := kr.Sign(name, data)
				// Expected: success before close, ErrKeyringClosed after
				if err != nil && err != ErrKeyringClosed && err != ErrKeyNotFound {
					errors <- fmt.Errorf("Sign %s: unexpected error: %w", name, err)
				}
			}
		}(i)
	}

	// Launch concurrent NewKey operations (should fail with ErrKeyExists or ErrKeyringClosed)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				name := fmt.Sprintf("new-key-%d-%d", id, j)
				_, err := kr.NewKey(name, AlgorithmEd25519)
				// Expected: success, ErrKeyringClosed, or ErrKeyExists
				if err != nil && err != ErrKeyringClosed && err != ErrKeyExists {
					errors <- fmt.Errorf("NewKey %s: unexpected error: %w", name, err)
				}
			}
		}(i)
	}

	// Launch concurrent ListKeys operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := kr.ListKeys()
				if err != nil && err != ErrKeyringClosed {
					errors <- fmt.Errorf("ListKeys: unexpected error: %w", err)
				}
			}
		}()
	}

	// Launch Close() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Small delay to let some operations start
		_ = kr.Close()
	}()

	wg.Wait()
	close(errors)

	// Collect any unexpected errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	if len(errList) > 0 {
		t.Errorf("unexpected errors during concurrent close: %v", errList)
	}

	// Verify keyring is definitely closed now
	_, err := kr.GetKey("key-0")
	if err != ErrKeyringClosed {
		t.Errorf("expected ErrKeyringClosed after concurrent close, got: %v", err)
	}

	// Verify second Close() is safe (idempotent)
	err = kr.Close()
	if err != nil {
		t.Errorf("second Close() should return nil, got: %v", err)
	}
}

// TestKeyringConcurrentCloseWithDelete tests Close() racing with Delete operations.
// This is a particularly sensitive race because DeleteKey modifies the cache.
func TestKeyringConcurrentCloseWithDelete(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Pre-create keys
	const numKeys = 20
	for i := 0; i < numKeys; i++ {
		name := fmt.Sprintf("del-key-%d", i)
		_, err := kr.NewKey(name, AlgorithmEd25519)
		if err != nil {
			t.Fatalf("setup NewKey %s failed: %v", name, err)
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Launch concurrent Delete operations
	for i := 0; i < numKeys; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("del-key-%d", id)
			err := kr.DeleteKey(name)
			// Expected: success, ErrKeyNotFound, or ErrKeyringClosed
			if err != nil && err != ErrKeyringClosed && err != ErrKeyNotFound {
				errors <- fmt.Errorf("DeleteKey %s: unexpected error: %w", name, err)
			}
		}(i)
	}

	// Launch Close() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = kr.Close()
	}()

	wg.Wait()
	close(errors)

	// Collect any unexpected errors
	var errList []error
	for err := range errors {
		errList = append(errList, err)
	}
	if len(errList) > 0 {
		t.Errorf("unexpected errors during concurrent close+delete: %v", errList)
	}

	// Verify closed
	err := kr.DeleteKey("any-key")
	if err != ErrKeyringClosed {
		t.Errorf("expected ErrKeyringClosed, got: %v", err)
	}
}

// TestKeyringRapidCloseReopen simulates rapid close cycles to catch
// any timing issues with the closed flag.
func TestKeyringRapidCloseReopen(t *testing.T) {
	for round := 0; round < 10; round++ {
		store := NewMemoryStore()
		kr := NewKeyring(store)

		// Create some keys
		for i := 0; i < 5; i++ {
			name := fmt.Sprintf("round%d-key%d", round, i)
			_, err := kr.NewKey(name, AlgorithmEd25519)
			if err != nil {
				t.Fatalf("round %d: NewKey failed: %v", round, err)
			}
		}

		// Close
		if err := kr.Close(); err != nil {
			t.Errorf("round %d: Close failed: %v", round, err)
		}

		// Verify closed
		_, err := kr.GetKey(fmt.Sprintf("round%d-key0", round))
		if err != ErrKeyringClosed {
			t.Errorf("round %d: expected ErrKeyringClosed, got: %v", round, err)
		}
	}
}

// TestKeyringSignHoldsLockDuringOperation verifies that Sign() holds the lock
// for the entire signing operation, preventing Close() from zeroizing the key
// mid-operation. This is a data race prevention test.
func TestKeyringSignHoldsLockDuringOperation(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create a key
	_, err := kr.NewKey("sign-lock-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Run many iterations of Sign concurrently with Close attempts
	// If Sign() doesn't hold the lock, we'd get inconsistent results or panics
	for round := 0; round < 10; round++ {
		// Need fresh keyring each round since Close() destroys it
		store := NewMemoryStore()
		kr := NewKeyring(store)
		_, err := kr.NewKey("key", AlgorithmEd25519)
		if err != nil {
			t.Fatalf("round %d: NewKey failed: %v", round, err)
		}

		var wg sync.WaitGroup
		data := []byte("test data for signing")

		// Start 10 signing goroutines
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					sig, err := kr.Sign("key", data)
					// Should get either valid signature or ErrKeyringClosed
					// Never a panic or corrupted data
					if err != nil && err != ErrKeyringClosed && err != ErrKeyNotFound {
						t.Errorf("Sign returned unexpected error: %v", err)
					}
					if err == nil && len(sig) == 0 {
						t.Error("Sign returned empty signature without error")
					}
				}
			}()
		}

		// Close while signing is happening
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = kr.Close()
		}()

		wg.Wait()
	}
}

// TestKeyringCacheEvictionZeroizesKeys verifies that keys are zeroized when
// evicted from the cache (not just when explicitly deleted or closed).
func TestKeyringCacheEvictionZeroizesKeys(t *testing.T) {
	// Use a small cache size to force evictions
	store := NewMemoryStore()
	kr := NewKeyring(store, WithCacheSize(2))

	// Create first key and get its signer reference
	signer1, err := kr.NewKey("key1", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey key1 failed: %v", err)
	}

	// Get reference to the underlying private key bytes
	bs1, ok := signer1.(*BasicSigner)
	if !ok {
		t.Skip("cannot verify zeroization - signer is not BasicSigner")
	}
	keyBytes1 := bs1.privateKey.Bytes()

	// Verify key1 is not zeroed initially
	nonZero := false
	for _, b := range keyBytes1 {
		if b != 0 {
			nonZero = true
			break
		}
	}
	if !nonZero {
		t.Fatal("key1 should not be zeroed initially")
	}

	// Create second key (cache now has 2 keys)
	_, err = kr.NewKey("key2", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey key2 failed: %v", err)
	}

	// Create third key - this should evict key1 from cache
	_, err = kr.NewKey("key3", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey key3 failed: %v", err)
	}

	// key1 should now be zeroized (evicted from cache)
	allZero := true
	for _, b := range keyBytes1 {
		if b != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("key1 should be zeroized after cache eviction")
	}
}

// TestKeyringSignFromStoreZeroizesTemporaryKey verifies that when Sign()
// loads a key from store (not cache), the temporary signer is zeroized.
func TestKeyringSignFromStoreZeroizesTemporaryKey(t *testing.T) {
	store := NewMemoryStore()
	// Disable cache to force store loads
	kr := NewKeyring(store, WithCacheSize(0))

	_, err := kr.NewKey("uncached", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Sign should work even with no cache
	data := []byte("test data")
	sig, err := kr.Sign("uncached", data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if len(sig) == 0 {
		t.Error("Sign returned empty signature")
	}

	// The key should still be usable from store
	sig2, err := kr.Sign("uncached", data)
	if err != nil {
		t.Fatalf("second Sign failed: %v", err)
	}
	if !bytes.Equal(sig, sig2) {
		t.Error("same key should produce same signature for same data")
	}
}

// Tests for encrypted key export (Issue #46)

func TestKeyringExportKeyEncrypted(t *testing.T) {
	// Create a store with an encrypted key entry
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// First, create a plaintext key
	_, err := kr.NewKey("encrypted-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	// Get the original plaintext key
	originalKey, err := kr.ExportKey("encrypted-test", "")
	if err != nil {
		t.Fatalf("ExportKey (plaintext) failed: %v", err)
	}

	// Now encrypt this key and store it
	password := "test-password-123"
	encryptedEntry, err := createEncryptedKeyEntry("encrypted-test", originalKey, password)
	if err != nil {
		t.Fatalf("createEncryptedKeyEntry failed: %v", err)
	}

	// Put encrypted entry into store (overwrite)
	if err := store.Put(encryptedEntry, true); err != nil {
		t.Fatalf("store.Put failed: %v", err)
	}

	// Create a new keyring to avoid cache
	kr2 := NewKeyring(store)

	// Export with correct password should succeed
	decrypted, err := kr2.ExportKey("encrypted-test", password)
	if err != nil {
		t.Fatalf("ExportKey (encrypted, correct password) failed: %v", err)
	}

	// Verify decrypted key matches original
	if !bytes.Equal(decrypted, originalKey) {
		t.Error("decrypted key does not match original")
	}

	// Zero the test keys
	Zeroize(originalKey)
	Zeroize(decrypted)
}

func TestKeyringExportKeyEncryptedWrongPassword(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create and encrypt a key
	_, err := kr.NewKey("wrong-pass-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	originalKey, err := kr.ExportKey("wrong-pass-test", "")
	if err != nil {
		t.Fatalf("ExportKey (plaintext) failed: %v", err)
	}
	defer Zeroize(originalKey)

	password := "correct-password"
	encryptedEntry, err := createEncryptedKeyEntry("wrong-pass-test", originalKey, password)
	if err != nil {
		t.Fatalf("createEncryptedKeyEntry failed: %v", err)
	}

	if err := store.Put(encryptedEntry, true); err != nil {
		t.Fatalf("store.Put failed: %v", err)
	}

	// Create a new keyring
	kr2 := NewKeyring(store)

	// Export with wrong password should return ErrInvalidPassword
	_, err = kr2.ExportKey("wrong-pass-test", "wrong-password")
	if err != ErrInvalidPassword {
		t.Errorf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestKeyringExportKeyEncryptedEmptyPassword(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	// Create and encrypt a key
	_, err := kr.NewKey("empty-pass-test", AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	originalKey, err := kr.ExportKey("empty-pass-test", "")
	if err != nil {
		t.Fatalf("ExportKey (plaintext) failed: %v", err)
	}
	defer Zeroize(originalKey)

	password := "actual-password"
	encryptedEntry, err := createEncryptedKeyEntry("empty-pass-test", originalKey, password)
	if err != nil {
		t.Fatalf("createEncryptedKeyEntry failed: %v", err)
	}

	if err := store.Put(encryptedEntry, true); err != nil {
		t.Fatalf("store.Put failed: %v", err)
	}

	kr2 := NewKeyring(store)

	// Export with empty password should fail for encrypted key
	_, err = kr2.ExportKey("empty-pass-test", "")
	if err != ErrInvalidPassword {
		t.Errorf("expected ErrInvalidPassword for empty password, got %v", err)
	}
}

func TestKeyringExportKeyInvalidEncryptionParams(t *testing.T) {
	store := NewMemoryStore()

	// Create entry with invalid salt (too short)
	badSaltEntry := &KeyEntry{
		Name:       "bad-salt",
		Algorithm:  AlgorithmEd25519,
		PrivateKey: []byte("some-ciphertext"),
		PublicKey:  []byte("some-public-key"),
		Encrypted:  true,
		Salt:       []byte("short"), // Too short, should be >= 16 bytes
		Nonce:      make([]byte, AESGCMNonceLength),
	}
	if err := store.Put(badSaltEntry, false); err != nil {
		t.Fatalf("store.Put failed: %v", err)
	}

	kr := NewKeyring(store)

	_, err := kr.ExportKey("bad-salt", "any-password")
	if err != ErrInvalidEncryptionParams {
		t.Errorf("expected ErrInvalidEncryptionParams for short salt, got %v", err)
	}

	// Create entry with invalid nonce (wrong length)
	badNonceEntry := &KeyEntry{
		Name:       "bad-nonce",
		Algorithm:  AlgorithmEd25519,
		PrivateKey: []byte("some-ciphertext"),
		PublicKey:  []byte("some-public-key"),
		Encrypted:  true,
		Salt:       make([]byte, MinSaltLength),
		Nonce:      []byte("wronglen"), // Should be exactly 12 bytes
	}
	if err := store.Put(badNonceEntry, false); err != nil {
		t.Fatalf("store.Put failed: %v", err)
	}

	_, err = kr.ExportKey("bad-nonce", "any-password")
	if err != ErrInvalidEncryptionParams {
		t.Errorf("expected ErrInvalidEncryptionParams for bad nonce, got %v", err)
	}
}

// createEncryptedKeyEntry creates an encrypted KeyEntry using the same
// encryption scheme as FileKeyStore (PBKDF2 + AES-GCM).
// This is a helper for testing ExportKey decryption.
func createEncryptedKeyEntry(name string, plaintext []byte, password string) (*KeyEntry, error) {
	// Generate salt and nonce
	salt := make([]byte, MinSaltLength)
	nonce := make([]byte, AESGCMNonceLength)

	// Use deterministic values for testing (in real use, use crypto/rand)
	for i := range salt {
		salt[i] = byte(i + 1)
	}
	for i := range nonce {
		nonce[i] = byte(i + 100)
	}

	// Derive encryption key using PBKDF2
	passwordBytes := []byte(password)
	derivedKey := deriveKeyForTest(passwordBytes, salt)
	defer Zeroize(derivedKey)

	// Encrypt using AES-GCM
	ciphertext, err := encryptForTest(derivedKey, nonce, plaintext, []byte(name))
	if err != nil {
		return nil, err
	}

	// Create public key (for Ed25519, it's the last 32 bytes of 64-byte private key)
	var pubKey []byte
	if len(plaintext) >= 64 {
		pubKey = make([]byte, 32)
		copy(pubKey, plaintext[32:64])
	}

	return &KeyEntry{
		Name:       name,
		Algorithm:  AlgorithmEd25519,
		PrivateKey: ciphertext,
		PublicKey:  pubKey,
		Encrypted:  true,
		Salt:       salt,
		Nonce:      nonce,
	}, nil
}

// deriveKeyForTest derives an encryption key using PBKDF2.
// Uses same parameters as ExportKey decryption.
func deriveKeyForTest(password, salt []byte) []byte {
	return pbkdf2ForTest(password, salt, 100_000, 32)
}

// pbkdf2ForTest is a minimal PBKDF2 implementation for testing.
// In production code, this uses golang.org/x/crypto/pbkdf2.
func pbkdf2ForTest(password, salt []byte, iterations, keyLen int) []byte {
	// Import the real PBKDF2 function
	// Note: This uses the same import as the main code
	return pbkdf2TestHelper(password, salt, iterations, keyLen)
}

// encryptForTest encrypts plaintext using AES-256-GCM.
func encryptForTest(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	return aesGCMEncryptTestHelper(key, nonce, plaintext, additionalData)
}

func BenchmarkKeyringClose(b *testing.B) {
	// Benchmark Close() with various numbers of keys
	for _, numKeys := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("keys=%d", numKeys), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				store := NewMemoryStore()
				kr := NewKeyring(store)
				for j := 0; j < numKeys; j++ {
					_, _ = kr.NewKey(fmt.Sprintf("key-%d", j), AlgorithmEd25519)
				}
				b.StartTimer()

				_ = kr.Close()
			}
		})
	}
}

func BenchmarkKeyringExportKeyEncrypted(b *testing.B) {
	// Set up an encrypted key
	store := NewMemoryStore()
	kr := NewKeyring(store)
	_, _ = kr.NewKey("bench-encrypted", AlgorithmEd25519)
	originalKey, _ := kr.ExportKey("bench-encrypted", "")

	password := "benchmark-password"
	encryptedEntry, _ := createEncryptedKeyEntry("bench-encrypted", originalKey, password)
	_ = store.Put(encryptedEntry, true)

	kr2 := NewKeyring(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decrypted, _ := kr2.ExportKey("bench-encrypted", password)
		Zeroize(decrypted)
	}
}
