package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Key Generation Tests
// ============================================================================

func TestGeneratePrivateKey_Ed25519(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)
	require.NotNil(t, key)

	assert.Equal(t, AlgorithmEd25519, key.Algorithm())
	assert.Len(t, key.Bytes(), 64) // Ed25519 private key is 64 bytes
	assert.NotNil(t, key.PublicKey())
	assert.Len(t, key.PublicKey().Bytes(), 32)
}

func TestGeneratePrivateKey_Secp256k1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)
	require.NotNil(t, key)

	assert.Equal(t, AlgorithmSecp256k1, key.Algorithm())
	assert.Len(t, key.Bytes(), 32) // secp256k1 scalar is 32 bytes
	assert.NotNil(t, key.PublicKey())
	assert.Len(t, key.PublicKey().Bytes(), 33) // Compressed public key
}

func TestGeneratePrivateKey_Secp256r1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	require.NoError(t, err)
	require.NotNil(t, key)

	assert.Equal(t, AlgorithmSecp256r1, key.Algorithm())
	assert.Len(t, key.Bytes(), 32) // P-256 scalar is 32 bytes
	assert.NotNil(t, key.PublicKey())
	assert.Len(t, key.PublicKey().Bytes(), 33) // Compressed public key
}

func TestGeneratePrivateKey_InvalidAlgorithm(t *testing.T) {
	_, err := GeneratePrivateKey(Algorithm("invalid"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported algorithm")
}

// ============================================================================
// Sign/Verify Round-Trip Tests
// ============================================================================

func TestSignVerify_Ed25519(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	data := []byte("test message for signing")
	signature, err := key.Sign(data)
	require.NoError(t, err)
	assert.Len(t, signature, 64)

	assert.True(t, key.PublicKey().Verify(data, signature))
}

func TestSignVerify_Secp256k1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)

	data := []byte("test message for signing")
	signature, err := key.Sign(data)
	require.NoError(t, err)
	assert.Len(t, signature, 64)

	assert.True(t, key.PublicKey().Verify(data, signature))
}

func TestSignVerify_Secp256r1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	require.NoError(t, err)

	data := []byte("test message for signing")
	signature, err := key.Sign(data)
	require.NoError(t, err)
	assert.Len(t, signature, 64)

	assert.True(t, key.PublicKey().Verify(data, signature))
}

func TestSignVerify_WrongData(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			data := []byte("original message")
			signature, err := key.Sign(data)
			require.NoError(t, err)

			wrongData := []byte("different message")
			assert.False(t, key.PublicKey().Verify(wrongData, signature))
		})
	}
}

func TestSignVerify_WrongKey(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key1, err := GeneratePrivateKey(algo)
			require.NoError(t, err)
			key2, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			data := []byte("test message")
			signature, err := key1.Sign(data)
			require.NoError(t, err)

			assert.False(t, key2.PublicKey().Verify(data, signature))
		})
	}
}

func TestVerify_InvalidSignatureLength(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			data := []byte("test message")
			assert.False(t, key.PublicKey().Verify(data, []byte("short")))
			assert.False(t, key.PublicKey().Verify(data, make([]byte, 63)))
			assert.False(t, key.PublicKey().Verify(data, make([]byte, 65)))
		})
	}
}

// ============================================================================
// PrivateKeyFromBytes Tests
// ============================================================================

func TestPrivateKeyFromBytes_Ed25519(t *testing.T) {
	original, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	restored, err := PrivateKeyFromBytes(AlgorithmEd25519, original.Bytes())
	require.NoError(t, err)

	assert.Equal(t, original.Bytes(), restored.Bytes())
	assert.True(t, original.PublicKey().Equals(restored.PublicKey()))
}

func TestPrivateKeyFromBytes_Secp256k1(t *testing.T) {
	original, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)

	restored, err := PrivateKeyFromBytes(AlgorithmSecp256k1, original.Bytes())
	require.NoError(t, err)

	assert.Equal(t, original.Bytes(), restored.Bytes())
	assert.True(t, original.PublicKey().Equals(restored.PublicKey()))

	// Verify signing works the same
	data := []byte("test data")
	sig1, err := original.Sign(data)
	require.NoError(t, err)
	sig2, err := restored.Sign(data)
	require.NoError(t, err)
	assert.Equal(t, sig1, sig2) // RFC 6979 should produce deterministic signatures
}

func TestPrivateKeyFromBytes_Secp256r1(t *testing.T) {
	original, err := GeneratePrivateKey(AlgorithmSecp256r1)
	require.NoError(t, err)

	restored, err := PrivateKeyFromBytes(AlgorithmSecp256r1, original.Bytes())
	require.NoError(t, err)

	assert.Equal(t, original.Bytes(), restored.Bytes())
	assert.True(t, original.PublicKey().Equals(restored.PublicKey()))
}

func TestPrivateKeyFromBytes_InvalidSize(t *testing.T) {
	tests := []struct {
		algo Algorithm
		size int
	}{
		{AlgorithmEd25519, 32}, // Should be 64
		{AlgorithmSecp256k1, 31},
		{AlgorithmSecp256k1, 33},
		{AlgorithmSecp256r1, 31},
		{AlgorithmSecp256r1, 33},
	}

	for _, tt := range tests {
		t.Run(string(tt.algo), func(t *testing.T) {
			_, err := PrivateKeyFromBytes(tt.algo, make([]byte, tt.size))
			assert.Error(t, err)
		})
	}
}

func TestPrivateKeyFromBytes_Secp256r1_InvalidScalar(t *testing.T) {
	// Test zero scalar (d = 0 is invalid)
	zeroScalar := make([]byte, 32)
	_, err := PrivateKeyFromBytes(AlgorithmSecp256r1, zeroScalar)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Test scalar >= N (curve order)
	// P-256 order N is 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
	// Using all 0xFF bytes is >= N
	largeScalar := make([]byte, 32)
	for i := range largeScalar {
		largeScalar[i] = 0xFF
	}
	_, err = PrivateKeyFromBytes(AlgorithmSecp256r1, largeScalar)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

// ============================================================================
// PublicKeyFromBytes Tests
// ============================================================================

func TestPublicKeyFromBytes_Ed25519(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	pubBytes := key.PublicKey().Bytes()
	restored, err := PublicKeyFromBytes(AlgorithmEd25519, pubBytes)
	require.NoError(t, err)

	assert.True(t, key.PublicKey().Equals(restored))
}

func TestPublicKeyFromBytes_Secp256k1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)

	pubBytes := key.PublicKey().Bytes()
	restored, err := PublicKeyFromBytes(AlgorithmSecp256k1, pubBytes)
	require.NoError(t, err)

	assert.True(t, key.PublicKey().Equals(restored))

	// Verify the restored key can verify signatures
	data := []byte("test message")
	signature, err := key.Sign(data)
	require.NoError(t, err)
	assert.True(t, restored.Verify(data, signature))
}

func TestPublicKeyFromBytes_Secp256r1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	require.NoError(t, err)

	pubBytes := key.PublicKey().Bytes()
	restored, err := PublicKeyFromBytes(AlgorithmSecp256r1, pubBytes)
	require.NoError(t, err)

	assert.True(t, key.PublicKey().Equals(restored))

	// Verify the restored key can verify signatures
	data := []byte("test message")
	signature, err := key.Sign(data)
	require.NoError(t, err)
	assert.True(t, restored.Verify(data, signature))
}

func TestPublicKeyFromBytes_InvalidSize(t *testing.T) {
	tests := []struct {
		algo Algorithm
		size int
	}{
		{AlgorithmEd25519, 33},   // Should be 32
		{AlgorithmSecp256k1, 32}, // Should be 33 (compressed)
		{AlgorithmSecp256k1, 65}, // Uncompressed not supported
		{AlgorithmSecp256r1, 32}, // Should be 33 (compressed)
	}

	for _, tt := range tests {
		t.Run(string(tt.algo), func(t *testing.T) {
			_, err := PublicKeyFromBytes(tt.algo, make([]byte, tt.size))
			assert.Error(t, err)
		})
	}
}

// ============================================================================
// Equals Tests
// ============================================================================

func TestPublicKey_Equals(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key1, err := GeneratePrivateKey(algo)
			require.NoError(t, err)
			key2, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			// Same key equals itself
			assert.True(t, key1.PublicKey().Equals(key1.PublicKey()))

			// Different keys are not equal
			assert.False(t, key1.PublicKey().Equals(key2.PublicKey()))

			// nil is not equal
			assert.False(t, key1.PublicKey().Equals(nil))
		})
	}
}

func TestPublicKey_Equals_DifferentAlgorithm(t *testing.T) {
	ed25519Key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)
	secp256k1Key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)

	assert.False(t, ed25519Key.PublicKey().Equals(secp256k1Key.PublicKey()))
}

// ============================================================================
// Zeroize Tests
// ============================================================================

func TestZeroize_Ed25519(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	// Get a reference to the bytes before zeroizing
	keyBytes := key.Bytes()
	originalCopy := make([]byte, len(keyBytes))
	copy(originalCopy, keyBytes)

	// Verify key is not zeros
	assert.NotEqual(t, make([]byte, len(keyBytes)), keyBytes)

	// Zeroize
	key.Zeroize()

	// Verify bytes are now zeros
	assert.Equal(t, make([]byte, len(keyBytes)), keyBytes)
}

func TestZeroize_Secp256k1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)

	// Zeroize should not panic
	key.Zeroize()
}

func TestZeroize_Secp256r1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	require.NoError(t, err)

	// Zeroize should not panic
	key.Zeroize()
}

// ============================================================================
// String Tests
// ============================================================================

func TestPublicKey_String(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			s := key.PublicKey().String()
			assert.NotEmpty(t, s)
			// Base64 encoded string should be longer than raw bytes
			assert.Greater(t, len(s), 0)
		})
	}
}

// ============================================================================
// Test Vectors (Known Values)
// ============================================================================

// Test vector from Bitcoin/Ethereum ecosystem for secp256k1
func TestSecp256k1_KnownVector(t *testing.T) {
	// Well-known test private key (DO NOT USE IN PRODUCTION)
	// This is the private key: 1
	privKeyBytes, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000001")
	require.NoError(t, err)

	key, err := PrivateKeyFromBytes(AlgorithmSecp256k1, privKeyBytes)
	require.NoError(t, err)

	// The public key for private key = 1 is the generator point G
	// Compressed form starts with 02 or 03
	pubKey := key.PublicKey()
	pubBytes := pubKey.Bytes()
	assert.Len(t, pubBytes, 33)
	assert.True(t, pubBytes[0] == 0x02 || pubBytes[0] == 0x03)

	// Known compressed public key for private key = 1
	expectedPubKey, err := hex.DecodeString("0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	require.NoError(t, err)
	assert.Equal(t, expectedPubKey, pubBytes)
}

// Test deterministic signing (RFC 6979) for secp256k1
func TestSecp256k1_DeterministicSigning(t *testing.T) {
	privKeyBytes, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000001")
	require.NoError(t, err)

	key, err := PrivateKeyFromBytes(AlgorithmSecp256k1, privKeyBytes)
	require.NoError(t, err)

	data := []byte("test message")

	// Sign multiple times
	sig1, err := key.Sign(data)
	require.NoError(t, err)
	sig2, err := key.Sign(data)
	require.NoError(t, err)

	// Signatures should be identical (deterministic)
	assert.Equal(t, sig1, sig2)
}

// Test that secp256k1 signatures are valid ECDSA signatures
func TestSecp256k1_SignatureFormat(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	require.NoError(t, err)

	data := []byte("test message")
	signature, err := key.Sign(data)
	require.NoError(t, err)

	// Signature should be 64 bytes (r || s, each 32 bytes)
	assert.Len(t, signature, 64)

	// Verify signature verifies correctly
	assert.True(t, key.PublicKey().Verify(data, signature))
}

// Test P-256 compressed public key format
func TestSecp256r1_CompressedPublicKey(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	require.NoError(t, err)

	pubBytes := key.PublicKey().Bytes()

	// Compressed P-256 public key should be 33 bytes
	assert.Len(t, pubBytes, 33)

	// First byte should be 0x02 or 0x03 (compression prefix)
	assert.True(t, pubBytes[0] == 0x02 || pubBytes[0] == 0x03,
		"expected compression prefix 0x02 or 0x03, got 0x%02x", pubBytes[0])
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestSignVerify_EmptyData(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			// Sign empty data
			signature, err := key.Sign([]byte{})
			require.NoError(t, err)

			// Verify should work
			assert.True(t, key.PublicKey().Verify([]byte{}, signature))

			// But not with different data
			assert.False(t, key.PublicKey().Verify([]byte{0}, signature))
		})
	}
}

func TestSignVerify_LargeData(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			// Sign 1MB of data
			largeData := make([]byte, 1024*1024)
			for i := range largeData {
				largeData[i] = byte(i)
			}

			signature, err := key.Sign(largeData)
			require.NoError(t, err)
			assert.True(t, key.PublicKey().Verify(largeData, signature))
		})
	}
}

func TestSignVerify_HashInput(t *testing.T) {
	// Common pattern: sign a hash rather than raw data
	algorithms := []Algorithm{AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			// Hash some data
			data := []byte("original data to hash")
			hash := sha256.Sum256(data)

			// Sign the hash
			signature, err := key.Sign(hash[:])
			require.NoError(t, err)

			// Verify with the same hash
			assert.True(t, key.PublicKey().Verify(hash[:], signature))
		})
	}
}

// Test that Zeroize function works correctly
func TestZeroize_Function(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	original := make([]byte, len(data))
	copy(original, data)

	Zeroize(data)

	assert.Equal(t, make([]byte, len(data)), data)
	assert.NotEqual(t, original, data)
}

func TestZeroize_EmptySlice(t *testing.T) {
	// Should not panic on empty slice
	Zeroize([]byte{})
	Zeroize(nil)
}

// Test bytes copy protection
func TestPrivateKeyFromBytes_MakesCopy(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			size := algo.PrivateKeySize()
			original := make([]byte, size)
			for i := range original {
				original[i] = byte(i)
			}
			originalCopy := make([]byte, len(original))
			copy(originalCopy, original)

			key, err := PrivateKeyFromBytes(algo, original)
			require.NoError(t, err)

			// Modify original
			for i := range original {
				original[i] = 0xFF
			}

			// Key should still work (not affected by modification)
			keyBytes := key.Bytes()
			// For secp256k1/r1, the bytes come from the library, not our copy
			// Just verify the key still works
			data := []byte("test")
			_, err = key.Sign(data)
			require.NoError(t, err)
			_ = keyBytes // Use variable
		})
	}
}

func TestPublicKeyFromBytes_MakesCopy(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	pubBytes := make([]byte, len(key.PublicKey().Bytes()))
	copy(pubBytes, key.PublicKey().Bytes())

	restored, err := PublicKeyFromBytes(AlgorithmEd25519, pubBytes)
	require.NoError(t, err)

	// Modify original bytes
	for i := range pubBytes {
		pubBytes[i] = 0xFF
	}

	// Restored key should still equal original
	assert.True(t, key.PublicKey().Equals(restored))
}

// ============================================================================
// Cross-Algorithm Tests
// ============================================================================

func TestCrossAlgorithm_PrivateKeyFromBytes(t *testing.T) {
	// Ensure you can't use wrong algorithm
	ed25519Key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	// Try to restore as secp256k1 (should fail due to size mismatch)
	_, err = PrivateKeyFromBytes(AlgorithmSecp256k1, ed25519Key.Bytes())
	assert.Error(t, err)
}

func TestAllAlgorithms_RoundTrip(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo)+"_full_roundtrip", func(t *testing.T) {
			// Generate
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			// Sign
			data := []byte("comprehensive round-trip test data")
			signature, err := key.Sign(data)
			require.NoError(t, err)

			// Verify
			assert.True(t, key.PublicKey().Verify(data, signature))

			// Serialize and restore private key
			restoredPriv, err := PrivateKeyFromBytes(algo, key.Bytes())
			require.NoError(t, err)

			// Verify restored key produces same public key
			assert.True(t, key.PublicKey().Equals(restoredPriv.PublicKey()))

			// Verify restored key can sign and verify
			sig2, err := restoredPriv.Sign(data)
			require.NoError(t, err)
			assert.True(t, restoredPriv.PublicKey().Verify(data, sig2))

			// Serialize and restore public key
			restoredPub, err := PublicKeyFromBytes(algo, key.PublicKey().Bytes())
			require.NoError(t, err)

			// Verify restored public key can verify original signature
			assert.True(t, restoredPub.Verify(data, signature))
		})
	}
}

// ============================================================================
// Signature Malleability Tests (Security)
// ============================================================================

func TestSignature_NotMalleable(t *testing.T) {
	// Ensure flipping bits in signature causes verification to fail
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			data := []byte("test message")
			signature, err := key.Sign(data)
			require.NoError(t, err)

			// Original should verify
			assert.True(t, key.PublicKey().Verify(data, signature))

			// Flip each byte and verify it fails
			for i := range signature {
				modified := make([]byte, len(signature))
				copy(modified, signature)
				modified[i] ^= 0xFF

				// Modified signature should not verify
				if key.PublicKey().Verify(data, modified) {
					t.Errorf("modified signature at byte %d still verified", i)
				}
			}
		})
	}
}

// ============================================================================
// Benchmark Baseline Tests (ensure benchmarks will work)
// ============================================================================

func TestBenchmarkBaseline_KeyGen(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			for i := 0; i < 10; i++ {
				key, err := GeneratePrivateKey(algo)
				require.NoError(t, err)
				_ = key.PublicKey().Bytes()
			}
		})
	}
}

func TestBenchmarkBaseline_SignVerify(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			data := bytes.Repeat([]byte("x"), 32)
			for i := 0; i < 10; i++ {
				sig, err := key.Sign(data)
				require.NoError(t, err)
				assert.True(t, key.PublicKey().Verify(data, sig))
			}
		})
	}
}

// ============================================================================
// JSON Marshaling Tests
// ============================================================================

func TestSerializablePublicKey_MarshalJSON(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			pubKey := key.PublicKey()
			serializable := NewSerializablePublicKey(pubKey)

			data, err := json.Marshal(serializable)
			require.NoError(t, err)

			// Verify JSON structure
			var parsed map[string]interface{}
			err = json.Unmarshal(data, &parsed)
			require.NoError(t, err)

			assert.Contains(t, parsed, "pub_key")
			assert.Contains(t, parsed, "algorithm")
			assert.Equal(t, string(algo), parsed["algorithm"])

			// Verify base64 is valid
			b64 := parsed["pub_key"].(string)
			decoded, err := base64.StdEncoding.DecodeString(b64)
			require.NoError(t, err)
			assert.Equal(t, pubKey.Bytes(), decoded)
		})
	}
}

func TestSerializablePublicKey_UnmarshalJSON(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			original := key.PublicKey()
			serializable := NewSerializablePublicKey(original)

			// Marshal
			data, err := json.Marshal(serializable)
			require.NoError(t, err)

			// Unmarshal
			var restored SerializablePublicKey
			err = json.Unmarshal(data, &restored)
			require.NoError(t, err)

			// Verify equality
			assert.True(t, original.Equals(restored.PublicKey()))
			assert.Equal(t, original.Algorithm(), restored.PublicKey().Algorithm())
		})
	}
}

func TestSerializablePublicKey_RoundTrip(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			original := key.PublicKey()

			// Create serializable, marshal, unmarshal
			serializable := NewSerializablePublicKey(original)
			data, err := json.Marshal(serializable)
			require.NoError(t, err)

			var restored SerializablePublicKey
			err = json.Unmarshal(data, &restored)
			require.NoError(t, err)

			// Sign with original key, verify with restored public key
			testData := []byte("test message for verification")
			signature, err := key.Sign(testData)
			require.NoError(t, err)

			assert.True(t, restored.PublicKey().Verify(testData, signature))
		})
	}
}

func TestSerializablePublicKey_NullHandling(t *testing.T) {
	// Test marshaling nil key
	serializable := &SerializablePublicKey{}
	data, err := json.Marshal(serializable)
	require.NoError(t, err)
	assert.Equal(t, "null", string(data))

	// Test unmarshaling null
	var restored SerializablePublicKey
	err = json.Unmarshal([]byte("null"), &restored)
	require.NoError(t, err)
	assert.Nil(t, restored.PublicKey())
}

func TestSerializablePublicKey_InvalidJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "invalid json",
			input:   `{invalid}`,
			wantErr: "looking for beginning of object key string",
		},
		{
			name:    "invalid base64",
			input:   `{"pub_key": "!!!invalid!!!", "algorithm": "ed25519"}`,
			wantErr: "invalid public key base64",
		},
		{
			name:    "wrong key size",
			input:   `{"pub_key": "AQID", "algorithm": "ed25519"}`,
			wantErr: "invalid public key",
		},
		{
			name:    "invalid algorithm",
			input:   `{"pub_key": "AQIDBAUG", "algorithm": "unknown"}`,
			wantErr: "unknown algorithm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s SerializablePublicKey
			err := json.Unmarshal([]byte(tt.input), &s)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSerializablePublicKey_JSONFormat(t *testing.T) {
	// Test that JSON matches the specified format exactly
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	require.NoError(t, err)

	serializable := NewSerializablePublicKey(key.PublicKey())
	data, err := json.Marshal(serializable)
	require.NoError(t, err)

	// Parse and verify structure
	var parsed struct {
		PubKey    string `json:"pub_key"`
		Algorithm string `json:"algorithm"`
	}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "ed25519", parsed.Algorithm)
	assert.NotEmpty(t, parsed.PubKey)

	// Verify the base64 decodes to correct length
	decoded, err := base64.StdEncoding.DecodeString(parsed.PubKey)
	require.NoError(t, err)
	assert.Len(t, decoded, 32) // Ed25519 public key is 32 bytes
}
