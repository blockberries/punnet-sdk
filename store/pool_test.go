package store

import (
	"sync"
	"testing"
)

func TestObjectPool_Basic(t *testing.T) {
	pool := NewObjectPool(func() *testStruct {
		return &testStruct{}
	})

	obj := pool.Get()
	if obj == nil {
		t.Fatal("expected non-nil object from pool")
	}

	obj.Value = 42
	pool.Put(obj)

	// Note: sync.Pool doesn't guarantee we get the same object back
	obj2 := pool.Get()
	if obj2 == nil {
		t.Fatal("expected non-nil object from pool")
	}
}

func TestObjectPool_MultipleGetPut(t *testing.T) {
	pool := NewObjectPool(func() int {
		return 0
	})

	// Get and put multiple times
	for i := 0; i < 100; i++ {
		obj := pool.Get()
		pool.Put(obj)
	}
}

func TestObjectPool_Concurrent(t *testing.T) {
	pool := NewObjectPool(func() *testStruct {
		return &testStruct{}
	})

	var wg sync.WaitGroup
	concurrency := 10
	operations := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				obj := pool.Get()
				obj.Value = j
				pool.Put(obj)
			}
		}()
	}

	wg.Wait()
}

func TestObjectPool_PanicNilNew(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil new function")
		}
	}()

	NewObjectPool[int](nil)
}

func TestObjectPool_PanicNilGet(t *testing.T) {
	var pool *ObjectPool[int]

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for Get on nil pool")
		}
	}()

	pool.Get()
}

func TestBufferPool_Basic(t *testing.T) {
	bp := NewBufferPool(1024)

	buf := bp.Get()
	if len(buf) != 1024 {
		t.Errorf("expected buffer size 1024, got %d", len(buf))
	}

	// Use buffer
	copy(buf, []byte("test data"))

	bp.Put(buf)

	// Get another buffer - should be cleared
	buf2 := bp.Get()
	if len(buf2) != 1024 {
		t.Errorf("expected buffer size 1024, got %d", len(buf2))
	}
}

func TestBufferPool_WrongSize(t *testing.T) {
	bp := NewBufferPool(1024)

	buf := make([]byte, 512)
	bp.Put(buf) // Should not be returned to pool

	// Pool should still work
	buf2 := bp.Get()
	if len(buf2) != 1024 {
		t.Errorf("expected buffer size 1024, got %d", len(buf2))
	}
}

func TestBufferPool_Cleared(t *testing.T) {
	bp := NewBufferPool(256)

	buf := bp.Get()
	for i := range buf {
		buf[i] = byte(i)
	}

	bp.Put(buf)

	// Getting from pool should give cleared buffer
	buf2 := bp.Get()
	for i, b := range buf2 {
		if b != 0 {
			t.Errorf("expected cleared buffer at index %d, got %d", i, b)
			break
		}
	}
}

func TestBufferPool_Concurrent(t *testing.T) {
	bp := NewBufferPool(4096)

	var wg sync.WaitGroup
	concurrency := 10
	operations := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				buf := bp.Get()
				copy(buf, []byte("test"))
				bp.Put(buf)
			}
		}()
	}

	wg.Wait()
}

func TestKeyPool_Basic(t *testing.T) {
	kp := NewKeyPool()

	buf := kp.Get()
	if buf == nil {
		t.Fatal("expected non-nil buffer from key pool")
	}
	if len(buf) != 0 {
		t.Errorf("expected empty buffer, got len=%d", len(buf))
	}

	buf = append(buf, []byte("key")...)
	kp.Put(buf)
}

func TestKeyPool_CopyKey(t *testing.T) {
	kp := NewKeyPool()

	original := []byte("test-key")
	copied := kp.CopyKey(original)

	if string(copied) != string(original) {
		t.Error("copied key doesn't match original")
	}

	// Verify it's a copy
	copied[0] = 'X'
	if original[0] == 'X' {
		t.Error("modification to copy affected original")
	}
}

func TestKeyPool_CopyKeyNil(t *testing.T) {
	kp := NewKeyPool()

	copied := kp.CopyKey(nil)
	if copied != nil {
		t.Error("expected nil copy of nil key")
	}
}

func TestKeyPool_Concurrent(t *testing.T) {
	kp := NewKeyPool()

	var wg sync.WaitGroup
	concurrency := 10
	operations := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				buf := kp.Get()
				buf = append(buf, []byte("key")...)
				kp.Put(buf)
			}
		}(i)
	}

	wg.Wait()
}

func TestDefaultPools(t *testing.T) {
	// Test global pools exist and work
	key := DefaultKeyPool.CopyKey([]byte("test"))
	if string(key) != "test" {
		t.Error("DefaultKeyPool failed")
	}

	buf := DefaultBufferPool.Get()
	if len(buf) != 4096 {
		t.Error("DefaultBufferPool wrong size")
	}
	DefaultBufferPool.Put(buf)

	small := SmallBufferPool.Get()
	if len(small) != 256 {
		t.Error("SmallBufferPool wrong size")
	}
	SmallBufferPool.Put(small)

	large := LargeBufferPool.Get()
	if len(large) != 65536 {
		t.Error("LargeBufferPool wrong size")
	}
	LargeBufferPool.Put(large)
}

type testStruct struct {
	Value int
}

func BenchmarkObjectPool_GetPut(b *testing.B) {
	pool := NewObjectPool(func() *testStruct {
		return &testStruct{}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := pool.Get()
		obj.Value = i
		pool.Put(obj)
	}
}

func BenchmarkObjectPool_Concurrent(b *testing.B) {
	pool := NewObjectPool(func() *testStruct {
		return &testStruct{}
	})

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.Get()
			obj.Value = 42
			pool.Put(obj)
		}
	})
}

func BenchmarkBufferPool_GetPut(b *testing.B) {
	bp := NewBufferPool(4096)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := bp.Get()
		bp.Put(buf)
	}
}

func BenchmarkKeyPool_CopyKey(b *testing.B) {
	kp := NewKeyPool()
	key := []byte("test-key-with-reasonable-length")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copied := kp.CopyKey(key)
		kp.Put(copied)
	}
}

func BenchmarkAllocation_WithoutPool(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &testStruct{}
	}
}

func BenchmarkAllocation_WithPool(b *testing.B) {
	pool := NewObjectPool(func() *testStruct {
		return &testStruct{}
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := pool.Get()
		pool.Put(obj)
	}
}
