package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
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

// TestSecp256r1PrivateKeyFromBytes_ScalarValidation tests the scalar range validation
// for secp256r1 private keys. Valid scalars must be in range [1, n-1].
func TestSecp256r1PrivateKeyFromBytes_ScalarValidation(t *testing.T) {
	// P-256 curve order n
	// n = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551
	nHex := "FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632551"
	nBytes, err := hex.DecodeString(nHex)
	if err != nil {
		t.Fatalf("failed to decode n: %v", err)
	}

	tests := []struct {
		name    string
		scalar  []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "scalar = 0 (all zeros)",
			scalar:  make([]byte, 32),
			wantErr: true,
			errMsg:  "scalar out of range",
		},
		{
			name:    "scalar = n (curve order)",
			scalar:  nBytes,
			wantErr: true,
			errMsg:  "scalar out of range",
		},
		{
			name: "scalar = n + 1 (greater than n)",
			scalar: func() []byte {
				// n + 1 = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632552
				nPlus1, _ := hex.DecodeString("FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632552")
				return nPlus1
			}(),
			wantErr: true,
			errMsg:  "scalar out of range",
		},
		{
			name: "scalar = n - 1 (valid, maximum allowed)",
			scalar: func() []byte {
				// n - 1 = 0xFFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632550
				nMinus1, _ := hex.DecodeString("FFFFFFFF00000000FFFFFFFFFFFFFFFFBCE6FAADA7179E84F3B9CAC2FC632550")
				return nMinus1
			}(),
			wantErr: false,
		},
		{
			name:    "scalar = 1 (valid, minimum allowed)",
			scalar:  append(make([]byte, 31), 1),
			wantErr: false,
		},
		{
			name: "scalar = 0xFF...FF (all bits set, greater than n)",
			scalar: func() []byte {
				b := make([]byte, 32)
				for i := range b {
					b[i] = 0xFF
				}
				return b
			}(),
			wantErr: true,
			errMsg:  "scalar out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := PrivateKeyFromBytes(AlgorithmSecp256r1, tt.scalar)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %s, got nil", tt.name)
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %s: %v", tt.name, err)
					return
				}
				// Verify the key is functional
				message := []byte("test message")
				sig, err := key.Sign(message)
				if err != nil {
					t.Errorf("valid key failed to sign: %v", err)
					return
				}
				if !key.PublicKey().Verify(message, sig) {
					t.Error("valid key's signature failed to verify")
				}
			}
		})
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
