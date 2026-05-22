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

	// Jailed indicates if the validator is jailed (excluded from
	// the active set, but may un-jail in the future).
	Jailed bool `cramberry:"3"`

	// Description contains human-readable validator info
	Description string `cramberry:"4"`

	// Commission rate in basis points (100 = 1%)
	Commission uint32 `cramberry:"5"`

	// TotalDelegation is the total amount delegated to this validator
	TotalDelegation uint64 `cramberry:"6"`

	// Tombstoned indicates the validator has been permanently
	// removed (e.g. double-slashed). Tombstoned validators can never
	// re-enter the active set even if they un-jail.
	// PLAN §7 Phase 2.7.
	Tombstoned bool `cramberry:"7"`

	// Bond is the validator-register bond posted at
	// MsgCreateValidator time and held in module.bonds. Refunded
	// on clean deregister; forfeited to module.ct on jail or
	// tombstone. PLAN §7 Phase 2.8 / D23.
	Bond uint64 `cramberry:"8"`
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

// BAPIUnbondingEntry is one row in the unbonding queue. Each
// MsgUndelegate splits the delegator's stake from the validator
// pool and parks it in an entry that matures at MaturityHeight; on
// the first EndBlock with Height >= MaturityHeight the staking
// module transfers the tokens from "staking.pool" back to the
// delegator and deletes the row.
//
// MaturityHeight defaults to currentHeight + UnbondingPeriodBlocks
// (spec §4 — 21 days). The maturity is computed at undelegate time
// and never re-evaluated, so a later parameter change does not
// affect entries already in the queue.
//
// Seq distinguishes multiple unbondings that share a maturity
// height (same block, same delegator+validator) so the typed-store
// key is unique. It is a monotonic counter per BAPIValidatorStore
// instance; the value is consensus-irrelevant because the store
// key only needs uniqueness within a height.
type BAPIUnbondingEntry struct {
	Delegator       string `cramberry:"1"`
	ValidatorPubKey string `cramberry:"2"`
	Amount          uint64 `cramberry:"3"`
	MaturityHeight  uint64 `cramberry:"4"`
	Seq             uint64 `cramberry:"5"`
}

// BAPIActiveSetEntry is one validator's record in the previous-epoch
// active set snapshot. The set is keyed by hex pubkey under
// "active_set/"; EndBlock at epoch-close diffs the fresh top-N against
// this snapshot to emit ValidatorUpdates, then replaces it.
//
// Power is the voting power as of the epoch close, not the current
// TotalDelegation — they may differ when slashing or mid-epoch
// delegation changes have accumulated.
type BAPIActiveSetEntry struct {
	PubKey []byte `cramberry:"1"`
	Power  uint64 `cramberry:"2"`
}

// BAPIBootstrapInfo records the vesting state of one bootstrap
// validator's allocation. Created at genesis from
// TokenomicsGenesis.BootstrapValidators; consulted by handleUndelegate
// to block early exits and (Phase 2.5) by EndBlock to release the
// per-block vested share.
//
//   - LockedAmount: the initial BL share assigned to this validator
//     at genesis. Equals AllocPctBootstrap × TotalSupply ÷
//     len(BootstrapValidators) plus the rounding remainder
//     distributed to the first validator.
//   - VestStartHeight: the chain height at which vesting begins
//     (genesis_height + BootstrapLockBlocks).
//   - VestedAmount: the cumulative amount released so far. Bumped
//     by EndBlock during the vesting window; reaches LockedAmount
//     after VestStartHeight + BootstrapVestBlocks.
//
// Keyed by hex pubkey under "bootstrap/" so the bootstrap-validator
// set is iterable for per-block vesting.
type BAPIBootstrapInfo struct {
	PubKey          []byte `cramberry:"1"`
	LockedAmount    uint64 `cramberry:"2"`
	VestStartHeight uint64 `cramberry:"3"`
	VestedAmount    uint64 `cramberry:"4"`
}

// BAPIValidatorStore provides typed access to validator data.
type BAPIValidatorStore struct {
	validators  *TypedStore[*BAPIValidator]
	delegations *TypedStore[*BAPIDelegation]
	unbondings  *TypedStore[*BAPIUnbondingEntry]
	activeSet   *TypedStore[*BAPIActiveSetEntry]
	bootstrap   *TypedStore[*BAPIBootstrapInfo]
}

// NewBAPIValidatorStore creates a new validator store backed by blockberry's StateStore.
func NewBAPIValidatorStore(store statestore.StateStore) *BAPIValidatorStore {
	return &BAPIValidatorStore{
		validators:  NewTypedStore[*BAPIValidator](store, "validators/"),
		delegations: NewTypedStore[*BAPIDelegation](store, "delegations/"),
		unbondings:  NewTypedStore[*BAPIUnbondingEntry](store, "unbondings/"),
		activeSet:   NewTypedStore[*BAPIActiveSetEntry](store, "active_set/"),
		bootstrap:   NewTypedStore[*BAPIBootstrapInfo](store, "bootstrap/"),
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

// unbondingKey is the typed-store key for an unbonding entry. The
// format is `<paddedHeight>/<seq>` so lexical iteration order
// matches numeric maturity-height order — EndBlock can scan from
// the start and stop on the first entry whose height exceeds the
// current block height.
//
// 20 digits for height covers uint64.Max; 20 for seq is overkill
// but cheap and keeps the format uniform.
func unbondingKey(maturityHeight, seq uint64) string {
	return fmt.Sprintf("%020d/%020d", maturityHeight, seq)
}

// AddUnbondingEntry persists a new unbonding entry. Called by
// MsgUndelegate after the delegation record has been decremented;
// the entry remains in the queue until EndBlock at MaturityHeight
// dequeues it.
func (s *BAPIValidatorStore) AddUnbondingEntry(ctx context.Context, entry *BAPIUnbondingEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}
	if entry.Delegator == "" {
		return fmt.Errorf("delegator empty")
	}
	if entry.ValidatorPubKey == "" {
		return fmt.Errorf("validator_pub_key empty")
	}
	if entry.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return s.unbondings.Set(ctx, unbondingKey(entry.MaturityHeight, entry.Seq), entry)
}

// DeleteUnbondingEntry removes a single unbonding entry by its
// (maturity, seq) identifier. Used by EndBlock after the matured
// transfer is emitted.
func (s *BAPIValidatorStore) DeleteUnbondingEntry(ctx context.Context, maturityHeight, seq uint64) error {
	return s.unbondings.Delete(ctx, unbondingKey(maturityHeight, seq))
}

// IterateMaturedUnbondings walks every unbonding entry with
// MaturityHeight ≤ currentHeight, invoking fn for each. Return
// true from fn to stop early. Walks in ascending-height order
// because the key format is height-padded.
//
// Stops scanning as soon as it sees an entry beyond the current
// height, so the cost is O(matured + 1) rather than O(all).
func (s *BAPIValidatorStore) IterateMaturedUnbondings(currentHeight uint64, fn func(e *BAPIUnbondingEntry) bool) error {
	stopped := false
	return s.unbondings.IterateRelative(func(_ string, e *BAPIUnbondingEntry) bool {
		if stopped {
			return true
		}
		if e == nil {
			return false
		}
		if e.MaturityHeight > currentHeight {
			stopped = true
			return true
		}
		return fn(e)
	})
}

// GetActiveSet returns the validator set last persisted by an
// epoch-close EndBlock, sorted by hex pubkey for deterministic
// diffing. An empty result is legitimate — it means no epoch has
// closed yet (genesis-and-no-blocks chain).
//
// PLAN §7 Phase 2.3 / D6 (validator set refreshes per epoch).
func (s *BAPIValidatorStore) GetActiveSet() ([]*BAPIActiveSetEntry, error) {
	var out []*BAPIActiveSetEntry
	err := s.activeSet.IterateRelative(func(_ string, e *BAPIActiveSetEntry) bool {
		if e != nil {
			out = append(out, e)
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetActiveSetEntry persists one validator's row in the active set
// snapshot. Called by EndBlock at epoch-close after computing the
// new top-N.
func (s *BAPIValidatorStore) SetActiveSetEntry(ctx context.Context, entry *BAPIActiveSetEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}
	if len(entry.PubKey) == 0 {
		return fmt.Errorf("pubkey empty")
	}
	return s.activeSet.Set(ctx, hex.EncodeToString(entry.PubKey), entry)
}

// DeleteActiveSetEntry removes a validator from the active set
// snapshot. Used when an epoch boundary evicts a validator that was
// in the previous active set.
func (s *BAPIValidatorStore) DeleteActiveSetEntry(ctx context.Context, pubKey []byte) error {
	if len(pubKey) == 0 {
		return fmt.Errorf("pubkey empty")
	}
	return s.activeSet.Delete(ctx, hex.EncodeToString(pubKey))
}

// SetBootstrapInfo persists one bootstrap validator's vesting record.
// Called at InitGenesis for every TokenomicsGenesis.BootstrapValidators
// entry; called again by Phase 2.5's EndBlock to advance VestedAmount.
func (s *BAPIValidatorStore) SetBootstrapInfo(ctx context.Context, info *BAPIBootstrapInfo) error {
	if info == nil {
		return fmt.Errorf("info is nil")
	}
	if len(info.PubKey) == 0 {
		return fmt.Errorf("pubkey empty")
	}
	return s.bootstrap.Set(ctx, hex.EncodeToString(info.PubKey), info)
}

// GetBootstrapInfo returns the bootstrap-validator vesting record
// for `pubKey`, or nil + ErrNotFound when the validator was never a
// bootstrap validator.
func (s *BAPIValidatorStore) GetBootstrapInfo(ctx context.Context, pubKey []byte) (*BAPIBootstrapInfo, error) {
	if len(pubKey) == 0 {
		return nil, fmt.Errorf("pubkey empty")
	}
	return s.bootstrap.Get(ctx, hex.EncodeToString(pubKey))
}

// IterateBootstrapInfos walks every bootstrap validator's record,
// invoking fn for each. Used by Phase 2.5's EndBlock vesting loop.
func (s *BAPIValidatorStore) IterateBootstrapInfos(fn func(info *BAPIBootstrapInfo) bool) error {
	return s.bootstrap.IterateRelative(func(_ string, info *BAPIBootstrapInfo) bool {
		if info == nil {
			return false
		}
		return fn(info)
	})
}

// IterateDelegations walks every delegation row, invoking fn for
// each. Used by Phase 2.6's slashing pass to apply the proportional
// burn to each delegator. Return true from fn to stop early.
//
// Returns ErrIterationUnsupported if the underlying StateStore is
// not iterable. Iteration order is the underlying tree's ascending
// byte order over the "delegator/<hex pubkey>" keys.
func (s *BAPIValidatorStore) IterateDelegations(fn func(d *BAPIDelegation) bool) error {
	return s.delegations.IterateRelative(func(_ string, d *BAPIDelegation) bool {
		if d == nil {
			return false
		}
		return fn(d)
	})
}
