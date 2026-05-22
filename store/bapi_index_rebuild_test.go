package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	bapitypes "github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// TestBAPIAccountStore_TrackedKeysRebuildAfterRestart pins the PLAN
// follow-up: when a BAPIAccountStore is constructed atop a StateStore
// that already contains account data (i.e. the underlying tree was
// committed in a previous process and reopened here), the in-memory
// key index is empty until something touches each key. TrackedKeys
// now lazy-rebuilds from the underlying Iterable on first call.
func TestBAPIAccountStore_TrackedKeysRebuildAfterRestart(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	defer ss.Close()

	// Phase 1: write through accountStore A, commit, drop A.
	storeA := store.NewBAPIAccountStore(ss)
	for _, name := range []string{"alice", "bob", "charlie"} {
		require.NoError(t, storeA.Set(ctx, &types.Account{
			Name:      types.AccountName(name),
			Authority: types.NewAuthority(1, []byte{1, 2, 3}, 1),
		}))
	}

	// Phase 2: construct a fresh accountStore B over the same
	// underlying state store. Its in-memory keySet starts empty —
	// simulates a process restart where the index has been lost.
	storeB := store.NewBAPIAccountStore(ss)

	// First TrackedKeys call must trigger the rebuild from
	// statestore.Iterable and return the keys that were written
	// through storeA.
	keys := storeB.TrackedKeys()
	require.ElementsMatch(t, []string{"alice", "bob", "charlie"}, keys,
		"TrackedKeys after restart must rebuild from iterable store")
}

func TestBAPIAccountStore_TrackedKeysRebuildIdempotent(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	defer ss.Close()

	storeA := store.NewBAPIAccountStore(ss)
	require.NoError(t, storeA.Set(ctx, &types.Account{
		Name:      types.AccountName("alice"),
		Authority: types.NewAuthority(1, []byte{1, 2, 3}, 1),
	}))

	storeB := store.NewBAPIAccountStore(ss)
	require.Equal(t, []string{"alice"}, storeB.TrackedKeys())

	// New write after the rebuild must be visible.
	require.NoError(t, storeB.Set(ctx, &types.Account{
		Name:      types.AccountName("bob"),
		Authority: types.NewAuthority(1, []byte{4, 5, 6}, 1),
	}))
	require.ElementsMatch(t, []string{"alice", "bob"}, storeB.TrackedKeys())

	// Calling TrackedKeys again must not re-trigger the rebuild or
	// double-count anything.
	require.ElementsMatch(t, []string{"alice", "bob"}, storeB.TrackedKeys())
}

func TestBAPIBalanceStore_TrackedKeysRebuildAfterRestart(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	defer ss.Close()

	storeA := store.NewBAPIBalanceStore(ss)
	require.NoError(t, storeA.Set(ctx, "alice", "coin", 100))
	require.NoError(t, storeA.Set(ctx, "bob", "coin", 200))
	require.NoError(t, storeA.Set(ctx, "alice", "gas", 50))

	storeB := store.NewBAPIBalanceStore(ss)
	keys := storeB.TrackedKeys()
	require.ElementsMatch(t, []string{"alice/coin", "alice/gas", "bob/coin"}, keys)
}

func TestBAPIValidatorStore_IterateValidators(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	defer ss.Close()

	vs := store.NewBAPIValidatorStore(ss)
	for _, v := range []*store.BAPIValidator{
		{PubKey: bapitypes.PublicKey{Type: bapitypes.KeyTypeEd25519, Data: []byte("alice-pubkey--32-bytes-test--01")}, Power: 100},
		{PubKey: bapitypes.PublicKey{Type: bapitypes.KeyTypeEd25519, Data: []byte("bob-pubkey----32-bytes-test--02")}, Power: 200, Jailed: true},
		{PubKey: bapitypes.PublicKey{Type: bapitypes.KeyTypeEd25519, Data: []byte("charlie-pubkey-32-bytes-test03")}, Power: 0},
	} {
		require.NoError(t, vs.SetValidator(ctx, v))
	}

	var seen []uint64
	err = vs.IterateValidators(func(v *store.BAPIValidator) bool {
		seen = append(seen, v.Power)
		return false
	})
	require.NoError(t, err)
	require.Len(t, seen, 3, "IterateValidators must visit every validator including jailed and zero-power")
}

func TestBAPIValidatorStore_IterateValidators_EarlyStop(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	defer ss.Close()

	vs := store.NewBAPIValidatorStore(ss)
	for i := 0; i < 5; i++ {
		require.NoError(t, vs.SetValidator(ctx, &store.BAPIValidator{
			PubKey: bapitypes.PublicKey{
				Type: bapitypes.KeyTypeEd25519,
				Data: []byte("pubkey-pad-32-bytes-for-test-" + string(rune('a'+i)) + "x"),
			},
			Power: uint64(i + 1),
		}))
	}

	var count int
	err = vs.IterateValidators(func(_ *store.BAPIValidator) bool {
		count++
		return count >= 2 // stop after 2
	})
	require.NoError(t, err)
	require.Equal(t, 2, count, "early-stop must terminate iteration")
}

func TestBAPIAccountStore_IterateAccounts(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	defer ss.Close()

	as := store.NewBAPIAccountStore(ss)
	for _, name := range []string{"alice", "bob", "charlie"} {
		require.NoError(t, as.Set(ctx, &types.Account{
			Name:      types.AccountName(name),
			Authority: types.NewAuthority(1, []byte{1, 2, 3}, 1),
		}))
	}

	var seen []string
	err = as.IterateAccounts(func(a *types.Account) bool {
		seen = append(seen, string(a.Name))
		return false
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"alice", "bob", "charlie"}, seen)
}
