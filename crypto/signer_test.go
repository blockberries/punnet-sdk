package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"
)

// mockSignDoc implements SignBytesProvider for testing.
type mockSignDoc struct {
	data      []byte
	shouldErr bool
}

func (m *mockSignDoc) GetSignBytes() ([]byte, error) {
	if m.shouldErr {
		return nil, ErrInvalidAlgorithm // reuse existing error
	}
	hash := sha256.Sum256(m.data)
	return hash[:], nil
}

func TestNewSigner(t *testing.T) {
	for _, algo := range []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			privateKey, err := GeneratePrivateKey(algo)
			if err != nil {
				t.Fatalf("GeneratePrivateKey failed: %v", err)
			}

			signer := NewSigner(privateKey)

			// Verify interface methods
			if signer.Algorithm() != algo {
				t.Errorf("Algorithm() = %v, want %v", signer.Algorithm(), algo)
			}

			pubKey := signer.PublicKey()
			if pubKey == nil {
				t.Error("PublicKey() returned nil")
			}

			if pubKey.Algorithm() != algo {
				t.Errorf("PublicKey().Algorithm() = %v, want %v", pubKey.Algorithm(), algo)
			}
		})
	}
}

func TestBasicSigner_Sign(t *testing.T) {
	for _, algo := range []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			privateKey, err := GeneratePrivateKey(algo)
			if err != nil {
				t.Fatalf("GeneratePrivateKey failed: %v", err)
			}

			signer := NewSigner(privateKey)
			message := []byte("test message for signing")

			sig, err := signer.Sign(message)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}

			if len(sig) == 0 {
				t.Error("Sign returned empty signature")
			}

			// Verify signature
			if !signer.PublicKey().Verify(message, sig) {
				t.Error("signature verification failed")
			}
		})
	}
}

func TestBasicSigner_ThreadSafety(t *testing.T) {
	privateKey, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	signer := NewSigner(privateKey)
	message := []byte("concurrent signing test")

	// Run concurrent Sign operations
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				sig, err := signer.Sign(message)
				if err != nil {
					t.Errorf("Sign failed: %v", err)
					return
				}
				if !signer.PublicKey().Verify(message, sig) {
					t.Error("verification failed")
					return
				}
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestSignature_JSONRoundtrip(t *testing.T) {
	for _, algo := range []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			privateKey, err := GeneratePrivateKey(algo)
			if err != nil {
				t.Fatalf("GeneratePrivateKey failed: %v", err)
			}

			message := []byte("test message")
			sig, err := privateKey.Sign(message)
			if err != nil {
				t.Fatalf("Sign failed: %v", err)
			}

			original := &Signature{
				PubKey:    privateKey.PublicKey().Bytes(),
				Signature: sig,
				Algorithm: algo,
			}

			// Marshal to JSON
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal back
			var decoded Signature
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Verify fields match
			if !bytes.Equal(decoded.PubKey, original.PubKey) {
				t.Error("PubKey mismatch after JSON roundtrip")
			}
			if !bytes.Equal(decoded.Signature, original.Signature) {
				t.Error("Signature mismatch after JSON roundtrip")
			}
			if decoded.Algorithm != original.Algorithm {
				t.Errorf("Algorithm = %v, want %v", decoded.Algorithm, original.Algorithm)
			}

			// Verify the decoded signature still verifies
			pubKey, err := PublicKeyFromBytes(decoded.Algorithm, decoded.PubKey)
			if err != nil {
				t.Fatalf("PublicKeyFromBytes failed: %v", err)
			}
			if !pubKey.Verify(message, decoded.Signature) {
				t.Error("signature verification failed after JSON roundtrip")
			}
		})
	}
}

func TestSignSignDoc_AllAlgorithms(t *testing.T) {
	for _, algo := range []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1} {
		t.Run(algo.String(), func(t *testing.T) {
			privateKey, err := GeneratePrivateKey(algo)
			if err != nil {
				t.Fatalf("GeneratePrivateKey failed: %v", err)
			}

			signDoc := &mockSignDoc{data: []byte(`{"chain_id":"test","account":"alice"}`)}

			sig, err := SignSignDoc(signDoc, privateKey)
			if err != nil {
				t.Fatalf("SignSignDoc failed: %v", err)
			}

			// Verify Signature fields
			if sig == nil {
				t.Fatal("SignSignDoc returned nil")
			}

			if !bytes.Equal(sig.PubKey, privateKey.PublicKey().Bytes()) {
				t.Error("PubKey mismatch")
			}

			if sig.Algorithm != algo {
				t.Errorf("Algorithm = %v, want %v", sig.Algorithm, algo)
			}

			if len(sig.Signature) == 0 {
				t.Error("Signature is empty")
			}

			// Verify the signature is valid
			signBytes, _ := signDoc.GetSignBytes()
			pubKey := privateKey.PublicKey()
			if !pubKey.Verify(signBytes, sig.Signature) {
				t.Error("signature verification failed")
			}
		})
	}
}

func TestSignSignDoc_GetSignBytesError(t *testing.T) {
	privateKey, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	signDoc := &mockSignDoc{shouldErr: true}

	sig, err := SignSignDoc(signDoc, privateKey)
	if err == nil {
		t.Error("expected error when GetSignBytes fails")
	}
	if sig != nil {
		t.Error("expected nil signature on error")
	}
}

func TestSignSignDoc_Determinism(t *testing.T) {
	// Ed25519 signatures are deterministic for the same key and message.
	// secp256k1 with RFC 6979 is also deterministic.
	seed := sha256.Sum256([]byte("deterministic-key-seed"))
	privateKey, err := PrivateKeyFromBytes(AlgorithmEd25519, append(seed[:], seed[:]...))
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	signDoc := &mockSignDoc{data: []byte(`{"chain_id":"test"}`)}

	// Sign twice
	sig1, err := SignSignDoc(signDoc, privateKey)
	if err != nil {
		t.Fatalf("first SignSignDoc failed: %v", err)
	}

	sig2, err := SignSignDoc(signDoc, privateKey)
	if err != nil {
		t.Fatalf("second SignSignDoc failed: %v", err)
	}

	// Ed25519 signatures should be identical
	if !bytes.Equal(sig1.Signature, sig2.Signature) {
		t.Error("Ed25519 signatures should be deterministic")
	}
}

func TestSignSignDoc_DifferentMessages(t *testing.T) {
	privateKey, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		t.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	signDoc1 := &mockSignDoc{data: []byte(`{"chain_id":"chain-a"}`)}
	signDoc2 := &mockSignDoc{data: []byte(`{"chain_id":"chain-b"}`)}

	sig1, err := SignSignDoc(signDoc1, privateKey)
	if err != nil {
		t.Fatalf("first SignSignDoc failed: %v", err)
	}

	sig2, err := SignSignDoc(signDoc2, privateKey)
	if err != nil {
		t.Fatalf("second SignSignDoc failed: %v", err)
	}

	// Different messages should produce different signatures
	if bytes.Equal(sig1.Signature, sig2.Signature) {
		t.Error("different messages should produce different signatures")
	}

	// Both should verify against their respective sign bytes
	signBytes1, _ := signDoc1.GetSignBytes()
	signBytes2, _ := signDoc2.GetSignBytes()

	pubKey := privateKey.PublicKey()
	if !pubKey.Verify(signBytes1, sig1.Signature) {
		t.Error("sig1 verification failed")
	}
	if !pubKey.Verify(signBytes2, sig2.Signature) {
		t.Error("sig2 verification failed")
	}

	// Cross-verification should fail
	if pubKey.Verify(signBytes1, sig2.Signature) {
		t.Error("cross-verification should fail (sig2 on signBytes1)")
	}
	if pubKey.Verify(signBytes2, sig1.Signature) {
		t.Error("cross-verification should fail (sig1 on signBytes2)")
	}
}

func TestSignSignDoc_Secp256k1Determinism(t *testing.T) {
	// secp256k1 uses RFC 6979 for deterministic k, so signatures are deterministic
	seed := sha256.Sum256([]byte("secp256k1-test-seed"))
	privateKey, err := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	signDoc := &mockSignDoc{data: []byte(`{"test":"secp256k1"}`)}

	sig1, err := SignSignDoc(signDoc, privateKey)
	if err != nil {
		t.Fatalf("first SignSignDoc failed: %v", err)
	}

	sig2, err := SignSignDoc(signDoc, privateKey)
	if err != nil {
		t.Fatalf("second SignSignDoc failed: %v", err)
	}

	// RFC 6979 makes secp256k1 signatures deterministic
	if !bytes.Equal(sig1.Signature, sig2.Signature) {
		t.Error("secp256k1 signatures with RFC 6979 should be deterministic")
	}
}

// BenchmarkSignSignDoc measures signing performance.
// Results indicate hot-path latency for transaction signing.
func BenchmarkSignSignDoc(b *testing.B) {
	for _, algo := range []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1} {
		b.Run(algo.String(), func(b *testing.B) {
			privateKey, err := GeneratePrivateKey(algo)
			if err != nil {
				b.Fatalf("GeneratePrivateKey failed: %v", err)
			}

			signDoc := &mockSignDoc{data: []byte(`{"chain_id":"bench-chain","account":"bench-account","messages":[{"type":"test"}]}`)}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := SignSignDoc(signDoc, privateKey)
				if err != nil {
					b.Fatalf("SignSignDoc failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkNewSigner measures signer creation overhead.
// Should be O(1) with zero allocations.
func BenchmarkNewSigner(b *testing.B) {
	privateKey, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatalf("GeneratePrivateKey failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewSigner(privateKey)
	}
}
