package crypto

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SecurityVectorFile is the root structure of the security test vectors JSON file.
type SecurityVectorFile struct {
	Version         string            `json:"version"`
	Generated       string            `json:"generated"`
	Description     string            `json:"description"`
	SecurityVectors []SecurityVector  `json:"security_vectors"`
	Notes           map[string]string `json:"notes"`
}

// SecurityVector represents a single malformed signature test case.
type SecurityVector struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Category       string `json:"category"`
	Algorithm      string `json:"algorithm"`
	PublicKeyHex   string `json:"public_key_hex"`
	MessageHex     string `json:"message_hex"`
	SignatureHex   string `json:"signature_hex"`
	ExpectedResult string `json:"expected_result"`
	Reason         string `json:"reason"`
}

// TestSecurityVectors loads and runs all security test vectors from the JSON file.
// These vectors test that implementations properly reject malformed inputs.
func TestSecurityVectors(t *testing.T) {
	filename := filepath.Join("..", "testdata", "security_vectors.json")

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("security_vectors.json not found at %s", filename)
	}

	// Load vectors
	jsonBytes, err := os.ReadFile(filename)
	require.NoError(t, err)

	var vectorFile SecurityVectorFile
	err = json.Unmarshal(jsonBytes, &vectorFile)
	require.NoError(t, err)

	t.Logf("Loaded %d security vectors from %s (version %s)",
		len(vectorFile.SecurityVectors), filename, vectorFile.Version)

	// Run each vector
	for _, vector := range vectorFile.SecurityVectors {
		t.Run(vector.Name, func(t *testing.T) {
			testSecurityVector(t, vector)
		})
	}
}

// testSecurityVector tests a single security vector.
func testSecurityVector(t *testing.T, vector SecurityVector) {
	t.Helper()

	// Decode message
	message, err := hex.DecodeString(vector.MessageHex)
	require.NoError(t, err, "should decode message hex")

	// Decode signature (may be empty)
	var signature []byte
	if vector.SignatureHex != "" {
		signature, err = hex.DecodeString(vector.SignatureHex)
		require.NoError(t, err, "should decode signature hex")
	}

	// Decode public key
	publicKeyBytes, err := hex.DecodeString(vector.PublicKeyHex)
	if err != nil {
		// If we can't decode the public key hex, this is expected for some vectors
		if vector.ExpectedResult == "reject" {
			t.Logf("Public key hex decode failed (expected for this vector): %v", err)
			return
		}
		t.Fatalf("unexpected public key decode error: %v", err)
	}

	// Get the algorithm
	var algo Algorithm
	switch vector.Algorithm {
	case "ed25519":
		algo = AlgorithmEd25519
	case "secp256k1":
		algo = AlgorithmSecp256k1
	case "secp256r1":
		algo = AlgorithmSecp256r1
	default:
		t.Fatalf("unknown algorithm: %s", vector.Algorithm)
	}

	// Try to create public key - may fail for invalid pubkey vectors
	pubKey, pubKeyErr := PublicKeyFromBytes(algo, publicKeyBytes)

	// Handle invalid public key vectors
	if vector.Category == "invalid_pubkey" {
		if pubKeyErr != nil {
			t.Logf("Public key creation failed as expected: %v", pubKeyErr)
			return // Test passes - we correctly rejected the invalid pubkey
		}
		// If we got here, the pubkey was accepted. Try verification - it should fail.
		result := pubKey.Verify(message, signature)
		assert.False(t, result, "verification with invalid/wrong pubkey should fail: %s", vector.Reason)
		return
	}

	// For other categories, we need a valid public key
	if pubKeyErr != nil {
		t.Fatalf("unexpected public key error for category %s: %v", vector.Category, pubKeyErr)
	}

	// Verify the signature
	result := pubKey.Verify(message, signature)

	// All security vectors should result in verification failure
	if vector.ExpectedResult == "reject" {
		assert.False(t, result, "signature verification should fail: %s", vector.Reason)
		t.Logf("Correctly rejected: %s", vector.Reason)
	} else {
		// Future: could add "accept" vectors for valid edge cases
		t.Logf("Vector result: %v", result)
	}
}

// TestSecurityVectorCategories ensures all expected categories are tested.
func TestSecurityVectorCategories(t *testing.T) {
	filename := filepath.Join("..", "testdata", "security_vectors.json")

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Skip("security_vectors.json not found")
	}

	jsonBytes, err := os.ReadFile(filename)
	require.NoError(t, err)

	var vectorFile SecurityVectorFile
	err = json.Unmarshal(jsonBytes, &vectorFile)
	require.NoError(t, err)

	categories := make(map[string]int)
	algorithms := make(map[string]int)

	for _, v := range vectorFile.SecurityVectors {
		categories[v.Category]++
		algorithms[v.Algorithm]++
	}

	// Verify we have vectors for each expected category
	assert.Greater(t, categories["invalid_signature"], 0, "should have invalid_signature vectors")
	assert.Greater(t, categories["wrong_length"], 0, "should have wrong_length vectors")
	assert.Greater(t, categories["out_of_range"], 0, "should have out_of_range vectors")
	assert.Greater(t, categories["invalid_pubkey"], 0, "should have invalid_pubkey vectors")
	assert.Greater(t, categories["boundary"], 0, "should have boundary vectors")

	// Verify we have vectors for each algorithm
	assert.Greater(t, algorithms["ed25519"], 0, "should have ed25519 vectors")
	assert.Greater(t, algorithms["secp256k1"], 0, "should have secp256k1 vectors")
	assert.Greater(t, algorithms["secp256r1"], 0, "should have secp256r1 vectors")

	t.Logf("Categories: %v", categories)
	t.Logf("Algorithms: %v", algorithms)
}

// TestMalformedSignatureLengths tests signature length validation directly.
func TestMalformedSignatureLengths(t *testing.T) {
	testCases := []struct {
		name      string
		algo      Algorithm
		keyBytes  int
		sigLength int
		wantValid bool
	}{
		// Ed25519 cases
		{"ed25519_sig_0", AlgorithmEd25519, 32, 0, false},
		{"ed25519_sig_1", AlgorithmEd25519, 32, 1, false},
		{"ed25519_sig_31", AlgorithmEd25519, 32, 31, false},
		{"ed25519_sig_63", AlgorithmEd25519, 32, 63, false},
		{"ed25519_sig_64", AlgorithmEd25519, 32, 64, false}, // Wrong size for ed25519
		{"ed25519_sig_65", AlgorithmEd25519, 32, 65, false},

		// secp256k1 cases
		{"secp256k1_sig_0", AlgorithmSecp256k1, 33, 0, false},
		{"secp256k1_sig_1", AlgorithmSecp256k1, 33, 1, false},
		{"secp256k1_sig_63", AlgorithmSecp256k1, 33, 63, false},
		{"secp256k1_sig_65", AlgorithmSecp256k1, 33, 65, false},

		// secp256r1 cases
		{"secp256r1_sig_0", AlgorithmSecp256r1, 33, 0, false},
		{"secp256r1_sig_63", AlgorithmSecp256r1, 33, 63, false},
		{"secp256r1_sig_65", AlgorithmSecp256r1, 33, 65, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate a valid key pair
			key, err := GeneratePrivateKey(tc.algo)
			require.NoError(t, err)

			// Create malformed signature of specified length
			malformedSig := make([]byte, tc.sigLength)
			for i := range malformedSig {
				malformedSig[i] = byte(i % 256)
			}

			// Try to verify
			result := key.PublicKey().Verify([]byte("test message"), malformedSig)
			assert.False(t, result, "malformed signature of length %d should not verify", tc.sigLength)
		})
	}
}

// TestZeroComponentSignatures tests that signatures with zero R or S are rejected.
func TestZeroComponentSignatures(t *testing.T) {
	algorithms := []Algorithm{AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(algo.String(), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			message := []byte("test message")

			// R = 0
			t.Run("R_zero", func(t *testing.T) {
				sig := make([]byte, 64)
				// Leave R as all zeros
				// Put some non-zero value in S
				for i := 32; i < 64; i++ {
					sig[i] = byte(i)
				}

				result := key.PublicKey().Verify(message, sig)
				assert.False(t, result, "signature with R=0 should not verify")
			})

			// S = 0
			t.Run("S_zero", func(t *testing.T) {
				sig := make([]byte, 64)
				// Put some non-zero value in R
				for i := 0; i < 32; i++ {
					sig[i] = byte(i + 1)
				}
				// Leave S as all zeros

				result := key.PublicKey().Verify(message, sig)
				assert.False(t, result, "signature with S=0 should not verify")
			})

			// Both zero
			t.Run("both_zero", func(t *testing.T) {
				sig := make([]byte, 64)
				// All zeros

				result := key.PublicKey().Verify(message, sig)
				assert.False(t, result, "signature with R=0 and S=0 should not verify")
			})
		})
	}
}

// TestOverflowSignatureComponents tests signatures with R or S >= curve order.
func TestOverflowSignatureComponents(t *testing.T) {
	t.Run("secp256k1", func(t *testing.T) {
		key, err := GeneratePrivateKey(AlgorithmSecp256k1)
		require.NoError(t, err)

		message := []byte("test message")

		// secp256k1 curve order n
		// 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141
		curveOrderHex := "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"
		curveOrder, err := hex.DecodeString(curveOrderHex)
		require.NoError(t, err)

		// Test R = n (exactly curve order)
		t.Run("R_equals_n", func(t *testing.T) {
			sig := make([]byte, 64)
			copy(sig[:32], curveOrder)
			copy(sig[32:], make([]byte, 32)) // S = 0 to keep it simple

			result := key.PublicKey().Verify(message, sig)
			assert.False(t, result, "signature with R=n should not verify")
		})

		// Test R = n + 1 (just over curve order)
		t.Run("R_greater_than_n", func(t *testing.T) {
			sig := make([]byte, 64)
			rOverflow, _ := hex.DecodeString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364142")
			copy(sig[:32], rOverflow)

			result := key.PublicKey().Verify(message, sig)
			assert.False(t, result, "signature with R>n should not verify")
		})

		// Test R = 0xFFFF...FFFF (maximum 256-bit value)
		t.Run("R_max_256bit", func(t *testing.T) {
			sig := make([]byte, 64)
			for i := 0; i < 32; i++ {
				sig[i] = 0xFF
			}

			result := key.PublicKey().Verify(message, sig)
			assert.False(t, result, "signature with R=max should not verify")
		})
	})

	t.Run("secp256r1", func(t *testing.T) {
		key, err := GeneratePrivateKey(AlgorithmSecp256r1)
		require.NoError(t, err)

		message := []byte("test message")

		// secp256r1 curve order n
		// 0xffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551
		curveOrderHex := "ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551"
		curveOrder, err := hex.DecodeString(curveOrderHex)
		require.NoError(t, err)

		// Test S = n (exactly curve order)
		t.Run("S_equals_n", func(t *testing.T) {
			sig := make([]byte, 64)
			copy(sig[:32], []byte{0x01}) // R = 1
			copy(sig[32:], curveOrder)

			result := key.PublicKey().Verify(message, sig)
			assert.False(t, result, "signature with S=n should not verify")
		})

		// Test S = 0xFFFF...FFFF (maximum 256-bit value)
		t.Run("S_max_256bit", func(t *testing.T) {
			sig := make([]byte, 64)
			sig[0] = 0x01 // R = 1
			for i := 32; i < 64; i++ {
				sig[i] = 0xFF
			}

			result := key.PublicKey().Verify(message, sig)
			assert.False(t, result, "signature with S=max should not verify")
		})
	})
}

// TestInvalidPublicKeyFormats tests that invalid public key formats are rejected.
func TestInvalidPublicKeyFormats(t *testing.T) {
	t.Run("secp256k1", func(t *testing.T) {
		// Invalid prefix
		t.Run("invalid_prefix_0x00", func(t *testing.T) {
			invalidPubKey := make([]byte, 33)
			invalidPubKey[0] = 0x00 // Invalid prefix
			for i := 1; i < 33; i++ {
				invalidPubKey[i] = byte(i)
			}

			_, err := PublicKeyFromBytes(AlgorithmSecp256k1, invalidPubKey)
			assert.Error(t, err, "should reject pubkey with 0x00 prefix")
		})

		t.Run("invalid_prefix_0x05", func(t *testing.T) {
			invalidPubKey := make([]byte, 33)
			invalidPubKey[0] = 0x05 // Invalid prefix
			for i := 1; i < 33; i++ {
				invalidPubKey[i] = byte(i)
			}

			_, err := PublicKeyFromBytes(AlgorithmSecp256k1, invalidPubKey)
			assert.Error(t, err, "should reject pubkey with 0x05 prefix")
		})

		// Wrong length
		t.Run("wrong_length_32", func(t *testing.T) {
			shortPubKey := make([]byte, 32)
			shortPubKey[0] = 0x02

			_, err := PublicKeyFromBytes(AlgorithmSecp256k1, shortPubKey)
			assert.Error(t, err, "should reject 32-byte pubkey")
		})

		t.Run("wrong_length_34", func(t *testing.T) {
			longPubKey := make([]byte, 34)
			longPubKey[0] = 0x02

			_, err := PublicKeyFromBytes(AlgorithmSecp256k1, longPubKey)
			assert.Error(t, err, "should reject 34-byte pubkey")
		})

		// Point not on curve
		t.Run("point_not_on_curve", func(t *testing.T) {
			// This X coordinate likely doesn't have a valid Y on secp256k1
			invalidPubKey := make([]byte, 33)
			invalidPubKey[0] = 0x02
			for i := 1; i < 33; i++ {
				invalidPubKey[i] = 0xFF
			}

			_, err := PublicKeyFromBytes(AlgorithmSecp256k1, invalidPubKey)
			assert.Error(t, err, "should reject point not on curve")
		})
	})

	t.Run("secp256r1", func(t *testing.T) {
		// Invalid prefix
		t.Run("invalid_prefix_0x01", func(t *testing.T) {
			invalidPubKey := make([]byte, 33)
			invalidPubKey[0] = 0x01 // Invalid prefix
			for i := 1; i < 33; i++ {
				invalidPubKey[i] = byte(i)
			}

			_, err := PublicKeyFromBytes(AlgorithmSecp256r1, invalidPubKey)
			assert.Error(t, err, "should reject pubkey with 0x01 prefix")
		})

		// Wrong length
		t.Run("wrong_length_32", func(t *testing.T) {
			shortPubKey := make([]byte, 32)

			_, err := PublicKeyFromBytes(AlgorithmSecp256r1, shortPubKey)
			assert.Error(t, err, "should reject 32-byte pubkey")
		})

		// Point not on curve
		t.Run("point_not_on_curve", func(t *testing.T) {
			invalidPubKey := make([]byte, 33)
			invalidPubKey[0] = 0x02
			for i := 1; i < 33; i++ {
				invalidPubKey[i] = 0xFF
			}

			_, err := PublicKeyFromBytes(AlgorithmSecp256r1, invalidPubKey)
			assert.Error(t, err, "should reject point not on curve")
		})
	})

	t.Run("ed25519", func(t *testing.T) {
		// Wrong length
		t.Run("wrong_length_31", func(t *testing.T) {
			shortPubKey := make([]byte, 31)

			_, err := PublicKeyFromBytes(AlgorithmEd25519, shortPubKey)
			assert.Error(t, err, "should reject 31-byte pubkey")
		})

		t.Run("wrong_length_33", func(t *testing.T) {
			longPubKey := make([]byte, 33)

			_, err := PublicKeyFromBytes(AlgorithmEd25519, longPubKey)
			assert.Error(t, err, "should reject 33-byte pubkey")
		})

		// Ed25519 accepts any 32 bytes as a public key at the encoding level,
		// but verification will fail for non-valid points
		t.Run("verification_fails_for_invalid_point", func(t *testing.T) {
			// Create a pubkey with all 0xFF - likely not a valid Ed25519 point
			invalidPubKey := make([]byte, 32)
			for i := range invalidPubKey {
				invalidPubKey[i] = 0xFF
			}

			pubKey, err := PublicKeyFromBytes(AlgorithmEd25519, invalidPubKey)
			// Ed25519 doesn't validate the point at creation time
			if err == nil {
				// Verification should fail
				result := pubKey.Verify([]byte("test"), make([]byte, 64))
				assert.False(t, result, "verification with invalid pubkey should fail")
			}
		})
	})
}

// TestWrongKeyVerification tests that a signature doesn't verify with a different key.
func TestWrongKeyVerification(t *testing.T) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		t.Run(algo.String(), func(t *testing.T) {
			// Generate two different key pairs
			key1, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			key2, err := GeneratePrivateKey(algo)
			require.NoError(t, err)

			message := []byte("test message")

			// Sign with key1
			sig, err := key1.Sign(message)
			require.NoError(t, err)

			// Verify with key1 should succeed
			assert.True(t, key1.PublicKey().Verify(message, sig),
				"signature should verify with correct key")

			// Verify with key2 should fail
			assert.False(t, key2.PublicKey().Verify(message, sig),
				"signature should NOT verify with different key")
		})
	}
}
