package capability

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// BalanceCapability provides controlled access to balance operations
type BalanceCapability interface {
	// ModuleName returns the module this capability is scoped to
	ModuleName() string

	// GetBalance retrieves a balance by account and denomination
	// Returns zero balance if not found
	GetBalance(ctx context.Context, account types.AccountName, denom string) (uint64, error)

	// SetBalance sets a balance for an account and denomination
	SetBalance(ctx context.Context, account types.AccountName, denom string, amount uint64) error

	// AddBalance adds to an account's balance
	// Returns error if overflow would occur
	AddBalance(ctx context.Context, account types.AccountName, denom string, amount uint64) error

	// SubBalance subtracts from an account's balance
	// Returns error if insufficient funds
	SubBalance(ctx context.Context, account types.AccountName, denom string, amount uint64) error

	// Transfer transfers tokens from one account to another
	// This operation is atomic with automatic rollback on error
	Transfer(ctx context.Context, from, to types.AccountName, denom string, amount uint64) error

	// GetAccountBalances retrieves all balances for an account
	GetAccountBalances(ctx context.Context, account types.AccountName) (types.Coins, error)

	// HasBalance checks if a balance exists for an account and denomination
	HasBalance(ctx context.Context, account types.AccountName, denom string) (bool, error)

	// IterateBalances iterates over all balances
	IterateBalances(ctx context.Context, callback func(store.Balance) error) error

	// IterateAccountBalances iterates over all balances for a specific account
	IterateAccountBalances(ctx context.Context, account types.AccountName, callback func(store.Balance) error) error
}

// balanceCapability is the implementation of BalanceCapability
type balanceCapability struct {
	moduleName string
	store      *store.BalanceStore
}

// ModuleName returns the module this capability is scoped to
func (bc *balanceCapability) ModuleName() string {
	if bc == nil {
		return ""
	}
	return bc.moduleName
}

// GetBalance retrieves a balance by account and denomination
func (bc *balanceCapability) GetBalance(ctx context.Context, account types.AccountName, denom string) (uint64, error) {
	if bc == nil || bc.store == nil {
		return 0, ErrCapabilityNil
	}

	if !account.IsValid() {
		return 0, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return 0, fmt.Errorf("denomination cannot be empty")
	}

	balance, err := bc.store.Get(ctx, account, denom)
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	return balance.Amount, nil
}

// SetBalance sets a balance for an account and denomination
func (bc *balanceCapability) SetBalance(ctx context.Context, account types.AccountName, denom string, amount uint64) error {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	if !account.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return fmt.Errorf("denomination cannot be empty")
	}

	balance := store.NewBalance(account, denom, amount)
	if err := bc.store.Set(ctx, balance); err != nil {
		return fmt.Errorf("failed to set balance: %w", err)
	}

	return nil
}

// AddBalance adds to an account's balance
func (bc *balanceCapability) AddBalance(ctx context.Context, account types.AccountName, denom string, amount uint64) error {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	if !account.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return fmt.Errorf("denomination cannot be empty")
	}

	if amount == 0 {
		return nil
	}

	if err := bc.store.AddAmount(ctx, account, denom, amount); err != nil {
		return fmt.Errorf("failed to add balance: %w", err)
	}

	return nil
}

// SubBalance subtracts from an account's balance
func (bc *balanceCapability) SubBalance(ctx context.Context, account types.AccountName, denom string, amount uint64) error {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	if !account.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return fmt.Errorf("denomination cannot be empty")
	}

	if amount == 0 {
		return nil
	}

	if err := bc.store.SubAmount(ctx, account, denom, amount); err != nil {
		return fmt.Errorf("failed to subtract balance: %w", err)
	}

	return nil
}

// Transfer transfers tokens from one account to another
func (bc *balanceCapability) Transfer(ctx context.Context, from, to types.AccountName, denom string, amount uint64) error {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	if !from.IsValid() {
		return fmt.Errorf("%w: invalid sender account name", types.ErrInvalidAccount)
	}

	if !to.IsValid() {
		return fmt.Errorf("%w: invalid receiver account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return fmt.Errorf("denomination cannot be empty")
	}

	if amount == 0 {
		return nil
	}

	if from == to {
		return fmt.Errorf("cannot transfer to self")
	}

	if err := bc.store.Transfer(ctx, from, to, denom, amount); err != nil {
		return fmt.Errorf("failed to transfer: %w", err)
	}

	return nil
}

// GetAccountBalances retrieves all balances for an account
func (bc *balanceCapability) GetAccountBalances(ctx context.Context, account types.AccountName) (types.Coins, error) {
	if bc == nil || bc.store == nil {
		return nil, ErrCapabilityNil
	}

	if !account.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	coins, err := bc.store.GetAccountBalances(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get account balances: %w", err)
	}

	// Return defensive copy
	result := make(types.Coins, len(coins))
	copy(result, coins)
	return result, nil
}

// HasBalance checks if a balance exists for an account and denomination
func (bc *balanceCapability) HasBalance(ctx context.Context, account types.AccountName, denom string) (bool, error) {
	if bc == nil || bc.store == nil {
		return false, ErrCapabilityNil
	}

	if !account.IsValid() {
		return false, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return false, fmt.Errorf("denomination cannot be empty")
	}

	return bc.store.Has(ctx, account, denom)
}

// IterateBalances iterates over all balances
func (bc *balanceCapability) IterateBalances(ctx context.Context, callback func(store.Balance) error) (err error) {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	if callback == nil {
		return fmt.Errorf("callback cannot be nil")
	}

	iter, iterErr := bc.store.Iterator(ctx)
	if iterErr != nil {
		return fmt.Errorf("failed to create iterator: %w", iterErr)
	}
	defer func() {
		if closeErr := iter.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close iterator: %w", closeErr)
		}
	}()

	for iter.Valid() {
		balance, valErr := iter.Value()
		if valErr != nil {
			return fmt.Errorf("failed to get value: %w", valErr)
		}

		if err := callback(balance); err != nil {
			return err
		}

		if err := iter.Next(); err != nil {
			return fmt.Errorf("failed to advance iterator: %w", err)
		}
	}

	return nil
}

// IterateAccountBalances iterates over all balances for a specific account
func (bc *balanceCapability) IterateAccountBalances(ctx context.Context, account types.AccountName, callback func(store.Balance) error) (err error) {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	if !account.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if callback == nil {
		return fmt.Errorf("callback cannot be nil")
	}

	iter, iterErr := bc.store.AccountIterator(ctx, account)
	if iterErr != nil {
		return fmt.Errorf("failed to create iterator: %w", iterErr)
	}
	defer func() {
		if closeErr := iter.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close iterator: %w", closeErr)
		}
	}()

	for iter.Valid() {
		balance, valErr := iter.Value()
		if valErr != nil {
			return fmt.Errorf("failed to get value: %w", valErr)
		}

		if err := callback(balance); err != nil {
			return err
		}

		if err := iter.Next(); err != nil {
			return fmt.Errorf("failed to advance iterator: %w", err)
		}
	}

	return nil
}

// Flush flushes pending changes to backing store
func (bc *balanceCapability) Flush(ctx context.Context) error {
	if bc == nil || bc.store == nil {
		return ErrCapabilityNil
	}

	return bc.store.Flush(ctx)
}
