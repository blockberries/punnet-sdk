package crypto

// Signer represents an entity that can sign data.
// Implementations should be thread-safe.
type Signer interface {
	// Sign signs the given data and returns a signature.
	// Complexity: O(n) where n is data length.
	// Memory: Allocates new slice for signature (64 bytes for Ed25519).
	Sign(data []byte) ([]byte, error)

	// PublicKey returns the signer's public key.
	// Complexity: O(1).
	PublicKey() PublicKey

	// Algorithm returns the signing algorithm.
	// Complexity: O(1).
	Algorithm() Algorithm
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

// SignBytesProvider is implemented by types that can provide bytes for signing.
// This is used to avoid import cycles between crypto and types packages.
type SignBytesProvider interface {
	GetSignBytes() ([]byte, error)
}

// SignSignDoc signs a SignDoc (or any SignBytesProvider) using a PrivateKey.
// Returns a Signature containing the public key, signature bytes, and algorithm.
//
// Complexity: O(n) where n is SignDoc serialized size.
// Memory: 3 allocations (sign bytes, signature, Signature struct).
//
// Example:
//
//	sig, err := crypto.SignSignDoc(signDoc, privateKey)
//	if err != nil {
//	    return err
//	}
//	// sig.PubKey, sig.Signature, sig.Algorithm are populated
func SignSignDoc(signDoc SignBytesProvider, privateKey PrivateKey) (*Signature, error) {
	signBytes, err := signDoc.GetSignBytes()
	if err != nil {
		return nil, err
	}

	sig, err := privateKey.Sign(signBytes)
	if err != nil {
		return nil, err
	}

	return &Signature{
		PubKey:    privateKey.PublicKey().Bytes(),
		Signature: sig,
		Algorithm: privateKey.Algorithm(),
	}, nil
}
