package staking

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// BAPIStakingModule provides validator and delegation management for BAPI-based applications.
// It implements runtime.BAPIModule, runtime.BAPIBlockProcessor, and runtime.BAPIGenesisInitializer.
type BAPIStakingModule struct {
	validatorStore *store.BAPIValidatorStore
	balanceStore   *store.BAPIBalanceStore
}

// NewBAPIStakingModule creates a new BAPI staking module with the given stores.
func NewBAPIStakingModule(validatorStore *store.BAPIValidatorStore, balanceStore *store.BAPIBalanceStore) (*BAPIStakingModule, error) {
	if validatorStore == nil {
		return nil, fmt.Errorf("validator store cannot be nil")
	}
	if balanceStore == nil {
		return nil, fmt.Errorf("balance store cannot be nil")
	}

	return &BAPIStakingModule{
		validatorStore: validatorStore,
		balanceStore:   balanceStore,
	}, nil
}

// Name returns the module's unique name.
func (m *BAPIStakingModule) Name() string {
	return ModuleName
}

// RegisterMsgHandlers returns message handlers keyed by message type.
func (m *BAPIStakingModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgCreateValidator: m.handleCreateValidator,
		TypeMsgDelegate:        m.handleDelegate,
		TypeMsgUndelegate:      m.handleUndelegate,
	}
}

// RegisterQueryHandlers returns query handlers keyed by query path.
func (m *BAPIStakingModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/staking/validator":   m.handleQueryValidator,
		"/staking/delegation":  m.handleQueryDelegation,
	}
}

// BeginBlock is called at the beginning of each block.
func (m *BAPIStakingModule) BeginBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	// No begin-block processing needed
	return nil, nil
}

// EndBlock is called at the end of each block.
// It returns validator updates for consensus.
func (m *BAPIStakingModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	// For now, return empty validator updates
	// In a full implementation, we would track power changes during the block
	// and return the accumulated updates here
	return nil, nil, nil
}

// InitGenesis initializes the module's state from genesis data.
func (m *BAPIStakingModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil // No genesis data for staking module is acceptable
	}

	var genesisState StakingGenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		return fmt.Errorf("unmarshal staking genesis: %w", err)
	}

	// Create all genesis validators
	for _, validator := range genesisState.Validators {
		pubKey, err := hex.DecodeString(validator.PubKey)
		if err != nil {
			return fmt.Errorf("invalid validator pubkey: %w", err)
		}

		bapiValidator := &store.BAPIValidator{
			PubKey: types.PublicKey{
				Type: types.KeyTypeEd25519,
				Data: pubKey,
			},
			Power:           validator.Power,
			Jailed:          false,
			Description:     validator.Description,
			Commission:      validator.Commission,
			TotalDelegation: 0,
		}

		if err := m.validatorStore.SetValidator(ctx, bapiValidator); err != nil {
			return fmt.Errorf("set genesis validator: %w", err)
		}
	}

	return nil
}

// ExportGenesis exports the module's state for genesis.
func (m *BAPIStakingModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	// For now, return empty state - full export would require iteration
	genesisState := StakingGenesisState{
		Validators: []GenesisValidator{},
	}
	return json.Marshal(genesisState)
}

// StakingGenesisState represents the staking module's genesis state.
type StakingGenesisState struct {
	Validators []GenesisValidator `json:"validators"`
}

// GenesisValidator represents a validator in genesis.
type GenesisValidator struct {
	PubKey      string `json:"pub_key"` // hex-encoded
	Power       uint64 `json:"power"`
	Description string `json:"description"`
	Commission  uint32 `json:"commission"` // basis points
}

// handleCreateValidator handles MsgCreateValidator.
func (m *BAPIStakingModule) handleCreateValidator(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	createMsg, ok := msg.(*MsgCreateValidator)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgCreateValidator, got %T", msg)
	}

	// Verify the delegator is the transaction signer
	if createMsg.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check if validator already exists
	exists, err := m.validatorStore.HasValidator(ctx, createMsg.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check validator existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("validator with public key %x already exists", createMsg.PubKey)
	}

	// Create validator
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: createMsg.PubKey,
		},
		Power:           uint64(createMsg.InitialPower),
		Jailed:          false,
		Description:     "",
		Commission:      uint32(createMsg.Commission),
		TotalDelegation: 0,
	}

	// Store validator
	if err := m.validatorStore.SetValidator(ctx, validator); err != nil {
		return nil, fmt.Errorf("failed to set validator: %w", err)
	}

	// Return event effect
	return []effects.Effect{
		effects.NewEventEffect("staking.validator_created", map[string][]byte{
			"delegator":  []byte(createMsg.Delegator),
			"pub_key":    []byte(hex.EncodeToString(createMsg.PubKey)),
			"power":      []byte(fmt.Sprintf("%d", createMsg.InitialPower)),
			"commission": []byte(fmt.Sprintf("%d", createMsg.Commission)),
			"height":     []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleDelegate handles MsgDelegate.
func (m *BAPIStakingModule) handleDelegate(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	delegateMsg, ok := msg.(*MsgDelegate)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgDelegate, got %T", msg)
	}

	// Verify the delegator is the transaction signer
	if delegateMsg.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check validator exists
	exists, err := m.validatorStore.HasValidator(ctx, delegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check validator: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: validator not found", ptypes.ErrNotFound)
	}

	// Check delegator has sufficient balance
	balance, err := m.balanceStore.GetAmount(ctx, string(delegateMsg.Delegator), delegateMsg.Amount.Denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	if balance < delegateMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient balance for delegation (have %d, need %d)",
			ptypes.ErrInsufficientFunds, balance, delegateMsg.Amount.Amount)
	}

	// Return effects: transfer tokens to staking pool and update delegation
	return []effects.Effect{
		// Transfer tokens from delegator to staking pool
		effects.TransferEffect{
			From:   delegateMsg.Delegator,
			To:     ptypes.AccountName("staking.pool"),
			Amount: ptypes.Coins{delegateMsg.Amount},
		},
		// Emit event
		effects.NewEventEffect("staking.delegated", map[string][]byte{
			"delegator": []byte(delegateMsg.Delegator),
			"validator": []byte(hex.EncodeToString(delegateMsg.Validator)),
			"amount":    []byte(fmt.Sprintf("%d", delegateMsg.Amount.Amount)),
			"denom":     []byte(delegateMsg.Amount.Denom),
			"height":    []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleUndelegate handles MsgUndelegate.
func (m *BAPIStakingModule) handleUndelegate(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	undelegateMsg, ok := msg.(*MsgUndelegate)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgUndelegate, got %T", msg)
	}

	// Verify the delegator is the transaction signer
	if undelegateMsg.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check delegation exists and is sufficient
	delegation, err := m.validatorStore.GetDelegation(ctx, string(undelegateMsg.Delegator), undelegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check delegation: %w", err)
	}
	if delegation.Amount == 0 {
		return nil, fmt.Errorf("%w: delegation not found", ptypes.ErrNotFound)
	}
	if delegation.Amount < undelegateMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient delegation (have %d, want %d)",
			ptypes.ErrInsufficientFunds, delegation.Amount, undelegateMsg.Amount.Amount)
	}

	// Return effects: transfer tokens back from staking pool
	return []effects.Effect{
		// Transfer tokens from staking pool back to delegator
		effects.TransferEffect{
			From:   ptypes.AccountName("staking.pool"),
			To:     undelegateMsg.Delegator,
			Amount: ptypes.Coins{undelegateMsg.Amount},
		},
		// Emit event
		effects.NewEventEffect("staking.undelegated", map[string][]byte{
			"delegator": []byte(undelegateMsg.Delegator),
			"validator": []byte(hex.EncodeToString(undelegateMsg.Validator)),
			"amount":    []byte(fmt.Sprintf("%d", undelegateMsg.Amount.Amount)),
			"denom":     []byte(undelegateMsg.Amount.Denom),
			"height":    []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleQueryValidator handles validator queries.
func (m *BAPIStakingModule) handleQueryValidator(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
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

	var validator *store.BAPIValidator

	// Get validator at specific height if requested
	if height > 0 {
		validator, err = m.validatorStore.GetValidatorAtHeight(ctx, pubKey, height)
	} else {
		validator, err = m.validatorStore.GetValidator(ctx, pubKey)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get validator: %w", err)
	}

	// Return JSON response
	response := map[string]interface{}{
		"pub_key":          hex.EncodeToString(validator.PubKey.Data),
		"power":            validator.Power,
		"jailed":           validator.Jailed,
		"description":      validator.Description,
		"commission":       validator.Commission,
		"total_delegation": validator.TotalDelegation,
	}

	return json.Marshal(response)
}

// handleQueryDelegation handles delegation queries.
func (m *BAPIStakingModule) handleQueryDelegation(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Expect format: "delegator/validator_hex"
	parts := splitOnce(string(data), '/')
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid query format: expected delegator/validator")
	}

	delegator := ptypes.AccountName(parts[0])
	if !delegator.IsValid() {
		return nil, fmt.Errorf("%w: invalid delegator account", ptypes.ErrInvalidAccount)
	}

	validator, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid validator public key: %w", err)
	}

	delegation, err := m.validatorStore.GetDelegation(ctx, string(delegator), validator)
	if err != nil {
		return nil, fmt.Errorf("failed to get delegation: %w", err)
	}

	// Return JSON response
	response := map[string]interface{}{
		"delegator":   delegation.Delegator,
		"validator":   delegation.ValidatorPubKey,
		"amount":      delegation.Amount,
	}

	return json.Marshal(response)
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule             = (*BAPIStakingModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIStakingModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIStakingModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIStakingModule)(nil)
)
