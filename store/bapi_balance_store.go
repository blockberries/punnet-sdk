package store

import (
	"context"
	"fmt"

	"github.com/blockberries/blockberry/pkg/statestore"
)

// BAPIBalance represents an account's balance for a specific denomination.
type BAPIBalance struct {
	Account string `cramberry:"1"`
	Denom   string `cramberry:"2"`
	Amount  uint64 `cramberry:"3"`
}

// BAPIBalanceStore provides typed access to balance data.
// Keys are formatted as "account/denom" for efficient range queries.
type BAPIBalanceStore struct {
	*TypedStore[*BAPIBalance]
}

// NewBAPIBalanceStore creates a new balance store backed by blockberry's StateStore.
func NewBAPIBalanceStore(store statestore.StateStore) *BAPIBalanceStore {
	return &BAPIBalanceStore{
		TypedStore: NewTypedStore[*BAPIBalance](store, "balances/"),
	}
}

// balanceKey creates the key for an account/denom pair.
func balanceKey(account, denom string) string {
	return account + "/" + denom
}

// Get retrieves a balance by account and denomination.
func (s *BAPIBalanceStore) Get(ctx context.Context, account, denom string) (*BAPIBalance, error) {
	if account == "" {
		return nil, fmt.Errorf("empty account")
	}
	if denom == "" {
		return nil, fmt.Errorf("empty denom")
	}
	balance, err := s.TypedStore.Get(ctx, balanceKey(account, denom))
	if err == ErrNotFound {
		// Return zero balance instead of error
		return &BAPIBalance{
			Account: account,
			Denom:   denom,
			Amount:  0,
		}, nil
	}
	return balance, err
}

// Set stores a balance.
func (s *BAPIBalanceStore) Set(ctx context.Context, account, denom string, amount uint64) error {
	if account == "" {
		return fmt.Errorf("empty account")
	}
	if denom == "" {
		return fmt.Errorf("empty denom")
	}

	balance := &BAPIBalance{
		Account: account,
		Denom:   denom,
		Amount:  amount,
	}
	return s.TypedStore.Set(ctx, balanceKey(account, denom), balance)
}

// Delete removes a balance.
func (s *BAPIBalanceStore) Delete(ctx context.Context, account, denom string) error {
	if account == "" {
		return fmt.Errorf("empty account")
	}
	if denom == "" {
		return fmt.Errorf("empty denom")
	}
	return s.TypedStore.Delete(ctx, balanceKey(account, denom))
}

// GetAmount retrieves just the amount for an account/denom pair.
func (s *BAPIBalanceStore) GetAmount(ctx context.Context, account, denom string) (uint64, error) {
	balance, err := s.Get(ctx, account, denom)
	if err != nil {
		return 0, err
	}
	return balance.Amount, nil
}

// AddAmount adds to a balance, returning the new amount.
// Returns an error if the addition would overflow.
func (s *BAPIBalanceStore) AddAmount(ctx context.Context, account, denom string, amount uint64) (uint64, error) {
	current, err := s.GetAmount(ctx, account, denom)
	if err != nil {
		return 0, err
	}

	// Check for overflow
	newAmount := current + amount
	if newAmount < current {
		return 0, fmt.Errorf("balance overflow: %d + %d", current, amount)
	}

	if err := s.Set(ctx, account, denom, newAmount); err != nil {
		return 0, err
	}
	return newAmount, nil
}

// SubAmount subtracts from a balance, returning the new amount.
// Returns an error if the subtraction would underflow.
func (s *BAPIBalanceStore) SubAmount(ctx context.Context, account, denom string, amount uint64) (uint64, error) {
	current, err := s.GetAmount(ctx, account, denom)
	if err != nil {
		return 0, err
	}

	// Check for underflow
	if amount > current {
		return 0, fmt.Errorf("insufficient balance: have %d, need %d", current, amount)
	}

	newAmount := current - amount
	if err := s.Set(ctx, account, denom, newAmount); err != nil {
		return 0, err
	}
	return newAmount, nil
}

// Transfer moves tokens from one account to another.
// Returns an error if the sender has insufficient balance or if any operation would overflow.
func (s *BAPIBalanceStore) Transfer(ctx context.Context, from, to, denom string, amount uint64) error {
	if amount == 0 {
		return nil // No-op for zero transfers
	}

	// Subtract from sender
	_, err := s.SubAmount(ctx, from, denom, amount)
	if err != nil {
		return fmt.Errorf("debit %s: %w", from, err)
	}

	// Add to recipient
	_, err = s.AddAmount(ctx, to, denom, amount)
	if err != nil {
		// Note: In a real implementation, we'd want to rollback the debit
		// For now, effects handle atomicity
		return fmt.Errorf("credit %s: %w", to, err)
	}

	return nil
}

// GetWithProof retrieves a balance with a Merkle proof.
func (s *BAPIBalanceStore) GetWithProof(ctx context.Context, account, denom string) (*BAPIBalance, *statestore.Proof, error) {
	if account == "" {
		return nil, nil, fmt.Errorf("empty account")
	}
	if denom == "" {
		return nil, nil, fmt.Errorf("empty denom")
	}
	return s.TypedStore.GetWithProof(ctx, balanceKey(account, denom))
}

// GetAtHeight retrieves a balance at a specific block height.
func (s *BAPIBalanceStore) GetAtHeight(ctx context.Context, account, denom string, height int64) (*BAPIBalance, error) {
	if account == "" {
		return nil, fmt.Errorf("empty account")
	}
	if denom == "" {
		return nil, fmt.Errorf("empty denom")
	}
	return s.TypedStore.GetAtHeight(ctx, balanceKey(account, denom), height)
}
