package store

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// Balance represents an account's balance for a specific denomination
type Balance struct {
	Account types.AccountName `json:"account"`
	Denom   string            `json:"denom"`
	Amount  uint64            `json:"amount"`
}

// NewBalance creates a new balance
func NewBalance(account types.AccountName, denom string, amount uint64) Balance {
	return Balance{
		Account: account,
		Denom:   denom,
		Amount:  amount,
	}
}

// IsValid checks if the balance is valid
func (b Balance) IsValid() bool {
	return b.Account.IsValid() && b.Denom != ""
}

// BalanceKey creates a unique key for a balance
// Format: account/denom
func BalanceKey(account types.AccountName, denom string) []byte {
	return []byte(fmt.Sprintf("%s/%s", account, denom))
}

// BalanceStore is a typed store for Balance objects
type BalanceStore struct {
	store ObjectStore[Balance]
}

// NewBalanceStore creates a new balance store
func NewBalanceStore(backing BackingStore) *BalanceStore {
	serializer := NewJSONSerializer[Balance]()
	store := NewCachedObjectStore(backing, serializer, 10000, 100000)

	return &BalanceStore{
		store: store,
	}
}

// Get retrieves a balance by account and denomination
func (bs *BalanceStore) Get(ctx context.Context, account types.AccountName, denom string) (Balance, error) {
	var zero Balance

	if bs == nil || bs.store == nil {
		return zero, ErrStoreNil
	}

	if !account.IsValid() {
		return zero, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return zero, fmt.Errorf("%w: empty denomination", ErrInvalidKey)
	}

	key := BalanceKey(account, denom)
	balance, err := bs.store.Get(ctx, key)
	if err != nil {
		// Return zero balance if not found
		if err == ErrNotFound {
			return NewBalance(account, denom, 0), nil
		}
		return zero, err
	}

	return balance, nil
}

// Set stores a balance
func (bs *BalanceStore) Set(ctx context.Context, balance Balance) error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	if !balance.IsValid() {
		return fmt.Errorf("%w: invalid balance", ErrInvalidValue)
	}

	key := BalanceKey(balance.Account, balance.Denom)
	return bs.store.Set(ctx, key, balance)
}

// Delete removes a balance
func (bs *BalanceStore) Delete(ctx context.Context, account types.AccountName, denom string) error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	if !account.IsValid() {
		return fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return fmt.Errorf("%w: empty denomination", ErrInvalidKey)
	}

	key := BalanceKey(account, denom)
	return bs.store.Delete(ctx, key)
}

// Has checks if a balance exists
func (bs *BalanceStore) Has(ctx context.Context, account types.AccountName, denom string) (bool, error) {
	if bs == nil || bs.store == nil {
		return false, ErrStoreNil
	}

	if !account.IsValid() {
		return false, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return false, fmt.Errorf("%w: empty denomination", ErrInvalidKey)
	}

	key := BalanceKey(account, denom)
	return bs.store.Has(ctx, key)
}

// Iterator returns an iterator over all balances
func (bs *BalanceStore) Iterator(ctx context.Context) (Iterator[Balance], error) {
	if bs == nil || bs.store == nil {
		return nil, ErrStoreNil
	}

	return bs.store.Iterator(ctx, nil, nil)
}

// AccountIterator returns an iterator over all balances for a specific account
func (bs *BalanceStore) AccountIterator(ctx context.Context, account types.AccountName) (Iterator[Balance], error) {
	if bs == nil || bs.store == nil {
		return nil, ErrStoreNil
	}

	if !account.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	// Create prefix for account
	prefix := []byte(fmt.Sprintf("%s/", account))

	// Calculate end boundary - handles 0xFF overflow correctly
	end := prefixBound(prefix)

	return bs.store.Iterator(ctx, prefix, end)
}

// GetAccountBalances retrieves all balances for an account
func (bs *BalanceStore) GetAccountBalances(ctx context.Context, account types.AccountName) (types.Coins, error) {
	if bs == nil || bs.store == nil {
		return nil, ErrStoreNil
	}

	if !account.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	iter, err := bs.AccountIterator(ctx, account)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	coins := make(types.Coins, 0)
	for iter.Valid() {
		balance, err := iter.Value()
		if err != nil {
			return nil, err
		}

		if balance.Amount > 0 {
			coins = append(coins, types.Coin{
				Denom:  balance.Denom,
				Amount: balance.Amount,
			})
		}

		if err := iter.Next(); err != nil {
			return nil, err
		}
	}

	return coins, nil
}

// AddAmount adds to an account's balance
func (bs *BalanceStore) AddAmount(ctx context.Context, account types.AccountName, denom string, amount uint64) error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	if amount == 0 {
		return nil
	}

	balance, err := bs.Get(ctx, account, denom)
	if err != nil {
		return err
	}

	// Check for overflow
	if balance.Amount > ^uint64(0)-amount {
		return fmt.Errorf("balance overflow")
	}

	balance.Amount += amount
	return bs.Set(ctx, balance)
}

// SubAmount subtracts from an account's balance
func (bs *BalanceStore) SubAmount(ctx context.Context, account types.AccountName, denom string, amount uint64) error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	if amount == 0 {
		return nil
	}

	balance, err := bs.Get(ctx, account, denom)
	if err != nil {
		return err
	}

	if balance.Amount < amount {
		return types.ErrInsufficientFunds
	}

	balance.Amount -= amount
	return bs.Set(ctx, balance)
}

// Transfer transfers amount from one account to another
// NOTE: This is not fully atomic. In concurrent scenarios, interleaved transfers
// on the same accounts can cause lost updates. The runtime layer must ensure
// that conflicting transfers are serialized via the effect system's dependency graph.
func (bs *BalanceStore) Transfer(ctx context.Context, from, to types.AccountName, denom string, amount uint64) error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	if amount == 0 {
		return nil
	}

	// Validate sender has sufficient balance before making changes
	fromBalance, err := bs.Get(ctx, from, denom)
	if err != nil {
		return fmt.Errorf("failed to get sender balance: %w", err)
	}
	if fromBalance.Amount < amount {
		return fmt.Errorf("insufficient balance: has %d, needs %d", fromBalance.Amount, amount)
	}

	// Subtract from sender
	if err := bs.SubAmount(ctx, from, denom, amount); err != nil {
		return fmt.Errorf("failed to subtract from sender: %w", err)
	}

	// Add to receiver
	if err := bs.AddAmount(ctx, to, denom, amount); err != nil {
		// Attempt to rollback sender subtraction
		if rollbackErr := bs.AddAmount(ctx, from, denom, amount); rollbackErr != nil {
			// Rollback failed - this is a critical state inconsistency
			return fmt.Errorf("transfer failed and rollback failed: original error: %w, rollback error: %v", err, rollbackErr)
		}
		return fmt.Errorf("failed to add to receiver (rolled back): %w", err)
	}

	return nil
}

// Flush writes any pending changes to the underlying storage
func (bs *BalanceStore) Flush(ctx context.Context) error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	return bs.store.Flush(ctx)
}

// Close releases any resources held by the store
func (bs *BalanceStore) Close() error {
	if bs == nil || bs.store == nil {
		return ErrStoreNil
	}

	return bs.store.Close()
}
