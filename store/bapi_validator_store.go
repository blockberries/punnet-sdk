package store

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
)

// BAPIValidator represents a validator with its state.
type BAPIValidator struct {
	// PubKey is the validator's public key
	PubKey types.PublicKey `cramberry:"1"`

	// Power is the current voting power (0 means removed)
	Power uint64 `cramberry:"2"`

	// Jailed indicates if the validator is jailed
	Jailed bool `cramberry:"3"`

	// Description contains human-readable validator info
	Description string `cramberry:"4"`

	// Commission rate in basis points (100 = 1%)
	Commission uint32 `cramberry:"5"`

	// TotalDelegation is the total amount delegated to this validator
	TotalDelegation uint64 `cramberry:"6"`
}

// Address returns the validator's address (first 20 bytes of pubkey hash).
func (v *BAPIValidator) Address() types.ValidatorAddress {
	var addr types.ValidatorAddress
	// Simple address derivation: first 20 bytes of pubkey
	if len(v.PubKey.Data) >= 20 {
		copy(addr[:], v.PubKey.Data[:20])
	}
	return addr
}

// BAPIDelegation represents a delegation from an account to a validator.
type BAPIDelegation struct {
	// Delegator is the account name delegating tokens
	Delegator string `cramberry:"1"`

	// ValidatorPubKey is the validator's public key (hex encoded)
	ValidatorPubKey string `cramberry:"2"`

	// Amount is the delegated amount
	Amount uint64 `cramberry:"3"`
}

// BAPIValidatorStore provides typed access to validator data.
type BAPIValidatorStore struct {
	validators  *TypedStore[*BAPIValidator]
	delegations *TypedStore[*BAPIDelegation]
}

// NewBAPIValidatorStore creates a new validator store backed by blockberry's StateStore.
func NewBAPIValidatorStore(store statestore.StateStore) *BAPIValidatorStore {
	return &BAPIValidatorStore{
		validators:  NewTypedStore[*BAPIValidator](store, "validators/"),
		delegations: NewTypedStore[*BAPIDelegation](store, "delegations/"),
	}
}

// validatorKey creates the key for a validator (hex-encoded pubkey).
func validatorKey(pubKey []byte) string {
	return hex.EncodeToString(pubKey)
}

// delegationKey creates the key for a delegation.
func delegationKey(delegator string, validatorPubKey []byte) string {
	return delegator + "/" + hex.EncodeToString(validatorPubKey)
}

// GetValidator retrieves a validator by public key.
func (s *BAPIValidatorStore) GetValidator(ctx context.Context, pubKey []byte) (*BAPIValidator, error) {
	if len(pubKey) == 0 {
		return nil, fmt.Errorf("empty public key")
	}
	return s.validators.Get(ctx, validatorKey(pubKey))
}

// SetValidator stores a validator.
func (s *BAPIValidatorStore) SetValidator(ctx context.Context, validator *BAPIValidator) error {
	if validator == nil {
		return ErrInvalidValue
	}
	if len(validator.PubKey.Data) == 0 {
		return fmt.Errorf("validator missing public key")
	}
	return s.validators.Set(ctx, validatorKey(validator.PubKey.Data), validator)
}

// DeleteValidator removes a validator.
func (s *BAPIValidatorStore) DeleteValidator(ctx context.Context, pubKey []byte) error {
	if len(pubKey) == 0 {
		return fmt.Errorf("empty public key")
	}
	return s.validators.Delete(ctx, validatorKey(pubKey))
}

// HasValidator checks if a validator exists.
func (s *BAPIValidatorStore) HasValidator(ctx context.Context, pubKey []byte) (bool, error) {
	if len(pubKey) == 0 {
		return false, fmt.Errorf("empty public key")
	}
	return s.validators.Has(ctx, validatorKey(pubKey))
}

// UpdatePower updates a validator's voting power.
// Returns an error if the validator doesn't exist.
func (s *BAPIValidatorStore) UpdatePower(ctx context.Context, pubKey []byte, power uint64) error {
	validator, err := s.GetValidator(ctx, pubKey)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	}

	validator.Power = power
	return s.SetValidator(ctx, validator)
}

// JailValidator marks a validator as jailed.
func (s *BAPIValidatorStore) JailValidator(ctx context.Context, pubKey []byte) error {
	validator, err := s.GetValidator(ctx, pubKey)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	}

	validator.Jailed = true
	validator.Power = 0 // Remove from active set
	return s.SetValidator(ctx, validator)
}

// UnjailValidator removes the jailed status from a validator.
func (s *BAPIValidatorStore) UnjailValidator(ctx context.Context, pubKey []byte) error {
	validator, err := s.GetValidator(ctx, pubKey)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	}

	validator.Jailed = false
	return s.SetValidator(ctx, validator)
}

// GetDelegation retrieves a delegation.
func (s *BAPIValidatorStore) GetDelegation(ctx context.Context, delegator string, validatorPubKey []byte) (*BAPIDelegation, error) {
	if delegator == "" {
		return nil, fmt.Errorf("empty delegator")
	}
	if len(validatorPubKey) == 0 {
		return nil, fmt.Errorf("empty validator public key")
	}

	delegation, err := s.delegations.Get(ctx, delegationKey(delegator, validatorPubKey))
	if err == ErrNotFound {
		// Return zero delegation instead of error
		return &BAPIDelegation{
			Delegator:       delegator,
			ValidatorPubKey: hex.EncodeToString(validatorPubKey),
			Amount:          0,
		}, nil
	}
	return delegation, err
}

// SetDelegation stores a delegation.
func (s *BAPIValidatorStore) SetDelegation(ctx context.Context, delegation *BAPIDelegation) error {
	if delegation == nil {
		return ErrInvalidValue
	}
	if delegation.Delegator == "" {
		return fmt.Errorf("empty delegator")
	}
	if delegation.ValidatorPubKey == "" {
		return fmt.Errorf("empty validator public key")
	}

	validatorPubKey, err := hex.DecodeString(delegation.ValidatorPubKey)
	if err != nil {
		return fmt.Errorf("invalid validator public key: %w", err)
	}

	return s.delegations.Set(ctx, delegationKey(delegation.Delegator, validatorPubKey), delegation)
}

// DeleteDelegation removes a delegation.
func (s *BAPIValidatorStore) DeleteDelegation(ctx context.Context, delegator string, validatorPubKey []byte) error {
	if delegator == "" {
		return fmt.Errorf("empty delegator")
	}
	if len(validatorPubKey) == 0 {
		return fmt.Errorf("empty validator public key")
	}
	return s.delegations.Delete(ctx, delegationKey(delegator, validatorPubKey))
}

// Delegate adds to a delegation and updates the validator's total delegation.
// Returns an error if the validator doesn't exist.
func (s *BAPIValidatorStore) Delegate(ctx context.Context, delegator string, validatorPubKey []byte, amount uint64) error {
	// Get current delegation
	delegation, err := s.GetDelegation(ctx, delegator, validatorPubKey)
	if err != nil && err != ErrNotFound {
		return fmt.Errorf("get delegation: %w", err)
	}

	// Update delegation amount
	newAmount := delegation.Amount + amount
	if newAmount < delegation.Amount {
		return fmt.Errorf("delegation overflow")
	}
	delegation.Amount = newAmount

	// Update validator's total delegation
	validator, err := s.GetValidator(ctx, validatorPubKey)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	}

	newTotal := validator.TotalDelegation + amount
	if newTotal < validator.TotalDelegation {
		return fmt.Errorf("total delegation overflow")
	}
	validator.TotalDelegation = newTotal

	// Save both
	if err := s.SetDelegation(ctx, delegation); err != nil {
		return fmt.Errorf("set delegation: %w", err)
	}
	if err := s.SetValidator(ctx, validator); err != nil {
		return fmt.Errorf("set validator: %w", err)
	}

	return nil
}

// Undelegate removes from a delegation and updates the validator's total delegation.
// Returns an error if the delegation is insufficient.
func (s *BAPIValidatorStore) Undelegate(ctx context.Context, delegator string, validatorPubKey []byte, amount uint64) error {
	// Get current delegation
	delegation, err := s.GetDelegation(ctx, delegator, validatorPubKey)
	if err != nil {
		return fmt.Errorf("get delegation: %w", err)
	}

	if amount > delegation.Amount {
		return fmt.Errorf("insufficient delegation: have %d, want %d", delegation.Amount, amount)
	}

	delegation.Amount -= amount

	// Update validator's total delegation
	validator, err := s.GetValidator(ctx, validatorPubKey)
	if err != nil {
		return fmt.Errorf("get validator: %w", err)
	}

	if amount > validator.TotalDelegation {
		return fmt.Errorf("total delegation underflow")
	}
	validator.TotalDelegation -= amount

	// Save both
	if err := s.SetDelegation(ctx, delegation); err != nil {
		return fmt.Errorf("set delegation: %w", err)
	}
	if err := s.SetValidator(ctx, validator); err != nil {
		return fmt.Errorf("set validator: %w", err)
	}

	return nil
}

// GetValidatorWithProof retrieves a validator with a Merkle proof.
func (s *BAPIValidatorStore) GetValidatorWithProof(ctx context.Context, pubKey []byte) (*BAPIValidator, *statestore.Proof, error) {
	if len(pubKey) == 0 {
		return nil, nil, fmt.Errorf("empty public key")
	}
	return s.validators.GetWithProof(ctx, validatorKey(pubKey))
}

// GetValidatorAtHeight retrieves a validator at a specific block height.
func (s *BAPIValidatorStore) GetValidatorAtHeight(ctx context.Context, pubKey []byte, height int64) (*BAPIValidator, error) {
	if len(pubKey) == 0 {
		return nil, fmt.Errorf("empty public key")
	}
	return s.validators.GetAtHeight(ctx, validatorKey(pubKey), height)
}

// IterateValidators walks every validator currently in the store,
// invoking fn for each. Returns true from fn to stop iteration.
//
// Returns ErrIterationUnsupported if the underlying StateStore is not
// iterable (test in-memory stores). Used by genesis export and any
// query handler that needs a full validator listing without reaching
// into the typed-store internals.
//
// Iteration order is the underlying tree's ascending byte order over
// hex-encoded pubkeys, which is deterministic and suitable for
// consensus-critical exports.
func (s *BAPIValidatorStore) IterateValidators(fn func(v *BAPIValidator) bool) error {
	return s.validators.IterateRelative(func(_ string, v *BAPIValidator) bool {
		return fn(v)
	})
}
