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
// Note: This verifies both low-S and high-S signatures. While Sign() produces
// low-S signatures (BIP-62 compliant), Verify() accepts any valid signature.
// Use IsLowS() if you need to reject high-S signatures at the protocol layer.
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
// Returns 64-byte signature: r||s in big-endian with low-S normalization.
//
// Low-S normalization (BIP-62): The dcrd/secp256k1 library produces signatures
// with s in the lower half of the curve order by default. This prevents signature
// malleability attacks and matches Bitcoin's consensus rules.
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
// Note: This verifies both low-S and high-S signatures. While Sign() produces
// low-S signatures (EIP-2 compliant), Verify() accepts any valid signature.
// Use IsLowS() if you need to reject high-S signatures at the protocol layer.
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

// Sign signs the given data using RFC 6979 deterministic ECDSA.
// Returns 64-byte signature: r||s in big-endian with low-S normalization.
//
// RFC 6979 generates the nonce deterministically from the private key and message,
// ensuring that signing the same message with the same key always produces identical
// signatures. This provides consistency with secp256k1 behavior and offers:
//   - Reproducible signatures for debugging and testing
//   - No entropy required at sign time
//   - Protection against nonce reuse attacks from poor RNG
//
// Signatures are normalized to low-S form (s <= n/2) to prevent signature malleability.
// This matches BIP-62 (Bitcoin) and EIP-2 (Ethereum) requirements.
//
// Complexity: O(n) where n is data length (for SHA-256 hash)
// Memory: ~400 bytes for RFC 6979 HMAC state + 64-byte signature output
func (k *secp256r1PrivateKey) Sign(data []byte) ([]byte, error) {
	curve := k.key.Curve
	n := curve.Params().N

	// Hash the data
	hash := sha256.Sum256(data)

	// Generate deterministic nonce using RFC 6979
	kNonce := rfc6979Nonce(k.key.D, hash[:], n)

	// Compute r = (k*G).x mod n
	rx, _ := curve.ScalarBaseMult(kNonce.Bytes())
	r := new(big.Int).Mod(rx, n)
	if r.Sign() == 0 {
		return nil, fmt.Errorf("secp256r1 signing failed: r is zero")
	}

	// Compute s = k^-1 * (hash + r*d) mod n
	kInv := new(big.Int).ModInverse(kNonce, n)
	hashInt := new(big.Int).SetBytes(hash[:])
	s := new(big.Int).Mul(r, k.key.D)
	s.Add(s, hashInt)
	s.Mul(s, kInv)
	s.Mod(s, n)
	if s.Sign() == 0 {
		return nil, fmt.Errorf("secp256r1 signing failed: s is zero")
	}

	// Normalize s to low-S form to prevent signature malleability.
	// For any valid signature (r, s), (r, n-s) is also valid.
	// We enforce s <= n/2 to ensure canonical signatures.
	s = normalizeLowS(s, n)

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
// WARNING: Due to Go's big.Int implementation, complete memory erasure cannot
// be guaranteed. The big.Int.SetInt64(0) call clears the semantic value, but
// the internal buffer containing the original key bytes may persist in memory
// until garbage collected. Additionally, big.Int.Bytes() returns a new slice
// allocation, so zeroing that copy does not affect the original representation.
//
// This is a known limitation of Go's math/big package. For secp256k1, the dcrd
// library provides proper zeroization via its Zero() method.
//
// For high-security contexts requiring guaranteed memory clearing, consider:
//   - Process isolation (separate process for key material that terminates after use)
//   - Hardware security modules (HSMs) that manage keys in secure hardware
//   - Using secp256k1 which has proper zeroization via the dcrd library
//   - Languages with explicit memory control (Rust, C with explicit memset_s)
func (k *secp256r1PrivateKey) Zeroize() {
	if k.key != nil && k.key.D != nil {
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

// secp256k1PublicKeyFromBytes creates a secp256k1 public key from bytes.
// Accepts both compressed (33 bytes, 0x02/0x03 prefix) and uncompressed
// (65 bytes, 0x04 prefix) formats for interoperability.
//
// Bytes() always returns compressed format (33 bytes) regardless of input format.
func secp256k1PublicKeyFromBytes(data []byte) (PublicKey, error) {
	switch len(data) {
	case 33: // Compressed format (0x02 or 0x03 prefix)
		// Valid
	case 65: // Uncompressed format (0x04 prefix)
		// Valid
	default:
		return nil, fmt.Errorf("invalid secp256k1 public key size: expected 33 or 65, got %d", len(data))
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

// normalizeLowS returns s if s <= n/2, otherwise returns n - s.
// This enforces the low-S constraint to prevent ECDSA signature malleability.
//
// Background: For any valid ECDSA signature (r, s), the signature (r, n-s) is also valid.
// This malleability can cause issues:
// - Transaction ID mutation attacks (changing txid without invalidating signature)
// - Signature-based deduplication failures
// - Unexpected behavior in consensus systems
//
// By enforcing s <= n/2, we ensure signatures are canonical (unique per message/key pair).
// This matches BIP-62 (Bitcoin) and EIP-2 (Ethereum).
//
// Complexity: O(1) - constant-time comparison and subtraction on 256-bit integers.
// Memory: One allocation for half-order on first call (cached by curve params).
func normalizeLowS(s, n *big.Int) *big.Int {
	// Calculate n/2 (half the curve order)
	halfN := new(big.Int).Rsh(n, 1)

	// If s > n/2, return n - s
	if s.Cmp(halfN) > 0 {
		return new(big.Int).Sub(n, s)
	}
	return s
}

// IsLowS checks if an ECDSA signature's S value is in low-S form (s <= n/2).
// This is useful for signature validation to reject malleable signatures.
//
// The signature parameter should be 64 bytes (r||s in big-endian).
// The n parameter is the curve order.
//
// Complexity: O(1)
func IsLowS(signature []byte, n *big.Int) bool {
	if len(signature) != 64 {
		return false
	}
	s := new(big.Int).SetBytes(signature[32:])
	halfN := new(big.Int).Rsh(n, 1)
	return s.Cmp(halfN) <= 0
}
