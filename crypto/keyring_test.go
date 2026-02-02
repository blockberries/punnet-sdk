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
