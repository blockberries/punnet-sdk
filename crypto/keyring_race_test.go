package crypto

import (
	"crypto/ed25519"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// TestKeyringConcurrentNewKeySameName tests that when N goroutines all try to
// create a key with the same name, exactly one succeeds and the rest get ErrKeyExists.
// This is an adversarial test for race conditions in the check-then-set pattern.
func TestKeyringConcurrentNewKeySameName(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	const numGoroutines = 100
	const keyName = "alice"

	var (
		wg           sync.WaitGroup
		successCount atomic.Int32
		existsCount  atomic.Int32
		otherErrors  atomic.Int32
	)

	// Start all goroutines
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			_, err := kr.NewKey(keyName, AlgorithmEd25519)
			if err == nil {
				successCount.Add(1)
			} else if errors.Is(err, ErrKeyExists) {
				existsCount.Add(1)
			} else {
				otherErrors.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	// Verify exactly one goroutine succeeded
	successes := successCount.Load()
	exists := existsCount.Load()
	others := otherErrors.Load()

	if successes != 1 {
		t.Errorf("expected exactly 1 success, got %d (ErrKeyExists: %d, other errors: %d)",
			successes, exists, others)
	}

	if exists != numGoroutines-1 {
		t.Errorf("expected %d ErrKeyExists, got %d", numGoroutines-1, exists)
	}

	// Verify the key exists and is valid
	signer, err := kr.GetKey(keyName)
	if err != nil {
		t.Fatalf("GetKey after concurrent create failed: %v", err)
	}

	// Verify we can sign with the key
	data := []byte("test message")
	sig, err := signer.Sign(data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if !signer.PublicKey().Verify(data, sig) {
		t.Error("signature verification failed")
	}

	// Verify there's exactly one key in the store
	keys, err := kr.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

// TestKeyringConcurrentImportDelete tests concurrent ImportKey and DeleteKey
// operations for the same key name. The test verifies:
// - No panics occur
// - No race conditions detected (run with -race)
// - Operations complete without deadlock
//
// Note: Due to the keyring's cache semantics (see keyring.go comments), GetKey
// may return cached data even after DeleteKey succeeds in the store. This is
// documented as eventual consistency and is the expected behavior.
func TestKeyringConcurrentImportDelete(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	const numIterations = 100
	const keyName = "bob"

	// Generate a valid private key for importing
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	var wg sync.WaitGroup

	// Run multiple rounds of concurrent import/delete
	for round := 0; round < numIterations; round++ {
		wg.Add(2)

		// Goroutine A: ImportKey
		go func() {
			defer wg.Done()
			// May succeed or fail with ErrKeyExists, shouldn't panic or race
			_, _ = kr.ImportKey(keyName, priv, AlgorithmEd25519)
		}()

		// Goroutine B: DeleteKey
		go func() {
			defer wg.Done()
			// May succeed or fail with ErrKeyNotFound, shouldn't panic or race
			_ = kr.DeleteKey(keyName)
		}()

		wg.Wait()

		// Verify the store is in a consistent state
		// Note: We only check store.Has here because GetKey may return cached
		// data that's stale (this is documented as eventual consistency)
		_, hasErr := store.Has(keyName)
		if hasErr != nil {
			t.Errorf("round %d: Has error: %v", round, hasErr)
		}

		// If key exists in store, GetKey must succeed
		// (cache may return stale data, but store is authoritative)
		exists, _ := store.Has(keyName)
		if exists {
			_, getErr := kr.GetKey(keyName)
			if getErr != nil {
				t.Errorf("round %d: key exists in store but GetKey failed: %v", round, getErr)
			}
		}
	}

	// Clean up
	_ = kr.DeleteKey(keyName)
}

// TestKeyringConcurrentImportDeleteSameKey is a more aggressive version that
// spawns many goroutines all racing on the same key.
// This test verifies no panics or races occur under high contention.
func TestKeyringConcurrentImportDeleteSameKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	const numGoroutines = 50
	const keyName = "charlie"

	// Generate valid keys for importing
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Half import, half delete - all at once
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = kr.ImportKey(keyName, priv, AlgorithmEd25519)
		}()
		go func() {
			defer wg.Done()
			_ = kr.DeleteKey(keyName)
		}()
	}

	wg.Wait()

	// Verify store is in consistent state
	exists, hasErr := store.Has(keyName)
	if hasErr != nil {
		t.Fatalf("final Has error: %v", hasErr)
	}

	// If key exists in store, GetKey must work
	// Note: GetKey may succeed even if Has=false due to cache (eventual consistency)
	if exists {
		_, getErr := kr.GetKey(keyName)
		if getErr != nil {
			t.Errorf("key exists in store but GetKey failed: %v", getErr)
		}
	}

	// Clean up
	_ = kr.DeleteKey(keyName)
}

// TestKeyringHighChurnKeyRotation simulates rapid key add/delete while signing.
// This represents a realistic adversarial scenario where keys are being rotated
// while signing operations are in flight.
func TestKeyringHighChurnKeyRotation(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	const (
		numSigners       = 10  // Goroutines signing
		numRotators      = 5   // Goroutines rotating keys
		operationsPerSig = 100 // Operations per signer
		keyBaseName      = "rotating-key"
	)

	// Create initial keys
	for i := 0; i < 5; i++ {
		keyName := keyBaseName + "-" + string(rune('a'+i))
		_, err := kr.NewKey(keyName, AlgorithmEd25519)
		if err != nil {
			t.Fatalf("failed to create initial key %s: %v", keyName, err)
		}
	}

	var (
		wg           sync.WaitGroup
		signSuccess  atomic.Int64
		signNotFound atomic.Int64
		signOther    atomic.Int64
		stopRotation atomic.Bool
	)

	// Signer goroutines
	for i := 0; i < numSigners; i++ {
		wg.Add(1)
		go func(signerID int) {
			defer wg.Done()
			data := []byte("test message from signer")

			for op := 0; op < operationsPerSig; op++ {
				// Try to sign with a rotating key
				keyName := keyBaseName + "-" + string(rune('a'+(op+signerID)%5))
				_, err := kr.Sign(keyName, data)

				if err == nil {
					signSuccess.Add(1)
				} else if errors.Is(err, ErrKeyNotFound) {
					signNotFound.Add(1)
				} else {
					signOther.Add(1)
					t.Errorf("signer %d: unexpected error: %v", signerID, err)
				}
			}
		}(i)
	}

	// Key rotation goroutines
	for i := 0; i < numRotators; i++ {
		wg.Add(1)
		go func(rotatorID int) {
			defer wg.Done()

			rotation := 0
			for !stopRotation.Load() {
				// Rotate keys (delete and recreate)
				keyName := keyBaseName + "-" + string(rune('a'+(rotation+rotatorID)%5))

				// Delete (may fail if already deleted)
				_ = kr.DeleteKey(keyName)

				// Recreate (may fail if someone else created it)
				_, _ = kr.NewKey(keyName, AlgorithmEd25519)

				rotation++
				if rotation > 50 {
					break // Limit rotations
				}
			}
		}(i)
	}

	// Wait for signers to finish first
	for i := 0; i < numSigners; i++ {
		// Signers complete based on their operation count
	}

	// Let it run then stop rotations
	stopRotation.Store(true)
	wg.Wait()

	successes := signSuccess.Load()
	notFounds := signNotFound.Load()
	others := signOther.Load()

	t.Logf("Sign results: success=%d, notFound=%d, otherError=%d",
		successes, notFounds, others)

	// Verify no unexpected errors
	if others > 0 {
		t.Errorf("had %d unexpected sign errors", others)
	}

	// Verify total operations make sense
	totalOps := successes + notFounds + others
	expectedOps := int64(numSigners * operationsPerSig)
	if totalOps != expectedOps {
		t.Errorf("expected %d total operations, got %d", expectedOps, totalOps)
	}
}

// TestKeyringConcurrentNewKeyDifferentNames tests concurrent NewKey with
// different names to ensure no cross-contamination.
func TestKeyringConcurrentNewKeyDifferentNames(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	const numGoroutines = 50

	var wg sync.WaitGroup
	errs := make(chan error, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			keyName := "key-" + string(rune('A'+id/26)) + string(rune('a'+id%26))
			_, err := kr.NewKey(keyName, AlgorithmEd25519)
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	// Collect errors
	var errList []error
	for err := range errs {
		errList = append(errList, err)
	}

	if len(errList) > 0 {
		t.Errorf("got %d errors creating different keys: %v", len(errList), errList)
	}

	// Verify all keys exist
	keys, err := kr.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys failed: %v", err)
	}
	if len(keys) != numGoroutines {
		t.Errorf("expected %d keys, got %d", numGoroutines, len(keys))
	}
}

// TestKeyringConcurrentSignSameKey tests many goroutines signing with the same key.
func TestKeyringConcurrentSignSameKey(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store)

	const keyName = "signing-key"
	const numGoroutines = 100

	signer, err := kr.NewKey(keyName, AlgorithmEd25519)
	if err != nil {
		t.Fatalf("NewKey failed: %v", err)
	}

	pubKey := signer.PublicKey()
	data := []byte("concurrent signing test")

	var wg sync.WaitGroup
	signatures := make(chan []byte, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			sig, err := kr.Sign(keyName, data)
			if err != nil {
				t.Errorf("Sign failed: %v", err)
				return
			}
			signatures <- sig
		}()
	}

	wg.Wait()
	close(signatures)

	// Verify all signatures
	count := 0
	for sig := range signatures {
		if !pubKey.Verify(data, sig) {
			t.Error("signature verification failed")
		}
		count++
	}

	if count != numGoroutines {
		t.Errorf("expected %d signatures, got %d", numGoroutines, count)
	}
}

// TestKeyringConcurrentCacheEviction tests cache behavior under high concurrency.
func TestKeyringConcurrentCacheEviction(t *testing.T) {
	store := NewMemoryStore()
	kr := NewKeyring(store, WithCacheSize(5)) // Small cache to force evictions

	const numKeys = 20
	const numAccessors = 10
	const accessesPerGoroutine = 50

	// Create keys
	for i := 0; i < numKeys; i++ {
		keyName := "cache-key-" + string(rune('a'+i))
		if _, err := kr.NewKey(keyName, AlgorithmEd25519); err != nil {
			t.Fatalf("failed to create key %s: %v", keyName, err)
		}
	}

	var wg sync.WaitGroup
	var errors atomic.Int32

	// Concurrent accessors hitting different keys
	wg.Add(numAccessors)
	for i := 0; i < numAccessors; i++ {
		go func(accessorID int) {
			defer wg.Done()
			for j := 0; j < accessesPerGoroutine; j++ {
				keyIdx := (accessorID + j) % numKeys
				keyName := "cache-key-" + string(rune('a'+keyIdx))

				_, err := kr.GetKey(keyName)
				if err != nil {
					errors.Add(1)
					t.Errorf("accessor %d: GetKey %s failed: %v", accessorID, keyName, err)
				}
			}
		}(i)
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("had %d errors during concurrent cache access", errors.Load())
	}
}
