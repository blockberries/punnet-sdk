package crypto

import (
	"bytes"
	"crypto/ed25519"
	"sync"
	"testing"
)

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
		kr.DeleteKey(name)
		_, _ = kr.NewKey(name, AlgorithmEd25519)
	}
}

func BenchmarkKeyringGetKeyCached(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)
	kr.NewKey("bench", AlgorithmEd25519)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.GetKey("bench")
	}
}

func BenchmarkKeyringGetKeyUncached(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store, WithCacheSize(0)) // Disable cache
	kr.NewKey("bench", AlgorithmEd25519)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.GetKey("bench")
	}
}

func BenchmarkKeyringSign(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)
	kr.NewKey("bench", AlgorithmEd25519)
	data := []byte("benchmark signing data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.Sign("bench", data)
	}
}

func BenchmarkKeyringSignParallel(b *testing.B) {
	store := NewMemoryStore()
	kr := NewKeyring(store)
	kr.NewKey("bench", AlgorithmEd25519)
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
		kr.NewKey(string(rune('a'+i/26))+string(rune('a'+i%26)), AlgorithmEd25519)
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
	store.Put(entry, false)

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
		store.Put(entry, true) // Overwrite mode
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
		{"", true},                           // empty
		{"valid-key", false},                 // valid
		{"key_with_underscore", false},       // valid
		{"key.with.dots", false},             // valid
		{"../etc/passwd", true},              // path traversal
		{"..\\windows\\system32", true},      // windows path traversal
		{"/absolute/path", true},             // absolute path
		{"key\x00null", true},                // null byte
		{"key\nwith\nnewlines", true},        // control chars
		{string(make([]byte, 300)), true},    // too long
	}

	for _, tt := range tests {
		_, err := kr.NewKey(tt.name, AlgorithmEd25519)
		if (err != nil) != tt.wantErr {
			t.Errorf("NewKey(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
		// Clean up if key was created
		if err == nil {
			kr.DeleteKey(tt.name)
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
			kr.GetKey(n) // May succeed or fail, shouldn't panic
		}(name)
		go func(n string) {
			defer wg.Done()
			kr.DeleteKey(n) // May succeed or fail, shouldn't panic
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
