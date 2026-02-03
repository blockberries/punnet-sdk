package vectors

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateAndVerifyVectors generates test vectors and verifies them.
func TestGenerateAndVerifyVectors(t *testing.T) {
	vectorFile, err := GenerateTestVectors()
	require.NoError(t, err, "should generate test vectors")
	require.NotEmpty(t, vectorFile.Vectors, "should have vectors")

	t.Logf("Generated %d test vectors", len(vectorFile.Vectors))

	for _, vector := range vectorFile.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			verifyVector(t, vector)
		})
	}
}

// verifyVector verifies a single test vector.
func verifyVector(t *testing.T, vector TestVector) {
	t.Helper()

	// 1. Build SignDoc from input
	signDoc := buildSignDocFromInput(vector.Input)

	// 2. Verify JSON serialization matches expected
	signDocJSON, err := signDoc.ToJSON()
	require.NoError(t, err, "should serialize SignDoc to JSON")
	assert.JSONEq(t, vector.Expected.SignDocJSON, string(signDocJSON),
		"SignDoc JSON should match expected (vector: %s)", vector.Name)

	// 3. Verify sign bytes (hash) matches expected
	signBytes, err := signDoc.GetSignBytes()
	require.NoError(t, err, "should get sign bytes")
	assert.Equal(t, vector.Expected.SignBytesHex, hex.EncodeToString(signBytes),
		"sign bytes should match expected (vector: %s)", vector.Name)

	// 4. Verify signatures
	for algoName, sigVector := range vector.Expected.Signatures {
		t.Run("algorithm_"+algoName, func(t *testing.T) {
			verifySignatureVector(t, algoName, sigVector, signBytes)
		})
	}
}

// verifySignatureVector verifies a signature for a specific algorithm.
func verifySignatureVector(t *testing.T, algoName string, sigVector TestVectorSignature, signBytes []byte) {
	t.Helper()

	switch algoName {
	case "ed25519":
		verifyEd25519Signature(t, sigVector, signBytes)
	case "ed25519_seed":
		// This is seed information only, not a signature to verify
		t.Log("Seed vector - no signature to verify")
	case "secp256k1":
		t.Skip("secp256k1 not yet implemented")
	case "secp256r1":
		t.Skip("secp256r1 not yet implemented")
	default:
		t.Errorf("unknown algorithm: %s", algoName)
	}
}

// verifyEd25519Signature verifies an Ed25519 signature.
func verifyEd25519Signature(t *testing.T, sigVector TestVectorSignature, signBytes []byte) {
	t.Helper()

	// Skip if signature is empty (e.g., seed vectors)
	if sigVector.SignatureHex == "" {
		return
	}

	// Decode keys
	privateKeyBytes, err := hex.DecodeString(sigVector.PrivateKeyHex)
	require.NoError(t, err, "should decode private key")

	publicKeyBytes, err := hex.DecodeString(sigVector.PublicKeyHex)
	require.NoError(t, err, "should decode public key")

	signatureBytes, err := hex.DecodeString(sigVector.SignatureHex)
	require.NoError(t, err, "should decode signature")

	// Verify key sizes
	assert.Equal(t, ed25519.PrivateKeySize, len(privateKeyBytes),
		"private key should be %d bytes", ed25519.PrivateKeySize)
	assert.Equal(t, ed25519.PublicKeySize, len(publicKeyBytes),
		"public key should be %d bytes", ed25519.PublicKeySize)
	assert.Equal(t, ed25519.SignatureSize, len(signatureBytes),
		"signature should be %d bytes", ed25519.SignatureSize)

	// Create keys
	privateKey := ed25519.PrivateKey(privateKeyBytes)
	publicKey := ed25519.PublicKey(publicKeyBytes)

	// Verify public key derivation from private key
	derivedPubKey := privateKey.Public().(ed25519.PublicKey)
	assert.Equal(t, publicKeyBytes, []byte(derivedPubKey),
		"public key should be derivable from private key")

	// Verify signature matches expected
	computedSig := ed25519.Sign(privateKey, signBytes)
	assert.Equal(t, signatureBytes, computedSig,
		"computed signature should match expected")

	// Verify signature is valid
	valid := ed25519.Verify(publicKey, signBytes, signatureBytes)
	assert.True(t, valid, "signature should verify successfully")
}

// TestVectorDeterminism verifies that generating vectors twice produces identical results.
func TestVectorDeterminism(t *testing.T) {
	vectors1, err := GenerateTestVectors()
	require.NoError(t, err)

	vectors2, err := GenerateTestVectors()
	require.NoError(t, err)

	require.Equal(t, len(vectors1.Vectors), len(vectors2.Vectors),
		"should generate same number of vectors")

	for i := range vectors1.Vectors {
		v1 := vectors1.Vectors[i]
		v2 := vectors2.Vectors[i]

		assert.Equal(t, v1.Name, v2.Name, "vector names should match")
		assert.Equal(t, v1.Expected.SignDocJSON, v2.Expected.SignDocJSON,
			"SignDoc JSON should be deterministic")
		assert.Equal(t, v1.Expected.SignBytesHex, v2.Expected.SignBytesHex,
			"sign bytes should be deterministic")

		for algo, sig1 := range v1.Expected.Signatures {
			sig2, ok := v2.Expected.Signatures[algo]
			require.True(t, ok, "should have signature for %s", algo)
			assert.Equal(t, sig1.SignatureHex, sig2.SignatureHex,
				"signature should be deterministic for %s", algo)
		}
	}
}

// TestWellKnownTestKeys verifies the test keys are derived correctly.
func TestWellKnownTestKeys(t *testing.T) {
	// Verify Ed25519 key derivation
	t.Run("ed25519_key_derivation", func(t *testing.T) {
		// The seed should be deterministic
		seed := WellKnownTestKeys.Ed25519.Seed
		assert.Len(t, seed, 32, "Ed25519 seed should be 32 bytes")

		// Deriving a key from the seed should produce the same key
		derivedKey := ed25519.NewKeyFromSeed(seed)
		assert.Equal(t, []byte(WellKnownTestKeys.Ed25519.PrivateKey), []byte(derivedKey),
			"key derivation should be reproducible")

		// The public key should match
		derivedPubKey := derivedKey.Public().(ed25519.PublicKey)
		assert.Equal(t, []byte(WellKnownTestKeys.Ed25519.PublicKey), []byte(derivedPubKey),
			"public key should match")
	})
}

// TestSignDocRoundtrip verifies that SignDoc serialization is reversible.
func TestSignDocRoundtrip(t *testing.T) {
	vectors, err := GenerateTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Build SignDoc from input
			original := buildSignDocFromInput(vector.Input)

			// Serialize to JSON
			jsonBytes, err := original.ToJSON()
			require.NoError(t, err)

			// Parse back
			parsed, err := types.ParseSignDoc(jsonBytes)
			require.NoError(t, err)

			// Serialize again
			jsonBytes2, err := parsed.ToJSON()
			require.NoError(t, err)

			// Should be identical
			assert.Equal(t, jsonBytes, jsonBytes2,
				"roundtrip serialization should be identical")

			// Sign bytes should also match
			signBytes1, _ := original.GetSignBytes()
			signBytes2, _ := parsed.GetSignBytes()
			assert.Equal(t, signBytes1, signBytes2,
				"sign bytes should match after roundtrip")
		})
	}
}

// TestWriteVectorsFile generates and writes the test vectors file.
func TestWriteVectorsFile(t *testing.T) {
	if os.Getenv("GENERATE_VECTORS") != "1" {
		t.Skip("Set GENERATE_VECTORS=1 to generate vectors file")
	}

	vectors, err := GenerateTestVectors()
	require.NoError(t, err)

	// Marshal with indentation for readability
	jsonBytes, err := json.MarshalIndent(vectors, "", "  ")
	require.NoError(t, err)

	// Write to testdata directory
	filename := filepath.Join("..", "..", "testdata", "signing_vectors.json")
	err = os.WriteFile(filename, jsonBytes, 0644)
	require.NoError(t, err)

	t.Logf("Wrote test vectors to %s", filename)
}

// TestLoadAndVerifyVectorsFile loads vectors from file and verifies them.
func TestLoadAndVerifyVectorsFile(t *testing.T) {
	filename := filepath.Join("..", "..", "testdata", "signing_vectors.json")

	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Skip("testdata/signing_vectors.json not found - run with GENERATE_VECTORS=1 first")
	}

	// Load vectors
	jsonBytes, err := os.ReadFile(filename)
	require.NoError(t, err)

	var vectorFile TestVectorFile
	err = json.Unmarshal(jsonBytes, &vectorFile)
	require.NoError(t, err)

	t.Logf("Loaded %d vectors from %s (version %s)",
		len(vectorFile.Vectors), filename, vectorFile.Version)

	// Verify each vector
	for _, vector := range vectorFile.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			verifyVector(t, vector)
		})
	}
}

// TestVectorCategories ensures all expected categories are present.
func TestVectorCategories(t *testing.T) {
	vectors, err := GenerateTestVectors()
	require.NoError(t, err)

	categories := make(map[string]int)
	for _, v := range vectors.Vectors {
		categories[v.Category]++
	}

	// Ensure we have vectors in each category
	assert.Greater(t, categories["serialization"], 0, "should have serialization vectors")
	assert.Greater(t, categories["algorithm"], 0, "should have algorithm vectors")
	assert.Greater(t, categories["edge_case"], 0, "should have edge case vectors")

	t.Logf("Vector categories: %v", categories)
}

// TestSignatureVerificationWithCryptoPackage tests using the crypto package.
func TestSignatureVerificationWithCryptoPackage(t *testing.T) {
	vectors, err := GenerateTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Build SignDoc
			signDoc := buildSignDocFromInput(vector.Input)
			signBytes, err := signDoc.GetSignBytes()
			require.NoError(t, err)

			// Get Ed25519 signature if present
			ed25519Sig, ok := vector.Expected.Signatures["ed25519"]
			if !ok || ed25519Sig.SignatureHex == "" {
				t.Skip("No Ed25519 signature in this vector")
			}

			// Decode signature
			sigBytes, err := hex.DecodeString(ed25519Sig.SignatureHex)
			require.NoError(t, err)

			pubKeyBytes, err := hex.DecodeString(ed25519Sig.PublicKeyHex)
			require.NoError(t, err)

			// Verify using standard ed25519
			pubKey := ed25519.PublicKey(pubKeyBytes)
			valid := ed25519.Verify(pubKey, signBytes, sigBytes)
			assert.True(t, valid, "signature should verify")
		})
	}
}
