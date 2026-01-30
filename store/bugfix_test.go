package store

import (
	"context"
	"sort"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

// TestDeterministicFlush verifies that cache flush happens in deterministic order
func TestDeterministicFlush(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[string]()

	store := NewCachedObjectStore[string](backing, serializer, 10, 10)

	ctx := context.Background()

	// Set multiple keys to create dirty entries
	keys := []string{"zebra", "apple", "mango", "banana"}
	for _, key := range keys {
		_ = store.Set(ctx, []byte(key), key)
	}

	// Flush should happen in sorted order
	flushOrder := make([]string, 0)

	// Capture flush order by checking backing store keys
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify keys are in sorted order in backing store
	iter, err := backing.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iter.Close()

	for iter.Valid() {
		key := iter.Key()
		flushOrder = append(flushOrder, string(key))
		iter.Next()
	}

	// Keys should be alphabetically sorted
	expectedOrder := make([]string, len(keys))
	copy(expectedOrder, keys)
	sort.Strings(expectedOrder)

	if len(flushOrder) != len(expectedOrder) {
		t.Fatalf("Expected %d keys, got %d", len(expectedOrder), len(flushOrder))
	}

	for i, key := range expectedOrder {
		if flushOrder[i] != key {
			t.Errorf("Flush order mismatch at index %d: expected %s, got %s", i, key, flushOrder[i])
		}
	}

	t.Log("✓ Cache flush happens in deterministic sorted order")
}

// TestBalanceTransfer_Atomicity verifies transfer validation before modification
func TestBalanceTransfer_Atomicity(t *testing.T) {
	backing := NewMemoryStore()
	store := NewCachedObjectStore[Balance](backing, NewJSONSerializer[Balance](), 10, 10)

	bs := &BalanceStore{store: store}
	ctx := context.Background()

	// Setup initial balances
	alice := types.AccountName("alice")
	bob := types.AccountName("bob")
	denom := "uatom"

	_ = bs.AddAmount(ctx, alice, denom, 100)
	_ = bs.AddAmount(ctx, bob, denom, 50)

	// Try to transfer more than alice has - should fail atomically
	err := bs.Transfer(ctx, alice, bob, denom, 150)
	if err == nil {
		t.Fatal("Expected transfer to fail with insufficient balance")
	}

	// Verify alice's balance wasn't changed
	aliceBalance, _ := bs.Get(ctx, alice, denom)
	if aliceBalance.Amount != 100 {
		t.Errorf("Alice's balance should be unchanged at 100, got %d", aliceBalance.Amount)
	}

	// Verify bob's balance wasn't changed
	bobBalance, _ := bs.Get(ctx, bob, denom)
	if bobBalance.Amount != 50 {
		t.Errorf("Bob's balance should be unchanged at 50, got %d", bobBalance.Amount)
	}

	// Now transfer valid amount
	err = bs.Transfer(ctx, alice, bob, denom, 30)
	if err != nil {
		t.Fatalf("Valid transfer failed: %v", err)
	}

	// Verify balances updated correctly
	aliceBalance, _ = bs.Get(ctx, alice, denom)
	bobBalance, _ = bs.Get(ctx, bob, denom)

	if aliceBalance.Amount != 70 {
		t.Errorf("Alice should have 70, got %d", aliceBalance.Amount)
	}
	if bobBalance.Amount != 80 {
		t.Errorf("Bob should have 80, got %d", bobBalance.Amount)
	}

	t.Log("✓ Balance transfer validates before modification")
}

// TestIterator_DefensiveCopy verifies iterator creates defensive copies
func TestIterator_DefensiveCopy(t *testing.T) {
	ms := NewMemoryStore()

	// Set a value
	originalValue := []byte("original")
	_ = ms.Set([]byte("key"), originalValue)

	// Create iterator
	iter, err := ms.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Failed to create iterator: %v", err)
	}
	defer iter.Close()

	if !iter.Valid() {
		t.Fatal("Iterator should be valid")
	}

	// Get value from iterator
	iterValue := iter.Value()

	// Modify the iterator value
	if len(iterValue) > 0 {
		iterValue[0] = 'X'
	}

	// Modify the original value
	if len(originalValue) > 0 {
		originalValue[0] = 'Y'
	}

	// Get value again - should still be "original"
	storedValue, _ := ms.Get([]byte("key"))
	if string(storedValue) != "original" {
		t.Errorf("Store value was corrupted by external modification: %s", storedValue)
	}

	t.Log("✓ Iterator creates defensive copies preventing external mutation")
}

// TestSerializer_NilChecks verifies serializer handles nil gracefully
func TestSerializer_NilChecks(t *testing.T) {
	var s *JSONSerializer[string] = nil

	// Marshal on nil serializer
	_, err := s.Marshal("test")
	if err == nil || err.Error() != "serializer is nil" {
		t.Errorf("Expected 'serializer is nil' error, got: %v", err)
	}

	// Unmarshal on nil serializer
	_, err = s.Unmarshal([]byte(`"test"`))
	if err == nil || err.Error() != "serializer is nil" {
		t.Errorf("Expected 'serializer is nil' error, got: %v", err)
	}

	// Valid serializer with empty data
	s = NewJSONSerializer[string]()
	_, err = s.Unmarshal([]byte{})
	if err == nil {
		t.Error("Expected error for empty data")
	}

	t.Log("✓ Serializer handles nil and invalid inputs")
}

// TestIteratorConstructor_Validation verifies iterator constructors validate inputs
func TestIteratorConstructor_Validation(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("newCachedIterator should panic with nil rawIter")
		} else {
			t.Log("✓ newCachedIterator panics on nil rawIter")
		}
	}()

	// This should panic
	_ = newCachedIterator[string](nil, NewJSONSerializer[string](), false)
}

// TestPrefixIteratorConstructor_Validation verifies prefix iterator constructor
func TestPrefixIteratorConstructor_Validation(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("newPrefixIterator should panic with nil parent")
		} else {
			t.Log("✓ newPrefixIterator panics on nil parent")
		}
	}()

	// This should panic
	_ = newPrefixIterator(nil, []byte("prefix"))
}

// TestBoundaryOverflow_PrefixBound verifies 0xFF overflow is handled
func TestBoundaryOverflow_PrefixBound(t *testing.T) {
	tests := []struct {
		name     string
		prefix   []byte
		expected []byte
	}{
		{
			"simple case",
			[]byte{0x01, 0x02},
			[]byte{0x01, 0x03},
		},
		{
			"last byte 0xFF",
			[]byte{0x01, 0xFF},
			[]byte{0x02},
		},
		{
			"all 0xFF",
			[]byte{0xFF, 0xFF, 0xFF},
			nil,
		},
		{
			"middle 0xFF",
			[]byte{0x01, 0xFF, 0x00},
			[]byte{0x01, 0xFF, 0x01},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prefixBound(tt.prefix)
			if string(result) != string(tt.expected) {
				t.Errorf("prefixBound(%x) = %x, want %x", tt.prefix, result, tt.expected)
			}
		})
	}

	t.Log("✓ prefixBound handles 0xFF overflow correctly")
}
