package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

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
//
// The store also maintains an in-memory key index (PLAN B2-4) so callers can
// enumerate balances without iterating the underlying merkle store. The
// index is populated by Set/Delete and by the effect executor
// (NotifyBalanceWrite); on the first TrackedKeys call after process
// startup it is also rebuilt from the underlying StateStore's Iterate
// hook if available (closes the post-restart empty-list gap).
type BAPIBalanceStore struct {
	*TypedStore[*BAPIBalance]

	mu      sync.RWMutex
	keySet  map[string]struct{}
	rebuilt bool
}

// NewBAPIBalanceStore creates a new balance store backed by blockberry's StateStore.
func NewBAPIBalanceStore(store statestore.StateStore) *BAPIBalanceStore {
	return &BAPIBalanceStore{
		TypedStore: NewTypedStore[*BAPIBalance](store, "balances/"),
		keySet:     make(map[string]struct{}),
	}
}

// TrackKey records `key` (relative to the store's prefix, e.g. "alice/stake")
// in the in-memory index. Called by Set and by the effect executor (via
// BAPIStoreProvider.NotifyBalanceWrite) so writes that bypass Set are still
// visible to TrackedKeys / handleQueryAllBalances.
//
// The index is not persisted; if the process restarts and nobody re-seeds it,
// TrackedKeys will return an empty list until balances are touched again.
// Genesis InitGenesis paths re-seed the index because they call Set directly.
func (s *BAPIBalanceStore) TrackKey(key string) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keySet[key] = struct{}{}
}

// UntrackKey removes `key` from the in-memory index. Called by Delete and by
// the effect executor when a delete effect targets a balance row.
func (s *BAPIBalanceStore) UntrackKey(key string) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keySet, key)
}

// TrackedKeys returns the sorted list of known "account/denom" keys that
// have been written through this store instance. Sorted output is
// required for deterministic query results.
//
// On the first call after process startup, if the underlying StateStore
// supports iteration, rebuilds the index from a prefix scan so balances
// written before the previous shutdown become enumerable without needing
// to be touched again.
func (s *BAPIBalanceStore) TrackedKeys() []string {
	if s == nil {
		return nil
	}
	s.ensureRebuilt()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.keySet))
	for k := range s.keySet {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ResetIndex clears the in-memory key index and arms the next TrackedKeys
// call to rebuild it from the underlying StateStore. Used by state-sync
// after the StateStore has been wholesale replaced by a snapshot import —
// without this, the cached keySet would still reflect the pre-import
// world. Safe to call concurrently with reads and writes.
func (s *BAPIBalanceStore) ResetIndex() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keySet = make(map[string]struct{})
	s.rebuilt = false
}

// ensureRebuilt performs the one-shot lazy index rebuild from the
// underlying iterable store. Safe to call concurrently and repeatedly.
func (s *BAPIBalanceStore) ensureRebuilt() {
	s.mu.Lock()
	if s.rebuilt {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	keys := make([]string, 0, 64)
	err := s.TypedStore.IterateRelative(func(relKey string, _ *BAPIBalance) bool {
		keys = append(keys, relKey)
		return false
	})

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rebuilt {
		return
	}
	if err == nil {
		for _, k := range keys {
			s.keySet[k] = struct{}{}
		}
	}
	s.rebuilt = true
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
	if err := s.TypedStore.Set(ctx, balanceKey(account, denom), balance); err != nil {
		return err
	}
	s.TrackKey(balanceKey(account, denom))
	return nil
}

// Delete removes a balance.
func (s *BAPIBalanceStore) Delete(ctx context.Context, account, denom string) error {
	if account == "" {
		return fmt.Errorf("empty account")
	}
	if denom == "" {
		return fmt.Errorf("empty denom")
	}
	if err := s.TypedStore.Delete(ctx, balanceKey(account, denom)); err != nil {
		return err
	}
	s.UntrackKey(balanceKey(account, denom))
	return nil
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
