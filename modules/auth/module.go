package auth

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/module"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/types"
)

// Module name
const ModuleName = "auth"

// AuthModule provides account management and authorization
type AuthModule struct {
	accountCap capability.AccountCapability
}

// NewAuthModule creates a new auth module with the given capability
func NewAuthModule(accountCap capability.AccountCapability) (*AuthModule, error) {
	if accountCap == nil {
		return nil, fmt.Errorf("account capability cannot be nil")
	}

	return &AuthModule{
		accountCap: accountCap,
	}, nil
}

// CreateModule creates the auth module using the module builder
func CreateModule(accountCap capability.AccountCapability) (module.Module, error) {
	if accountCap == nil {
		return nil, fmt.Errorf("account capability cannot be nil")
	}

	authMod, err := NewAuthModule(accountCap)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth module: %w", err)
	}

	return module.NewModuleBuilder(ModuleName).
		WithMsgHandler(TypeMsgCreateAccount, authMod.handleCreateAccount).
		WithMsgHandler(TypeMsgUpdateAuthority, authMod.handleUpdateAuthority).
		WithMsgHandler(TypeMsgDeleteAccount, authMod.handleDeleteAccount).
		WithQueryHandler("/account", authMod.handleQueryAccount).
		WithQueryHandler("/nonce", authMod.handleQueryNonce).
		Build()
}

// handleCreateAccount handles MsgCreateAccount
func (m *AuthModule) handleCreateAccount(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.accountCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	createMsg, ok := msg.(*MsgCreateAccount)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgCreateAccount")
	}

	// Verify the account creating itself is the transaction signer
	if createMsg.Name != ctx.Account() {
		return nil, fmt.Errorf("account name must match transaction account")
	}

	// Check if account already exists
	exists, err := m.accountCap.HasAccount(ctx.Context(), createMsg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check account existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("account %s already exists", createMsg.Name)
	}

	// Create defensive copy of public key
	pubKeyCopy := make([]byte, len(createMsg.PubKey))
	copy(pubKeyCopy, createMsg.PubKey)

	// Create new account with provided authority
	account := &types.Account{
		Name:      createMsg.Name,
		Authority: createMsg.Authority,
		Nonce:     0,
		CreatedAt: ctx.BlockTime(),
		UpdatedAt: ctx.BlockTime(),
	}

	// Validate the account
	if err := account.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid account: %w", err)
	}

	// Return write effect for the account
	return []effects.Effect{
		effects.WriteEffect[*types.Account]{
			Store:    "account",
			StoreKey: []byte(createMsg.Name),
			Value:    account,
		},
		effects.NewEventEffect("account.created", map[string][]byte{
			"account": []byte(createMsg.Name),
			"height":  []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// handleUpdateAuthority handles MsgUpdateAuthority
func (m *AuthModule) handleUpdateAuthority(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.accountCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	updateMsg, ok := msg.(*MsgUpdateAuthority)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgUpdateAuthority")
	}

	// Verify the account being updated is the transaction signer
	if updateMsg.Name != ctx.Account() {
		return nil, fmt.Errorf("account name must match transaction account")
	}

	// Get the existing account
	account, err := m.accountCap.GetAccount(ctx.Context(), updateMsg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Update the authority
	account.Authority = updateMsg.NewAuthority
	account.UpdatedAt = ctx.BlockTime()

	// Validate the updated account
	if err := account.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid account: %w", err)
	}

	// Return write effect for the updated account
	return []effects.Effect{
		effects.WriteEffect[*types.Account]{
			Store:    "account",
			StoreKey: []byte(updateMsg.Name),
			Value:    account,
		},
		effects.NewEventEffect("account.authority_updated", map[string][]byte{
			"account": []byte(updateMsg.Name),
			"height":  []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// handleDeleteAccount handles MsgDeleteAccount
func (m *AuthModule) handleDeleteAccount(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.accountCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	deleteMsg, ok := msg.(*MsgDeleteAccount)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgDeleteAccount")
	}

	// Verify the account being deleted is the transaction signer
	if deleteMsg.Name != ctx.Account() {
		return nil, fmt.Errorf("account name must match transaction account")
	}

	// Check if account exists
	exists, err := m.accountCap.HasAccount(ctx.Context(), deleteMsg.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check account existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: account %s not found", types.ErrNotFound, deleteMsg.Name)
	}

	// Return delete effect for the account
	return []effects.Effect{
		effects.DeleteEffect[*types.Account]{
			Store:    "account",
			StoreKey: []byte(deleteMsg.Name),
		},
		effects.NewEventEffect("account.deleted", map[string][]byte{
			"account": []byte(deleteMsg.Name),
			"height":  []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// QueryAccountRequest is the request for account query
type QueryAccountRequest struct {
	Name types.AccountName `json:"name"`
}

// QueryAccountResponse is the response for account query
type QueryAccountResponse struct {
	Account *types.Account `json:"account"`
}

// handleQueryAccount handles account queries
func (m *AuthModule) handleQueryAccount(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.accountCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	// Parse request (in production, would use proper deserialization)
	var req QueryAccountRequest
	// TODO: Proper deserialization
	_ = req

	// For now, treat data as account name
	name := types.AccountName(data)
	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	account, err := m.accountCap.GetAccount(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// TODO: Proper serialization
	return []byte(fmt.Sprintf("%v", account)), nil
}

// QueryNonceRequest is the request for nonce query
type QueryNonceRequest struct {
	Name types.AccountName `json:"name"`
}

// QueryNonceResponse is the response for nonce query
type QueryNonceResponse struct {
	Nonce uint64 `json:"nonce"`
}

// handleQueryNonce handles nonce queries
func (m *AuthModule) handleQueryNonce(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.accountCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	// For now, treat data as account name
	name := types.AccountName(data)
	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	nonce, err := m.accountCap.GetNonce(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	// TODO: Proper serialization
	return []byte(fmt.Sprintf("%d", nonce)), nil
}
