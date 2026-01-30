package store

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// Validator represents a blockchain validator
type Validator struct {
	// PubKey is the validator's public key
	PubKey []byte `json:"pub_key"`

	// Power is the validator's voting power
	Power int64 `json:"power"`

	// Delegator is the account that controls this validator
	Delegator types.AccountName `json:"delegator"`

	// Commission is the commission rate (0-10000, where 10000 = 100%)
	Commission uint64 `json:"commission"`

	// Active indicates if the validator is active
	Active bool `json:"active"`
}

// NewValidator creates a new validator
func NewValidator(pubKey []byte, power int64, delegator types.AccountName) Validator {
	// Create defensive copy of pubKey
	keyCopy := make([]byte, len(pubKey))
	copy(keyCopy, pubKey)

	return Validator{
		PubKey:     keyCopy,
		Power:      power,
		Delegator:  delegator,
		Commission: 0,
		Active:     true,
	}
}

// IsValid checks if the validator is valid
func (v Validator) IsValid() bool {
	return len(v.PubKey) > 0 && v.Delegator.IsValid() && v.Commission <= 10000
}

// ToValidatorUpdate converts a validator to a ValidatorUpdate
func (v Validator) ToValidatorUpdate() types.ValidatorUpdate {
	// Create defensive copy of pubKey
	keyCopy := make([]byte, len(v.PubKey))
	copy(keyCopy, v.PubKey)

	return types.ValidatorUpdate{
		PubKey: keyCopy,
		Power:  v.Power,
	}
}

// ValidatorKey creates a key from a public key
func ValidatorKey(pubKey []byte) []byte {
	// Create defensive copy
	key := make([]byte, len(pubKey))
	copy(key, pubKey)
	return key
}

// Delegation represents a delegation to a validator
type Delegation struct {
	// Delegator is the account delegating
	Delegator types.AccountName `json:"delegator"`

	// Validator is the validator's public key
	Validator []byte `json:"validator"`

	// Shares is the number of shares owned
	Shares uint64 `json:"shares"`
}

// NewDelegation creates a new delegation
func NewDelegation(delegator types.AccountName, validator []byte, shares uint64) Delegation {
	// Create defensive copy of validator pubkey
	valCopy := make([]byte, len(validator))
	copy(valCopy, validator)

	return Delegation{
		Delegator: delegator,
		Validator: valCopy,
		Shares:    shares,
	}
}

// IsValid checks if the delegation is valid
func (d Delegation) IsValid() bool {
	return d.Delegator.IsValid() && len(d.Validator) > 0
}

// DelegationKey creates a unique key for a delegation
// Format: delegator/validator
func DelegationKey(delegator types.AccountName, validator []byte) []byte {
	return []byte(fmt.Sprintf("%s/%x", delegator, validator))
}

// ValidatorStore is a typed store for Validator objects
type ValidatorStore struct {
	store ObjectStore[Validator]
}

// NewValidatorStore creates a new validator store
func NewValidatorStore(backing BackingStore) *ValidatorStore {
	serializer := NewJSONSerializer[Validator]()
	store := NewCachedObjectStore(backing, serializer, 1000, 10000)

	return &ValidatorStore{
		store: store,
	}
}

// Get retrieves a validator by public key
func (vs *ValidatorStore) Get(ctx context.Context, pubKey []byte) (Validator, error) {
	var zero Validator

	if vs == nil || vs.store == nil {
		return zero, ErrStoreNil
	}

	if len(pubKey) == 0 {
		return zero, fmt.Errorf("%w: empty public key", ErrInvalidKey)
	}

	key := ValidatorKey(pubKey)
	return vs.store.Get(ctx, key)
}

// Set stores a validator
func (vs *ValidatorStore) Set(ctx context.Context, validator Validator) error {
	if vs == nil || vs.store == nil {
		return ErrStoreNil
	}

	if !validator.IsValid() {
		return fmt.Errorf("%w: invalid validator", ErrInvalidValue)
	}

	key := ValidatorKey(validator.PubKey)
	return vs.store.Set(ctx, key, validator)
}

// Delete removes a validator by public key
func (vs *ValidatorStore) Delete(ctx context.Context, pubKey []byte) error {
	if vs == nil || vs.store == nil {
		return ErrStoreNil
	}

	if len(pubKey) == 0 {
		return fmt.Errorf("%w: empty public key", ErrInvalidKey)
	}

	key := ValidatorKey(pubKey)
	return vs.store.Delete(ctx, key)
}

// Has checks if a validator exists
func (vs *ValidatorStore) Has(ctx context.Context, pubKey []byte) (bool, error) {
	if vs == nil || vs.store == nil {
		return false, ErrStoreNil
	}

	if len(pubKey) == 0 {
		return false, fmt.Errorf("%w: empty public key", ErrInvalidKey)
	}

	key := ValidatorKey(pubKey)
	return vs.store.Has(ctx, key)
}

// Iterator returns an iterator over all validators
func (vs *ValidatorStore) Iterator(ctx context.Context) (Iterator[Validator], error) {
	if vs == nil || vs.store == nil {
		return nil, ErrStoreNil
	}

	return vs.store.Iterator(ctx, nil, nil)
}

// GetActiveValidators retrieves all active validators
func (vs *ValidatorStore) GetActiveValidators(ctx context.Context) ([]Validator, error) {
	if vs == nil || vs.store == nil {
		return nil, ErrStoreNil
	}

	iter, err := vs.Iterator(ctx)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	validators := make([]Validator, 0)
	for iter.Valid() {
		validator, err := iter.Value()
		if err != nil {
			return nil, err
		}

		if validator.Active && validator.Power > 0 {
			validators = append(validators, validator)
		}

		if err := iter.Next(); err != nil {
			return nil, err
		}
	}

	return validators, nil
}

// GetValidatorUpdates converts validators to ValidatorUpdate format
func (vs *ValidatorStore) GetValidatorUpdates(ctx context.Context) ([]types.ValidatorUpdate, error) {
	if vs == nil || vs.store == nil {
		return nil, ErrStoreNil
	}

	validators, err := vs.GetActiveValidators(ctx)
	if err != nil {
		return nil, err
	}

	updates := make([]types.ValidatorUpdate, len(validators))
	for i, validator := range validators {
		updates[i] = validator.ToValidatorUpdate()
	}

	return updates, nil
}

// SetPower updates a validator's power
func (vs *ValidatorStore) SetPower(ctx context.Context, pubKey []byte, power int64) error {
	if vs == nil || vs.store == nil {
		return ErrStoreNil
	}

	validator, err := vs.Get(ctx, pubKey)
	if err != nil {
		return err
	}

	validator.Power = power
	return vs.Set(ctx, validator)
}

// SetActive updates a validator's active status
func (vs *ValidatorStore) SetActive(ctx context.Context, pubKey []byte, active bool) error {
	if vs == nil || vs.store == nil {
		return ErrStoreNil
	}

	validator, err := vs.Get(ctx, pubKey)
	if err != nil {
		return err
	}

	validator.Active = active
	return vs.Set(ctx, validator)
}

// Flush writes any pending changes to the underlying storage
func (vs *ValidatorStore) Flush(ctx context.Context) error {
	if vs == nil || vs.store == nil {
		return ErrStoreNil
	}

	return vs.store.Flush(ctx)
}

// Close releases any resources held by the store
func (vs *ValidatorStore) Close() error {
	if vs == nil || vs.store == nil {
		return ErrStoreNil
	}

	return vs.store.Close()
}

// DelegationStore is a typed store for Delegation objects
type DelegationStore struct {
	store ObjectStore[Delegation]
}

// NewDelegationStore creates a new delegation store
func NewDelegationStore(backing BackingStore) *DelegationStore {
	serializer := NewJSONSerializer[Delegation]()
	store := NewCachedObjectStore(backing, serializer, 10000, 100000)

	return &DelegationStore{
		store: store,
	}
}

// Get retrieves a delegation
func (ds *DelegationStore) Get(ctx context.Context, delegator types.AccountName, validator []byte) (Delegation, error) {
	var zero Delegation

	if ds == nil || ds.store == nil {
		return zero, ErrStoreNil
	}

	if !delegator.IsValid() {
		return zero, fmt.Errorf("%w: invalid delegator", types.ErrInvalidAccount)
	}

	if len(validator) == 0 {
		return zero, fmt.Errorf("%w: empty validator public key", ErrInvalidKey)
	}

	key := DelegationKey(delegator, validator)
	return ds.store.Get(ctx, key)
}

// Set stores a delegation
func (ds *DelegationStore) Set(ctx context.Context, delegation Delegation) error {
	if ds == nil || ds.store == nil {
		return ErrStoreNil
	}

	if !delegation.IsValid() {
		return fmt.Errorf("%w: invalid delegation", ErrInvalidValue)
	}

	key := DelegationKey(delegation.Delegator, delegation.Validator)
	return ds.store.Set(ctx, key, delegation)
}

// Delete removes a delegation
func (ds *DelegationStore) Delete(ctx context.Context, delegator types.AccountName, validator []byte) error {
	if ds == nil || ds.store == nil {
		return ErrStoreNil
	}

	if !delegator.IsValid() {
		return fmt.Errorf("%w: invalid delegator", types.ErrInvalidAccount)
	}

	if len(validator) == 0 {
		return fmt.Errorf("%w: empty validator public key", ErrInvalidKey)
	}

	key := DelegationKey(delegator, validator)
	return ds.store.Delete(ctx, key)
}

// Has checks if a delegation exists
func (ds *DelegationStore) Has(ctx context.Context, delegator types.AccountName, validator []byte) (bool, error) {
	if ds == nil || ds.store == nil {
		return false, ErrStoreNil
	}

	if !delegator.IsValid() {
		return false, fmt.Errorf("%w: invalid delegator", types.ErrInvalidAccount)
	}

	if len(validator) == 0 {
		return false, fmt.Errorf("%w: empty validator public key", ErrInvalidKey)
	}

	key := DelegationKey(delegator, validator)
	return ds.store.Has(ctx, key)
}

// Iterator returns an iterator over all delegations
func (ds *DelegationStore) Iterator(ctx context.Context) (Iterator[Delegation], error) {
	if ds == nil || ds.store == nil {
		return nil, ErrStoreNil
	}

	return ds.store.Iterator(ctx, nil, nil)
}

// Flush writes any pending changes to the underlying storage
func (ds *DelegationStore) Flush(ctx context.Context) error {
	if ds == nil || ds.store == nil {
		return ErrStoreNil
	}

	return ds.store.Flush(ctx)
}

// Close releases any resources held by the store
func (ds *DelegationStore) Close() error {
	if ds == nil || ds.store == nil {
		return ErrStoreNil
	}

	return ds.store.Close()
}
