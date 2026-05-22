package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/types"
)

// BAPIAccountStore provides typed access to account data.
// It wraps blockberry's StateStore directly.
//
// The store also maintains an in-memory key index (PLAN B2-5) so callers like
// auth.ExportGenesis can enumerate accounts without iterating the merkle
// store. The index is populated three ways:
//
//  1. Set/Delete (write-time tracking on this process).
//  2. BAPIStoreProvider.NotifyAccountWrite from the effect executor.
//  3. Lazy rebuild from the underlying StateStore's Iterate hook on the
//     first TrackedKeys call after restart, when supported.
//
// Path 3 closes the previous post-restart gap where ExportGenesis would
// return an empty list until accounts were touched again. Backends that
// don't implement statestore.Iterable (in-memory test stores) silently
// fall back to write-time tracking.
type BAPIAccountStore struct {
	*TypedStore[*types.Account]

	mu        sync.RWMutex
	keySet    map[string]struct{}
	rebuilt   bool // true after first successful Iterate-backed rebuild
}

// NewBAPIAccountStore creates a new account store backed by blockberry's StateStore.
func NewBAPIAccountStore(store statestore.StateStore) *BAPIAccountStore {
	return &BAPIAccountStore{
		TypedStore: NewTypedStore[*types.Account](store, "accounts/"),
		keySet:     make(map[string]struct{}),
	}
}

// TrackKey records the relative key (e.g. "alice") in the in-memory index.
// Called by Set and by the effect executor (via NotifyAccountWrite) so write
// effects produced by auth handlers are visible to TrackedKeys.
func (s *BAPIAccountStore) TrackKey(key string) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keySet[key] = struct{}{}
}

// UntrackKey removes `key` from the in-memory index.
func (s *BAPIAccountStore) UntrackKey(key string) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keySet, key)
}

// TrackedKeys returns the sorted account names currently tracked by this
// store. Sorted output is required for deterministic genesis export and
// queries.
//
// On the first call after process startup, if the underlying StateStore
// supports iteration, this rebuilds the index from a prefix scan. That
// closes the post-restart "ExportGenesis returns empty" gap: keys that
// were written before the previous shutdown are recovered without
// needing to be touched again. The rebuild runs at most once per
// process; subsequent calls use the live keySet maintained by
// Set/Delete/NotifyAccountWrite.
func (s *BAPIAccountStore) TrackedKeys() []string {
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
func (s *BAPIAccountStore) ResetIndex() {
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
func (s *BAPIAccountStore) ensureRebuilt() {
	s.mu.Lock()
	if s.rebuilt {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	// Iterate without the mu held — IterateRelative may be slow on
	// large keyspaces, and write paths must remain unblocked.
	keys := make([]string, 0, 64)
	err := s.TypedStore.IterateRelative(func(relKey string, _ *types.Account) bool {
		keys = append(keys, relKey)
		return false
	})

	s.mu.Lock()
	defer s.mu.Unlock()
	// Race-safe check — another goroutine may have set rebuilt while
	// we were iterating. The double-iteration would be wasted work
	// but not incorrect.
	if s.rebuilt {
		return
	}
	if err == nil {
		for _, k := range keys {
			s.keySet[k] = struct{}{}
		}
	}
	// Even on ErrIterationUnsupported we set rebuilt=true so we don't
	// re-attempt on every TrackedKeys call.
	s.rebuilt = true
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
	if err := s.TypedStore.Set(ctx, string(account.Name), account); err != nil {
		return err
	}
	s.TrackKey(string(account.Name))
	return nil
}

// Delete removes an account by name.
func (s *BAPIAccountStore) Delete(ctx context.Context, name types.AccountName) error {
	if !name.IsValid() {
		return fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, name)
	}
	if err := s.TypedStore.Delete(ctx, string(name)); err != nil {
		return err
	}
	s.UntrackKey(string(name))
	return nil
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

// IterateAccounts walks every account currently in the store, invoking
// fn for each. Return true from fn to stop iteration.
//
// Returns store.ErrIterationUnsupported if the underlying StateStore is
// not iterable (test in-memory backends). Used by genesis export,
// account-listing RPC handlers, and any other code path that needs a
// full enumeration without reaching into the typed-store internals.
//
// Iteration order is the underlying tree's ascending byte order over
// account names — deterministic and suitable for consensus-critical
// exports.
func (s *BAPIAccountStore) IterateAccounts(fn func(a *types.Account) bool) error {
	return s.TypedStore.IterateRelative(func(_ string, a *types.Account) bool {
		return fn(a)
	})
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
