package vectors

import (
	stdecdsa "crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
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
	// Use the null-data-aware builder for vectors that test null message data
	var signDoc *types.SignDoc
	if hasNullMessageData(vector.Input) {
		signDoc = buildSignDocFromInputWithNullData(vector.Input)
	} else {
		signDoc = buildSignDocFromInput(vector.Input)
	}

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
		verifySecp256k1Signature(t, sigVector, signBytes)
	case "secp256k1_seed":
		// This is seed information only, not a signature to verify
		t.Log("Seed vector - no signature to verify")
	case "secp256r1":
		verifySecp256r1Signature(t, sigVector, signBytes)
	case "secp256r1_seed":
		// This is seed information only, not a signature to verify
		t.Log("Seed vector - no signature to verify")
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

// verifySecp256k1Signature verifies a secp256k1 signature.
func verifySecp256k1Signature(t *testing.T, sigVector TestVectorSignature, signBytes []byte) {
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
	assert.Equal(t, 32, len(privateKeyBytes),
		"private key should be 32 bytes")
	assert.Equal(t, 33, len(publicKeyBytes),
		"public key should be 33 bytes (compressed)")
	assert.Equal(t, 64, len(signatureBytes),
		"signature should be 64 bytes (R || S)")

	// Create private key
	privateKey := secp256k1.PrivKeyFromBytes(privateKeyBytes)

	// Verify public key derivation
	derivedPubKey := privateKey.PubKey()
	derivedCompressed := derivedPubKey.SerializeCompressed()
	assert.Equal(t, publicKeyBytes, derivedCompressed,
		"public key should be derivable from private key")

	// Parse the public key for verification
	pubKey, err := secp256k1.ParsePubKey(publicKeyBytes)
	require.NoError(t, err, "should parse public key")

	// The signature is R || S format - convert to secp256k1 signature
	r := new(secp256k1.ModNScalar)
	r.SetByteSlice(signatureBytes[:32])
	s := new(secp256k1.ModNScalar)
	s.SetByteSlice(signatureBytes[32:])
	sig := secp256k1ecdsa.NewSignature(r, s)

	// Verify signature
	valid := sig.Verify(signBytes, pubKey)
	assert.True(t, valid, "signature should verify successfully")
}

// verifySecp256r1Signature verifies a secp256r1 (P-256) signature.
func verifySecp256r1Signature(t *testing.T, sigVector TestVectorSignature, signBytes []byte) {
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
	assert.Equal(t, 32, len(privateKeyBytes),
		"private key should be 32 bytes")
	assert.Equal(t, 33, len(publicKeyBytes),
		"public key should be 33 bytes (compressed)")
	assert.Equal(t, 64, len(signatureBytes),
		"signature should be 64 bytes (R || S)")

	// Create private key
	curve := elliptic.P256()
	privateKey := new(stdecdsa.PrivateKey)
	privateKey.D = new(big.Int).SetBytes(privateKeyBytes)
	privateKey.PublicKey.Curve = curve
	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(privateKeyBytes)

	// Verify public key derivation (compress and compare)
	derivedCompressed := compressP256PublicKey(&privateKey.PublicKey)
	assert.Equal(t, publicKeyBytes, derivedCompressed,
		"public key should be derivable from private key")

	// Parse R and S from signature
	r := new(big.Int).SetBytes(signatureBytes[:32])
	s := new(big.Int).SetBytes(signatureBytes[32:])

	// Decompress public key for verification
	pubKey := decompressP256PublicKey(publicKeyBytes)
	require.NotNil(t, pubKey, "should decompress public key")

	// Verify signature
	valid := stdecdsa.Verify(pubKey, signBytes, r, s)
	assert.True(t, valid, "signature should verify successfully")
}

// decompressP256PublicKey decompresses a 33-byte compressed P-256 public key.
func decompressP256PublicKey(compressed []byte) *stdecdsa.PublicKey {
	if len(compressed) != 33 {
		return nil
	}

	prefix := compressed[0]
	if prefix != 0x02 && prefix != 0x03 {
		return nil
	}

	curve := elliptic.P256()
	x := new(big.Int).SetBytes(compressed[1:])

	// Calculate y² = x³ - 3x + b (mod p)
	// For P-256: p is the field prime, b is the curve parameter
	p := curve.Params().P
	b := curve.Params().B

	// y² = x³ + ax + b where a = -3 for P-256
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	x3.Mod(x3, p)

	threeX := new(big.Int).Mul(x, big.NewInt(3))
	threeX.Mod(threeX, p)

	y2 := new(big.Int).Sub(x3, threeX)
	y2.Add(y2, b)
	y2.Mod(y2, p)

	// y = sqrt(y²) mod p
	y := new(big.Int).ModSqrt(y2, p)
	if y == nil {
		return nil
	}

	// Select correct y based on prefix
	if prefix == 0x02 && y.Bit(0) != 0 {
		y.Sub(p, y)
	} else if prefix == 0x03 && y.Bit(0) == 0 {
		y.Sub(p, y)
	}

	return &stdecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}
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

	// Verify secp256k1 key derivation
	t.Run("secp256k1_key_derivation", func(t *testing.T) {
		// The seed should be deterministic
		seed := WellKnownTestKeys.Secp256k1.Seed
		assert.Len(t, seed, 32, "secp256k1 seed should be 32 bytes")

		// Deriving a key from the seed should produce the same key
		derivedKey := secp256k1.PrivKeyFromBytes(seed)
		assert.Equal(t, WellKnownTestKeys.Secp256k1.PrivateKey.Serialize(), derivedKey.Serialize(),
			"key derivation should be reproducible")

		// The public key should match
		derivedPubKey := derivedKey.PubKey()
		assert.Equal(t, WellKnownTestKeys.Secp256k1.PublicKey.SerializeCompressed(),
			derivedPubKey.SerializeCompressed(),
			"public key should match")
	})

	// Verify secp256r1 key derivation
	t.Run("secp256r1_key_derivation", func(t *testing.T) {
		// The seed should be deterministic
		seed := WellKnownTestKeys.Secp256r1.Seed
		assert.Len(t, seed, 32, "secp256r1 seed should be 32 bytes")

		// Deriving a key from the seed should produce the same key
		curve := elliptic.P256()
		derivedKey := new(stdecdsa.PrivateKey)
		derivedKey.D = new(big.Int).SetBytes(seed)
		derivedKey.PublicKey.Curve = curve
		derivedKey.PublicKey.X, derivedKey.PublicKey.Y = curve.ScalarBaseMult(seed)

		assert.Equal(t, WellKnownTestKeys.Secp256r1.PrivateKey.D.Bytes(), derivedKey.D.Bytes(),
			"key derivation should be reproducible")

		// The public key should match
		assert.Equal(t, WellKnownTestKeys.Secp256r1.PublicKey.X.Bytes(), derivedKey.PublicKey.X.Bytes(),
			"public key X should match")
		assert.Equal(t, WellKnownTestKeys.Secp256r1.PublicKey.Y.Bytes(), derivedKey.PublicKey.Y.Bytes(),
			"public key Y should match")
	})
}

// TestSignDocRoundtrip verifies that SignDoc serialization is reversible.
func TestSignDocRoundtrip(t *testing.T) {
	vectors, err := GenerateTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Build SignDoc from input - use null-aware builder for null data tests
			var original *types.SignDoc
			if hasNullMessageData(vector.Input) {
				original = buildSignDocFromInputWithNullData(vector.Input)
			} else {
				original = buildSignDocFromInput(vector.Input)
			}

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

// TestEmptyChainIDRejection verifies that empty chain_id is rejected.
//
// SECURITY: Empty chain_id would allow cross-chain replay attacks.
// This test documents the security requirement and verifies the validation.
//
// Related: Issue #82 - Document and validate empty chain_id rejection
func TestEmptyChainIDRejection(t *testing.T) {
	// Test with empty chain_id
	input := TestVectorInput{
		ChainID:         "", // INVALID: empty chain_id
		Account:         "alice",
		AccountSequence: "1",
		Nonce:           "1",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/test.msg",
				Data: []byte(`{}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{},
			GasLimit: "0",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)

	// ValidateBasic MUST reject empty chain_id
	err := signDoc.ValidateBasic()
	require.Error(t, err, "empty chain_id MUST be rejected to prevent cross-chain replay attacks")
	assert.Contains(t, err.Error(), "chain_id cannot be empty",
		"error message should clearly indicate the issue")

	t.Log("✓ Empty chain_id is rejected to prevent cross-chain replay attacks")
}

// TestSignatureVerificationWithCryptoPackage tests using the crypto package.
func TestSignatureVerificationWithCryptoPackage(t *testing.T) {
	vectors, err := GenerateTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Build SignDoc - use null-aware builder for null data tests
			var signDoc *types.SignDoc
			if hasNullMessageData(vector.Input) {
				signDoc = buildSignDocFromInputWithNullData(vector.Input)
			} else {
				signDoc = buildSignDocFromInput(vector.Input)
			}
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

// hasNullMessageData checks if any message in the input has nil/null data.
func hasNullMessageData(input TestVectorInput) bool {
	for _, msg := range input.Messages {
		if msg.Data == nil {
			return true
		}
	}
	return false
}
