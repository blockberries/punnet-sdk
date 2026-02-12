package effects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSerializableValue is a test type for serialization tests.
type testSerializableValue struct {
	Name  string `cramberry:"1"`
	Value int    `cramberry:"2"`
}

func TestWriteEffect_SerializedValue(t *testing.T) {
	t.Run("serializes value with cramberry", func(t *testing.T) {
		effect := WriteEffect[*testSerializableValue]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value: &testSerializableValue{
				Name:  "test",
				Value: 42,
			},
		}

		data, err := effect.SerializedValue()
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("caches serialized value", func(t *testing.T) {
		effect := WriteEffect[*testSerializableValue]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value: &testSerializableValue{
				Name:  "test",
				Value: 42,
			},
		}

		data1, err := effect.SerializedValue()
		require.NoError(t, err)

		data2, err := effect.SerializedValue()
		require.NoError(t, err)

		// Should be the same slice (cached)
		assert.Equal(t, data1, data2)
	})

	t.Run("MustSerializedValue returns value", func(t *testing.T) {
		effect := WriteEffect[*testSerializableValue]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value: &testSerializableValue{
				Name:  "test",
				Value: 42,
			},
		}

		data := effect.MustSerializedValue()
		assert.NotEmpty(t, data)
	})
}

func TestWriteEffect_Validate(t *testing.T) {
	t.Run("validates successfully with valid effect", func(t *testing.T) {
		effect := WriteEffect[*testSerializableValue]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value: &testSerializableValue{
				Name:  "test",
				Value: 42,
			},
		}

		err := effect.Validate()
		assert.NoError(t, err)
	})

	t.Run("fails with empty store", func(t *testing.T) {
		effect := WriteEffect[*testSerializableValue]{
			Store:    "",
			StoreKey: []byte("key1"),
			Value:    &testSerializableValue{},
		}

		err := effect.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "store name cannot be empty")
	})

	t.Run("fails with empty key", func(t *testing.T) {
		effect := WriteEffect[*testSerializableValue]{
			Store:    "test",
			StoreKey: nil,
			Value:    &testSerializableValue{},
		}

		err := effect.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "key cannot be empty")
	})
}

func TestWriteEffect_Key(t *testing.T) {
	effect := WriteEffect[*testSerializableValue]{
		Store:    "test",
		StoreKey: []byte("mykey"),
		Value:    &testSerializableValue{},
	}

	key := effect.Key()
	assert.Equal(t, []byte("test/mykey"), key)
}

func TestWriteEffect_Type(t *testing.T) {
	effect := WriteEffect[*testSerializableValue]{
		Store:    "test",
		StoreKey: []byte("key"),
		Value:    &testSerializableValue{},
	}

	assert.Equal(t, EffectTypeWrite, effect.Type())
}

func TestWriteEffect_Dependencies(t *testing.T) {
	effect := WriteEffect[*testSerializableValue]{
		Store:    "test",
		StoreKey: []byte("key"),
		Value:    &testSerializableValue{},
	}

	deps := effect.Dependencies()
	require.Len(t, deps, 1)
	assert.Equal(t, DependencyTypeGeneric, deps[0].Type)
	assert.Equal(t, []byte("test/key"), deps[0].Key)
	assert.False(t, deps[0].ReadOnly)
}

