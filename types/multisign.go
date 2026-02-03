package types

import (
	"fmt"
	"sync"

	"github.com/blockberries/punnet-sdk/crypto"
)

// MultiSignCoordinator collects signatures from multiple signers for a single SignDoc.
// It does NOT support progressive/partial signing - all signatures must be collected
// before completing the authorization.
//
// Thread-safe: All methods are safe for concurrent use.
//
// Complexity:
// - AddSignature: O(n) where n is existing signature count (duplicate check)
// - ImportSignature: O(n) where n is existing signature count (duplicate check + verify)
// - Complete: O(n) where n is signature count (deep copy)
// - Memory: O(n) where n is signature count
type MultiSignCoordinator struct {
	mu         sync.RWMutex
	signDoc    *SignDoc
	signatures []Signature
}

// NewMultiSignCoordinator creates a new coordinator for collecting signatures
// on the given SignDoc.
//
// PRECONDITION: signDoc must not be nil
// POSTCONDITION: Returned coordinator is ready to collect signatures
//
// Complexity: O(1), zero allocations for signature slice (lazy init)
func NewMultiSignCoordinator(signDoc *SignDoc) (*MultiSignCoordinator, error) {
	if signDoc == nil {
		return nil, fmt.Errorf("signDoc cannot be nil")
	}

	return &MultiSignCoordinator{
		signDoc:    signDoc,
		signatures: make([]Signature, 0),
	}, nil
}

// SignDoc returns the SignDoc being signed.
// The returned SignDoc should not be modified.
//
// Complexity: O(1)
func (c *MultiSignCoordinator) SignDoc() *SignDoc {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.signDoc
}

// AddSignature adds a pre-computed signature to the coordinator.
// Use this when the signer has already produced a signature.
//
// PRECONDITION: sig.ValidateBasic() passes
// POSTCONDITION: Signature is added if not duplicate
//
// Note: Does NOT verify the signature against the SignDoc.
// Use ImportSignature for remote signatures that need verification.
//
// Complexity: O(n) where n is existing signature count (duplicate check)
func (c *MultiSignCoordinator) AddSignature(sig Signature) error {
	// Validate signature structure before taking lock
	if err := sig.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for duplicate public key
	for _, existing := range c.signatures {
		if pubKeyEqual(existing.PubKey, sig.PubKey) {
			return fmt.Errorf("%w: duplicate signature from same public key", ErrDuplicateSignature)
		}
	}

	// Deep copy signature to prevent external mutation
	sigCopy := Signature{
		Algorithm: sig.Algorithm,
		PubKey:    make([]byte, len(sig.PubKey)),
		Signature: make([]byte, len(sig.Signature)),
	}
	copy(sigCopy.PubKey, sig.PubKey)
	copy(sigCopy.Signature, sig.Signature)

	c.signatures = append(c.signatures, sigCopy)
	return nil
}

// SignWithSigner signs the SignDoc with the given signer and adds the signature.
// Convenience method that handles SignDoc serialization internally.
//
// PRECONDITION: signer is not nil
// POSTCONDITION: Signature from signer is added if not duplicate
//
// Complexity: O(m + n) where m is SignDoc size and n is existing signature count
func (c *MultiSignCoordinator) SignWithSigner(signer crypto.Signer) error {
	if signer == nil {
		return fmt.Errorf("signer cannot be nil")
	}

	// Get sign bytes outside the lock to avoid holding it during crypto ops
	signBytes, err := c.signDoc.GetSignBytes()
	if err != nil {
		return fmt.Errorf("failed to get sign bytes: %w", err)
	}

	// Sign the data
	sigBytes, err := signer.Sign(signBytes)
	if err != nil {
		return fmt.Errorf("signing failed: %w", err)
	}

	sig := Signature{
		Algorithm: signer.Algorithm(),
		PubKey:    signer.PublicKey().Bytes(),
		Signature: sigBytes,
	}

	return c.AddSignature(sig)
}

// ExportSignDoc returns the SignDoc as canonical JSON bytes for distribution
// to remote signers.
//
// POSTCONDITION: Returned bytes are deterministic canonical JSON
//
// Complexity: O(m) where m is SignDoc size
func (c *MultiSignCoordinator) ExportSignDoc() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.signDoc.ToJSON()
}

// ImportSignature verifies and adds a signature from a remote signer.
// The signature is verified against the SignDoc before adding.
//
// PRECONDITION: pubKey algorithm matches sigBytes
// POSTCONDITION: Signature is added only if valid and not duplicate
//
// SECURITY: Always verifies signature before adding. Use this for
// signatures from untrusted sources.
//
// Complexity: O(m + n) where m is SignDoc size and n is existing signature count
func (c *MultiSignCoordinator) ImportSignature(pubKey crypto.PublicKey, sigBytes []byte) error {
	if pubKey == nil {
		return fmt.Errorf("%w: public key cannot be nil", ErrInvalidPublicKey)
	}

	// Get sign bytes for verification
	signBytes, err := c.signDoc.GetSignBytes()
	if err != nil {
		return fmt.Errorf("failed to get sign bytes: %w", err)
	}

	// Verify signature before adding
	if !pubKey.Verify(signBytes, sigBytes) {
		return fmt.Errorf("%w: signature verification failed", ErrInvalidSignature)
	}

	sig := Signature{
		Algorithm: pubKey.Algorithm(),
		PubKey:    pubKey.Bytes(),
		Signature: sigBytes,
	}

	return c.AddSignature(sig)
}

// Count returns the number of signatures collected so far.
//
// Complexity: O(1)
func (c *MultiSignCoordinator) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.signatures)
}

// Signatures returns a copy of all collected signatures.
//
// Complexity: O(n) where n is signature count (deep copy)
func (c *MultiSignCoordinator) Signatures() []Signature {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Deep copy to prevent external mutation
	result := make([]Signature, len(c.signatures))
	for i, sig := range c.signatures {
		result[i] = Signature{
			Algorithm: sig.Algorithm,
			PubKey:    make([]byte, len(sig.PubKey)),
			Signature: make([]byte, len(sig.Signature)),
		}
		copy(result[i].PubKey, sig.PubKey)
		copy(result[i].Signature, sig.Signature)
	}
	return result
}

// Complete returns an Authorization containing all collected signatures.
// Does NOT verify that sufficient signatures have been collected.
//
// POSTCONDITION: Returned Authorization contains deep copies of all signatures
//
// Complexity: O(n) where n is signature count (deep copy via NewAuthorization)
func (c *MultiSignCoordinator) Complete() *Authorization {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// NewAuthorization creates defensive deep copies
	return NewAuthorization(c.signatures...)
}

// Reset clears all collected signatures, allowing the coordinator to be reused.
//
// Complexity: O(1) (slice truncation)
func (c *MultiSignCoordinator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.signatures = c.signatures[:0]
}

// pubKeyEqual compares two public key byte slices for equality.
// Does NOT use constant-time comparison since this is not a secret comparison.
//
// Complexity: O(n) where n is key length
func pubKeyEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
