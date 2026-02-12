package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// BAPIAuthModule provides account management for BAPI-based applications.
// It implements runtime.BAPIModule and runtime.BAPIGenesisInitializer.
type BAPIAuthModule struct {
	accountStore *store.BAPIAccountStore
}

// NewBAPIAuthModule creates a new BAPI auth module with the given account store.
func NewBAPIAuthModule(accountStore *store.BAPIAccountStore) (*BAPIAuthModule, error) {
	if accountStore == nil {
		return nil, fmt.Errorf("account store cannot be nil")
	}

	return &BAPIAuthModule{
		accountStore: accountStore,
	}, nil
}

// Name returns the module's unique name.
func (m *BAPIAuthModule) Name() string {
	return ModuleName
}

// RegisterMsgHandlers returns message handlers keyed by message type.
func (m *BAPIAuthModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgCreateAccount:   m.handleCreateAccount,
		TypeMsgUpdateAuthority: m.handleUpdateAuthority,
		TypeMsgDeleteAccount:   m.handleDeleteAccount,
	}
}

// RegisterQueryHandlers returns query handlers keyed by query path.
func (m *BAPIAuthModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/auth/account": m.handleQueryAccount,
		"/auth/nonce":   m.handleQueryNonce,
	}
}

// InitGenesis initializes the module's state from genesis data.
func (m *BAPIAuthModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil // No genesis data for auth module is acceptable
	}

	var genesisState AuthGenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		return fmt.Errorf("unmarshal auth genesis: %w", err)
	}

	// Create all genesis accounts
	for _, account := range genesisState.Accounts {
		if err := account.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid genesis account %s: %w", account.Name, err)
		}
		if err := m.accountStore.Set(ctx, account); err != nil {
			return fmt.Errorf("set genesis account %s: %w", account.Name, err)
		}
	}

	return nil
}

// ExportGenesis exports the module's state for genesis.
func (m *BAPIAuthModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	// For now, return empty state - full export would require iteration
	genesisState := AuthGenesisState{
		Accounts: []*types.Account{},
	}
	return json.Marshal(genesisState)
}

// AuthGenesisState represents the auth module's genesis state.
type AuthGenesisState struct {
	Accounts []*types.Account `json:"accounts"`
}

// handleCreateAccount handles MsgCreateAccount.
func (m *BAPIAuthModule) handleCreateAccount(ctx context.Context, txCtx *runtime.BAPITxContext, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.accountStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	createMsg, ok := msg.(*MsgCreateAccount)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgCreateAccount, got %T", msg)
	}

	// Verify the account creating itself is the transaction signer
	if createMsg.Name != txCtx.Account {
		return nil, fmt.Errorf("account name must match transaction account")
	}

	// Check if account already exists
	exists, err := m.accountStore.Has(ctx, createMsg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check account existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("account %s already exists", createMsg.Name)
	}

	// Create defensive copy of public key
	pubKeyCopy := make([]byte, len(createMsg.PubKey))
	copy(pubKeyCopy, createMsg.PubKey)

	// Get block time from context
	blockTime := time.Now() // Default
	if txCtx.BAPIBlockContext != nil && (txCtx.BAPIBlockContext.Time.Seconds != 0 || txCtx.BAPIBlockContext.Time.Nanos != 0) {
		blockTime = txCtx.BAPIBlockContext.Time.ToTime()
	}

	// Create new account with provided authority
	account := &types.Account{
		Name:      createMsg.Name,
		Authority: createMsg.Authority,
		Nonce:     0,
		CreatedAt: blockTime,
		UpdatedAt: blockTime,
	}

	// Validate the account
	if err := account.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid account: %w", err)
	}

	// Return write effect for the account
	return []effects.Effect{
		effects.WriteEffect[*types.Account]{
			Store:    "accounts",
			StoreKey: []byte(createMsg.Name),
			Value:    account,
		},
		effects.NewEventEffect("account.created", map[string][]byte{
			"account": []byte(createMsg.Name),
			"height":  []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleUpdateAuthority handles MsgUpdateAuthority.
func (m *BAPIAuthModule) handleUpdateAuthority(ctx context.Context, txCtx *runtime.BAPITxContext, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.accountStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	updateMsg, ok := msg.(*MsgUpdateAuthority)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgUpdateAuthority, got %T", msg)
	}

	// Verify the account being updated is the transaction signer
	if updateMsg.Name != txCtx.Account {
		return nil, fmt.Errorf("account name must match transaction account")
	}

	// Get the existing account
	account, err := m.accountStore.Get(ctx, updateMsg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Get block time from context
	blockTime := time.Now() // Default
	if txCtx.BAPIBlockContext != nil && (txCtx.BAPIBlockContext.Time.Seconds != 0 || txCtx.BAPIBlockContext.Time.Nanos != 0) {
		blockTime = txCtx.BAPIBlockContext.Time.ToTime()
	}

	// Update the authority
	account.Authority = updateMsg.NewAuthority
	account.UpdatedAt = blockTime

	// Validate the updated account
	if err := account.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid account: %w", err)
	}

	// Return write effect for the updated account
	return []effects.Effect{
		effects.WriteEffect[*types.Account]{
			Store:    "accounts",
			StoreKey: []byte(updateMsg.Name),
			Value:    account,
		},
		effects.NewEventEffect("account.authority_updated", map[string][]byte{
			"account": []byte(updateMsg.Name),
			"height":  []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleDeleteAccount handles MsgDeleteAccount.
func (m *BAPIAuthModule) handleDeleteAccount(ctx context.Context, txCtx *runtime.BAPITxContext, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.accountStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	deleteMsg, ok := msg.(*MsgDeleteAccount)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgDeleteAccount, got %T", msg)
	}

	// Verify the account being deleted is the transaction signer
	if deleteMsg.Name != txCtx.Account {
		return nil, fmt.Errorf("account name must match transaction account")
	}

	// Check if account exists
	exists, err := m.accountStore.Has(ctx, deleteMsg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check account existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: account %s not found", types.ErrNotFound, deleteMsg.Name)
	}

	// Return delete effect for the account
	return []effects.Effect{
		effects.DeleteEffect[*types.Account]{
			Store:    "accounts",
			StoreKey: []byte(deleteMsg.Name),
		},
		effects.NewEventEffect("account.deleted", map[string][]byte{
			"account": []byte(deleteMsg.Name),
			"height":  []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleQueryAccount handles account queries.
func (m *BAPIAuthModule) handleQueryAccount(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.accountStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Parse account name from data
	name := types.AccountName(data)
	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	var account *types.Account
	var err error

	// Get account at specific height if requested
	if height > 0 {
		account, err = m.accountStore.GetAtHeight(ctx, name, height)
	} else {
		account, err = m.accountStore.Get(ctx, name)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Serialize response using JSON for now
	return json.Marshal(account)
}

// handleQueryNonce handles nonce queries.
func (m *BAPIAuthModule) handleQueryNonce(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.accountStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Parse account name from data
	name := types.AccountName(data)
	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	var nonce uint64

	// Get account at specific height if requested
	if height > 0 {
		account, err := m.accountStore.GetAtHeight(ctx, name, height)
		if err != nil {
			return nil, fmt.Errorf("failed to get account: %w", err)
		}
		nonce = account.Nonce
	} else {
		var err error
		nonce, err = m.accountStore.GetNonce(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("failed to get nonce: %w", err)
		}
	}

	// Return nonce as JSON
	return json.Marshal(nonce)
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule           = (*BAPIAuthModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIAuthModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIAuthModule)(nil)
)
