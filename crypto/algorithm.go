// Package crypto provides cryptographic primitives for the Punnet SDK.
package crypto

import (
	"encoding/json"
	"fmt"
)

// Algorithm represents a supported cryptographic signing algorithm.
// Complexity: All operations O(1)
type Algorithm string

const (
	// AlgorithmEd25519 is the Ed25519 signature algorithm.
	// Key size: 32 bytes, Signature size: 64 bytes.
	// Primary recommended algorithm for most use cases.
	AlgorithmEd25519 Algorithm = "ed25519"

	// AlgorithmSecp256k1 is the secp256k1 ECDSA algorithm.
	// Key size: 33 bytes (compressed), Signature size: 64 bytes.
	// Used for Ethereum/Bitcoin compatibility.
	AlgorithmSecp256k1 Algorithm = "secp256k1"

	// AlgorithmSecp256r1 is the P-256 (secp256r1) ECDSA algorithm.
	// Key size: 33 bytes (compressed), Signature size: 64 bytes.
	// Used for hardware security module compatibility.
	AlgorithmSecp256r1 Algorithm = "secp256r1"
)

// String returns the string representation of the algorithm.
func (a Algorithm) String() string {
	return string(a)
}

// IsValid returns true if the algorithm is a recognized type.
func (a Algorithm) IsValid() bool {
	switch a {
	case AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1:
		return true
	default:
		return false
	}
}

// PublicKeySize returns the expected public key size in bytes.
func (a Algorithm) PublicKeySize() int {
	switch a {
	case AlgorithmEd25519:
		return 32
	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		return 33 // Compressed form
	default:
		return 0
	}
}

// PrivateKeySize returns the expected private key size in bytes.
func (a Algorithm) PrivateKeySize() int {
	switch a {
	case AlgorithmEd25519:
		return 64 // Ed25519 private key includes public key
	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		return 32
	default:
		return 0
	}
}

// SignatureSize returns the expected signature size in bytes.
func (a Algorithm) SignatureSize() int {
	switch a {
	case AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1:
		return 64
	default:
		return 0
	}
}

// MarshalJSON implements json.Marshaler.
func (a Algorithm) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(a))
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *Algorithm) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	alg := Algorithm(s)
	if !alg.IsValid() {
		return fmt.Errorf("unsupported algorithm: %s", s)
	}
	*a = alg
	return nil
}
