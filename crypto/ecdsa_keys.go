package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	dcrecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// secp256k1PublicKey implements PublicKey for secp256k1.
type secp256k1PublicKey struct {
	key *secp256k1.PublicKey
}

// Bytes returns the compressed public key bytes (33 bytes).
func (k *secp256k1PublicKey) Bytes() []byte {
	return k.key.SerializeCompressed()
}

// Algorithm returns secp256k1.
func (k *secp256k1PublicKey) Algorithm() Algorithm {
	return AlgorithmSecp256k1
}

// Verify verifies a signature (64 bytes: r||s in big-endian).
//
// Note: ECDSA signatures have inherent malleability. For any valid signature (r, s),
// the signature (r, n-s) is also valid. This implementation does not enforce low-S
// normalization. For consensus-critical applications where signature uniqueness matters,
// consider canonicalizing signatures at the protocol layer.
func (k *secp256k1PublicKey) Verify(data, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}

	// Parse r and s from signature (32 bytes each)
	var r, s secp256k1.ModNScalar
	if r.SetByteSlice(signature[:32]) {
		return false // overflow
	}
	if s.SetByteSlice(signature[32:]) {
		return false // overflow
	}

	// Verify the signature
	sig := dcrecdsa.NewSignature(&r, &s)
	hash := sha256.Sum256(data)
	return sig.Verify(hash[:], k.key)
}

// Equals checks equality using constant-time comparison.
func (k *secp256k1PublicKey) Equals(other PublicKey) bool {
	if other == nil || other.Algorithm() != AlgorithmSecp256k1 {
		return false
	}
	return subtle.ConstantTimeCompare(k.Bytes(), other.Bytes()) == 1
}

// String returns Base64-encoded public key.
func (k *secp256k1PublicKey) String() string {
	return base64.StdEncoding.EncodeToString(k.Bytes())
}

// secp256k1PrivateKey implements PrivateKey for secp256k1.
type secp256k1PrivateKey struct {
	key *secp256k1.PrivateKey
}

// Bytes returns the raw private key bytes (32 bytes).
func (k *secp256k1PrivateKey) Bytes() []byte {
	return k.key.Serialize()
}

// Algorithm returns secp256k1.
func (k *secp256k1PrivateKey) Algorithm() Algorithm {
	return AlgorithmSecp256k1
}

// PublicKey returns the corresponding public key.
func (k *secp256k1PrivateKey) PublicKey() PublicKey {
	return &secp256k1PublicKey{key: k.key.PubKey()}
}

// Sign signs the given data using RFC 6979 deterministic signatures.
// Returns 64-byte signature: r||s in big-endian.
func (k *secp256k1PrivateKey) Sign(data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	sig := dcrecdsa.Sign(k.key, hash[:])

	// Extract r and s as 32-byte arrays
	r := sig.R()
	s := sig.S()
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// Combine into 64-byte signature (r||s)
	signature := make([]byte, 64)
	copy(signature[:32], rBytes[:])
	copy(signature[32:], sBytes[:])

	return signature, nil
}

// Zeroize overwrites the private key with zeros.
func (k *secp256k1PrivateKey) Zeroize() {
	k.key.Zero()
}

// secp256r1PublicKey implements PublicKey for secp256r1 (P-256).
type secp256r1PublicKey struct {
	key *ecdsa.PublicKey
}

// Bytes returns the compressed public key bytes (33 bytes).
func (k *secp256r1PublicKey) Bytes() []byte {
	return elliptic.MarshalCompressed(k.key.Curve, k.key.X, k.key.Y)
}

// Algorithm returns secp256r1.
func (k *secp256r1PublicKey) Algorithm() Algorithm {
	return AlgorithmSecp256r1
}

// Verify verifies a signature (64 bytes: r||s in big-endian).
//
// Note: ECDSA signatures have inherent malleability. For any valid signature (r, s),
// the signature (r, n-s) is also valid. This implementation does not enforce low-S
// normalization. For consensus-critical applications where signature uniqueness matters,
// consider canonicalizing signatures at the protocol layer.
func (k *secp256r1PublicKey) Verify(data, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}

	// Parse r and s from signature
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	// Hash the data
	hash := sha256.Sum256(data)

	return ecdsa.Verify(k.key, hash[:], r, s)
}

// Equals checks equality using constant-time comparison.
func (k *secp256r1PublicKey) Equals(other PublicKey) bool {
	if other == nil || other.Algorithm() != AlgorithmSecp256r1 {
		return false
	}
	return subtle.ConstantTimeCompare(k.Bytes(), other.Bytes()) == 1
}

// String returns Base64-encoded public key.
func (k *secp256r1PublicKey) String() string {
	return base64.StdEncoding.EncodeToString(k.Bytes())
}

// secp256r1PrivateKey implements PrivateKey for secp256r1 (P-256).
type secp256r1PrivateKey struct {
	key *ecdsa.PrivateKey
}

// Bytes returns the raw private key bytes (32 bytes).
func (k *secp256r1PrivateKey) Bytes() []byte {
	// Pad D to 32 bytes
	dBytes := k.key.D.Bytes()
	result := make([]byte, 32)
	copy(result[32-len(dBytes):], dBytes)
	return result
}

// Algorithm returns secp256r1.
func (k *secp256r1PrivateKey) Algorithm() Algorithm {
	return AlgorithmSecp256r1
}

// PublicKey returns the corresponding public key.
func (k *secp256r1PrivateKey) PublicKey() PublicKey {
	return &secp256r1PublicKey{key: &k.key.PublicKey}
}

// Sign signs the given data using Go's standard ECDSA signing.
// Returns 64-byte signature: r||s in big-endian.
//
// Note: This uses rand.Reader for entropy, producing non-deterministic signatures.
// Unlike secp256k1 (which uses RFC 6979), signing the same message twice may produce
// different valid signatures. This is acceptable for most use cases but means
// signatures cannot be used for deduplication or replay detection.
func (k *secp256r1PrivateKey) Sign(data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)

	r, s, err := ecdsa.Sign(rand.Reader, k.key, hash[:])
	if err != nil {
		return nil, fmt.Errorf("secp256r1 signing failed: %w", err)
	}

	// Encode r and s as 32-byte big-endian values
	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)

	return signature, nil
}

// Zeroize clears the private key scalar.
//
// Note: Go's big.Int.SetInt64(0) sets the value to zero but does not securely
// overwrite the underlying byte buffer. This is a known limitation of big.Int.
// For secp256k1, the dcrd library's Zero() method handles this properly.
// In practice, Go's garbage collector may retain the original bytes in memory.
func (k *secp256r1PrivateKey) Zeroize() {
	if k.key.D != nil {
		k.key.D.SetInt64(0)
	}
}

// generateSecp256k1PrivateKey generates a new secp256k1 private key.
func generateSecp256k1PrivateKey() (PrivateKey, error) {
	key, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secp256k1 key: %w", err)
	}
	return &secp256k1PrivateKey{key: key}, nil
}

// generateSecp256r1PrivateKey generates a new secp256r1 private key.
func generateSecp256r1PrivateKey() (PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secp256r1 key: %w", err)
	}
	return &secp256r1PrivateKey{key: key}, nil
}

// secp256k1PrivateKeyFromBytes creates a secp256k1 private key from 32 bytes.
func secp256k1PrivateKeyFromBytes(data []byte) (PrivateKey, error) {
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid secp256k1 private key size: expected 32, got %d", len(data))
	}

	key := secp256k1.PrivKeyFromBytes(data)
	return &secp256k1PrivateKey{key: key}, nil
}

// secp256r1PrivateKeyFromBytes creates a secp256r1 private key from 32 bytes.
func secp256r1PrivateKeyFromBytes(data []byte) (PrivateKey, error) {
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid secp256r1 private key size: expected 32, got %d", len(data))
	}

	d := new(big.Int).SetBytes(data)
	curve := elliptic.P256()

	// Validate that d is within range [1, n-1]
	if d.Sign() <= 0 || d.Cmp(curve.Params().N) >= 0 {
		return nil, fmt.Errorf("invalid secp256r1 private key: out of range")
	}

	// Compute public key
	x, y := curve.ScalarBaseMult(data)

	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: d,
	}

	return &secp256r1PrivateKey{key: key}, nil
}

// secp256k1PublicKeyFromBytes creates a secp256k1 public key from compressed bytes (33 bytes).
func secp256k1PublicKeyFromBytes(data []byte) (PublicKey, error) {
	if len(data) != 33 {
		return nil, fmt.Errorf("invalid secp256k1 public key size: expected 33, got %d", len(data))
	}

	key, err := secp256k1.ParsePubKey(data)
	if err != nil {
		return nil, fmt.Errorf("invalid secp256k1 public key: %w", err)
	}

	return &secp256k1PublicKey{key: key}, nil
}

// secp256r1PublicKeyFromBytes creates a secp256r1 public key from compressed bytes (33 bytes).
func secp256r1PublicKeyFromBytes(data []byte) (PublicKey, error) {
	if len(data) != 33 {
		return nil, fmt.Errorf("invalid secp256r1 public key size: expected 33, got %d", len(data))
	}

	curve := elliptic.P256()
	x, y := elliptic.UnmarshalCompressed(curve, data)
	if x == nil {
		return nil, fmt.Errorf("invalid secp256r1 public key: failed to decompress")
	}

	key := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}

	return &secp256r1PublicKey{key: key}, nil
}
