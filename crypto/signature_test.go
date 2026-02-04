package crypto

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"testing"
)

func TestIsLowSForAlgorithm_Secp256k1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test message for low-S check")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// dcrd produces low-S signatures by default
	if !IsLowSForAlgorithm(sig, AlgorithmSecp256k1) {
		t.Error("secp256k1 Sign() should produce low-S signatures")
	}

	// Create high-S version
	highS := MakeHighS(sig, AlgorithmSecp256k1)
	if highS == nil {
		t.Fatal("MakeHighS returned nil")
	}

	if IsLowSForAlgorithm(highS, AlgorithmSecp256k1) {
		t.Error("high-S signature should not pass IsLowSForAlgorithm check")
	}

	// Both should verify
	pubKey := key.PublicKey()
	if !pubKey.Verify(message, sig) {
		t.Error("low-S signature should verify")
	}
	if !pubKey.Verify(message, highS) {
		t.Error("high-S signature should also verify (malleability)")
	}
}

func TestIsLowSForAlgorithm_Secp256r1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test message for low-S check")

	// Sign multiple times to increase chance of hitting the normalization path
	// (roughly 50% of raw signatures need normalization)
	for i := 0; i < 10; i++ {
		sig, err := key.Sign(message)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}

		if !IsLowSForAlgorithm(sig, AlgorithmSecp256r1) {
			t.Errorf("secp256r1 Sign() should produce low-S signatures (iteration %d)", i)
		}

		if !key.PublicKey().Verify(message, sig) {
			t.Errorf("signature should verify (iteration %d)", i)
		}
	}
}

func TestNormalizeSignature_Secp256k1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test normalization")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Create high-S version
	highS := MakeHighS(sig, AlgorithmSecp256k1)
	if highS == nil {
		t.Fatal("MakeHighS returned nil")
	}

	// Normalize it back
	normalized := NormalizeSignature(highS, AlgorithmSecp256k1)
	if normalized == nil {
		t.Fatal("NormalizeSignature returned nil")
	}

	if !IsLowSForAlgorithm(normalized, AlgorithmSecp256k1) {
		t.Error("normalized signature should be low-S")
	}

	// r should be unchanged
	if !bytes.Equal(sig[:32], normalized[:32]) {
		t.Error("r component should be unchanged after normalization")
	}

	// s should match original low-S signature
	if !bytes.Equal(sig[32:], normalized[32:]) {
		t.Error("s component should match original after round-trip")
	}

	// Normalized signature should verify
	if !key.PublicKey().Verify(message, normalized) {
		t.Error("normalized signature should verify")
	}
}

func TestNormalizeSignature_Secp256r1(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test normalization")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Create high-S version
	highS := MakeHighS(sig, AlgorithmSecp256r1)
	if highS == nil {
		t.Fatal("MakeHighS returned nil")
	}

	if IsLowSForAlgorithm(highS, AlgorithmSecp256r1) {
		t.Error("high-S should not be low-S")
	}

	// Normalize it back
	normalized := NormalizeSignature(highS, AlgorithmSecp256r1)
	if normalized == nil {
		t.Fatal("NormalizeSignature returned nil")
	}

	if !IsLowSForAlgorithm(normalized, AlgorithmSecp256r1) {
		t.Error("normalized signature should be low-S")
	}

	// r should be unchanged
	if !bytes.Equal(sig[:32], normalized[:32]) {
		t.Error("r component should be unchanged after normalization")
	}

	// Normalized signature should verify
	if !key.PublicKey().Verify(message, normalized) {
		t.Error("normalized signature should verify")
	}
}

func TestNormalizeSignature_AlreadyLowS(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("test idempotent normalization")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Normalizing an already-low-S signature should return equivalent result
	normalized := NormalizeSignature(sig, AlgorithmSecp256k1)
	if normalized == nil {
		t.Fatal("NormalizeSignature returned nil")
	}

	if !bytes.Equal(sig, normalized) {
		t.Error("normalizing low-S signature should return equivalent bytes")
	}
}

func TestNormalizeSignature_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		sig  []byte
		algo Algorithm
	}{
		{"nil signature", nil, AlgorithmSecp256k1},
		{"empty signature", []byte{}, AlgorithmSecp256k1},
		{"short signature", make([]byte, 63), AlgorithmSecp256k1},
		{"long signature", make([]byte, 65), AlgorithmSecp256k1},
		{"invalid algorithm", make([]byte, 64), AlgorithmEd25519},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeSignature(tt.sig, tt.algo)
			if result != nil {
				t.Errorf("expected nil for %s, got %v", tt.name, result)
			}
		})
	}
}

func TestIsLowSForAlgorithm_InvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		sig  []byte
		algo Algorithm
	}{
		{"nil signature", nil, AlgorithmSecp256k1},
		{"empty signature", []byte{}, AlgorithmSecp256k1},
		{"short signature", make([]byte, 63), AlgorithmSecp256k1},
		{"long signature", make([]byte, 65), AlgorithmSecp256k1},
		{"invalid algorithm", make([]byte, 64), AlgorithmEd25519},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLowSForAlgorithm(tt.sig, tt.algo)
			if result {
				t.Errorf("expected false for %s", tt.name)
			}
		})
	}
}

func TestMakeHighS_RoundTrip(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	message := []byte("round trip test")
	sig, err := key.Sign(message)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// low-S -> high-S -> low-S should round-trip
	highS := MakeHighS(sig, AlgorithmSecp256k1)
	lowS := NormalizeSignature(highS, AlgorithmSecp256k1)

	if !bytes.Equal(sig, lowS) {
		t.Error("round-trip should produce identical signature")
	}
}

func TestSecp256k1SignProducesLowS(t *testing.T) {
	// Sign many messages to verify dcrd consistently produces low-S
	seed := sha256.Sum256([]byte("deterministic-key-for-testing"))
	key, err := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	for i := 0; i < 100; i++ {
		message := []byte{byte(i)}
		sig, err := key.Sign(message)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}

		if !IsLowSForAlgorithm(sig, AlgorithmSecp256k1) {
			t.Errorf("secp256k1 signature %d was not low-S", i)
		}
	}
}

func TestSecp256r1SignProducesLowS(t *testing.T) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	// secp256r1 uses random nonces, so sign multiple times
	for i := 0; i < 100; i++ {
		message := []byte{byte(i)}
		sig, err := key.Sign(message)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}

		if !IsLowSForAlgorithm(sig, AlgorithmSecp256r1) {
			t.Errorf("secp256r1 signature %d was not low-S", i)
		}
	}
}

func TestHighSSignatureVerifies(t *testing.T) {
	// Verify that high-S signatures still verify (important for accepting
	// signatures from systems that don't enforce low-S)
	for _, algo := range []Algorithm{AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			key, err := GeneratePrivateKey(algo)
			if err != nil {
				t.Fatalf("GeneratePrivateKey failed: %v", err)
			}

			message := []byte("high-S verification test")
			sig, err := key.Sign(message)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}

			// Create high-S version
			highS := MakeHighS(sig, algo)

			// Should still verify
			if !key.PublicKey().Verify(message, highS) {
				t.Error("high-S signature should verify")
			}
		})
	}
}

func TestCurveOrder(t *testing.T) {
	// Verify curve order constants are correct
	k1Order := CurveOrder(AlgorithmSecp256k1)
	if k1Order == nil {
		t.Fatal("CurveOrder(secp256k1) returned nil")
	}
	// secp256k1 order should be 256 bits
	if k1Order.BitLen() != 256 {
		t.Errorf("secp256k1 order should be 256 bits, got %d", k1Order.BitLen())
	}

	r1Order := CurveOrder(AlgorithmSecp256r1)
	if r1Order == nil {
		t.Fatal("CurveOrder(secp256r1) returned nil")
	}
	if r1Order.BitLen() != 256 {
		t.Errorf("secp256r1 order should be 256 bits, got %d", r1Order.BitLen())
	}

	// Ed25519 not supported
	if CurveOrder(AlgorithmEd25519) != nil {
		t.Error("CurveOrder should return nil for ed25519")
	}
}

func TestHalfCurveOrder(t *testing.T) {
	// Verify half-order is correctly computed
	k1Half := HalfCurveOrder(AlgorithmSecp256k1)
	k1Full := CurveOrder(AlgorithmSecp256k1)
	if k1Half == nil || k1Full == nil {
		t.Fatal("curve order returned nil")
	}

	// 2 * halfN should be >= N (due to integer division)
	doubled := new(big.Int).Mul(k1Half, big.NewInt(2))
	if doubled.Cmp(k1Full) > 0 {
		t.Error("2 * halfN should be <= N")
	}

	// halfN should be N/2 (with integer division)
	expected := new(big.Int).Rsh(k1Full, 1)
	if k1Half.Cmp(expected) != 0 {
		t.Error("halfN should equal N >> 1")
	}
}

func TestCurveOrder_DefensiveCopy(t *testing.T) {
	// Verify that CurveOrder returns a defensive copy that can be safely mutated
	// without affecting subsequent calls (issue #185)
	for _, algo := range []Algorithm{AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			// Get original value
			original := CurveOrder(algo)
			if original == nil {
				t.Fatal("CurveOrder returned nil")
			}
			originalBytes := original.Bytes()

			// Mutate the returned value
			original.Add(original, big.NewInt(1))

			// Get a fresh copy - should be unaffected by our mutation
			fresh := CurveOrder(algo)
			if fresh == nil {
				t.Fatal("CurveOrder returned nil after mutation")
			}

			if !bytes.Equal(fresh.Bytes(), originalBytes) {
				t.Error("CurveOrder() was corrupted by caller mutation - defensive copy not working")
			}
		})
	}
}

func TestHalfCurveOrder_DefensiveCopy(t *testing.T) {
	// Verify that HalfCurveOrder returns a defensive copy that can be safely mutated
	// without affecting subsequent calls (issue #185)
	for _, algo := range []Algorithm{AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			// Get original value
			original := HalfCurveOrder(algo)
			if original == nil {
				t.Fatal("HalfCurveOrder returned nil")
			}
			originalBytes := original.Bytes()

			// Mutate the returned value
			original.Add(original, big.NewInt(1))

			// Get a fresh copy - should be unaffected by our mutation
			fresh := HalfCurveOrder(algo)
			if fresh == nil {
				t.Fatal("HalfCurveOrder returned nil after mutation")
			}

			if !bytes.Equal(fresh.Bytes(), originalBytes) {
				t.Error("HalfCurveOrder() was corrupted by caller mutation - defensive copy not working")
			}
		})
	}
}
