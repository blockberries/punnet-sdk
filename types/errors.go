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
)
