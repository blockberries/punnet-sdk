package crypto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlgorithm_String(t *testing.T) {
	tests := []struct {
		alg      Algorithm
		expected string
	}{
		{AlgorithmEd25519, "ed25519"},
		{AlgorithmSecp256k1, "secp256k1"},
		{AlgorithmSecp256r1, "secp256r1"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.alg.String())
		})
	}
}

func TestAlgorithm_IsValid(t *testing.T) {
	tests := []struct {
		alg   Algorithm
		valid bool
	}{
		{AlgorithmEd25519, true},
		{AlgorithmSecp256k1, true},
		{AlgorithmSecp256r1, true},
		{Algorithm("unknown"), false},
		{Algorithm(""), false},
		{Algorithm("ED25519"), false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.alg), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.alg.IsValid())
		})
	}
}

func TestAlgorithm_JSON(t *testing.T) {
	t.Run("marshal", func(t *testing.T) {
		data, err := json.Marshal(AlgorithmEd25519)
		require.NoError(t, err)
		assert.Equal(t, `"ed25519"`, string(data))
	})

	t.Run("unmarshal valid", func(t *testing.T) {
		var alg Algorithm
		err := json.Unmarshal([]byte(`"secp256k1"`), &alg)
		require.NoError(t, err)
		assert.Equal(t, AlgorithmSecp256k1, alg)
	})

	t.Run("unmarshal invalid algorithm", func(t *testing.T) {
		var alg Algorithm
		err := json.Unmarshal([]byte(`"unknown"`), &alg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown algorithm")
	})

	t.Run("unmarshal non-string", func(t *testing.T) {
		var alg Algorithm
		err := json.Unmarshal([]byte(`123`), &alg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a string")
	})
}

func TestAlgorithm_KeySize(t *testing.T) {
	assert.Equal(t, 32, AlgorithmEd25519.KeySize())
	assert.Equal(t, 33, AlgorithmSecp256k1.KeySize())
	assert.Equal(t, 33, AlgorithmSecp256r1.KeySize())
	assert.Equal(t, 0, Algorithm("unknown").KeySize())
}

func TestAlgorithm_SignatureSize(t *testing.T) {
	assert.Equal(t, 64, AlgorithmEd25519.SignatureSize())
	assert.Equal(t, 64, AlgorithmSecp256k1.SignatureSize())
	assert.Equal(t, 64, AlgorithmSecp256r1.SignatureSize())
	assert.Equal(t, 0, Algorithm("unknown").SignatureSize())
}
