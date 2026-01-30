package capability

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// ValidatorCapability provides controlled access to validator and delegation operations
type ValidatorCapability interface {
	// ModuleName returns the module this capability is scoped to
	ModuleName() string

	// GetValidator retrieves a validator by public key
	GetValidator(ctx context.Context, pubKey []byte) (store.Validator, error)

	// SetValidator stores or updates a validator
	SetValidator(ctx context.Context, validator store.Validator) error

	// DeleteValidator removes a validator
	DeleteValidator(ctx context.Context, pubKey []byte) error

	// HasValidator checks if a validator exists
	HasValidator(ctx context.Context, pubKey []byte) (bool, error)

	// GetActiveValidators retrieves all active validators
	GetActiveValidators(ctx context.Context) ([]store.Validator, error)

	// GetValidatorSet returns validator updates for consensus
	GetValidatorSet(ctx context.Context) ([]types.ValidatorUpdate, error)

	// SetValidatorPower updates a validator's voting power
	SetValidatorPower(ctx context.Context, pubKey []byte, power int64) error

	// SetValidatorActive updates a validator's active status
	SetValidatorActive(ctx context.Context, pubKey []byte, active bool) error

	// IterateValidators iterates over all validators
	IterateValidators(ctx context.Context, callback func(store.Validator) error) error

	// GetDelegation retrieves a delegation
	GetDelegation(ctx context.Context, delegator types.AccountName, validator []byte) (store.Delegation, error)

	// SetDelegation stores or updates a delegation
	SetDelegation(ctx context.Context, delegation store.Delegation) error

	// DeleteDelegation removes a delegation
	DeleteDelegation(ctx context.Context, delegator types.AccountName, validator []byte) error

	// HasDelegation checks if a delegation exists
	HasDelegation(ctx context.Context, delegator types.AccountName, validator []byte) (bool, error)

	// IterateDelegations iterates over all delegations
	IterateDelegations(ctx context.Context, callback func(store.Delegation) error) error
}

// validatorCapability is the implementation of ValidatorCapability
type validatorCapability struct {
	moduleName      string
	validatorStore  *store.ValidatorStore
	delegationStore *store.DelegationStore
}

// ModuleName returns the module this capability is scoped to
func (vc *validatorCapability) ModuleName() string {
	if vc == nil {
		return ""
	}
	return vc.moduleName
}

// GetValidator retrieves a validator by public key
func (vc *validatorCapability) GetValidator(ctx context.Context, pubKey []byte) (store.Validator, error) {
	var zero store.Validator

	if vc == nil || vc.validatorStore == nil {
		return zero, ErrCapabilityNil
	}

	if len(pubKey) == 0 {
		return zero, fmt.Errorf("public key cannot be empty")
	}

	validator, err := vc.validatorStore.Get(ctx, pubKey)
	if err != nil {
		return zero, fmt.Errorf("failed to get validator: %w", err)
	}

	return validator, nil
}

// SetValidator stores or updates a validator
func (vc *validatorCapability) SetValidator(ctx context.Context, validator store.Validator) error {
	if vc == nil || vc.validatorStore == nil {
		return ErrCapabilityNil
	}

	if !validator.IsValid() {
		return fmt.Errorf("invalid validator")
	}

	if err := vc.validatorStore.Set(ctx, validator); err != nil {
		return fmt.Errorf("failed to set validator: %w", err)
	}

	return nil
}

// DeleteValidator removes a validator
func (vc *validatorCapability) DeleteValidator(ctx context.Context, pubKey []byte) error {
	if vc == nil || vc.validatorStore == nil {
		return ErrCapabilityNil
	}

	if len(pubKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	if err := vc.validatorStore.Delete(ctx, pubKey); err != nil {
		return fmt.Errorf("failed to delete validator: %w", err)
	}

	return nil
}

// HasValidator checks if a validator exists
func (vc *validatorCapability) HasValidator(ctx context.Context, pubKey []byte) (bool, error) {
	if vc == nil || vc.validatorStore == nil {
		return false, ErrCapabilityNil
	}

	if len(pubKey) == 0 {
		return false, fmt.Errorf("public key cannot be empty")
	}

	return vc.validatorStore.Has(ctx, pubKey)
}

// GetActiveValidators retrieves all active validators
func (vc *validatorCapability) GetActiveValidators(ctx context.Context) ([]store.Validator, error) {
	if vc == nil || vc.validatorStore == nil {
		return nil, ErrCapabilityNil
	}

	validators, err := vc.validatorStore.GetActiveValidators(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active validators: %w", err)
	}

	// Return defensive copy
	result := make([]store.Validator, len(validators))
	copy(result, validators)
	return result, nil
}

// GetValidatorSet returns validator updates for consensus
func (vc *validatorCapability) GetValidatorSet(ctx context.Context) ([]types.ValidatorUpdate, error) {
	if vc == nil || vc.validatorStore == nil {
		return nil, ErrCapabilityNil
	}

	updates, err := vc.validatorStore.GetValidatorUpdates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get validator updates: %w", err)
	}

	// Return defensive copy
	result := make([]types.ValidatorUpdate, len(updates))
	copy(result, updates)
	return result, nil
}

// SetValidatorPower updates a validator's voting power
func (vc *validatorCapability) SetValidatorPower(ctx context.Context, pubKey []byte, power int64) error {
	if vc == nil || vc.validatorStore == nil {
		return ErrCapabilityNil
	}

	if len(pubKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	if err := vc.validatorStore.SetPower(ctx, pubKey, power); err != nil {
		return fmt.Errorf("failed to set validator power: %w", err)
	}

	return nil
}

// SetValidatorActive updates a validator's active status
func (vc *validatorCapability) SetValidatorActive(ctx context.Context, pubKey []byte, active bool) error {
	if vc == nil || vc.validatorStore == nil {
		return ErrCapabilityNil
	}

	if len(pubKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	if err := vc.validatorStore.SetActive(ctx, pubKey, active); err != nil {
		return fmt.Errorf("failed to set validator active status: %w", err)
	}

	return nil
}

// IterateValidators iterates over all validators
func (vc *validatorCapability) IterateValidators(ctx context.Context, callback func(store.Validator) error) error {
	if vc == nil || vc.validatorStore == nil {
		return ErrCapabilityNil
	}

	if callback == nil {
		return fmt.Errorf("callback cannot be nil")
	}

	iter, err := vc.validatorStore.Iterator(ctx)
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.Valid() {
		validator, err := iter.Value()
		if err != nil {
			return fmt.Errorf("failed to get value: %w", err)
		}

		if err := callback(validator); err != nil {
			return err
		}

		if err := iter.Next(); err != nil {
			return fmt.Errorf("failed to advance iterator: %w", err)
		}
	}

	return nil
}

// GetDelegation retrieves a delegation
func (vc *validatorCapability) GetDelegation(ctx context.Context, delegator types.AccountName, validator []byte) (store.Delegation, error) {
	var zero store.Delegation

	if vc == nil || vc.delegationStore == nil {
		return zero, ErrCapabilityNil
	}

	if !delegator.IsValid() {
		return zero, fmt.Errorf("%w: invalid delegator account name", types.ErrInvalidAccount)
	}

	if len(validator) == 0 {
		return zero, fmt.Errorf("validator public key cannot be empty")
	}

	delegation, err := vc.delegationStore.Get(ctx, delegator, validator)
	if err != nil {
		return zero, fmt.Errorf("failed to get delegation: %w", err)
	}

	return delegation, nil
}

// SetDelegation stores or updates a delegation
func (vc *validatorCapability) SetDelegation(ctx context.Context, delegation store.Delegation) error {
	if vc == nil || vc.delegationStore == nil {
		return ErrCapabilityNil
	}

	if !delegation.IsValid() {
		return fmt.Errorf("invalid delegation")
	}

	if err := vc.delegationStore.Set(ctx, delegation); err != nil {
		return fmt.Errorf("failed to set delegation: %w", err)
	}

	return nil
}

// DeleteDelegation removes a delegation
func (vc *validatorCapability) DeleteDelegation(ctx context.Context, delegator types.AccountName, validator []byte) error {
	if vc == nil || vc.delegationStore == nil {
		return ErrCapabilityNil
	}

	if !delegator.IsValid() {
		return fmt.Errorf("%w: invalid delegator account name", types.ErrInvalidAccount)
	}

	if len(validator) == 0 {
		return fmt.Errorf("validator public key cannot be empty")
	}

	if err := vc.delegationStore.Delete(ctx, delegator, validator); err != nil {
		return fmt.Errorf("failed to delete delegation: %w", err)
	}

	return nil
}

// HasDelegation checks if a delegation exists
func (vc *validatorCapability) HasDelegation(ctx context.Context, delegator types.AccountName, validator []byte) (bool, error) {
	if vc == nil || vc.delegationStore == nil {
		return false, ErrCapabilityNil
	}

	if !delegator.IsValid() {
		return false, fmt.Errorf("%w: invalid delegator account name", types.ErrInvalidAccount)
	}

	if len(validator) == 0 {
		return false, fmt.Errorf("validator public key cannot be empty")
	}

	return vc.delegationStore.Has(ctx, delegator, validator)
}

// IterateDelegations iterates over all delegations
func (vc *validatorCapability) IterateDelegations(ctx context.Context, callback func(store.Delegation) error) error {
	if vc == nil || vc.delegationStore == nil {
		return ErrCapabilityNil
	}

	if callback == nil {
		return fmt.Errorf("callback cannot be nil")
	}

	iter, err := vc.delegationStore.Iterator(ctx)
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.Valid() {
		delegation, err := iter.Value()
		if err != nil {
			return fmt.Errorf("failed to get value: %w", err)
		}

		if err := callback(delegation); err != nil {
			return err
		}

		if err := iter.Next(); err != nil {
			return fmt.Errorf("failed to advance iterator: %w", err)
		}
	}

	return nil
}

// Flush flushes pending changes to backing store
func (vc *validatorCapability) Flush(ctx context.Context) error {
	if vc == nil || vc.validatorStore == nil || vc.delegationStore == nil {
		return ErrCapabilityNil
	}

	// Flush both validator and delegation stores
	if err := vc.validatorStore.Flush(ctx); err != nil {
		return fmt.Errorf("failed to flush validator store: %w", err)
	}

	if err := vc.delegationStore.Flush(ctx); err != nil {
		return fmt.Errorf("failed to flush delegation store: %w", err)
	}

	return nil
}
