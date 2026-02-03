package store

import (
	"bytes"
	"sync"
	"testing"
)

func TestMemoryStore_GetSet(t *testing.T) {
	ms := NewMemoryStore()

	key := []byte("key1")
	value := []byte("value1")

	err := ms.Set(key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := ms.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(got, value) {
		t.Errorf("expected %s, got %s", value, got)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	ms := NewMemoryStore()

	_, err := ms.Get([]byte("nonexistent"))
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	ms := NewMemoryStore()

	key := []byte("key1")
	value := []byte("value1")

	_ = ms.Set(key, value)

	err := ms.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = ms.Get(key)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemoryStore_Has(t *testing.T) {
	ms := NewMemoryStore()

	key := []byte("key1")
	value := []byte("value1")

	has, err := ms.Has(key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if has {
		t.Error("expected Has to return false for nonexistent key")
	}

	_ = ms.Set(key, value)

	has, err = ms.Has(key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if !has {
		t.Error("expected Has to return true for existing key")
	}
}

func TestMemoryStore_DefensiveCopy(t *testing.T) {
	ms := NewMemoryStore()

	key := []byte("key1")
	value := []byte("value1")

	_ = ms.Set(key, value)

	// Modify original
	value[0] = 'X'

	got, err := ms.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got[0] == 'X' {
		t.Error("store did not make defensive copy of value")
	}
}

func TestMemoryStore_Iterator(t *testing.T) {
	ms := NewMemoryStore()

	// Add some keys
	keys := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
	}

	for _, key := range keys {
		_ = ms.Set(key, key)
	}

	iter, err := ms.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}
	t.Cleanup(func() {
		if err := iter.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	count := 0
	for iter.Valid() {
		count++
		iter.Next()
	}

	if count != len(keys) {
		t.Errorf("expected %d items, got %d", len(keys), count)
	}
}

func TestMemoryStore_IteratorRange(t *testing.T) {
	ms := NewMemoryStore()

	// Add keys a-z
	for i := 0; i < 26; i++ {
		key := []byte{byte('a' + i)}
		_ = ms.Set(key, key)
	}

	// Iterate from 'd' to 'h' (exclusive)
	start := []byte("d")
	end := []byte("h")

	iter, err := ms.Iterator(start, end)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}
	t.Cleanup(func() {
		if err := iter.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	expected := []string{"d", "e", "f", "g"}
	got := make([]string, 0)

	for iter.Valid() {
		key := iter.Key()
		got = append(got, string(key))
		iter.Next()
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(got))
	}

	for i, exp := range expected {
		if got[i] != exp {
			t.Errorf("at index %d: expected %s, got %s", i, exp, got[i])
		}
	}
}

func TestMemoryStore_ReverseIterator(t *testing.T) {
	ms := NewMemoryStore()

	keys := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
	}

	for _, key := range keys {
		_ = ms.Set(key, key)
	}

	iter, err := ms.ReverseIterator(nil, nil)
	if err != nil {
		t.Fatalf("ReverseIterator failed: %v", err)
	}
	t.Cleanup(func() {
		if err := iter.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	expected := []string{"c", "b", "a"}
	got := make([]string, 0)

	for iter.Valid() {
		key := iter.Key()
		got = append(got, string(key))
		iter.Next()
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(got))
	}

	for i, exp := range expected {
		if got[i] != exp {
			t.Errorf("at index %d: expected %s, got %s", i, exp, got[i])
		}
	}
}

func TestMemoryStore_IteratorValue(t *testing.T) {
	ms := NewMemoryStore()

	key := []byte("key1")
	value := []byte("value1")
	_ = ms.Set(key, value)

	iter, err := ms.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}
	t.Cleanup(func() {
		if err := iter.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	if !iter.Valid() {
		t.Fatal("expected iterator to be valid")
	}

	gotValue := iter.Value()
	if !bytes.Equal(gotValue, value) {
		t.Errorf("expected %s, got %s", value, gotValue)
	}
}

func TestMemoryStore_IteratorDefensiveCopy(t *testing.T) {
	ms := NewMemoryStore()

	key := []byte("key1")
	value := []byte("value1")
	_ = ms.Set(key, value)

	iter, err := ms.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}
	t.Cleanup(func() {
		if err := iter.Close(); err != nil {
			t.Errorf("iter Close failed: %v", err)
		}
	})

	gotKey := iter.Key()
	gotValue := iter.Value()

	// Modify returned values
	gotKey[0] = 'X'
	gotValue[0] = 'X'

	// Get again - should be unchanged
	iter2, _ := ms.Iterator(nil, nil)
	t.Cleanup(func() {
		if err := iter2.Close(); err != nil {
			t.Errorf("iter2 Close failed: %v", err)
		}
	})

	key2 := iter2.Key()
	value2 := iter2.Value()

	if key2[0] == 'X' {
		t.Error("iterator did not make defensive copy of key")
	}
	if value2[0] == 'X' {
		t.Error("iterator did not make defensive copy of value")
	}
}

func TestMemoryStore_ValidateKey(t *testing.T) {
	ms := NewMemoryStore()

	tests := []struct {
		name    string
		key     []byte
		wantErr bool
	}{
		{"valid key", []byte("key"), false},
		{"nil key", nil, true},
		{"empty key", []byte{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ms.Set(tt.key, []byte("value"))
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
			}

			_, err = ms.Get(tt.key)
			if (err != nil) != tt.wantErr {
				if err != ErrNotFound { // Get might return ErrNotFound for invalid keys
					t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestMemoryStore_Concurrent(t *testing.T) {
	ms := NewMemoryStore()

	var wg sync.WaitGroup
	concurrency := 10
	operations := 100

	// Concurrent writes
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				key := []byte{byte('a' + (id*operations+j)%26)}
				value := []byte{byte('A' + (id*operations+j)%26)}
				_ = ms.Set(key, value)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				key := []byte{byte('a' + j%26)}
				_, _ = ms.Get(key)
			}
		}()
	}

	wg.Wait()
}

func TestMemoryIterator_Close(t *testing.T) {
	ms := NewMemoryStore()
	_ = ms.Set([]byte("key"), []byte("value"))

	iter, err := ms.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}

	err = iter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// After close, Valid should return false
	if iter.Valid() {
		t.Error("expected iterator to be invalid after close")
	}

	// Close again should be safe
	err = iter.Close()
	if err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

func TestMemoryIterator_Error(t *testing.T) {
	ms := NewMemoryStore()
	_ = ms.Set([]byte("key"), []byte("value"))

	iter, err := ms.Iterator(nil, nil)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}

	// Error should be nil for memory iterator
	if iter.Error() != nil {
		t.Errorf("expected no error, got %v", iter.Error())
	}

	iter.Close()

	// After close, error should indicate closed
	if iter.Error() != ErrIteratorClosed {
		t.Errorf("expected ErrIteratorClosed, got %v", iter.Error())
	}
}

func BenchmarkMemoryStore_Set(b *testing.B) {
	ms := NewMemoryStore()
	key := []byte("benchmark-key")
	value := []byte("benchmark-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ms.Set(key, value)
	}
}

func BenchmarkMemoryStore_Get(b *testing.B) {
	ms := NewMemoryStore()
	key := []byte("benchmark-key")
	value := []byte("benchmark-value")
	_ = ms.Set(key, value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ms.Get(key)
	}
}

func BenchmarkMemoryStore_Concurrent(b *testing.B) {
	ms := NewMemoryStore()

	// Pre-populate
	for i := 0; i < 100; i++ {
		key := []byte{byte('a' + i%26)}
		_ = ms.Set(key, key)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []byte{byte('a' + i%26)}
			if i%2 == 0 {
				_ = ms.Set(key, key)
			} else {
				_, _ = ms.Get(key)
			}
			i++
		}
	})
}
