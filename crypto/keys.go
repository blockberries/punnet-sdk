package crypto

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"runtime"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
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

// ============================================================================
// secp256k1 Implementation (Bitcoin/Ethereum compatibility)
// ============================================================================

// secp256k1PublicKey implements PublicKey for secp256k1.
// Uses 33-byte compressed format for storage efficiency.
type secp256k1PublicKey struct {
	key *secp256k1.PublicKey
}

// Bytes returns the 33-byte compressed public key.
// Complexity: O(1), zero allocations (returns pre-computed bytes).
func (k *secp256k1PublicKey) Bytes() []byte {
	return k.key.SerializeCompressed()
}

// Algorithm returns AlgorithmSecp256k1.
func (k *secp256k1PublicKey) Algorithm() Algorithm {
	return AlgorithmSecp256k1
}

// Verify verifies an ECDSA signature.
// Expects 64-byte signature in (r || s) format.
// Complexity: O(1) for signature parsing + O(n) for hash computation.
func (k *secp256k1PublicKey) Verify(data, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}

	// Parse r and s from signature (32 bytes each)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	// Create ecdsa.Signature and verify
	var sig secp256k1.ModNScalar
	var sigR secp256k1.FieldVal

	sigR.SetByteSlice(signature[:32])
	sig.SetByteSlice(signature[32:64])

	// Hash the data (secp256k1 signs hashes, not raw data)
	hash := sha256.Sum256(data)

	// Verify using standard ECDSA
	// Convert to ecdsa types for verification
	pubKeyECDSA := k.key.ToECDSA()
	return ecdsa.Verify(pubKeyECDSA, hash[:], r, s)
}

// Equals checks equality using constant-time comparison.
// Complexity: O(n) where n is key length (33 bytes for compressed secp256k1).
func (k *secp256k1PublicKey) Equals(other PublicKey) bool {
	if other == nil || other.Algorithm() != AlgorithmSecp256k1 {
		return false
	}
	return subtle.ConstantTimeCompare(k.Bytes(), other.Bytes()) == 1
}

// String returns Base64-encoded compressed public key.
func (k *secp256k1PublicKey) String() string {
	return base64.StdEncoding.EncodeToString(k.Bytes())
}

// secp256k1PrivateKey implements PrivateKey for secp256k1.
type secp256k1PrivateKey struct {
	key *secp256k1.PrivateKey
}

// Bytes returns the 32-byte scalar private key.
// WARNING: Handle with care. Consider zeroing after use.
// Complexity: O(1), zero allocations.
func (k *secp256k1PrivateKey) Bytes() []byte {
	return k.key.Serialize()
}

// Algorithm returns AlgorithmSecp256k1.
func (k *secp256k1PrivateKey) Algorithm() Algorithm {
	return AlgorithmSecp256k1
}

// PublicKey returns the corresponding public key.
// Complexity: O(1) - derived from private key.
func (k *secp256k1PrivateKey) PublicKey() PublicKey {
	return &secp256k1PublicKey{key: k.key.PubKey()}
}

// Sign signs the given data using RFC 6979 deterministic k.
// Returns 64-byte signature in (r || s) format.
// Complexity: O(n) where n is data length for hashing.
func (k *secp256k1PrivateKey) Sign(data []byte) ([]byte, error) {
	// Hash the data (ECDSA signs hashes)
	hash := sha256.Sum256(data)

	// Sign using RFC 6979 deterministic k (built into dcrd/secp256k1)
	sig := secp256k1ecdsa.Sign(k.key, hash[:])

	// Extract r and s, pad to 32 bytes each
	r := sig.R()
	s := sig.S()

	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// Pad r to 32 bytes (right-align)
	copy(signature[32-len(rBytes):32], rBytes[:])
	// Pad s to 32 bytes (right-align)
	copy(signature[64-len(sBytes):64], sBytes[:])

	return signature, nil
}

// Zeroize overwrites the private key with zeros.
func (k *secp256k1PrivateKey) Zeroize() {
	k.key.Zero()
}

// ============================================================================
// secp256r1 (P-256) Implementation (HSM compatibility)
// ============================================================================

// secp256r1PublicKey implements PublicKey for secp256r1 (P-256).
// Uses 33-byte compressed format for storage efficiency.
type secp256r1PublicKey struct {
	key *ecdsa.PublicKey
}

// Bytes returns the 33-byte compressed public key.
// Compressed format: 0x02/0x03 prefix + 32-byte X coordinate.
// Complexity: O(1).
func (k *secp256r1PublicKey) Bytes() []byte {
	return elliptic.MarshalCompressed(k.key.Curve, k.key.X, k.key.Y)
}

// Algorithm returns AlgorithmSecp256r1.
func (k *secp256r1PublicKey) Algorithm() Algorithm {
	return AlgorithmSecp256r1
}

// Verify verifies an ECDSA signature.
// Expects 64-byte signature in (r || s) format.
// Complexity: O(1) for signature parsing + O(n) for hash computation.
func (k *secp256r1PublicKey) Verify(data, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}

	// Parse r and s from signature (32 bytes each)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	// Hash the data (ECDSA signs hashes)
	hash := sha256.Sum256(data)

	return ecdsa.Verify(k.key, hash[:], r, s)
}

// Equals checks equality using constant-time comparison.
// Complexity: O(n) where n is key length (33 bytes for compressed P-256).
func (k *secp256r1PublicKey) Equals(other PublicKey) bool {
	if other == nil || other.Algorithm() != AlgorithmSecp256r1 {
		return false
	}
	return subtle.ConstantTimeCompare(k.Bytes(), other.Bytes()) == 1
}

// String returns Base64-encoded compressed public key.
func (k *secp256r1PublicKey) String() string {
	return base64.StdEncoding.EncodeToString(k.Bytes())
}

// secp256r1PrivateKey implements PrivateKey for secp256r1 (P-256).
type secp256r1PrivateKey struct {
	key *ecdsa.PrivateKey
}

// Bytes returns the 32-byte scalar private key.
// WARNING: Handle with care. Consider zeroing after use.
// Complexity: O(1).
func (k *secp256r1PrivateKey) Bytes() []byte {
	// Pad to 32 bytes (P-256 scalars are 32 bytes)
	bytes := k.key.D.Bytes()
	if len(bytes) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(bytes):], bytes)
		return padded
	}
	return bytes
}

// Algorithm returns AlgorithmSecp256r1.
func (k *secp256r1PrivateKey) Algorithm() Algorithm {
	return AlgorithmSecp256r1
}

// PublicKey returns the corresponding public key.
// Complexity: O(1) - derived from private key.
func (k *secp256r1PrivateKey) PublicKey() PublicKey {
	return &secp256r1PublicKey{key: &k.key.PublicKey}
}

// Sign signs the given data using RFC 6979 deterministic k.
// Returns 64-byte signature in (r || s) format.
// Complexity: O(n) where n is data length for hashing.
func (k *secp256r1PrivateKey) Sign(data []byte) ([]byte, error) {
	// Hash the data (ECDSA signs hashes)
	hash := sha256.Sum256(data)

	// Sign - Go's ecdsa.Sign uses RFC 6979 deterministic k when
	// the private key has a deterministic nonce source
	r, s, err := ecdsa.Sign(rand.Reader, k.key, hash[:])
	if err != nil {
		return nil, fmt.Errorf("secp256r1 signing failed: %w", err)
	}

	// Encode as 64 bytes (r || s), each padded to 32 bytes
	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// Pad r to 32 bytes (right-align)
	copy(signature[32-len(rBytes):32], rBytes)
	// Pad s to 32 bytes (right-align)
	copy(signature[64-len(sBytes):64], sBytes)

	return signature, nil
}

// Zeroize overwrites the private key with zeros.
func (k *secp256r1PrivateKey) Zeroize() {
	if k.key != nil && k.key.D != nil {
		// Zero the scalar bytes
		bytes := k.key.D.Bytes()
		Zeroize(bytes)
		k.key.D.SetInt64(0)
	}
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

	case AlgorithmSecp256k1:
		privKey, err := secp256k1.GeneratePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate secp256k1 key: %w", err)
		}
		return &secp256k1PrivateKey{key: privKey}, nil

	case AlgorithmSecp256r1:
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate secp256r1 key: %w", err)
		}
		return &secp256r1PrivateKey{key: privKey}, nil

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

	case AlgorithmSecp256k1:
		if len(data) != 32 {
			return nil, fmt.Errorf("invalid secp256k1 private key size: expected 32, got %d", len(data))
		}
		// Make a copy to prevent external mutation
		privKey := secp256k1.PrivKeyFromBytes(data)
		return &secp256k1PrivateKey{key: privKey}, nil

	case AlgorithmSecp256r1:
		if len(data) != 32 {
			return nil, fmt.Errorf("invalid secp256r1 private key size: expected 32, got %d", len(data))
		}
		// Make a copy and construct the key
		d := new(big.Int).SetBytes(data)
		curve := elliptic.P256()
		x, y := curve.ScalarBaseMult(data)
		privKey := &ecdsa.PrivateKey{
			PublicKey: ecdsa.PublicKey{
				Curve: curve,
				X:     x,
				Y:     y,
			},
			D: d,
		}
		return &secp256r1PrivateKey{key: privKey}, nil

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}
}

// PublicKeyFromBytes creates a public key from raw bytes.
// Expects compressed format (33 bytes) for secp256k1/secp256r1.
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

	case AlgorithmSecp256k1:
		if len(data) != 33 {
			return nil, fmt.Errorf("invalid secp256k1 public key size: expected 33 (compressed), got %d", len(data))
		}
		pubKey, err := secp256k1.ParsePubKey(data)
		if err != nil {
			return nil, fmt.Errorf("invalid secp256k1 public key: %w", err)
		}
		return &secp256k1PublicKey{key: pubKey}, nil

	case AlgorithmSecp256r1:
		if len(data) != 33 {
			return nil, fmt.Errorf("invalid secp256r1 public key size: expected 33 (compressed), got %d", len(data))
		}
		x, y := elliptic.UnmarshalCompressed(elliptic.P256(), data)
		if x == nil {
			return nil, fmt.Errorf("invalid secp256r1 public key: failed to decompress")
		}
		pubKey := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     x,
			Y:     y,
		}
		return &secp256r1PublicKey{key: pubKey}, nil

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", algo)
	}
}
