package crypto

import (
	"encoding/json"
	"fmt"
)

// Algorithm represents a supported signature algorithm.
// Complexity: All operations are O(1).
type Algorithm string

const (
	// AlgorithmEd25519 is the recommended algorithm for most use cases.
	// 32-byte public key, 64-byte private key, 64-byte signature.
	AlgorithmEd25519 Algorithm = "ed25519"

	// AlgorithmSecp256k1 provides Ethereum/Bitcoin compatibility.
	// 33-byte compressed public key, 32-byte private key, 64-byte signature.
	AlgorithmSecp256k1 Algorithm = "secp256k1"

	// AlgorithmSecp256r1 (P-256) is preferred for HSM compatibility.
	// 33-byte compressed public key, 32-byte private key, 64-byte signature.
	AlgorithmSecp256r1 Algorithm = "secp256r1"
)

// String returns the algorithm as a string.
func (a Algorithm) String() string {
	return string(a)
}

// IsValid returns true if the algorithm is supported.
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
