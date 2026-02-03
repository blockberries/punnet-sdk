package crypto

// Signer is the interface for signing operations.
// Implementations must never expose private key material.
type Signer interface {
	// Algorithm returns the signing algorithm.
	Algorithm() Algorithm

	// PublicKey returns the public key.
	PublicKey() PublicKey

	// Sign signs the message and returns the signature.
	// The message should typically be a hash of the actual data.
	Sign(message []byte) ([]byte, error)
}

// Signature represents a cryptographic signature with metadata.
type Signature struct {
	PubKey    []byte    `json:"pub_key"`
	Signature []byte    `json:"signature"`
	Algorithm Algorithm `json:"algorithm"`
}

// BasicSigner wraps a PrivateKey to implement Signer.
// Thread-safe: signing operations are stateless.
type BasicSigner struct {
	privateKey PrivateKey
}

// NewSigner creates a new Signer from a PrivateKey.
// Complexity: O(1), zero allocations.
func NewSigner(privateKey PrivateKey) Signer {
	return &BasicSigner{privateKey: privateKey}
}

// Sign signs the given data.
func (s *BasicSigner) Sign(data []byte) ([]byte, error) {
	return s.privateKey.Sign(data)
}

// PublicKey returns the signer's public key.
func (s *BasicSigner) PublicKey() PublicKey {
	return s.privateKey.PublicKey()
}

// Algorithm returns the signing algorithm.
func (s *BasicSigner) Algorithm() Algorithm {
	return s.privateKey.Algorithm()
}
