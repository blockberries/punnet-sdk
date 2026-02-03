package auth

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// SupportedSignDocVersions references the canonical list from the types package.
// This ensures consistency between the auth module and core types.
var SupportedSignDocVersions = types.SupportedSignDocVersions

// TransactionValidator provides stateless and stateful validation of transactions
// using SignDoc-based verification.
//
// INVARIANT: All validation methods are deterministic - same inputs produce same outputs.
// INVARIANT: Validation failures always return descriptive errors for debugging.
type TransactionValidator struct {
	chainID string
}

// NewTransactionValidator creates a new validator bound to the specified chain.
//
// PRECONDITION: chainID is non-empty
// POSTCONDITION: Returned validator will reject all transactions with different chain IDs
func NewTransactionValidator(chainID string) (*TransactionValidator, error) {
	if chainID == "" {
		return nil, fmt.Errorf("chainID cannot be empty")
	}
	return &TransactionValidator{chainID: chainID}, nil
}

// ValidateSignDocVersion checks if the given SignDoc version is supported.
// This is a convenience wrapper around types.ValidateSignDocVersion for use
// within the auth module.
//
// PRECONDITION: version is a non-empty string
// POSTCONDITION: Returns nil if version is in SupportedSignDocVersions
// POSTCONDITION: Returns ErrUnsupportedVersion if version is not supported
//
// SECURITY: Rejecting unknown versions prevents forward-compatibility attacks
// where an attacker might exploit differences in how different nodes interpret
// a new version format.
func ValidateSignDocVersion(version string) error {
	return types.ValidateSignDocVersion(version)
}

// ValidateReplayProtection validates that a transaction's nonce matches the
// expected account sequence for replay protection.
//
// PRECONDITION: tx is not nil
// PRECONDITION: chainID is non-empty
// PRECONDITION: expectedSequence is the current nonce for the signing account
//
// POSTCONDITION: Returns nil if transaction nonce matches expectedSequence
// POSTCONDITION: Returns ErrSequenceMismatch if nonces differ
//
// SECURITY: This function provides replay protection by ensuring each transaction
// can only be processed once. The nonce must exactly match the account's current
// sequence number.
//
// INVARIANT: A transaction passing this validation cannot be replayed on:
//   - The same chain after nonce increment (sequence mismatch)
//
// NOTE: Chain ID binding is enforced at signature verification time through the
// SignDoc hash - signatures are bound to a specific chain ID.
func ValidateReplayProtection(tx *types.Transaction, chainID string, expectedSequence uint64) error {
	if tx == nil {
		return fmt.Errorf("%w: transaction is nil", types.ErrInvalidTransaction)
	}

	if chainID == "" {
		return fmt.Errorf("%w: chainID cannot be empty", types.ErrInvalidTransaction)
	}

	// Verify account sequence
	// SECURITY: If transaction nonce differs from expectedSequence, this indicates
	// either a stale transaction or a replay attempt.
	// - tx.Nonce < expectedSequence: Transaction already processed (replay attempt)
	// - tx.Nonce > expectedSequence: Future transaction (should fail)
	if tx.Nonce != expectedSequence {
		return fmt.Errorf("%w: expected %d, got %d", types.ErrSequenceMismatch, expectedSequence, tx.Nonce)
	}

	return nil
}

// ValidateSignDoc performs comprehensive validation of a SignDoc.
//
// PRECONDITION: signDoc is not nil
// POSTCONDITION: Returns nil if all validations pass
// POSTCONDITION: Returns appropriate error for each validation failure
//
// This function validates:
// 1. SignDoc version is supported
// 2. Basic structural validation (non-empty fields, etc.)
func ValidateSignDoc(signDoc *types.SignDoc) error {
	if signDoc == nil {
		return fmt.Errorf("%w: SignDoc is nil", types.ErrInvalidTransaction)
	}

	// Validate version
	if err := ValidateSignDocVersion(signDoc.Version); err != nil {
		return err
	}

	// Perform basic validation
	if err := signDoc.ValidateBasic(); err != nil {
		return err
	}

	return nil
}

// ValidateTransaction performs full transaction validation including SignDoc verification.
//
// PRECONDITION: tx is not nil
// PRECONDITION: chainID is non-empty
// PRECONDITION: account is not nil and contains the expected nonce
//
// POSTCONDITION: Returns nil if transaction is valid
// POSTCONDITION: Returns appropriate error describing the validation failure
//
// This function validates:
// 1. Basic transaction structure
// 2. SignDoc version is supported
// 3. Replay protection (chain ID and sequence)
// 4. SignDoc roundtrip determinism
//
// NOTE: This does NOT verify signatures. Use Transaction.VerifyAuthorization for full verification.
func (v *TransactionValidator) ValidateTransaction(tx *types.Transaction, account *types.Account) error {
	if tx == nil {
		return fmt.Errorf("%w: transaction is nil", types.ErrInvalidTransaction)
	}

	if account == nil {
		return fmt.Errorf("%w: account is nil", types.ErrInvalidTransaction)
	}

	// Basic transaction validation
	if err := tx.ValidateBasic(); err != nil {
		return err
	}

	// Reconstruct SignDoc
	signDoc, err := tx.ToSignDoc(v.chainID, account.Nonce)
	if err != nil {
		return fmt.Errorf("%w: failed to reconstruct SignDoc: %v", types.ErrInvalidTransaction, err)
	}

	// Validate SignDoc version
	if err := ValidateSignDocVersion(signDoc.Version); err != nil {
		return err
	}

	// Validate replay protection
	if err := ValidateReplayProtection(tx, v.chainID, account.Nonce); err != nil {
		return err
	}

	// Validate roundtrip determinism
	// SECURITY: This catches non-deterministic serialization bugs and tampering attempts
	if err := tx.ValidateSignDocRoundtrip(v.chainID, account.Nonce); err != nil {
		return err
	}

	return nil
}

// ValidateForMempool validates a transaction for mempool admission.
// This is a lightweight check suitable for high-throughput scenarios.
//
// PRECONDITION: tx is not nil
// PRECONDITION: account is not nil
//
// POSTCONDITION: Returns nil if transaction can be admitted to mempool
// POSTCONDITION: Returns error if transaction should be rejected
//
// NOTE: Mempool validation is less strict than block validation to allow
// for transactions that may become valid in future blocks (e.g., slightly
// ahead nonce). However, we still enforce chain ID binding strictly.
func (v *TransactionValidator) ValidateForMempool(tx *types.Transaction, account *types.Account) error {
	if tx == nil {
		return fmt.Errorf("%w: transaction is nil", types.ErrInvalidTransaction)
	}

	if account == nil {
		return fmt.Errorf("%w: account is nil", types.ErrInvalidTransaction)
	}

	// Basic validation is always required
	if err := tx.ValidateBasic(); err != nil {
		return err
	}

	// Reconstruct SignDoc
	signDoc, err := tx.ToSignDoc(v.chainID, account.Nonce)
	if err != nil {
		return fmt.Errorf("%w: failed to reconstruct SignDoc: %v", types.ErrInvalidTransaction, err)
	}

	// Version validation is always required
	if err := ValidateSignDocVersion(signDoc.Version); err != nil {
		return err
	}

	// Chain ID must always match (no cross-chain replay)
	// SECURITY: This is non-negotiable even for mempool
	if signDoc.ChainID != v.chainID {
		return fmt.Errorf("%w: expected %q, got %q", types.ErrChainIDMismatch, v.chainID, signDoc.ChainID)
	}

	// For mempool, we allow sequence to be current or slightly ahead
	// to handle concurrent transaction submission
	// INVARIANT: sequence must be >= current account nonce
	if tx.Nonce < account.Nonce {
		return fmt.Errorf("%w: transaction nonce %d is behind account nonce %d",
			types.ErrSequenceMismatch, tx.Nonce, account.Nonce)
	}

	return nil
}

// ValidateForBlockProposal validates a transaction for inclusion in a block proposal.
// This is the strictest validation level.
//
// PRECONDITION: tx is not nil
// PRECONDITION: account is not nil
// PRECONDITION: getter is not nil (for delegation verification)
//
// POSTCONDITION: Returns nil if transaction is valid for block inclusion
// POSTCONDITION: Returns error if transaction should not be included
//
// This validation includes full signature verification.
func (v *TransactionValidator) ValidateForBlockProposal(
	tx *types.Transaction,
	account *types.Account,
	getter types.AccountGetter,
) error {
	// First perform structural validation
	if err := v.ValidateTransaction(tx, account); err != nil {
		return err
	}

	// Then verify signatures (the most expensive check)
	// SECURITY: This is the authoritative check that proves the account holder
	// authorized this specific transaction
	if err := tx.VerifyAuthorization(v.chainID, account, getter); err != nil {
		return err
	}

	return nil
}

// ChainID returns the chain ID this validator is bound to.
func (v *TransactionValidator) ChainID() string {
	return v.chainID
}
