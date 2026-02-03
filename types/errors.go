package types

import "errors"

var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("not found")

	// ErrUnauthorized indicates insufficient authorization
	ErrUnauthorized = errors.New("unauthorized")

	// ErrInvalidAccount indicates an invalid account name
	ErrInvalidAccount = errors.New("invalid account name")

	// ErrInvalidAuthority indicates an invalid authority structure
	ErrInvalidAuthority = errors.New("invalid authority")

	// ErrInvalidAuthorization indicates invalid authorization
	ErrInvalidAuthorization = errors.New("invalid authorization")

	// ErrAuthorizationCycle indicates a cycle in delegation chain
	ErrAuthorizationCycle = errors.New("authorization cycle detected")

	// ErrInsufficientWeight indicates authorization weight below threshold
	ErrInsufficientWeight = errors.New("insufficient authorization weight")

	// ErrInvalidSignature indicates an invalid signature
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrInvalidCoin indicates an invalid coin (negative amount, empty denom)
	ErrInvalidCoin = errors.New("invalid coin")

	// ErrInsufficientFunds indicates insufficient balance for operation
	ErrInsufficientFunds = errors.New("insufficient funds")

	// ErrInvalidMessage indicates an invalid message
	ErrInvalidMessage = errors.New("invalid message")

	// ErrInvalidTransaction indicates an invalid transaction
	ErrInvalidTransaction = errors.New("invalid transaction")

	// ErrConflictingEffects indicates conflicting effects in execution
	ErrConflictingEffects = errors.New("conflicting effects")

	// ErrInvalidEffect indicates an invalid effect
	ErrInvalidEffect = errors.New("invalid effect")

	// ErrMaxRecursionDepth indicates maximum recursion depth exceeded
	ErrMaxRecursionDepth = errors.New("maximum recursion depth exceeded")

	// ErrSignDocMismatch indicates SignDoc reconstruction produced different bytes.
	// SECURITY: This error indicates potential non-deterministic serialization or tampering.
	ErrSignDocMismatch = errors.New("SignDoc reconstruction mismatch: non-deterministic serialization detected")

	// ErrInvalidPublicKey indicates a malformed or unsupported public key
	ErrInvalidPublicKey = errors.New("invalid public key")

	// ErrUnsupportedAlgorithm indicates an unknown or unsupported signature algorithm
	ErrUnsupportedAlgorithm = errors.New("unsupported signature algorithm")

	// ErrDuplicateSignature indicates duplicate signatures from the same public key.
	// SECURITY: This error prevents attackers from submitting multiple copies of the
	// same signature to inflate their authorization weight.
	ErrDuplicateSignature = errors.New("duplicate signature from same public key")

	// ErrChainIDMismatch indicates a transaction was signed for a different chain.
	// SECURITY: This prevents cross-chain replay attacks where a valid transaction
	// on one chain is replayed on another chain with the same account addresses.
	ErrChainIDMismatch = errors.New("chain ID mismatch")

	// ErrSequenceMismatch indicates a transaction nonce does not match the expected account sequence.
	// SECURITY: This prevents replay attacks where a previously valid transaction
	// is submitted again after it has already been processed.
	ErrSequenceMismatch = errors.New("account sequence mismatch")

	// ErrUnsupportedVersion indicates a SignDoc version that is not supported.
	// SECURITY: Rejecting unknown versions prevents forward-compatibility attacks
	// where nodes with different version support might interpret transactions differently.
	ErrUnsupportedVersion = errors.New("unsupported SignDoc version")
)
