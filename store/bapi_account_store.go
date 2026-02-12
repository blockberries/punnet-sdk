package store

import (
	"context"
	"fmt"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/types"
)

// BAPIAccountStore provides typed access to account data.
// It wraps blockberry's StateStore directly.
type BAPIAccountStore struct {
	*TypedStore[*types.Account]
}

// NewBAPIAccountStore creates a new account store backed by blockberry's StateStore.
func NewBAPIAccountStore(store statestore.StateStore) *BAPIAccountStore {
	return &BAPIAccountStore{
		TypedStore: NewTypedStore[*types.Account](store, "accounts/"),
	}
}

// Get retrieves an account by name.
func (s *BAPIAccountStore) Get(ctx context.Context, name types.AccountName) (*types.Account, error) {
	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, name)
	}
	return s.TypedStore.Get(ctx, string(name))
}

// Set stores an account.
func (s *BAPIAccountStore) Set(ctx context.Context, account *types.Account) error {
	if account == nil {
		return ErrInvalidValue
	}
	if err := account.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid account: %w", err)
	}
	return s.TypedStore.Set(ctx, string(account.Name), account)
}

// Delete removes an account by name.
func (s *BAPIAccountStore) Delete(ctx context.Context, name types.AccountName) error {
	if !name.IsValid() {
		return fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, name)
	}
	return s.TypedStore.Delete(ctx, string(name))
}

// Has checks if an account exists.
func (s *BAPIAccountStore) Has(ctx context.Context, name types.AccountName) (bool, error) {
	if !name.IsValid() {
		return false, fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, name)
	}
	return s.TypedStore.Has(ctx, string(name))
}

// GetWithProof retrieves an account with a Merkle proof.
func (s *BAPIAccountStore) GetWithProof(ctx context.Context, name types.AccountName) (*types.Account, *statestore.Proof, error) {
	if !name.IsValid() {
		return nil, nil, fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, name)
	}
	return s.TypedStore.GetWithProof(ctx, string(name))
}

// GetAtHeight retrieves an account at a specific block height.
func (s *BAPIAccountStore) GetAtHeight(ctx context.Context, name types.AccountName, height int64) (*types.Account, error) {
	if !name.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, name)
	}
	return s.TypedStore.GetAtHeight(ctx, string(name), height)
}

// IncrementNonce atomically increments the account's nonce.
func (s *BAPIAccountStore) IncrementNonce(ctx context.Context, name types.AccountName) error {
	account, err := s.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}

	account.Nonce++

	return s.Set(ctx, account)
}

// GetNonce returns the current nonce for an account.
func (s *BAPIAccountStore) GetNonce(ctx context.Context, name types.AccountName) (uint64, error) {
	account, err := s.Get(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("get account: %w", err)
	}
	return account.Nonce, nil
}
