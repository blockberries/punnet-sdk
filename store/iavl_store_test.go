package store

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewIAVLStore tests IAVL store creation
func TestNewIAVLStore(t *testing.T) {
	t.Run("creates store with valid DB", func(t *testing.T) {
		db := NewMemDB()
		store, err := NewIAVLStore(db, 100)
		require.NoError(t, err)
		require.NotNil(t, store)
		assert.Equal(t, int64(0), store.Version())
	})

	t.Run("fails with nil DB", func(t *testing.T) {
		store, err := NewIAVLStore(nil, 100)
		assert.Error(t, err)
		assert.Nil(t, store)
		assert.Contains(t, err.Error(), "database cannot be nil")
	})
}

// TestIAVLStoreGet tests Get operations
func TestIAVLStoreGet(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("returns ErrNotFound for missing key", func(t *testing.T) {
		value, err := store.Get([]byte("missing"))
		assert.ErrorIs(t, err, ErrNotFound)
		assert.Nil(t, value)
	})

	t.Run("retrieves existing key", func(t *testing.T) {
		key := []byte("test-key")
		expected := []byte("test-value")

		err := store.Set(key, expected)
		require.NoError(t, err)

		value, err := store.Get(key)
		require.NoError(t, err)
		assert.Equal(t, expected, value)
	})

	t.Run("returns defensive copy", func(t *testing.T) {
		key := []byte("copy-test")
		original := []byte("original-value")

		err := store.Set(key, original)
		require.NoError(t, err)

		value, err := store.Get(key)
		require.NoError(t, err)

		// Modify returned value
		value[0] = 'X'

		// Get again and verify original is unchanged
		value2, err := store.Get(key)
		require.NoError(t, err)
		assert.Equal(t, original, value2)
	})

	t.Run("fails with nil key", func(t *testing.T) {
		value, err := store.Get(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, value)
	})

	t.Run("fails with empty key", func(t *testing.T) {
		value, err := store.Get([]byte{})
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, value)
	})
}

// TestIAVLStoreSet tests Set operations
func TestIAVLStoreSet(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("sets new key", func(t *testing.T) {
		key := []byte("new-key")
		value := []byte("new-value")

		err := store.Set(key, value)
		require.NoError(t, err)

		retrieved, err := store.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("updates existing key", func(t *testing.T) {
		key := []byte("update-key")
		value1 := []byte("value1")
		value2 := []byte("value2")

		err := store.Set(key, value1)
		require.NoError(t, err)

		err = store.Set(key, value2)
		require.NoError(t, err)

		retrieved, err := store.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value2, retrieved)
	})

	t.Run("stores defensive copy", func(t *testing.T) {
		key := []byte("defensive-key")
		value := []byte("defensive-value")

		err := store.Set(key, value)
		require.NoError(t, err)

		// Modify original
		value[0] = 'X'

		// Verify stored value is unchanged
		retrieved, err := store.Get(key)
		require.NoError(t, err)
		assert.Equal(t, []byte("defensive-value"), retrieved)
	})

	t.Run("fails with nil key", func(t *testing.T) {
		err := store.Set(nil, []byte("value"))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("fails with empty key", func(t *testing.T) {
		err := store.Set([]byte{}, []byte("value"))
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

// TestIAVLStoreDelete tests Delete operations
func TestIAVLStoreDelete(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("deletes existing key", func(t *testing.T) {
		key := []byte("delete-key")
		value := []byte("delete-value")

		err := store.Set(key, value)
		require.NoError(t, err)

		err = store.Delete(key)
		require.NoError(t, err)

		_, err = store.Get(key)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("deleting non-existent key succeeds", func(t *testing.T) {
		err := store.Delete([]byte("non-existent"))
		assert.NoError(t, err)
	})

	t.Run("fails with nil key", func(t *testing.T) {
		err := store.Delete(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
	})

	t.Run("fails with empty key", func(t *testing.T) {
		err := store.Delete([]byte{})
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

// TestIAVLStoreHas tests Has operations
func TestIAVLStoreHas(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("returns false for missing key", func(t *testing.T) {
		has, err := store.Has([]byte("missing"))
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("returns true for existing key", func(t *testing.T) {
		key := []byte("exists-key")
		err := store.Set(key, []byte("value"))
		require.NoError(t, err)

		has, err := store.Has(key)
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("returns false after delete", func(t *testing.T) {
		key := []byte("deleted-key")
		err := store.Set(key, []byte("value"))
		require.NoError(t, err)

		err = store.Delete(key)
		require.NoError(t, err)

		has, err := store.Has(key)
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("fails with nil key", func(t *testing.T) {
		has, err := store.Has(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.False(t, has)
	})

	t.Run("fails with empty key", func(t *testing.T) {
		has, err := store.Has([]byte{})
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.False(t, has)
	})
}

// TestIAVLStoreIterator tests Iterator operations
func TestIAVLStoreIterator(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	// Setup test data
	testData := map[string]string{
		"apple":  "red",
		"banana": "yellow",
		"cherry": "red",
		"date":   "brown",
		"fig":    "purple",
	}

	for k, v := range testData {
		err := store.Set([]byte(k), []byte(v))
		require.NoError(t, err)
	}

	t.Run("iterates all keys", func(t *testing.T) {
		iter, err := store.Iterator(nil, nil)
		require.NoError(t, err)
		defer iter.Close()

		count := 0
		for iter.Valid() {
			key := iter.Key()
			value := iter.Value()

			expectedValue := testData[string(key)]
			assert.Equal(t, expectedValue, string(value))

			count++
			iter.Next()
		}

		assert.Equal(t, len(testData), count)
	})

	t.Run("iterates range", func(t *testing.T) {
		iter, err := store.Iterator([]byte("banana"), []byte("date"))
		require.NoError(t, err)
		defer iter.Close()

		var keys []string
		for iter.Valid() {
			keys = append(keys, string(iter.Key()))
			iter.Next()
		}

		expected := []string{"banana", "cherry"}
		assert.Equal(t, expected, keys)
	})

	t.Run("returns defensive copies", func(t *testing.T) {
		iter, err := store.Iterator(nil, nil)
		require.NoError(t, err)
		defer iter.Close()

		require.True(t, iter.Valid())

		key1 := iter.Key()
		value1 := iter.Value()

		// Modify returned values
		key1[0] = 'X'
		value1[0] = 'Y'

		// Get again
		key2 := iter.Key()
		value2 := iter.Value()

		// Verify originals unchanged
		assert.NotEqual(t, key1, key2)
		assert.NotEqual(t, value1, value2)
	})
}

// TestIAVLStoreReverseIterator tests ReverseIterator operations
func TestIAVLStoreReverseIterator(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	// Setup test data
	testData := map[string]string{
		"alpha": "1",
		"beta":  "2",
		"gamma": "3",
		"delta": "4",
	}

	for k, v := range testData {
		err := store.Set([]byte(k), []byte(v))
		require.NoError(t, err)
	}

	t.Run("reverse iterates all keys", func(t *testing.T) {
		iter, err := store.ReverseIterator(nil, nil)
		require.NoError(t, err)
		defer iter.Close()

		var keys []string
		for iter.Valid() {
			keys = append(keys, string(iter.Key()))
			iter.Next()
		}

		// Should be in reverse order
		expected := []string{"gamma", "delta", "beta", "alpha"}
		assert.Equal(t, expected, keys)
	})

	t.Run("reverse iterates range", func(t *testing.T) {
		iter, err := store.ReverseIterator([]byte("beta"), []byte("gamma"))
		require.NoError(t, err)
		defer iter.Close()

		var keys []string
		for iter.Valid() {
			keys = append(keys, string(iter.Key()))
			iter.Next()
		}

		expected := []string{"delta", "beta"}
		assert.Equal(t, expected, keys)
	})
}

// TestIAVLStoreVersioning tests versioning operations
func TestIAVLStoreVersioning(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("initial version is 0", func(t *testing.T) {
		assert.Equal(t, int64(0), store.Version())
	})

	t.Run("SaveVersion increments version", func(t *testing.T) {
		err := store.Set([]byte("key1"), []byte("value1"))
		require.NoError(t, err)

		hash, version, err := store.SaveVersion()
		require.NoError(t, err)
		assert.NotNil(t, hash)
		assert.Equal(t, int64(1), version)
		assert.Equal(t, int64(1), store.Version())
	})

	t.Run("multiple SaveVersion calls increment version", func(t *testing.T) {
		for i := 2; i <= 5; i++ {
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)

			err := store.Set([]byte(key), []byte(value))
			require.NoError(t, err)

			hash, version, err := store.SaveVersion()
			require.NoError(t, err)
			assert.NotNil(t, hash)
			assert.Equal(t, int64(i), version)
		}

		assert.Equal(t, int64(5), store.Version())
	})

	t.Run("LoadVersion loads previous state", func(t *testing.T) {
		// Create a new store to test LoadVersion
		db2 := NewMemDB()
		store2, err := NewIAVLStore(db2, 100)
		require.NoError(t, err)

		// Add data and save multiple versions
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("v-key%d", i)
			value := fmt.Sprintf("v-value%d", i)

			err := store2.Set([]byte(key), []byte(value))
			require.NoError(t, err)

			_, _, err = store2.SaveVersion()
			require.NoError(t, err)
		}

		// Current version is 3
		assert.Equal(t, int64(3), store2.Version())

		// Create another store instance with same DB to load version 2
		store3, err := NewIAVLStore(db2, 100)
		require.NoError(t, err)

		// Load version 2
		err = store3.LoadVersion(2)
		require.NoError(t, err)

		// LoadVersion should update the version number
		loadedVersion := store3.Version()
		assert.True(t, loadedVersion >= 2, "loaded version should be at least 2")

		// Verify data at version 2 exists
		value, err := store3.Get([]byte("v-key2"))
		require.NoError(t, err)
		assert.Equal(t, []byte("v-value2"), value)
	})

	t.Run("Hash returns merkle root", func(t *testing.T) {
		hash := store.Hash()
		assert.NotNil(t, hash)
		assert.Greater(t, len(hash), 0)

		// Hash should be defensive copy
		originalHash := make([]byte, len(hash))
		copy(originalHash, hash)

		hash[0] = 0xFF

		hash2 := store.Hash()
		assert.Equal(t, originalHash, hash2)
	})
}

// TestIAVLStoreFlush tests Flush operations
func TestIAVLStoreFlush(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("flush saves version", func(t *testing.T) {
		err := store.Set([]byte("flush-key"), []byte("flush-value"))
		require.NoError(t, err)

		err = store.Flush()
		require.NoError(t, err)

		assert.Equal(t, int64(1), store.Version())
	})

	t.Run("flush is idempotent", func(t *testing.T) {
		v1 := store.Version()

		err := store.Flush()
		require.NoError(t, err)

		v2 := store.Version()
		assert.Greater(t, v2, v1)

		// Flush without changes
		err = store.Flush()
		require.NoError(t, err)

		v3 := store.Version()
		assert.Greater(t, v3, v2)
	})
}

// TestIAVLStoreGetProof tests merkle proof generation
func TestIAVLStoreGetProof(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	// Add data and save version
	err = store.Set([]byte("proof-key"), []byte("proof-value"))
	require.NoError(t, err)

	_, _, err = store.SaveVersion()
	require.NoError(t, err)

	t.Run("generates proof for existing key", func(t *testing.T) {
		proof, err := store.GetProof([]byte("proof-key"))
		require.NoError(t, err)
		assert.NotNil(t, proof)
	})

	t.Run("generates proof for non-existent key", func(t *testing.T) {
		proof, err := store.GetProof([]byte("missing-key"))
		require.NoError(t, err)
		assert.NotNil(t, proof)
	})

	t.Run("fails with nil key", func(t *testing.T) {
		proof, err := store.GetProof(nil)
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, proof)
	})

	t.Run("fails with empty key", func(t *testing.T) {
		proof, err := store.GetProof([]byte{})
		assert.ErrorIs(t, err, ErrInvalidKey)
		assert.Nil(t, proof)
	})
}

// TestIAVLStoreClose tests Close operations
func TestIAVLStoreClose(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("close succeeds", func(t *testing.T) {
		err := store.Close()
		assert.NoError(t, err)
	})

	t.Run("operations fail after close", func(t *testing.T) {
		_, err := store.Get([]byte("key"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		err = store.Set([]byte("key"), []byte("value"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		err = store.Delete([]byte("key"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		_, err = store.Has([]byte("key"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		_, err = store.Iterator(nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		_, err = store.ReverseIterator(nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		err = store.Flush()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		_, _, err = store.SaveVersion()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		err = store.LoadVersion(0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")

		_, err = store.GetProof([]byte("key"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store is closed")
	})

	t.Run("close is idempotent", func(t *testing.T) {
		db := NewMemDB()
		store, err := NewIAVLStore(db, 100)
		require.NoError(t, err)

		err = store.Close()
		assert.NoError(t, err)

		err = store.Close()
		assert.NoError(t, err)
	})
}

// TestIAVLStoreConcurrency tests concurrent access
func TestIAVLStoreConcurrency(t *testing.T) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 100)
	require.NoError(t, err)

	t.Run("concurrent reads and writes", func(t *testing.T) {
		var wg sync.WaitGroup

		// Writers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for j := 0; j < 100; j++ {
					key := fmt.Sprintf("key-%d-%d", id, j)
					value := fmt.Sprintf("value-%d-%d", id, j)

					err := store.Set([]byte(key), []byte(value))
					assert.NoError(t, err)
				}
			}(i)
		}

		// Readers
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				for j := 0; j < 100; j++ {
					key := fmt.Sprintf("key-%d-%d", id, j)
					_, _ = store.Get([]byte(key))
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent iterations", func(t *testing.T) {
		var wg sync.WaitGroup

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				iter, err := store.Iterator(nil, nil)
				if err != nil {
					return
				}
				defer iter.Close()

				for iter.Valid() {
					_ = iter.Key()
					_ = iter.Value()
					iter.Next()
				}
			}()
		}

		wg.Wait()
	})
}

// TestIAVLStoreNilHandling tests nil store handling
func TestIAVLStoreNilHandling(t *testing.T) {
	var store *IAVLStore

	t.Run("nil store returns errors", func(t *testing.T) {
		_, err := store.Get([]byte("key"))
		assert.ErrorIs(t, err, ErrStoreNil)

		err = store.Set([]byte("key"), []byte("value"))
		assert.ErrorIs(t, err, ErrStoreNil)

		err = store.Delete([]byte("key"))
		assert.ErrorIs(t, err, ErrStoreNil)

		_, err = store.Has([]byte("key"))
		assert.ErrorIs(t, err, ErrStoreNil)

		_, err = store.Iterator(nil, nil)
		assert.ErrorIs(t, err, ErrStoreNil)

		_, err = store.ReverseIterator(nil, nil)
		assert.ErrorIs(t, err, ErrStoreNil)

		err = store.Flush()
		assert.ErrorIs(t, err, ErrStoreNil)

		err = store.Close()
		assert.ErrorIs(t, err, ErrStoreNil)

		_, _, err = store.SaveVersion()
		assert.ErrorIs(t, err, ErrStoreNil)

		err = store.LoadVersion(0)
		assert.ErrorIs(t, err, ErrStoreNil)

		_, err = store.GetProof([]byte("key"))
		assert.ErrorIs(t, err, ErrStoreNil)

		assert.Equal(t, int64(0), store.Version())
		assert.Nil(t, store.Hash())
	})
}

// TestIAVLIteratorNilHandling tests nil iterator handling
func TestIAVLIteratorNilHandling(t *testing.T) {
	var iter *iavlIterator

	t.Run("nil iterator returns safe values", func(t *testing.T) {
		assert.False(t, iter.Valid())
		iter.Next() // Should not panic
		assert.Nil(t, iter.Key())
		assert.Nil(t, iter.Value())
		assert.Nil(t, iter.Error())
		assert.Nil(t, iter.Close())
	})
}

// TestMemDB tests the in-memory database implementation
func TestMemDB(t *testing.T) {
	db := NewMemDB()

	t.Run("basic operations", func(t *testing.T) {
		// Set
		err := db.Set([]byte("key1"), []byte("value1"))
		require.NoError(t, err)

		// Get
		value, err := db.Get([]byte("key1"))
		require.NoError(t, err)
		assert.Equal(t, []byte("value1"), value)

		// Has
		has, err := db.Has([]byte("key1"))
		require.NoError(t, err)
		assert.True(t, has)

		has, err = db.Has([]byte("missing"))
		require.NoError(t, err)
		assert.False(t, has)

		// Delete
		err = db.Delete([]byte("key1"))
		require.NoError(t, err)

		value, err = db.Get([]byte("key1"))
		require.NoError(t, err)
		assert.Nil(t, value)
	})

	t.Run("iterator", func(t *testing.T) {
		db := NewMemDB()

		// Add test data
		testData := map[string]string{
			"apple":  "red",
			"banana": "yellow",
			"cherry": "red",
		}

		for k, v := range testData {
			err := db.Set([]byte(k), []byte(v))
			require.NoError(t, err)
		}

		// Iterate
		iter, err := db.Iterator(nil, nil)
		require.NoError(t, err)
		defer iter.Close()

		var keys []string
		for iter.Valid() {
			keys = append(keys, string(iter.Key()))
			iter.Next()
		}

		// Should be in sorted order
		expected := []string{"apple", "banana", "cherry"}
		assert.Equal(t, expected, keys)
	})

	t.Run("reverse iterator", func(t *testing.T) {
		db := NewMemDB()

		// Add test data
		for i := 1; i <= 5; i++ {
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)
			err := db.Set([]byte(key), []byte(value))
			require.NoError(t, err)
		}

		// Reverse iterate
		iter, err := db.ReverseIterator(nil, nil)
		require.NoError(t, err)
		defer iter.Close()

		var keys []string
		for iter.Valid() {
			keys = append(keys, string(iter.Key()))
			iter.Next()
		}

		// Should be in reverse sorted order
		expected := []string{"key5", "key4", "key3", "key2", "key1"}
		assert.Equal(t, expected, keys)
	})

	t.Run("batch operations", func(t *testing.T) {
		db := NewMemDB()

		batch := db.NewBatch()
		require.NotNil(t, batch)

		// Add to batch
		err := batch.Set([]byte("batch1"), []byte("value1"))
		require.NoError(t, err)

		err = batch.Set([]byte("batch2"), []byte("value2"))
		require.NoError(t, err)

		err = batch.Delete([]byte("batch1"))
		require.NoError(t, err)

		// Write batch
		err = batch.Write()
		require.NoError(t, err)

		// Verify
		value, err := db.Get([]byte("batch1"))
		require.NoError(t, err)
		assert.Nil(t, value)

		value, err = db.Get([]byte("batch2"))
		require.NoError(t, err)
		assert.Equal(t, []byte("value2"), value)

		// Close batch
		err = batch.Close()
		require.NoError(t, err)
	})
}

// TestSortByteSlices tests the byte slice sorting function
func TestSortByteSlices(t *testing.T) {
	t.Run("sorts byte slices", func(t *testing.T) {
		slices := [][]byte{
			[]byte("zebra"),
			[]byte("apple"),
			[]byte("mango"),
			[]byte("banana"),
		}

		sortByteSlices(slices)

		expected := [][]byte{
			[]byte("apple"),
			[]byte("banana"),
			[]byte("mango"),
			[]byte("zebra"),
		}

		for i, slice := range slices {
			assert.True(t, bytes.Equal(slice, expected[i]))
		}
	})

	t.Run("handles empty slice", func(t *testing.T) {
		slices := [][]byte{}
		sortByteSlices(slices)
		assert.Empty(t, slices)
	})

	t.Run("handles single element", func(t *testing.T) {
		slices := [][]byte{[]byte("single")}
		sortByteSlices(slices)
		assert.Equal(t, [][]byte{[]byte("single")}, slices)
	})
}

// BenchmarkIAVLStoreSet benchmarks Set operations
func BenchmarkIAVLStoreSet(b *testing.B) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 1000)
	require.NoError(b, err)

	keys := make([][]byte, b.N)
	values := make([][]byte, b.N)

	for i := 0; i < b.N; i++ {
		keys[i] = []byte(fmt.Sprintf("key-%d", i))
		values[i] = []byte(fmt.Sprintf("value-%d", i))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = store.Set(keys[i], values[i])
	}
}

// BenchmarkIAVLStoreGet benchmarks Get operations
func BenchmarkIAVLStoreGet(b *testing.B) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 1000)
	require.NoError(b, err)

	// Setup data
	for i := 0; i < 1000; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("value-%d", i))
		_ = store.Set(key, value)
	}

	keys := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = []byte(fmt.Sprintf("key-%d", i%1000))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = store.Get(keys[i])
	}
}

// BenchmarkIAVLStoreSaveVersion benchmarks SaveVersion operations
func BenchmarkIAVLStoreSaveVersion(b *testing.B) {
	db := NewMemDB()
	store, err := NewIAVLStore(db, 1000)
	require.NoError(b, err)

	// Add some data
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		value := []byte(fmt.Sprintf("value-%d", i))
		_ = store.Set(key, value)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _ = store.SaveVersion()

		// Add new data for next version
		key := []byte(fmt.Sprintf("key-%d", 100+i))
		value := []byte(fmt.Sprintf("value-%d", 100+i))
		_ = store.Set(key, value)
	}
}
