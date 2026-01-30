package store

import (
	"bytes"
	"fmt"
	"sync"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	ics23 "github.com/cosmos/ics23/go"
	"github.com/cosmos/iavl"
)

// IAVLStore is an IAVL-backed implementation of BackingStore
// It wraps github.com/cosmos/iavl MutableTree and provides thread-safe operations,
// versioning, and merkle proof generation
type IAVLStore struct {
	mu      sync.RWMutex
	tree    *iavl.MutableTree
	version int64
	closed  bool
}

// NewIAVLStore creates a new IAVL-backed store
// db is the underlying database (can be nil for in-memory)
// cacheSize is the IAVL tree cache size (0 means no cache)
func NewIAVLStore(db dbm.DB, cacheSize int) (*IAVLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database cannot be nil")
	}

	// Create a no-op logger
	logger := log.NewNopLogger()

	tree := iavl.NewMutableTree(db, cacheSize, false, logger)

	// Load latest version if exists
	version, err := tree.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load tree: %w", err)
	}

	return &IAVLStore{
		tree:    tree,
		version: version,
		closed:  false,
	}, nil
}

// Get retrieves raw bytes by key
func (s *IAVLStore) Get(key []byte) ([]byte, error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Get from tree
	value, err := s.tree.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	if value == nil {
		return nil, ErrNotFound
	}

	// Return defensive copy
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Set stores raw bytes with the given key
func (s *IAVLStore) Set(key []byte, value []byte) error {
	if s == nil {
		return ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Create defensive copies
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	// Set in tree
	_, err := s.tree.Set(keyCopy, valueCopy)
	if err != nil {
		return fmt.Errorf("failed to set key: %w", err)
	}

	return nil
}

// Delete removes a key
func (s *IAVLStore) Delete(key []byte) error {
	if s == nil {
		return ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Delete from tree
	_, _, err := s.tree.Remove(key)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	return nil
}

// Has checks if a key exists
func (s *IAVLStore) Has(key []byte) (bool, error) {
	if s == nil {
		return false, ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return false, fmt.Errorf("store is closed")
	}

	// Check if key exists
	has, err := s.tree.Has(key)
	if err != nil {
		return false, fmt.Errorf("failed to check key: %w", err)
	}

	return has, nil
}

// Iterator returns an iterator over a range of keys
func (s *IAVLStore) Iterator(start, end []byte) (RawIterator, error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Create IAVL iterator (ascending)
	iter, err := s.tree.Iterator(start, end, true)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}

	return newIAVLIterator(iter, false), nil
}

// ReverseIterator returns a reverse iterator over a range of keys
func (s *IAVLStore) ReverseIterator(start, end []byte) (RawIterator, error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Create IAVL iterator (descending)
	iter, err := s.tree.Iterator(start, end, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}

	return newIAVLIterator(iter, true), nil
}

// Flush writes pending changes by saving a new version
func (s *IAVLStore) Flush() error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Save new version
	hash, version, err := s.tree.SaveVersion()
	if err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}

	s.version = version
	_ = hash // Hash is available for merkle proofs

	return nil
}

// Close releases resources
func (s *IAVLStore) Close() error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	// IAVL tree doesn't have a Close method, but we mark store as closed
	return nil
}

// SaveVersion saves the current state as a new version
// Returns the merkle root hash and version number
func (s *IAVLStore) SaveVersion() ([]byte, int64, error) {
	if s == nil {
		return nil, 0, ErrStoreNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, 0, fmt.Errorf("store is closed")
	}

	hash, version, err := s.tree.SaveVersion()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to save version: %w", err)
	}

	s.version = version

	// Return defensive copy of hash
	hashCopy := make([]byte, len(hash))
	copy(hashCopy, hash)

	return hashCopy, version, nil
}

// LoadVersion loads a specific version of the tree
// Note: For MutableTree, this loads the version as a base, but the tree
// remains mutable and will continue from the latest version when saved.
// To get an immutable snapshot at a specific version, use GetImmutable.
func (s *IAVLStore) LoadVersion(version int64) error {
	if s == nil {
		return ErrStoreNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// LoadVersion returns the actual version loaded
	v, err := s.tree.LoadVersion(version)
	if err != nil {
		return fmt.Errorf("failed to load version %d: %w", version, err)
	}

	// Update to the loaded version
	s.version = v
	return nil
}

// GetProof generates a merkle proof for a key at the current version
func (s *IAVLStore) GetProof(key []byte) (*ics23.CommitmentProof, error) {
	if s == nil {
		return nil, ErrStoreNil
	}

	if err := validateKey(key); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Get proof from tree at current version
	proof, err := s.tree.GetVersionedProof(key, s.version)
	if err != nil {
		return nil, fmt.Errorf("failed to get proof: %w", err)
	}

	return proof, nil
}

// Version returns the current version number
func (s *IAVLStore) Version() int64 {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.version
}

// Hash returns the merkle root hash of the current version
func (s *IAVLStore) Hash() []byte {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil
	}

	hash := s.tree.Hash()

	// Return defensive copy
	hashCopy := make([]byte, len(hash))
	copy(hashCopy, hash)

	return hashCopy
}

// iavlIterator wraps a dbm iterator to implement RawIterator
type iavlIterator struct {
	mu      sync.RWMutex
	iter    dbm.Iterator
	reverse bool
	closed  bool
	err     error
}

// newIAVLIterator creates a new IAVL iterator wrapper
func newIAVLIterator(iter dbm.Iterator, reverse bool) *iavlIterator {
	return &iavlIterator{
		iter:    iter,
		reverse: reverse,
		closed:  false,
	}
}

// Valid returns true if positioned at a valid entry
func (it *iavlIterator) Valid() bool {
	if it == nil {
		return false
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed {
		return false
	}

	return it.iter.Valid()
}

// Next advances to the next entry
func (it *iavlIterator) Next() {
	if it == nil {
		return
	}

	it.mu.Lock()
	defer it.mu.Unlock()

	if it.closed {
		return
	}

	it.iter.Next()
}

// Key returns the current key
func (it *iavlIterator) Key() []byte {
	if it == nil {
		return nil
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed || !it.iter.Valid() {
		return nil
	}

	// Return defensive copy
	key := it.iter.Key()
	result := make([]byte, len(key))
	copy(result, key)
	return result
}

// Value returns the current value
func (it *iavlIterator) Value() []byte {
	if it == nil {
		return nil
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed || !it.iter.Valid() {
		return nil
	}

	// Return defensive copy
	value := it.iter.Value()
	result := make([]byte, len(value))
	copy(result, value)
	return result
}

// Error returns any error that occurred during iteration
func (it *iavlIterator) Error() error {
	if it == nil {
		return nil
	}

	it.mu.RLock()
	defer it.mu.RUnlock()

	if it.closed {
		return ErrIteratorClosed
	}

	return it.err
}

// Close releases iterator resources
func (it *iavlIterator) Close() error {
	if it == nil {
		return nil
	}

	it.mu.Lock()
	defer it.mu.Unlock()

	if it.closed {
		return nil
	}

	it.closed = true

	// Close IAVL iterator
	if err := it.iter.Close(); err != nil {
		return fmt.Errorf("failed to close IAVL iterator: %w", err)
	}

	return nil
}

// MemDB is a simple in-memory database for testing
// This implements iavl.DB interface
type MemDB struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMemDB creates a new in-memory database
func NewMemDB() *MemDB {
	return &MemDB{
		data: make(map[string][]byte),
	}
}

// Get retrieves a value
func (db *MemDB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	value, ok := db.data[string(key)]
	if !ok {
		return nil, nil
	}

	// Return defensive copy
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Has checks if a key exists
func (db *MemDB) Has(key []byte) (bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	_, ok := db.data[string(key)]
	return ok, nil
}

// Set stores a value
func (db *MemDB) Set(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Store defensive copies
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	db.data[string(keyCopy)] = valueCopy
	return nil
}

// SetSync stores a value synchronously
func (db *MemDB) SetSync(key, value []byte) error {
	return db.Set(key, value)
}

// Delete removes a key
func (db *MemDB) Delete(key []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	delete(db.data, string(key))
	return nil
}

// DeleteSync removes a key synchronously
func (db *MemDB) DeleteSync(key []byte) error {
	return db.Delete(key)
}

// Iterator creates an iterator over a range
func (db *MemDB) Iterator(start, end []byte) (dbm.Iterator, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Collect keys in range
	var keys [][]byte
	for key := range db.data {
		keyBytes := []byte(key)

		if start != nil && bytes.Compare(keyBytes, start) < 0 {
			continue
		}
		if end != nil && bytes.Compare(keyBytes, end) >= 0 {
			continue
		}

		keys = append(keys, keyBytes)
	}

	// Sort keys
	sortByteSlices(keys)

	// Create items
	items := make([]kvPair, len(keys))
	for i, key := range keys {
		value := db.data[string(key)]
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)

		items[i] = kvPair{
			key:   key,
			value: valueCopy,
		}
	}

	return &memDBIterator{
		items: items,
		index: 0,
	}, nil
}

// ReverseIterator creates a reverse iterator over a range
func (db *MemDB) ReverseIterator(start, end []byte) (dbm.Iterator, error) {
	iter, err := db.Iterator(start, end)
	if err != nil {
		return nil, err
	}

	mIter := iter.(*memDBIterator)

	// Reverse items
	items := mIter.items
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}

	return &memDBIterator{
		items: items,
		index: 0,
	}, nil
}

// Close closes the database
func (db *MemDB) Close() error {
	return nil
}

// NewBatch creates a new batch
func (db *MemDB) NewBatch() dbm.Batch {
	return &memDBBatch{
		db:  db,
		ops: make(map[string]batchOp),
	}
}

// NewBatchWithSize creates a new batch with a specified size hint
func (db *MemDB) NewBatchWithSize(size int) dbm.Batch {
	return &memDBBatch{
		db:  db,
		ops: make(map[string]batchOp, size),
	}
}

// Print prints database contents (for debugging)
func (db *MemDB) Print() error {
	return nil
}

// Stats returns database statistics
func (db *MemDB) Stats() map[string]string {
	return make(map[string]string)
}

// memDBIterator implements iavl.Iterator for MemDB
type memDBIterator struct {
	items  []kvPair
	index  int
	closed bool
}

// Domain returns the iterator's domain
func (it *memDBIterator) Domain() ([]byte, []byte) {
	if len(it.items) == 0 {
		return nil, nil
	}
	return it.items[0].key, it.items[len(it.items)-1].key
}

// Valid returns true if positioned at a valid entry
func (it *memDBIterator) Valid() bool {
	return !it.closed && it.index >= 0 && it.index < len(it.items)
}

// Next advances to the next entry
func (it *memDBIterator) Next() {
	if !it.closed {
		it.index++
	}
}

// Key returns the current key
func (it *memDBIterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return it.items[it.index].key
}

// Value returns the current value
func (it *memDBIterator) Value() []byte {
	if !it.Valid() {
		return nil
	}
	return it.items[it.index].value
}

// Error returns any error
func (it *memDBIterator) Error() error {
	return nil
}

// Close closes the iterator
func (it *memDBIterator) Close() error {
	it.closed = true
	return nil
}

// batchOp represents a batch operation
type batchOp struct {
	delete bool
	value  []byte
}

// memDBBatch implements iavl.Batch for MemDB
type memDBBatch struct {
	db  *MemDB
	ops map[string]batchOp
}

// Set sets a value in the batch
func (b *memDBBatch) Set(key, value []byte) error {
	b.ops[string(key)] = batchOp{
		delete: false,
		value:  value,
	}
	return nil
}

// Delete marks a key for deletion
func (b *memDBBatch) Delete(key []byte) error {
	b.ops[string(key)] = batchOp{
		delete: true,
		value:  nil,
	}
	return nil
}

// Write commits the batch
func (b *memDBBatch) Write() error {
	for key, op := range b.ops {
		if op.delete {
			_ = b.db.Delete([]byte(key))
		} else {
			_ = b.db.Set([]byte(key), op.value)
		}
	}
	return nil
}

// WriteSync commits the batch synchronously
func (b *memDBBatch) WriteSync() error {
	return b.Write()
}

// Close closes the batch
func (b *memDBBatch) Close() error {
	b.ops = nil
	return nil
}

// GetByteSize returns an estimate of the batch size in bytes
func (b *memDBBatch) GetByteSize() (int, error) {
	size := 0
	for key, op := range b.ops {
		size += len(key)
		size += len(op.value)
	}
	return size, nil
}

// sortByteSlices sorts byte slices lexicographically
func sortByteSlices(slices [][]byte) {
	// Simple bubble sort for small datasets
	n := len(slices)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if bytes.Compare(slices[j], slices[j+1]) > 0 {
				slices[j], slices[j+1] = slices[j+1], slices[j]
			}
		}
	}
}
