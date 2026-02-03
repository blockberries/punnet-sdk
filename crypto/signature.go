package crypto

import (
	"crypto/elliptic"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Low-S signature normalization utilities for ECDSA.
//
// ECDSA signatures are malleable: for any valid signature (r, s), the signature
// (r, n-s) is also valid where n is the curve order. This can cause:
// - Transaction ID mutation attacks
// - Signature-based deduplication failures
// - Unexpected behavior in consensus systems
//
// BIP-62 (Bitcoin) and EIP-2 (Ethereum) both enforce "low-S" normalization:
// s must be in the lower half of the curve order (s <= n/2).
//
// Sign() methods produce low-S signatures. Verify() methods accept both forms.
// Use these helpers to check/normalize external signatures.

// Curve order constants (precomputed for efficiency).
var (
	// secp256k1N is the order of the secp256k1 curve.
	secp256k1N = secp256k1.Params().N

	// secp256k1HalfN is n/2 for secp256k1, used for low-S checks.
	secp256k1HalfN = new(big.Int).Rsh(secp256k1N, 1)

	// secp256r1N is the order of the secp256r1 (P-256) curve.
	secp256r1N = elliptic.P256().Params().N

	// secp256r1HalfN is n/2 for secp256r1.
	secp256r1HalfN = new(big.Int).Rsh(secp256r1N, 1)
)

// IsLowSForAlgorithm checks if a 64-byte signature has s in the lower half
// of the curve order for the specified algorithm.
//
// This is the canonical form required by BIP-62 and EIP-2.
// Returns false for invalid signature lengths or unsupported algorithms.
//
// Complexity: O(1) - single big.Int comparison
// Allocations: 1 big.Int (32 bytes)
func IsLowSForAlgorithm(sig []byte, algo Algorithm) bool {
	if len(sig) != 64 {
		return false
	}

	s := new(big.Int).SetBytes(sig[32:64])

	switch algo {
	case AlgorithmSecp256k1:
		return s.Cmp(secp256k1HalfN) <= 0
	case AlgorithmSecp256r1:
		return s.Cmp(secp256r1HalfN) <= 0
	default:
		return false
	}
}

// NormalizeSignature converts a high-S signature to low-S form for the
// specified algorithm. If the signature is already low-S, returns a copy.
//
// The transformation is: s' = n - s (where n is the curve order).
// This is safe because for any valid ECDSA signature (r, s),
// the signature (r, n-s) is also valid for the same message and key.
//
// Returns nil for invalid signature lengths or unsupported algorithms.
// Allocates a new slice; does not modify the input.
//
// Complexity: O(1)
// Allocations: 1 slice (64 bytes) + 2 big.Int temporaries
func NormalizeSignature(sig []byte, algo Algorithm) []byte {
	if len(sig) != 64 {
		return nil
	}

	var n, halfN *big.Int
	switch algo {
	case AlgorithmSecp256k1:
		n, halfN = secp256k1N, secp256k1HalfN
	case AlgorithmSecp256r1:
		n, halfN = secp256r1N, secp256r1HalfN
	default:
		return nil
	}

	s := new(big.Int).SetBytes(sig[32:64])

	// Already low-S, return a copy
	if s.Cmp(halfN) <= 0 {
		result := make([]byte, 64)
		copy(result, sig)
		return result
	}

	// Compute s' = n - s
	s.Sub(n, s)

	// Build normalized signature
	result := make([]byte, 64)
	copy(result[:32], sig[:32]) // r unchanged
	sBytes := s.Bytes()
	copy(result[64-len(sBytes):64], sBytes) // s' padded to 32 bytes

	return result
}

// MakeHighS creates a high-S version of a signature for testing purposes.
// If the signature is already high-S, returns a copy unchanged.
//
// This is the inverse of NormalizeSignature and is useful for testing
// that verification correctly handles both low-S and high-S signatures.
//
// Returns nil for invalid signature lengths or unsupported algorithms.
func MakeHighS(sig []byte, algo Algorithm) []byte {
	if len(sig) != 64 {
		return nil
	}

	var n, halfN *big.Int
	switch algo {
	case AlgorithmSecp256k1:
		n, halfN = secp256k1N, secp256k1HalfN
	case AlgorithmSecp256r1:
		n, halfN = secp256r1N, secp256r1HalfN
	default:
		return nil
	}

	s := new(big.Int).SetBytes(sig[32:64])

	// Already high-S, return a copy
	if s.Cmp(halfN) > 0 {
		result := make([]byte, 64)
		copy(result, sig)
		return result
	}

	// Compute s' = n - s (making it high-S)
	s.Sub(n, s)

	// Build high-S signature
	result := make([]byte, 64)
	copy(result[:32], sig[:32]) // r unchanged
	sBytes := s.Bytes()
	copy(result[64-len(sBytes):64], sBytes)

	return result
}

// CurveOrder returns the curve order (n) for the specified algorithm.
// Returns nil for unsupported algorithms.
func CurveOrder(algo Algorithm) *big.Int {
	switch algo {
	case AlgorithmSecp256k1:
		return secp256k1N
	case AlgorithmSecp256r1:
		return secp256r1N
	default:
		return nil
	}
}

// HalfCurveOrder returns n/2 for the specified algorithm.
// This is the threshold for low-S signatures (s <= n/2).
// Returns nil for unsupported algorithms.
func HalfCurveOrder(algo Algorithm) *big.Int {
	switch algo {
	case AlgorithmSecp256k1:
		return secp256k1HalfN
	case AlgorithmSecp256r1:
		return secp256r1HalfN
	default:
		return nil
	}
}
