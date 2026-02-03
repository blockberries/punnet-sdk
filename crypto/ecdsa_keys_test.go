package crypto

import (
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestSecp256k1KeyGeneration(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey(secp256k1) failed: %v", err)
	}

	if key.Algorithm() != AlgorithmSecp256k1 {
		t.Errorf("expected algorithm secp256k1, got %s", key.Algorithm())
	}

	if len(key.Bytes()) != 32 {
		t.Errorf("expected private key size 32, got %d", len(key.Bytes()))
	}

	pubKey := key.PublicKey()
	if pubKey.Algorithm() != AlgorithmSecp256k1 {
		t.Errorf("expected public key algorithm secp256k1, got %s", pubKey.Algorithm())
	}

	if len(pubKey.Bytes()) != 33 {
		t.Errorf("expected public key size 33, got %d", len(pubKey.Bytes()))
	}
}

func TestSecp256k1SignVerify(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test message for secp256k1")

	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if len(sig) != 64 {
		t.Errorf("expected signature size 64, got %d", len(sig))
	}

	pubKey := key.PublicKey()
	if !pubKey.Verify(message, sig) {
		t.Error("signature verification failed")
	}

	// Verify fails with different message
	if pubKey.Verify([]byte("different message"), sig) {
		t.Error("verification should fail with different message")
	}

	// Verify fails with corrupted signature
	corruptedSig := make([]byte, len(sig))
	copy(corruptedSig, sig)
	corruptedSig[0] ^= 0xFF
	if pubKey.Verify(message, corruptedSig) {
		t.Error("verification should fail with corrupted signature")
	}
}

func TestSecp256k1KeyFromBytes(t *testing.T) {
	// Deterministic seed for reproducibility
	seed := sha256.Sum256([]byte("test-secp256k1-key"))

	key, err := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	// Sign a message
	message := []byte("test message")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Recreate key from same bytes
	key2, err := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	// Sign again - should produce same signature (RFC 6979)
	sig2, err := key2.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if hex.EncodeToString(sig) != hex.EncodeToString(sig2) {
		t.Error("deterministic signatures should be equal")
	}
}

func TestSecp256r1KeyGeneration(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey(secp256r1) failed: %v", err)
	}

	if key.Algorithm() != AlgorithmSecp256r1 {
		t.Errorf("expected algorithm secp256r1, got %s", key.Algorithm())
	}

	if len(key.Bytes()) != 32 {
		t.Errorf("expected private key size 32, got %d", len(key.Bytes()))
	}

	pubKey := key.PublicKey()
	if pubKey.Algorithm() != AlgorithmSecp256r1 {
		t.Errorf("expected public key algorithm secp256r1, got %s", pubKey.Algorithm())
	}

	if len(pubKey.Bytes()) != 33 {
		t.Errorf("expected public key size 33, got %d", len(pubKey.Bytes()))
	}
}

func TestSecp256r1SignVerify(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test message for secp256r1")

	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if len(sig) != 64 {
		t.Errorf("expected signature size 64, got %d", len(sig))
	}

	pubKey := key.PublicKey()
	if !pubKey.Verify(message, sig) {
		t.Error("signature verification failed")
	}

	// Verify fails with different message
	if pubKey.Verify([]byte("different message"), sig) {
		t.Error("verification should fail with different message")
	}

	// Verify fails with corrupted signature
	corruptedSig := make([]byte, len(sig))
	copy(corruptedSig, sig)
	corruptedSig[0] ^= 0xFF
	if pubKey.Verify(message, corruptedSig) {
		t.Error("verification should fail with corrupted signature")
	}
}

func TestSecp256r1KeyFromBytes(t *testing.T) {
	// Deterministic seed for reproducibility
	seed := sha256.Sum256([]byte("test-secp256r1-key"))

	key, err := PrivateKeyFromBytes(AlgorithmSecp256r1, seed[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	// Verify key can sign and verify
	message := []byte("test message")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if !key.PublicKey().Verify(message, sig) {
		t.Error("signature verification failed")
	}
}

// TestSecp256r1_DeterministicSigning verifies that secp256r1 signatures are
// deterministic due to RFC 6979 nonce generation.
//
// This test complements TestSecp256k1_DeterministicSigning in keys_test.go.
// Both curves now use RFC 6979 for deterministic signing, ensuring identical
// signatures for the same key/message pair.
//
// Note: Issue #164 originally requested a non-determinism test, but PR #165
// implemented RFC 6979 for secp256r1. This test verifies the current behavior.
func TestSecp256r1_DeterministicSigning(t *testing.T) {
	// Use deterministic key from seed for reproducibility
	seed := sha256.Sum256([]byte("test-secp256r1-determinism"))
	key, err := PrivateKeyFromBytes(AlgorithmSecp256r1, seed[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	message := []byte("test message")

	// Sign multiple times
	sig1, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	sig2, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Signatures should be identical (RFC 6979 deterministic nonce)
	if hex.EncodeToString(sig1) != hex.EncodeToString(sig2) {
		t.Errorf("secp256r1 signatures should be deterministic (RFC 6979)\nsig1: %s\nsig2: %s",
			hex.EncodeToString(sig1), hex.EncodeToString(sig2))
	}

	// Verify signatures are valid
	if !key.PublicKey().Verify(message, sig1) {
		t.Error("sig1 verification failed")
	}
	if !key.PublicKey().Verify(message, sig2) {
		t.Error("sig2 verification failed")
	}
}

func TestSecp256k1PublicKeyFromBytes(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	pubKeyBytes := key.PublicKey().Bytes()

	pubKey, err := PublicKeyFromBytes(AlgorithmSecp256k1, pubKeyBytes)
	if err != nil {
		t.Fatalf("PublicKeyFromBytes failed: %v", err)
	}

	if !pubKey.Equals(key.PublicKey()) {
		t.Error("reconstructed public key should equal original")
	}

	// Verify signature
	message := []byte("test message")
	sig, _ := key.Sign(message)
	if !pubKey.Verify(message, sig) {
		t.Error("verification with reconstructed key failed")
	}
}

func TestSecp256r1PublicKeyFromBytes(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	pubKeyBytes := key.PublicKey().Bytes()

	pubKey, err := PublicKeyFromBytes(AlgorithmSecp256r1, pubKeyBytes)
	if err != nil {
		t.Fatalf("PublicKeyFromBytes failed: %v", err)
	}

	if !pubKey.Equals(key.PublicKey()) {
		t.Error("reconstructed public key should equal original")
	}

	// Verify signature
	message := []byte("test message")
	sig, _ := key.Sign(message)
	if !pubKey.Verify(message, sig) {
		t.Error("verification with reconstructed key failed")
	}
}

func TestSecp256k1Zeroize(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	// Zeroize the key
	key.Zeroize()

	// Note: We can't easily test that the key was actually zeroized
	// without inspecting internal state. The test here just ensures
	// the method doesn't panic.
}

func TestSecp256r1Zeroize(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	// Zeroize the key
	key.Zeroize()

	// Note: We can't easily test that the key was actually zeroized
	// without inspecting internal state. The test here just ensures
	// the method doesn't panic.
}

// TestSecp256r1LowSNormalization verifies that secp256r1 signatures are normalized
// to low-S form (s <= n/2) to prevent signature malleability.
func TestSecp256r1LowSNormalization(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	n := elliptic.P256().Params().N

	// Sign multiple messages and verify all signatures have low-S
	for i := 0; i < 100; i++ {
		message := []byte("test message " + string(rune(i)))
		sig, err := key.Sign(message)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}

		if !IsLowS(sig, n) {
			t.Errorf("iteration %d: signature S value is not in low-S form", i)
		}

		// Verify signature is still valid
		if !key.PublicKey().Verify(message, sig) {
			t.Errorf("iteration %d: signature verification failed", i)
		}
	}
}

// TestSecp256k1LowSNormalization verifies that secp256k1 signatures from dcrd
// are already in low-S form (RFC 6979 + dcrd implementation).
func TestSecp256k1LowSNormalization(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	n := secp256k1.Params().N

	// Sign multiple messages and verify all signatures have low-S
	for i := 0; i < 100; i++ {
		message := []byte("test message " + string(rune(i)))
		sig, err := key.Sign(message)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}

		if !IsLowS(sig, n) {
			t.Errorf("iteration %d: secp256k1 signature S value is not in low-S form", i)
		}
	}
}

// TestNormalizeLowS tests the normalizeLowS helper function directly.
func TestNormalizeLowS(t *testing.T) {
	// Use P-256 curve order for testing
	n := elliptic.P256().Params().N
	halfN := new(big.Int).Rsh(n, 1)

	tests := []struct {
		name     string
		s        *big.Int
		expected string // "unchanged" or "normalized"
	}{
		{
			name:     "s = 1 (already low)",
			s:        big.NewInt(1),
			expected: "unchanged",
		},
		{
			name:     "s = n/2 (boundary, should be unchanged)",
			s:        new(big.Int).Set(halfN),
			expected: "unchanged",
		},
		{
			name:     "s = n/2 + 1 (just above boundary, should normalize)",
			s:        new(big.Int).Add(halfN, big.NewInt(1)),
			expected: "normalized",
		},
		{
			name:     "s = n - 1 (max high-S, should normalize)",
			s:        new(big.Int).Sub(n, big.NewInt(1)),
			expected: "normalized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalS := new(big.Int).Set(tc.s)
			result := normalizeLowS(tc.s, n)

			// Result should always be <= n/2
			if result.Cmp(halfN) > 0 {
				t.Errorf("result %s is greater than n/2", result.String())
			}

			// Check if it was normalized or unchanged
			if tc.expected == "unchanged" {
				if result.Cmp(originalS) != 0 {
					t.Errorf("expected s to be unchanged, got %s", result.String())
				}
			} else {
				expectedNormalized := new(big.Int).Sub(n, originalS)
				if result.Cmp(expectedNormalized) != 0 {
					t.Errorf("expected normalized s = %s, got %s", expectedNormalized.String(), result.String())
				}
			}
		})
	}
}

// TestIsLowS tests the IsLowS helper function.
func TestIsLowS(t *testing.T) {
	n := elliptic.P256().Params().N
	halfN := new(big.Int).Rsh(n, 1)

	// Create a signature with low-S
	lowS := make([]byte, 64)
	lowS[63] = 0x01 // r = 0, s = 1
	if !IsLowS(lowS, n) {
		t.Error("s=1 should be low-S")
	}

	// Create a signature with s = n/2 (boundary)
	boundaryS := make([]byte, 64)
	halfNBytes := halfN.Bytes()
	copy(boundaryS[64-len(halfNBytes):], halfNBytes)
	if !IsLowS(boundaryS, n) {
		t.Error("s=n/2 should be low-S")
	}

	// Create a signature with high-S (s = n - 1)
	highS := make([]byte, 64)
	nMinus1 := new(big.Int).Sub(n, big.NewInt(1))
	nMinus1Bytes := nMinus1.Bytes()
	copy(highS[64-len(nMinus1Bytes):], nMinus1Bytes)
	if IsLowS(highS, n) {
		t.Error("s=n-1 should be high-S")
	}

	// Invalid signature length
	if IsLowS([]byte{0x00}, n) {
		t.Error("invalid length should return false")
	}
}

// TestHighSSignatureRejection verifies that manually constructed high-S signatures
// are correctly identified.
func TestHighSSignatureRejection(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	n := elliptic.P256().Params().N
	message := []byte("test message for high-S detection")

	// Get a valid low-S signature
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verify it's low-S
	if !IsLowS(sig, n) {
		t.Fatal("fresh signature should be low-S")
	}

	// Create malleable high-S version: (r, n-s)
	s := new(big.Int).SetBytes(sig[32:])
	highS := new(big.Int).Sub(n, s)
	highSSig := make([]byte, 64)
	copy(highSSig[:32], sig[:32]) // same r
	highSBytes := highS.Bytes()
	copy(highSSig[64-len(highSBytes):], highSBytes)

	// Verify high-S is detected
	if IsLowS(highSSig, n) {
		t.Error("malleable signature should be detected as high-S")
	}

	// Both signatures should verify (ECDSA allows both)
	if !key.PublicKey().Verify(message, sig) {
		t.Error("original signature should verify")
	}
	if !key.PublicKey().Verify(message, highSSig) {
		t.Error("malleable signature should also verify (ECDSA allows both forms)")
	}
}
