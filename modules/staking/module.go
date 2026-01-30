package staking

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/module"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// Module name
const ModuleName = "staking"

// StakingModule provides validator and delegation management
type StakingModule struct {
	validatorCap capability.ValidatorCapability
	balanceCap   capability.BalanceCapability
}

// NewStakingModule creates a new staking module with the given capabilities
func NewStakingModule(validatorCap capability.ValidatorCapability, balanceCap capability.BalanceCapability) (*StakingModule, error) {
	if validatorCap == nil {
		return nil, fmt.Errorf("validator capability cannot be nil")
	}
	if balanceCap == nil {
		return nil, fmt.Errorf("balance capability cannot be nil")
	}

	return &StakingModule{
		validatorCap: validatorCap,
		balanceCap:   balanceCap,
	}, nil
}

// CreateModule creates the staking module using the module builder
func CreateModule(validatorCap capability.ValidatorCapability, balanceCap capability.BalanceCapability) (module.Module, error) {
	if validatorCap == nil {
		return nil, fmt.Errorf("validator capability cannot be nil")
	}
	if balanceCap == nil {
		return nil, fmt.Errorf("balance capability cannot be nil")
	}

	stakingMod, err := NewStakingModule(validatorCap, balanceCap)
	if err != nil {
		return nil, fmt.Errorf("failed to create staking module: %w", err)
	}

	return module.NewModuleBuilder(ModuleName).
		WithDependency("bank"). // Staking depends on bank for token operations
		WithMsgHandler(TypeMsgCreateValidator, stakingMod.handleCreateValidator).
		WithMsgHandler(TypeMsgDelegate, stakingMod.handleDelegate).
		WithMsgHandler(TypeMsgUndelegate, stakingMod.handleUndelegate).
		WithQueryHandler("/validator", stakingMod.handleQueryValidator).
		WithQueryHandler("/validators", stakingMod.handleQueryValidators).
		WithQueryHandler("/delegation", stakingMod.handleQueryDelegation).
		Build()
}

// handleCreateValidator handles MsgCreateValidator
func (m *StakingModule) handleCreateValidator(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	createMsg, ok := msg.(*MsgCreateValidator)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgCreateValidator")
	}

	// Verify the delegator is the transaction signer
	if createMsg.Delegator != ctx.Account() {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check if validator already exists
	exists, err := m.validatorCap.HasValidator(ctx.Context(), createMsg.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check validator existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("validator with public key %x already exists", createMsg.PubKey)
	}

	// Create validator
	validator := store.NewValidator(createMsg.PubKey, createMsg.InitialPower, createMsg.Delegator)
	validator.Commission = createMsg.Commission
	validator.Active = true

	// Return write effect for the validator
	return []effects.Effect{
		effects.WriteEffect[store.Validator]{
			Store:    "validator",
			StoreKey: createMsg.PubKey,
			Value:    validator,
		},
		effects.NewEventEffect("staking.validator_created", map[string][]byte{
			"delegator":  []byte(createMsg.Delegator),
			"pub_key":    []byte(hex.EncodeToString(createMsg.PubKey)),
			"power":      []byte(fmt.Sprintf("%d", createMsg.InitialPower)),
			"commission": []byte(fmt.Sprintf("%d", createMsg.Commission)),
			"height":     []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// handleDelegate handles MsgDelegate
func (m *StakingModule) handleDelegate(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorCap == nil || m.balanceCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	delegateMsg, ok := msg.(*MsgDelegate)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgDelegate")
	}

	// Verify the delegator is the transaction signer
	if delegateMsg.Delegator != ctx.Account() {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check validator exists
	exists, err := m.validatorCap.HasValidator(ctx.Context(), delegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check validator: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: validator not found", types.ErrNotFound)
	}

	// Check delegator has sufficient balance
	balance, err := m.balanceCap.GetBalance(ctx.Context(), delegateMsg.Delegator, delegateMsg.Amount.Denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	if balance < delegateMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient balance for delegation", types.ErrInsufficientFunds)
	}

	// Get or create delegation
	var delegation store.Delegation
	hasDelegation, err := m.validatorCap.HasDelegation(ctx.Context(), delegateMsg.Delegator, delegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check delegation: %w", err)
	}

	if hasDelegation {
		delegation, err = m.validatorCap.GetDelegation(ctx.Context(), delegateMsg.Delegator, delegateMsg.Validator)
		if err != nil {
			return nil, fmt.Errorf("failed to get delegation: %w", err)
		}
		// Add to existing shares
		delegation.Shares += delegateMsg.Amount.Amount
	} else {
		// Create new delegation
		delegation = store.NewDelegation(delegateMsg.Delegator, delegateMsg.Validator, delegateMsg.Amount.Amount)
	}

	// Return effects: subtract balance and update delegation
	return []effects.Effect{
		effects.WriteEffect[uint64]{
			Store:    "balance_sub",
			StoreKey: []byte(fmt.Sprintf("%s/%s", delegateMsg.Delegator, delegateMsg.Amount.Denom)),
			Value:    delegateMsg.Amount.Amount,
		},
		effects.WriteEffect[store.Delegation]{
			Store:    "delegation",
			StoreKey: store.DelegationKey(delegateMsg.Delegator, delegateMsg.Validator),
			Value:    delegation,
		},
		effects.NewEventEffect("staking.delegated", map[string][]byte{
			"delegator": []byte(delegateMsg.Delegator),
			"validator": []byte(hex.EncodeToString(delegateMsg.Validator)),
			"amount":    []byte(fmt.Sprintf("%d", delegateMsg.Amount.Amount)),
			"denom":     []byte(delegateMsg.Amount.Denom),
			"height":    []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// handleUndelegate handles MsgUndelegate
func (m *StakingModule) handleUndelegate(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorCap == nil || m.balanceCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	undelegateMsg, ok := msg.(*MsgUndelegate)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgUndelegate")
	}

	// Verify the delegator is the transaction signer
	if undelegateMsg.Delegator != ctx.Account() {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check delegation exists
	hasDelegation, err := m.validatorCap.HasDelegation(ctx.Context(), undelegateMsg.Delegator, undelegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check delegation: %w", err)
	}
	if !hasDelegation {
		return nil, fmt.Errorf("%w: delegation not found", types.ErrNotFound)
	}

	// Get delegation
	delegation, err := m.validatorCap.GetDelegation(ctx.Context(), undelegateMsg.Delegator, undelegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to get delegation: %w", err)
	}

	// Check sufficient shares
	if delegation.Shares < undelegateMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient delegation shares", types.ErrInsufficientFunds)
	}

	// Update or delete delegation
	var delegationEffect effects.Effect
	if delegation.Shares == undelegateMsg.Amount.Amount {
		// Delete delegation if all shares are removed
		delegationEffect = effects.DeleteEffect[store.Delegation]{
			Store:    "delegation",
			StoreKey: store.DelegationKey(undelegateMsg.Delegator, undelegateMsg.Validator),
		}
	} else {
		// Update delegation with reduced shares
		delegation.Shares -= undelegateMsg.Amount.Amount
		delegationEffect = effects.WriteEffect[store.Delegation]{
			Store:    "delegation",
			StoreKey: store.DelegationKey(undelegateMsg.Delegator, undelegateMsg.Validator),
			Value:    delegation,
		}
	}

	// Return effects: add balance back and update/delete delegation
	return []effects.Effect{
		effects.WriteEffect[uint64]{
			Store:    "balance_add",
			StoreKey: []byte(fmt.Sprintf("%s/%s", undelegateMsg.Delegator, undelegateMsg.Amount.Denom)),
			Value:    undelegateMsg.Amount.Amount,
		},
		delegationEffect,
		effects.NewEventEffect("staking.undelegated", map[string][]byte{
			"delegator": []byte(undelegateMsg.Delegator),
			"validator": []byte(hex.EncodeToString(undelegateMsg.Validator)),
			"amount":    []byte(fmt.Sprintf("%d", undelegateMsg.Amount.Amount)),
			"denom":     []byte(undelegateMsg.Amount.Denom),
			"height":    []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// handleQueryValidator handles validator queries
func (m *StakingModule) handleQueryValidator(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.validatorCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	// Treat data as hex-encoded public key
	pubKey, err := hex.DecodeString(string(data))
	if err != nil {
		// Try raw bytes
		pubKey = data
	}

	if len(pubKey) == 0 {
		return nil, fmt.Errorf("public key cannot be empty")
	}

	validator, err := m.validatorCap.GetValidator(ctx, pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get validator: %w", err)
	}

	// TODO: Proper serialization
	return []byte(fmt.Sprintf("power=%d,active=%v,commission=%d",
		validator.Power, validator.Active, validator.Commission)), nil
}

// handleQueryValidators handles all validators query
func (m *StakingModule) handleQueryValidators(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.validatorCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	validators, err := m.validatorCap.GetActiveValidators(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get validators: %w", err)
	}

	// TODO: Proper serialization
	return []byte(fmt.Sprintf("count=%d", len(validators))), nil
}

// handleQueryDelegation handles delegation queries
func (m *StakingModule) handleQueryDelegation(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.validatorCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	// TODO: Proper deserialization
	// For now, expect format: "delegator/validator_hex"
	parts := splitOnce(string(data), '/')
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid query format: expected delegator/validator")
	}

	delegator := types.AccountName(parts[0])
	if !delegator.IsValid() {
		return nil, fmt.Errorf("%w: invalid delegator account", types.ErrInvalidAccount)
	}

	validator, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid validator public key: %w", err)
	}

	delegation, err := m.validatorCap.GetDelegation(ctx, delegator, validator)
	if err != nil {
		return nil, fmt.Errorf("failed to get delegation: %w", err)
	}

	// TODO: Proper serialization
	return []byte(fmt.Sprintf("shares=%d", delegation.Shares)), nil
}

// splitOnce splits a string on the first occurrence of a separator
func splitOnce(s string, sep rune) []string {
	for i, c := range s {
		if c == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
