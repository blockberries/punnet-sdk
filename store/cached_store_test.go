package store

import (
	"context"
	"testing"
)

type testObject struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

func TestCachedObjectStore_GetSet(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("key1")
	obj := testObject{ID: "1", Data: "test"}

	// Set
	err := store.Set(ctx, key, obj)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get from cache
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != obj.ID || got.Data != obj.Data {
		t.Errorf("expected %+v, got %+v", obj, got)
	}
}

func TestCachedObjectStore_GetNotFound(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()

	_, err := store.Get(ctx, []byte("nonexistent"))
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCachedObjectStore_Delete(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("key1")
	obj := testObject{ID: "1", Data: "test"}

	_ = store.Set(ctx, key, obj)

	err := store.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, key)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestCachedObjectStore_Has(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("key1")
	obj := testObject{ID: "1", Data: "test"}

	has, err := store.Has(ctx, key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if has {
		t.Error("expected Has to return false for nonexistent key")
	}

	_ = store.Set(ctx, key, obj)

	has, err = store.Has(ctx, key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if !has {
		t.Error("expected Has to return true for existing key")
	}
}

func TestCachedObjectStore_Flush(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("key1")
	obj := testObject{ID: "1", Data: "test"}

	// Set in cache
	_ = store.Set(ctx, key, obj)

	// Flush to backing store
	err := store.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify in backing store
	data, err := backing.Get(key)
	if err != nil {
		t.Fatalf("backing store Get failed: %v", err)
	}

	got, err := serializer.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.ID != obj.ID {
		t.Errorf("expected %s, got %s", obj.ID, got.ID)
	}
}

func TestCachedObjectStore_FlushDelete(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("key1")
	obj := testObject{ID: "1", Data: "test"}

	// Set and flush
	_ = store.Set(ctx, key, obj)
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Delete and flush
	_ = store.Delete(ctx, key)
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify deleted from backing store
	_, err := backing.Get(key)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound in backing store, got %v", err)
	}
}

func TestCachedObjectStore_CacheHit(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("key1")
	obj := testObject{ID: "1", Data: "test"}

	// Set and flush
	_ = store.Set(ctx, key, obj)
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Clear cache to force load from backing
	store.cache.Clear()

	// First get - cache miss
	got1, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Second get - cache hit
	got2, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got1.ID != got2.ID {
		t.Error("cached object differs from backing store object")
	}
}

func TestCachedObjectStore_GetBatch(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()

	// Set multiple objects
	objects := map[string]testObject{
		"key1": {ID: "1", Data: "data1"},
		"key2": {ID: "2", Data: "data2"},
		"key3": {ID: "3", Data: "data3"},
	}

	for key, obj := range objects {
		_ = store.Set(ctx, []byte(key), obj)
	}

	// Get batch
	keys := [][]byte{[]byte("key1"), []byte("key2"), []byte("key3")}
	results, err := store.GetBatch(ctx, keys)
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}

	if len(results) != len(objects) {
		t.Errorf("expected %d results, got %d", len(objects), len(results))
	}

	for key, expected := range objects {
		got, ok := results[key]
		if !ok {
			t.Errorf("missing key %s in results", key)
			continue
		}
		if got.ID != expected.ID {
			t.Errorf("for key %s: expected ID %s, got %s", key, expected.ID, got.ID)
		}
	}
}

func TestCachedObjectStore_SetBatch(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()

	// Set batch
	items := map[string]testObject{
		"key1": {ID: "1", Data: "data1"},
		"key2": {ID: "2", Data: "data2"},
	}

	err := store.SetBatch(ctx, items)
	if err != nil {
		t.Fatalf("SetBatch failed: %v", err)
	}

	// Verify
	for key, expected := range items {
		got, err := store.Get(ctx, []byte(key))
		if err != nil {
			t.Fatalf("Get failed for key %s: %v", key, err)
		}
		if got.ID != expected.ID {
			t.Errorf("for key %s: expected ID %s, got %s", key, expected.ID, got.ID)
		}
	}
}

func TestCachedObjectStore_DeleteBatch(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()

	// Set some objects
	keys := [][]byte{[]byte("key1"), []byte("key2"), []byte("key3")}
	for _, key := range keys {
		obj := testObject{ID: string(key), Data: "data"}
		_ = store.Set(ctx, key, obj)
	}

	// Delete batch
	err := store.DeleteBatch(ctx, keys)
	if err != nil {
		t.Fatalf("DeleteBatch failed: %v", err)
	}

	// Verify all deleted
	for _, key := range keys {
		_, err := store.Get(ctx, key)
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound for key %s, got %v", key, err)
		}
	}
}

func TestCachedObjectStore_Iterator(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()

	// Add objects and flush
	for i := 0; i < 5; i++ {
		key := []byte{byte('a' + i)}
		obj := testObject{ID: string(key), Data: "data"}
		_ = store.Set(ctx, key, obj)
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Iterate
	iter, err := store.Iterator(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Iterator failed: %v", err)
	}
	t.Cleanup(func() {
		if err := iter.Close(); err != nil {
			t.Errorf("iterator Close failed: %v", err)
		}
	})

	count := 0
	for iter.Valid() {
		_, err := iter.Value()
		if err != nil {
			t.Fatalf("Value failed: %v", err)
		}
		count++
		_ = iter.Next()
	}

	if count != 5 {
		t.Errorf("expected 5 items, got %d", count)
	}
}

func TestCachedObjectStore_ValidateKey(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	obj := testObject{ID: "1", Data: "test"}

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
			err := store.Set(ctx, tt.key, obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCachedObjectStore_Close(t *testing.T) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 100, 1000)

	ctx := context.Background()

	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	obj := testObject{ID: "1", Data: "test"}
	err = store.Set(ctx, []byte("key"), obj)
	if err == nil {
		t.Error("expected error for Set after close")
	}
}

func TestCachedObjectStore_NilStore(t *testing.T) {
	var store *CachedObjectStore[testObject]
	ctx := context.Background()

	_, err := store.Get(ctx, []byte("key"))
	if err != ErrStoreNil {
		t.Errorf("expected ErrStoreNil, got %v", err)
	}
}

func TestCachedIterator_Close(t *testing.T) {
	backing := NewMemoryStore()
	_ = backing.Set([]byte("key"), []byte(`{"id":"1","data":"test"}`))

	rawIter, _ := backing.Iterator(nil, nil)
	serializer := NewJSONSerializer[testObject]()
	iter := newCachedIterator(rawIter, serializer, false)

	err := iter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// After close, Valid should return false
	if iter.Valid() {
		t.Error("expected iterator to be invalid after close")
	}

	// Operations after close should return error
	_, err = iter.Key()
	if err != ErrIteratorClosed {
		t.Errorf("expected ErrIteratorClosed, got %v", err)
	}

	_, err = iter.Value()
	if err != ErrIteratorClosed {
		t.Errorf("expected ErrIteratorClosed, got %v", err)
	}
}

func BenchmarkCachedObjectStore_Set(b *testing.B) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 10000, 100000)
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	obj := testObject{ID: "bench", Data: "benchmark"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte{byte('a' + i%26)}
		_ = store.Set(ctx, key, obj)
	}
}

func BenchmarkCachedObjectStore_Get_CacheHit(b *testing.B) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 10000, 100000)
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()
	key := []byte("benchmark-key")
	obj := testObject{ID: "bench", Data: "benchmark"}
	_ = store.Set(ctx, key, obj)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, key)
	}
}

func BenchmarkCachedObjectStore_Get_CacheMiss(b *testing.B) {
	backing := NewMemoryStore()
	serializer := NewJSONSerializer[testObject]()
	store := NewCachedObjectStore(backing, serializer, 10000, 100000)
	b.Cleanup(func() {
		if err := store.Close(); err != nil {
			b.Errorf("Close failed: %v", err)
		}
	})

	ctx := context.Background()

	// Pre-populate backing store
	for i := 0; i < 26; i++ {
		key := []byte{byte('a' + i)}
		obj := testObject{ID: string(key), Data: "data"}
		data, _ := serializer.Marshal(obj)
		_ = backing.Set(key, data)
	}

	// Clear cache to force misses
	store.cache.Clear()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte{byte('a' + i%26)}
		_, _ = store.Get(ctx, key)
	}
}
