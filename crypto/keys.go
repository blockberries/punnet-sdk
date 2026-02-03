package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime"
)

// Zeroize securely overwrites a byte slice with zeros.
// Used to clear sensitive data (private keys) from memory.
//
// Implementation uses subtle.XORBytes(b, b, b) which XORs each byte with itself,
// producing zeros. This operation cannot be optimized away by the compiler because:
// 1. crypto/subtle functions are specifically designed to resist optimization
// 2. The operation has observable side effects (modifying memory)
// 3. runtime.KeepAlive ensures the slice isn't considered "dead" after zeroing
//
// This is more robust than a naive loop like `for i := range b { b[i] = 0 }`
// which compilers may detect as a dead store and eliminate entirely.
//
// Complexity: O(n) where n is slice length.
// Memory: Zero allocations.
// Benchmark: See BenchmarkZeroize in crypto_benchmark_test.go
func Zeroize(b []byte) {
	if len(b) == 0 {
		return
	}
	// XOR each byte with itself to produce zeros.
	// subtle.XORBytes cannot be optimized away by the compiler.
	subtle.XORBytes(b, b, b)
	// Prevent the compiler from treating b as dead after zeroing.
	// This ensures the zeroing operation is not eliminated as a dead store.
	runtime.KeepAlive(b)
}

// PublicKey represents a public key for signature verification.
type PublicKey interface {
	// Bytes returns the raw public key bytes.
	// Complexity: O(1), zero allocations (returns backing slice).
	Bytes() []byte

	// Algorithm returns the key's algorithm.
	// Complexity: O(1).
	Algorithm() Algorithm

	// Verify verifies a signature against this public key.
	// Complexity: O(n) where n is data length.
	Verify(data, signature []byte) bool

	// Equals checks if two public keys are equal.
	// Uses constant-time comparison to prevent timing attacks.
	Equals(other PublicKey) bool

	// String returns the Base64-encoded representation.
	String() string
}

// PrivateKey represents a private key for signing.
type PrivateKey interface {
	// Bytes returns the raw private key bytes.
	// WARNING: Handle with care. Consider zeroing after use.
	// Complexity: O(1), zero allocations.
	Bytes() []byte

	// Algorithm returns the key's algorithm.
	// Complexity: O(1).
	Algorithm() Algorithm

	// PublicKey returns the corresponding public key.
	// Complexity: O(1) for Ed25519 (derived from private key bytes).
	PublicKey() PublicKey

	// Sign signs the given data.
	// Complexity: O(n) where n is data length.
	Sign(data []byte) ([]byte, error)

	// Zeroize overwrites the private key bytes with zeros.
	// Call this when done with the key to minimize exposure in memory.
	// After calling Zeroize, the key is no longer usable.
	Zeroize()
}

// ed25519PublicKey implements PublicKey for Ed25519.
type ed25519PublicKey struct {
	key ed25519.PublicKey
}

// Bytes returns the raw public key bytes.
func (k *ed25519PublicKey) Bytes() []byte {
	return k.key
}

// Algorithm returns Ed25519.
func (k *ed25519PublicKey) Algorithm() Algorithm {
	return AlgorithmEd25519
}

// Verify verifies a signature.
func (k *ed25519PublicKey) Verify(data, signature []byte) bool {
	if len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(k.key, data, signature)
}

// Equals checks equality using constant-time comparison.
// Complexity: O(n) where n is key length (32 bytes for Ed25519).
// Uses crypto/subtle.ConstantTimeCompare to prevent timing attacks.
func (k *ed25519PublicKey) Equals(other PublicKey) bool {
	if other == nil || other.Algorithm() != AlgorithmEd25519 {
		return false
	}
	return subtle.ConstantTimeCompare(k.key, other.Bytes()) == 1
}

// String returns Base64-encoded public key.
func (k *ed25519PublicKey) String() string {
	return base64.StdEncoding.EncodeToString(k.key)
}

// ed25519PrivateKey implements PrivateKey for Ed25519.
type ed25519PrivateKey struct {
	key ed25519.PrivateKey
}

// Bytes returns the raw private key bytes.
func (k *ed25519PrivateKey) Bytes() []byte {
	return k.key
}

// Algorithm returns Ed25519.
func (k *ed25519PrivateKey) Algorithm() Algorithm {
	return AlgorithmEd25519
}

// PublicKey returns the corresponding public key.
// Ed25519 private key contains the public key in bytes [32:64].
func (k *ed25519PrivateKey) PublicKey() PublicKey {
	pub := k.key.Public().(ed25519.PublicKey)
	return &ed25519PublicKey{key: pub}
}

// Sign signs the given data.
func (k *ed25519PrivateKey) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(k.key, data), nil
}

// Zeroize overwrites the private key with zeros.
func (k *ed25519PrivateKey) Zeroize() {
	Zeroize(k.key)
}

// GeneratePrivateKey generates a new private key for the given algorithm.
// Complexity: O(1) for key generation, uses crypto/rand.
func GeneratePrivateKey(algo Algorithm) (PrivateKey, error) {
	switch algo {
	case AlgorithmEd25519:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
		}
		return &ed25519PrivateKey{key: priv}, nil

	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		// TODO: Implement secp256k1 and secp256r1 in issue #8
		return nil, fmt.Errorf("algorithm %s not yet implemented", algo)

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}
}

// PrivateKeyFromBytes creates a private key from raw bytes.
// The caller should zero the input data after this call returns if it's sensitive.
// Complexity: O(n) where n is byte length for validation.
func PrivateKeyFromBytes(algo Algorithm, data []byte) (PrivateKey, error) {
	switch algo {
	case AlgorithmEd25519:
		if len(data) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid ed25519 private key size: expected %d, got %d",
				ed25519.PrivateKeySize, len(data))
		}
		// Make a copy to prevent external mutation
		key := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
		copy(key, data)
		return &ed25519PrivateKey{key: key}, nil

	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		return nil, fmt.Errorf("algorithm %s not yet implemented", algo)

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}
}

// PublicKeyFromBytes creates a public key from raw bytes.
// Complexity: O(n) where n is byte length for validation.
func PublicKeyFromBytes(algo Algorithm, data []byte) (PublicKey, error) {
	switch algo {
	case AlgorithmEd25519:
		if len(data) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid ed25519 public key size: expected %d, got %d",
				ed25519.PublicKeySize, len(data))
		}
		// Make a copy to prevent external mutation
		key := make(ed25519.PublicKey, ed25519.PublicKeySize)
		copy(key, data)
		return &ed25519PublicKey{key: key}, nil

	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		return nil, fmt.Errorf("algorithm %s not yet implemented", algo)

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}
}
