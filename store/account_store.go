package store

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// AccountStore is a typed store for Account objects
type AccountStore struct {
	store ObjectStore[*types.Account]
}

// NewAccountStore creates a new account store
func NewAccountStore(backing BackingStore) *AccountStore {
	serializer := NewJSONSerializer[*types.Account]()
	store := NewCachedObjectStore(backing, serializer, 10000, 100000)

	return &AccountStore{
		store: store,
	}
}

// Get retrieves an account by name
func (as *AccountStore) Get(ctx context.Context, name types.AccountName) (*types.Account, error) {
	if as == nil || as.store == nil {
		return nil, ErrStoreNil
	}

	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	return as.store.Get(ctx, []byte(name))
}

// Set stores an account
func (as *AccountStore) Set(ctx context.Context, account *types.Account) error {
	if as == nil || as.store == nil {
		return ErrStoreNil
	}

	if account == nil {
		return ErrInvalidValue
	}

	if err := account.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid account: %w", err)
	}

	return as.store.Set(ctx, []byte(account.Name), account)
}

// Delete removes an account by name
func (as *AccountStore) Delete(ctx context.Context, name types.AccountName) error {
	if as == nil || as.store == nil {
		return ErrStoreNil
	}

	if !name.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	return as.store.Delete(ctx, []byte(name))
}

// Has checks if an account exists
func (as *AccountStore) Has(ctx context.Context, name types.AccountName) (bool, error) {
	if as == nil || as.store == nil {
		return false, ErrStoreNil
	}

	if !name.IsValid() {
		return false, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	return as.store.Has(ctx, []byte(name))
}

// Iterator returns an iterator over all accounts
func (as *AccountStore) Iterator(ctx context.Context) (Iterator[*types.Account], error) {
	if as == nil || as.store == nil {
		return nil, ErrStoreNil
	}

	return as.store.Iterator(ctx, nil, nil)
}

// GetBatch retrieves multiple accounts by names
func (as *AccountStore) GetBatch(ctx context.Context, names []types.AccountName) (map[types.AccountName]*types.Account, error) {
	if as == nil || as.store == nil {
		return nil, ErrStoreNil
	}

	if names == nil {
		return make(map[types.AccountName]*types.Account), nil
	}

	// Convert names to keys
	keys := make([][]byte, len(names))
	for i, name := range names {
		if !name.IsValid() {
			return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
		}
		keys[i] = []byte(name)
	}

	// Get from store
	results, err := as.store.GetBatch(ctx, keys)
	if err != nil {
		return nil, err
	}

	// Convert back to map with AccountName keys
	accounts := make(map[types.AccountName]*types.Account)
	for keyStr, account := range results {
		name := types.AccountName(keyStr)
		accounts[name] = account
	}

	return accounts, nil
}

// SetBatch stores multiple accounts atomically
func (as *AccountStore) SetBatch(ctx context.Context, accounts []*types.Account) error {
	if as == nil || as.store == nil {
		return ErrStoreNil
	}

	if accounts == nil {
		return nil
	}

	// Validate all accounts first
	for _, account := range accounts {
		if account == nil {
			return ErrInvalidValue
		}
		if err := account.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid account: %w", err)
		}
	}

	// Convert to map
	items := make(map[string]*types.Account)
	for _, account := range accounts {
		items[string(account.Name)] = account
	}

	return as.store.SetBatch(ctx, items)
}

// Flush writes any pending changes to the underlying storage
func (as *AccountStore) Flush(ctx context.Context) error {
	if as == nil || as.store == nil {
		return ErrStoreNil
	}

	return as.store.Flush(ctx)
}

// Close releases any resources held by the store
func (as *AccountStore) Close() error {
	if as == nil || as.store == nil {
		return ErrStoreNil
	}

	return as.store.Close()
}
