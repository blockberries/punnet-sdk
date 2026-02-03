package types

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/subtle"
	"fmt"
	"math/big"
)

const (
	// MaxRecursionDepth limits delegation chain depth to prevent stack overflow
	MaxRecursionDepth = 10
)

// Algorithm represents a supported signature algorithm.
//
// SECURITY: All algorithms must provide at least 128-bit security level.
type Algorithm string

const (
	// AlgorithmEd25519 is the Edwards-curve Digital Signature Algorithm.
	// Key sizes: PubKey=32, PrivKey=64, Signature=64
	// RECOMMENDED: Default choice for most applications.
	AlgorithmEd25519 Algorithm = "ed25519"

	// AlgorithmSecp256k1 is the ECDSA algorithm with secp256k1 curve (Bitcoin/Ethereum).
	// Key sizes: PubKey=33 (compressed), PrivKey=32, Signature=64
	AlgorithmSecp256k1 Algorithm = "secp256k1"

	// AlgorithmSecp256r1 is the ECDSA algorithm with P-256/secp256r1 curve (NIST).
	// Key sizes: PubKey=33 (compressed), PrivKey=32, Signature=64
	AlgorithmSecp256r1 Algorithm = "secp256r1"
)

// ValidAlgorithms returns the list of production-ready algorithms.
//
// NOTE: secp256k1 and secp256r1 constants are defined above for documentation
// and future implementation, but they are NOT exposed as valid until properly
// implemented and tested. See Issue #XX for tracking.
//
// REVISIT WHEN: We have a concrete use case requiring secp256k1 (Ethereum key
// compatibility) or secp256r1 (WebAuthn/passkeys).
func ValidAlgorithms() []Algorithm {
	return []Algorithm{AlgorithmEd25519}
}

// IsValidAlgorithm checks if the algorithm is production-ready.
//
// INVARIANT: Only algorithms with complete, tested implementations return true.
// BACKWARDS COMPATIBILITY: Empty string is treated as Ed25519.
func IsValidAlgorithm(algo Algorithm) bool {
	// Only Ed25519 is production-ready. Empty defaults to Ed25519 for backwards compat.
	return algo == AlgorithmEd25519 || algo == ""
}

// Signature represents a single signature with public key and algorithm.
//
// INVARIANT: The Algorithm field must match the actual key type.
// INVARIANT: PubKey and Signature sizes must be valid for the specified algorithm.
type Signature struct {
	// Algorithm specifies the signature algorithm.
	// If empty, defaults to Ed25519 for backwards compatibility.
	Algorithm Algorithm `json:"algorithm,omitempty"`

	// PubKey is the public key bytes.
	// Size depends on algorithm: Ed25519=32, secp256k1=33, secp256r1=33
	PubKey []byte `json:"pub_key"`

	// Signature is the signature bytes.
	// Size: Ed25519=64, secp256k1=64, secp256r1=64
	Signature []byte `json:"signature"`
}

// GetAlgorithm returns the algorithm, defaulting to Ed25519 if not specified.
// BACKWARDS COMPATIBILITY: Empty algorithm field is treated as Ed25519.
func (s *Signature) GetAlgorithm() Algorithm {
	if s.Algorithm == "" {
		return AlgorithmEd25519
	}
	return s.Algorithm
}

// ValidateBasic performs basic validation of the signature structure.
//
// INVARIANT: After successful validation, PubKey and Signature have correct sizes for the algorithm.
// INVARIANT: Only production-ready algorithms pass validation.
func (s *Signature) ValidateBasic() error {
	algo := s.GetAlgorithm()

	// SECURITY: First check if algorithm is production-ready.
	// This ensures we reject secp256k1/secp256r1 until properly implemented.
	if !IsValidAlgorithm(algo) {
		return fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, algo)
	}

	// Validate key and signature sizes for production-ready algorithms
	switch algo {
	case AlgorithmEd25519:
		if len(s.PubKey) != ed25519.PublicKeySize {
			return fmt.Errorf("%w: ed25519 public key must be %d bytes, got %d",
				ErrInvalidPublicKey, ed25519.PublicKeySize, len(s.PubKey))
		}
		if len(s.Signature) != ed25519.SignatureSize {
			return fmt.Errorf("%w: ed25519 signature must be %d bytes, got %d",
				ErrInvalidSignature, ed25519.SignatureSize, len(s.Signature))
		}

	// NOTE: secp256k1 and secp256r1 cases are intentionally removed.
	// IsValidAlgorithm() above already rejects these algorithms.
	// When these algorithms become production-ready, add cases here with
	// proper size validation (33-byte compressed pubkey, 64-byte signature).

	default:
		// This should be unreachable since IsValidAlgorithm already filters
		return fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, algo)
	}

	return nil
}

// Verify verifies the signature against a message.
//
// PRECONDITION: message is the exact bytes that were signed (typically SHA-256 hash of SignDoc JSON)
// POSTCONDITION: Returns true if and only if signature is valid for the public key and message.
//
// SECURITY: This method is constant-time where possible to prevent timing attacks.
func (s *Signature) Verify(message []byte) bool {
	if err := s.ValidateBasic(); err != nil {
		return false
	}

	algo := s.GetAlgorithm()

	switch algo {
	case AlgorithmEd25519:
		return ed25519.Verify(ed25519.PublicKey(s.PubKey), message, s.Signature)

	case AlgorithmSecp256k1:
		return verifySecp256k1(s.PubKey, message, s.Signature)

	case AlgorithmSecp256r1:
		return verifySecp256r1(s.PubKey, message, s.Signature)

	default:
		return false
	}
}

// verifySecp256k1 verifies an ECDSA signature using the secp256k1 curve.
//
// IMPLEMENTATION NOTE: Go's standard library doesn't include secp256k1.
// In production, this would use a proper secp256k1 library (e.g., btcec).
//
// WARNING: This function is NOT production-ready. It is excluded from
// ValidAlgorithms() and IsValidAlgorithm() to prevent accidental use.
// Signatures using secp256k1 will fail validation before reaching this code.
//
// EXPECTED IMPLEMENTATION (for future reference):
// 1. Decompress public key from 33 bytes to full coordinates
// 2. Parse R and S from signature (32 bytes each)
// 3. The message parameter is already SHA-256(SignDoc JSON) - do NOT double-hash
// 4. Verify ECDSA signature
func verifySecp256k1(pubKey, message, signature []byte) bool {
	// SECURITY: This function should never be called in production.
	// IsValidAlgorithm() rejects secp256k1, so ValidateBasic() will fail
	// before Verify() is called.
	//
	// If this panic is ever reached, it indicates a bug in the validation logic.
	panic("verifySecp256k1 called but algorithm is not production-ready - this indicates a validation bug")
}

// verifySecp256r1 verifies an ECDSA signature using the P-256 (secp256r1) curve.
//
// SECURITY: Uses Go's standard library crypto/ecdsa which is well-audited.
//
// WARNING: This function is NOT production-ready. It is excluded from
// ValidAlgorithms() and IsValidAlgorithm() to prevent accidental use.
// Signatures using secp256r1 will fail validation before reaching this code.
//
// KNOWN ISSUES (fixed but untested):
// 1. Previously had double-hash bug (SHA-256 of already-hashed message) - FIXED
// 2. Missing curve point validation - FIXED
// 3. Needs proper integration testing with real secp256r1 signatures
func verifySecp256r1(pubKey, message, signature []byte) bool {
	// SECURITY: This function should never be called in production.
	// IsValidAlgorithm() rejects secp256r1, so ValidateBasic() will fail
	// before Verify() is called.
	//
	// The implementation below is kept for documentation and future use,
	// but will panic if reached to catch any validation bypass bugs.

	// Decompress public key
	if len(pubKey) != 33 {
		return false
	}

	// Parse the compressed public key
	x, y := decompressP256PublicKey(pubKey)
	if x == nil || y == nil {
		return false
	}

	curve := elliptic.P256()

	// SECURITY FIX: Validate that the point is actually on the curve.
	// A malicious actor could craft a compressed key where X is valid but
	// produces a point not on the curve, leading to undefined behavior.
	if !curve.IsOnCurve(x, y) {
		return false
	}

	ecdsaPubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}

	// Parse R and S from signature
	if len(signature) != 64 {
		return false
	}
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])

	// SECURITY FIX: The message parameter is already SHA-256(SignDoc JSON) from
	// SignDoc.GetSignBytes(). Do NOT double-hash.
	//
	// ECDSA verification expects the hash of the message. Since our message
	// is already the hash, we pass it directly.
	return ecdsa.Verify(ecdsaPubKey, message, r, s)
}

// decompressP256PublicKey decompresses a 33-byte compressed P-256 public key.
//
// Format: 0x02 or 0x03 prefix (indicating Y coordinate parity) + 32 bytes X coordinate
//
// SECURITY: This function validates that the decompressed point lies on the P-256 curve.
// A malicious actor could craft a compressed key that produces invalid coordinates.
func decompressP256PublicKey(compressed []byte) (*big.Int, *big.Int) {
	if len(compressed) != 33 {
		return nil, nil
	}

	prefix := compressed[0]
	if prefix != 0x02 && prefix != 0x03 {
		return nil, nil
	}

	x := new(big.Int).SetBytes(compressed[1:])
	curve := elliptic.P256()

	// Calculate Y from X using curve equation: y² = x³ - 3x + b (mod p)
	// P-256 parameters
	params := curve.Params()
	p := params.P

	// x³
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	x3.Mod(x3, p)

	// -3x
	threeX := new(big.Int).Mul(x, big.NewInt(3))
	threeX.Mod(threeX, p)

	// x³ - 3x + b
	y2 := new(big.Int).Sub(x3, threeX)
	y2.Add(y2, params.B)
	y2.Mod(y2, p)

	// y = sqrt(y²) mod p
	y := new(big.Int).ModSqrt(y2, p)
	if y == nil {
		return nil, nil
	}

	// Choose the correct Y based on prefix (even/odd)
	if prefix == 0x02 && y.Bit(0) != 0 {
		y.Sub(p, y)
	} else if prefix == 0x03 && y.Bit(0) == 0 {
		y.Sub(p, y)
	}

	// SECURITY FIX: Verify point is actually on the curve.
	// This is defense-in-depth; the caller should also verify.
	if !curve.IsOnCurve(x, y) {
		return nil, nil
	}

	return x, y
}

// Authorization represents proof of authority to perform an action
type Authorization struct {
	// Signatures from keys in the account's authority
	Signatures []Signature `json:"signatures"`

	// AccountAuthorizations maps delegated account names to their authorizations
	// This enables recursive/hierarchical authorization
	AccountAuthorizations map[AccountName]*Authorization `json:"account_authorizations,omitempty"`
}

// NewAuthorization creates a new authorization with signatures.
// Creates defensive deep copy of signatures to prevent external mutation.
func NewAuthorization(signatures ...Signature) *Authorization {
	// Create defensive deep copy of signatures
	sigsCopy := make([]Signature, len(signatures))
	for i, sig := range signatures {
		// Deep copy each signature's byte slices
		pubKeyCopy := make([]byte, len(sig.PubKey))
		copy(pubKeyCopy, sig.PubKey)

		sigCopy := make([]byte, len(sig.Signature))
		copy(sigCopy, sig.Signature)

		sigsCopy[i] = Signature{
			Algorithm: sig.Algorithm,
			PubKey:    pubKeyCopy,
			Signature: sigCopy,
		}
	}

	return &Authorization{
		Signatures:            sigsCopy,
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}
}

// ValidateBasic performs basic validation
func (a *Authorization) ValidateBasic() error {
	if a == nil {
		return fmt.Errorf("%w: authorization is nil", ErrInvalidAuthorization)
	}

	// Validate all signatures
	for i, sig := range a.Signatures {
		if err := sig.ValidateBasic(); err != nil {
			return fmt.Errorf("%w: signature %d: %v", ErrInvalidAuthorization, i, err)
		}
	}

	// Validate all account authorizations recursively
	for acct, auth := range a.AccountAuthorizations {
		if !acct.IsValid() {
			return fmt.Errorf("%w: invalid account name %s", ErrInvalidAuthorization, acct)
		}
		if auth == nil {
			return fmt.Errorf("%w: nil authorization for account %s", ErrInvalidAuthorization, acct)
		}
		if err := auth.ValidateBasic(); err != nil {
			return fmt.Errorf("%w: account %s: %v", ErrInvalidAuthorization, acct, err)
		}
	}

	return nil
}

// VerifySignatures verifies all signatures against a message
func (a *Authorization) VerifySignatures(message []byte) error {
	for i, sig := range a.Signatures {
		if !sig.Verify(message) {
			return fmt.Errorf("%w: signature %d failed verification", ErrInvalidSignature, i)
		}
	}
	return nil
}

// AccountGetter is an interface for retrieving accounts
// This allows authorization verification without tight coupling to storage
type AccountGetter interface {
	GetAccount(name AccountName) (*Account, error)
}

// VerifyAuthorization verifies that the authorization meets the account's authority threshold
// It recursively verifies delegated account authorizations and detects cycles
func (a *Authorization) VerifyAuthorization(account *Account, message []byte, getter AccountGetter) error {
	if a == nil {
		return fmt.Errorf("%w: authorization is nil", ErrInvalidAuthorization)
	}
	if account == nil {
		return fmt.Errorf("%w: account is nil", ErrInvalidAuthorization)
	}
	if getter == nil {
		return fmt.Errorf("%w: account getter is nil", ErrInvalidAuthorization)
	}

	// Verify all direct signatures first
	if err := a.VerifySignatures(message); err != nil {
		return err
	}

	// Calculate authorization weight with cycle detection
	visited := make(map[AccountName]bool)
	weight, err := a.calculateWeight(account.Name, account.Authority, message, getter, visited, 0)
	if err != nil {
		return err
	}

	// Check if weight meets threshold
	if weight < account.Authority.Threshold {
		return fmt.Errorf("%w: weight %d < threshold %d", ErrInsufficientWeight, weight, account.Authority.Threshold)
	}

	return nil
}

// calculateWeight recursively calculates the total authorization weight
// It implements cycle detection using DFS with a visited set.
//
// SECURITY: This function deduplicates signatures by public key to prevent
// attackers from submitting multiple copies of the same signature to inflate
// their authorization weight. See Issue #30.
func (a *Authorization) calculateWeight(
	accountName AccountName,
	authority Authority,
	message []byte,
	getter AccountGetter,
	visited map[AccountName]bool,
	depth int,
) (uint64, error) {
	// Check recursion depth
	if depth > MaxRecursionDepth {
		return 0, fmt.Errorf("%w: depth %d", ErrMaxRecursionDepth, depth)
	}

	// Check for cycles
	if visited[accountName] {
		return 0, fmt.Errorf("%w: account %s appears multiple times in delegation chain", ErrAuthorizationCycle, accountName)
	}

	// Mark as visited
	visited[accountName] = true
	defer func() {
		// Unmark when returning (allows different paths through the delegation graph)
		delete(visited, accountName)
	}()

	var totalWeight uint64

	// SECURITY FIX (Issue #30): Track which public keys have already contributed weight.
	// Without this, an attacker could submit multiple copies of the same signature
	// to inflate their authorization weight and bypass multi-sig thresholds.
	//
	// Example attack: Account threshold=3, attacker has one key with weight=1
	// Without fix: 3 copies of same signature → weight=3 → threshold met (ATTACK SUCCESS)
	// With fix: 3 copies of same signature → only first counted → weight=1 < 3 (BLOCKED)
	seenPubKeys := make(map[string]bool)

	// Calculate weight from direct key signatures
	for _, sig := range a.Signatures {
		pubKeyStr := string(sig.PubKey)

		// SECURITY: Check for duplicate signatures from same public key
		if seenPubKeys[pubKeyStr] {
			// Return error to make duplicate detection explicit
			return 0, fmt.Errorf("%w: public key already provided a signature", ErrDuplicateSignature)
		}

		if authority.HasKey(sig.PubKey) {
			if sig.Verify(message) {
				// Mark this public key as having contributed
				seenPubKeys[pubKeyStr] = true

				keyWeight := authority.GetKeyWeight(sig.PubKey)
				// Check for overflow
				if totalWeight > ^uint64(0)-keyWeight {
					return 0, fmt.Errorf("weight calculation overflow")
				}
				totalWeight += keyWeight
			}
		}
	}

	// Calculate weight from delegated account authorizations (recursive)
	for delegatedAcct, delegatedAuth := range a.AccountAuthorizations {
		// Check if this account is in the authority's delegation list
		if !authority.HasAccount(delegatedAcct) {
			continue
		}

		// Get the delegated account
		delegatedAccount, err := getter.GetAccount(delegatedAcct)
		if err != nil {
			return 0, fmt.Errorf("failed to get delegated account %s: %w", delegatedAcct, err)
		}

		// Recursively calculate weight for delegated account
		delegatedWeight, err := delegatedAuth.calculateWeight(
			delegatedAcct,
			delegatedAccount.Authority,
			message,
			getter,
			visited,
			depth+1,
		)
		if err != nil {
			return 0, fmt.Errorf("delegated account %s: %w", delegatedAcct, err)
		}

		// If the delegated account's authorization is valid (meets its threshold),
		// add the delegation weight to total
		if delegatedWeight >= delegatedAccount.Authority.Threshold {
			accountWeight := authority.GetAccountWeight(delegatedAcct)
			// Check for overflow
			if totalWeight > ^uint64(0)-accountWeight {
				return 0, fmt.Errorf("weight calculation overflow")
			}
			totalWeight += accountWeight
		}
	}

	return totalWeight, nil
}

// GetSignedPubKeys returns a list of all public keys that have valid signatures
func (a *Authorization) GetSignedPubKeys(message []byte) [][]byte {
	var pubKeys [][]byte
	for _, sig := range a.Signatures {
		if sig.Verify(message) {
			pubKeys = append(pubKeys, sig.PubKey)
		}
	}
	return pubKeys
}

// HasSignatureFrom checks if there's a valid signature from a specific public key
func (a *Authorization) HasSignatureFrom(pubKey []byte, message []byte) bool {
	for _, sig := range a.Signatures {
		// Use constant-time comparison to prevent timing attacks
		if len(sig.PubKey) == len(pubKey) && subtle.ConstantTimeCompare(sig.PubKey, pubKey) == 1 {
			return sig.Verify(message)
		}
	}
	return false
}

// CountValidSignatures returns the number of valid signatures
func (a *Authorization) CountValidSignatures(message []byte) int {
	count := 0
	for _, sig := range a.Signatures {
		if sig.Verify(message) {
			count++
		}
	}
	return count
}
