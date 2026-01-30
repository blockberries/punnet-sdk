package types

import (
	"crypto/ed25519"
	"fmt"
)

const (
	// MaxRecursionDepth limits delegation chain depth to prevent stack overflow
	MaxRecursionDepth = 10
)

// Signature represents a single signature with public key
type Signature struct {
	// PubKey is the Ed25519 public key (32 bytes)
	PubKey []byte `json:"pub_key"`

	// Signature is the Ed25519 signature (64 bytes)
	Signature []byte `json:"signature"`
}

// ValidateBasic performs basic validation
func (s *Signature) ValidateBasic() error {
	if len(s.PubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: public key must be %d bytes, got %d", ErrInvalidSignature, ed25519.PublicKeySize, len(s.PubKey))
	}
	if len(s.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature must be %d bytes, got %d", ErrInvalidSignature, ed25519.SignatureSize, len(s.Signature))
	}
	return nil
}

// Verify verifies the signature against a message
func (s *Signature) Verify(message []byte) bool {
	if err := s.ValidateBasic(); err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(s.PubKey), message, s.Signature)
}

// Authorization represents proof of authority to perform an action
type Authorization struct {
	// Signatures from keys in the account's authority
	Signatures []Signature `json:"signatures"`

	// AccountAuthorizations maps delegated account names to their authorizations
	// This enables recursive/hierarchical authorization
	AccountAuthorizations map[AccountName]*Authorization `json:"account_authorizations,omitempty"`
}

// NewAuthorization creates a new authorization with signatures
func NewAuthorization(signatures ...Signature) *Authorization {
	return &Authorization{
		Signatures:            signatures,
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}
}

// ValidateBasic performs basic validation
func (a *Authorization) ValidateBasic() error {
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
// It implements cycle detection using DFS with a visited set
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

	// Calculate weight from direct key signatures
	for _, sig := range a.Signatures {
		if authority.HasKey(sig.PubKey) {
			if sig.Verify(message) {
				totalWeight += authority.GetKeyWeight(sig.PubKey)
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
			totalWeight += authority.GetAccountWeight(delegatedAcct)
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
		if string(sig.PubKey) == string(pubKey) {
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
