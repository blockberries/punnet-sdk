// Package crypto provides cryptographic primitives for the Punnet SDK.
package crypto

import (
	"encoding/json"
	"fmt"
)

// Algorithm represents a supported cryptographic algorithm.
type Algorithm string

const (
	// AlgorithmEd25519 is the Ed25519 signature algorithm (recommended).
	AlgorithmEd25519 Algorithm = "ed25519"

	// AlgorithmSecp256k1 is the secp256k1 signature algorithm (Ethereum/Bitcoin compatible).
	AlgorithmSecp256k1 Algorithm = "secp256k1"

	// AlgorithmSecp256r1 is the secp256r1/P-256 signature algorithm (HSM compatible).
	AlgorithmSecp256r1 Algorithm = "secp256r1"
)

// String returns the string representation of the algorithm.
func (a Algorithm) String() string {
	return string(a)
}

// IsValid returns true if the algorithm is a recognized algorithm type.
func (a Algorithm) IsValid() bool {
	switch a {
	case AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1:
		return true
	default:
		return false
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
		return fmt.Errorf("algorithm must be a string: %w", err)
	}
	alg := Algorithm(s)
	if !alg.IsValid() {
		return fmt.Errorf("unknown algorithm: %q", s)
	}
	*a = alg
	return nil
}

// KeySize returns the public key size in bytes for this algorithm.
func (a Algorithm) KeySize() int {
	switch a {
	case AlgorithmEd25519:
		return 32
	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		return 33 // compressed form
	default:
		return 0
	}
}

// SignatureSize returns the signature size in bytes for this algorithm.
func (a Algorithm) SignatureSize() int {
	switch a {
	case AlgorithmEd25519:
		return 64
	case AlgorithmSecp256k1, AlgorithmSecp256r1:
		return 64
	default:
		return 0
	}
}
