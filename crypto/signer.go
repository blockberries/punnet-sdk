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
