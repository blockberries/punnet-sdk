package capability

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// AccountCapability provides controlled access to account operations
type AccountCapability interface {
	// ModuleName returns the module this capability is scoped to
	ModuleName() string

	// GetAccount retrieves an account by name
	GetAccount(ctx context.Context, name types.AccountName) (*types.Account, error)

	// CreateAccount creates a new account with the given name and public key
	// Returns error if the account already exists
	CreateAccount(ctx context.Context, name types.AccountName, pubKey []byte) (*types.Account, error)

	// UpdateAccount updates an existing account
	// Returns error if the account does not exist
	UpdateAccount(ctx context.Context, account *types.Account) error

	// DeleteAccount removes an account
	DeleteAccount(ctx context.Context, name types.AccountName) error

	// HasAccount checks if an account exists
	HasAccount(ctx context.Context, name types.AccountName) (bool, error)

	// VerifyAuthorization verifies that an authorization meets the account's authority threshold
	// This enables hierarchical authorization with cycle detection
	VerifyAuthorization(ctx context.Context, account *types.Account, auth *types.Authorization, message []byte) error

	// IncrementNonce increments an account's nonce (for replay protection)
	IncrementNonce(ctx context.Context, name types.AccountName) error

	// GetNonce retrieves an account's current nonce
	GetNonce(ctx context.Context, name types.AccountName) (uint64, error)

	// IterateAccounts iterates over all accounts
	IterateAccounts(ctx context.Context, callback func(*types.Account) error) error
}

// accountCapability is the implementation of AccountCapability
type accountCapability struct {
	moduleName string
	store      *store.AccountStore
}

// ModuleName returns the module this capability is scoped to
func (ac *accountCapability) ModuleName() string {
	if ac == nil {
		return ""
	}
	return ac.moduleName
}

// GetAccount retrieves an account by name
func (ac *accountCapability) GetAccount(ctx context.Context, name types.AccountName) (*types.Account, error) {
	if ac == nil || ac.store == nil {
		return nil, ErrCapabilityNil
	}

	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	return ac.store.Get(ctx, name)
}

// CreateAccount creates a new account with the given name and public key
func (ac *accountCapability) CreateAccount(ctx context.Context, name types.AccountName, pubKey []byte) (*types.Account, error) {
	if ac == nil || ac.store == nil {
		return nil, ErrCapabilityNil
	}

	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if len(pubKey) == 0 {
		return nil, fmt.Errorf("public key cannot be empty")
	}

	// Check if account already exists
	exists, err := ac.store.Has(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to check account existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("account %s already exists", name)
	}

	// Create defensive copy of public key
	pubKeyCopy := make([]byte, len(pubKey))
	copy(pubKeyCopy, pubKey)

	// Create new account
	account := types.NewAccount(name, pubKeyCopy)

	// Store the account
	if err := ac.store.Set(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to store account: %w", err)
	}

	return account, nil
}

// UpdateAccount updates an existing account
func (ac *accountCapability) UpdateAccount(ctx context.Context, account *types.Account) error {
	if ac == nil || ac.store == nil {
		return ErrCapabilityNil
	}

	if account == nil {
		return fmt.Errorf("account cannot be nil")
	}

	if err := account.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid account: %w", err)
	}

	// Check if account exists
	exists, err := ac.store.Has(ctx, account.Name)
	if err != nil {
		return fmt.Errorf("failed to check account existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: account %s not found", types.ErrNotFound, account.Name)
	}

	// Update the account
	if err := ac.store.Set(ctx, account); err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	return nil
}

// DeleteAccount removes an account
func (ac *accountCapability) DeleteAccount(ctx context.Context, name types.AccountName) error {
	if ac == nil || ac.store == nil {
		return ErrCapabilityNil
	}

	if !name.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	return ac.store.Delete(ctx, name)
}

// HasAccount checks if an account exists
func (ac *accountCapability) HasAccount(ctx context.Context, name types.AccountName) (bool, error) {
	if ac == nil || ac.store == nil {
		return false, ErrCapabilityNil
	}

	if !name.IsValid() {
		return false, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	return ac.store.Has(ctx, name)
}

// VerifyAuthorization verifies that an authorization meets the account's authority threshold
func (ac *accountCapability) VerifyAuthorization(ctx context.Context, account *types.Account, auth *types.Authorization, message []byte) error {
	if ac == nil || ac.store == nil {
		return ErrCapabilityNil
	}

	if account == nil {
		return fmt.Errorf("account cannot be nil")
	}

	if auth == nil {
		return fmt.Errorf("authorization cannot be nil")
	}

	if message == nil {
		return fmt.Errorf("message cannot be nil")
	}

	// Create an account getter adapter that uses the store
	getter := &accountGetter{store: ac.store}

	// Use the account getter for recursive verification
	return auth.VerifyAuthorization(account, message, getter)
}

// IncrementNonce increments an account's nonce
func (ac *accountCapability) IncrementNonce(ctx context.Context, name types.AccountName) error {
	if ac == nil || ac.store == nil {
		return ErrCapabilityNil
	}

	if !name.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	// Get the account
	account, err := ac.store.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Increment nonce
	account.Nonce++

	// Update the account
	if err := ac.store.Set(ctx, account); err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	return nil
}

// GetNonce retrieves an account's current nonce
func (ac *accountCapability) GetNonce(ctx context.Context, name types.AccountName) (uint64, error) {
	if ac == nil || ac.store == nil {
		return 0, ErrCapabilityNil
	}

	if !name.IsValid() {
		return 0, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	account, err := ac.store.Get(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("failed to get account: %w", err)
	}

	return account.Nonce, nil
}

// IterateAccounts iterates over all accounts
func (ac *accountCapability) IterateAccounts(ctx context.Context, callback func(*types.Account) error) (err error) {
	if ac == nil || ac.store == nil {
		return ErrCapabilityNil
	}

	if callback == nil {
		return fmt.Errorf("callback cannot be nil")
	}

	iter, iterErr := ac.store.Iterator(ctx)
	if iterErr != nil {
		return fmt.Errorf("failed to create iterator: %w", iterErr)
	}
	defer func() {
		if closeErr := iter.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close iterator: %w", closeErr)
		}
	}()

	for iter.Valid() {
		account, valErr := iter.Value()
		if valErr != nil {
			return fmt.Errorf("failed to get value: %w", valErr)
		}

		if err := callback(account); err != nil {
			return err
		}

		if err := iter.Next(); err != nil {
			return fmt.Errorf("failed to advance iterator: %w", err)
		}
	}

	return nil
}

// Flush flushes pending changes to backing store
func (ac *accountCapability) Flush(ctx context.Context) error {
	if ac == nil || ac.store == nil {
		return ErrCapabilityNil
	}

	return ac.store.Flush(ctx)
}

// accountGetter implements types.AccountGetter interface for authorization verification
type accountGetter struct {
	store *store.AccountStore
}

// GetAccount retrieves an account by name (implements types.AccountGetter)
func (ag *accountGetter) GetAccount(name types.AccountName) (*types.Account, error) {
	if ag == nil || ag.store == nil {
		return nil, ErrCapabilityNil
	}

	// Use background context for recursive calls
	// This prevents context cancellation from affecting authorization verification
	return ag.store.Get(context.Background(), name)
}
