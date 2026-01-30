package store

import (
	"sync"
)

// ObjectPool provides pooling for objects to reduce allocations
// Uses sync.Pool for efficient memory reuse
type ObjectPool[T any] struct {
	pool *sync.Pool
	new  func() T
}

// NewObjectPool creates a new object pool with a constructor function
// The new function is called when the pool needs to create a new object
func NewObjectPool[T any](new func() T) *ObjectPool[T] {
	if new == nil {
		panic("new function cannot be nil")
	}

	pool := &ObjectPool[T]{
		new: new,
	}

	pool.pool = &sync.Pool{
		New: func() interface{} {
			return pool.new()
		},
	}

	return pool
}

// Get retrieves an object from the pool
// If the pool is empty, a new object is created using the constructor
func (p *ObjectPool[T]) Get() T {
	if p == nil || p.pool == nil {
		panic("cannot get from nil pool")
	}

	return p.pool.Get().(T)
}

// Put returns an object to the pool for reuse
// The object should be reset to a clean state before being returned
// Callers are responsible for resetting the object
func (p *ObjectPool[T]) Put(obj T) {
	if p == nil || p.pool == nil {
		return
	}

	// Note: Caller must reset the object before calling Put
	// This is critical for correctness - pooled objects must not
	// retain state from previous uses
	p.pool.Put(obj)
}

// BufferPool is a specialized pool for byte slices
type BufferPool struct {
	pool *sync.Pool
	size int
}

// NewBufferPool creates a new buffer pool with fixed-size buffers
func NewBufferPool(size int) *BufferPool {
	if size <= 0 {
		size = 4096 // Default 4KB buffers
	}

	bp := &BufferPool{
		size: size,
	}

	bp.pool = &sync.Pool{
		New: func() interface{} {
			buf := make([]byte, bp.size)
			return &buf
		},
	}

	return bp
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() []byte {
	if bp == nil || bp.pool == nil {
		return make([]byte, 4096)
	}

	bufPtr := bp.pool.Get().(*[]byte)
	return (*bufPtr)[:bp.size]
}

// Put returns a buffer to the pool
// The buffer will be reused, so callers should not retain references
func (bp *BufferPool) Put(buf []byte) {
	if bp == nil || bp.pool == nil {
		return
	}

	if len(buf) != bp.size {
		// Buffer is wrong size, don't return to pool
		return
	}

	// Clear the buffer before returning to pool
	for i := range buf {
		buf[i] = 0
	}

	bp.pool.Put(&buf)
}

// KeyPool is a specialized pool for key byte slices
// Used for defensive key copies
type KeyPool struct {
	pool *sync.Pool
}

// NewKeyPool creates a new key pool
func NewKeyPool() *KeyPool {
	kp := &KeyPool{}
	kp.pool = &sync.Pool{
		New: func() interface{} {
			// Pre-allocate reasonable key size (64 bytes)
			buf := make([]byte, 0, 64)
			return &buf
		},
	}
	return kp
}

// Get retrieves a key buffer from the pool
func (kp *KeyPool) Get() []byte {
	if kp == nil || kp.pool == nil {
		return make([]byte, 0, 64)
	}

	bufPtr := kp.pool.Get().(*[]byte)
	*bufPtr = (*bufPtr)[:0] // Reset length to 0
	return *bufPtr
}

// Put returns a key buffer to the pool
func (kp *KeyPool) Put(buf []byte) {
	if kp == nil || kp.pool == nil {
		return
	}

	// Reset the buffer
	buf = buf[:0]
	kp.pool.Put(&buf)
}

// CopyKey creates a defensive copy of a key using the pool
func (kp *KeyPool) CopyKey(key []byte) []byte {
	if key == nil {
		return nil
	}

	buf := kp.Get()
	buf = append(buf, key...)
	return buf
}

// Iterator pool for reusing iterator objects
type IteratorPool[T any] struct {
	pool *sync.Pool
	new  func() Iterator[T]
}

// NewIteratorPool creates a new iterator pool
func NewIteratorPool[T any](new func() Iterator[T]) *IteratorPool[T] {
	if new == nil {
		panic("new function cannot be nil")
	}

	ip := &IteratorPool[T]{
		new: new,
	}

	ip.pool = &sync.Pool{
		New: func() interface{} {
			return ip.new()
		},
	}

	return ip
}

// Get retrieves an iterator from the pool
func (ip *IteratorPool[T]) Get() Iterator[T] {
	if ip == nil || ip.pool == nil {
		panic("cannot get from nil iterator pool")
	}

	return ip.pool.Get().(Iterator[T])
}

// Put returns an iterator to the pool
// The iterator must be closed and reset before being returned
func (ip *IteratorPool[T]) Put(iter Iterator[T]) {
	if ip == nil || ip.pool == nil {
		return
	}

	// Note: Caller must close and reset iterator before calling Put
	ip.pool.Put(iter)
}

// Global pools for common types
var (
	// DefaultKeyPool is a global key pool for defensive copies
	DefaultKeyPool = NewKeyPool()

	// DefaultBufferPool is a global buffer pool (4KB buffers)
	DefaultBufferPool = NewBufferPool(4096)

	// SmallBufferPool is a global pool for small buffers (256 bytes)
	SmallBufferPool = NewBufferPool(256)

	// LargeBufferPool is a global pool for large buffers (64KB)
	LargeBufferPool = NewBufferPool(65536)
)
