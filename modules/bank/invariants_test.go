package bank

import (
	"context"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestBank(t *testing.T) (*BAPIBankModule, *store.BAPIBalanceStore) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	bs := store.NewBAPIBalanceStore(ss)
	mod, err := NewBAPIBankModule(bs)
	require.NoError(t, err)
	return mod, bs
}

func TestSupplyTotal_EmptyStore(t *testing.T) {
	mod, _ := newTestBank(t)
	total, err := mod.SupplyTotal(context.Background(), "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), total)
}

func TestSupplyTotal_SumsBalances(t *testing.T) {
	mod, bs := newTestBank(t)
	ctx := context.Background()

	require.NoError(t, bs.Set(ctx, "alice", "stake", 100))
	require.NoError(t, bs.Set(ctx, "bob", "stake", 250))
	require.NoError(t, bs.Set(ctx, "carol", "stake", 50))

	total, err := mod.SupplyTotal(ctx, "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(400), total)
}

func TestSupplyTotal_FiltersByDenom(t *testing.T) {
	mod, bs := newTestBank(t)
	ctx := context.Background()

	require.NoError(t, bs.Set(ctx, "alice", "stake", 100))
	require.NoError(t, bs.Set(ctx, "alice", "other", 999))
	require.NoError(t, bs.Set(ctx, "bob", "stake", 200))

	total, err := mod.SupplyTotal(ctx, "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(300), total, "other denom must not contribute")
}

func TestSupplyTotal_EmptyDenom(t *testing.T) {
	mod, _ := newTestBank(t)
	_, err := mod.SupplyTotal(context.Background(), "")
	require.Error(t, err)
}

func TestAssertSupplyConserved_Holds(t *testing.T) {
	mod, bs := newTestBank(t)
	ctx := context.Background()

	require.NoError(t, bs.Set(ctx, "alice", "stake", 600))
	require.NoError(t, bs.Set(ctx, "bob", "stake", 400))

	// 600 + 400 == 1000
	require.NoError(t, mod.AssertSupplyConserved(ctx, "stake", 1000))
}

func TestAssertSupplyConserved_Underflow(t *testing.T) {
	mod, bs := newTestBank(t)
	ctx := context.Background()

	require.NoError(t, bs.Set(ctx, "alice", "stake", 100))

	err := mod.AssertSupplyConserved(ctx, "stake", 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "supply conservation violated")
}

func TestAssertSupplyConserved_AfterTransfer(t *testing.T) {
	mod, bs := newTestBank(t)
	ctx := context.Background()

	// Seed 1000 across alice/bob.
	require.NoError(t, bs.Set(ctx, "alice", "stake", 1000))

	// Transfer 250 from alice to bob; balances now 750 + 250 = 1000.
	require.NoError(t, bs.Transfer(ctx, "alice", "bob", "stake", 250))

	require.NoError(t, mod.AssertSupplyConserved(ctx, "stake", 1000),
		"transfers must preserve total supply by construction")
}

func TestSupplyTotal_NilModule(t *testing.T) {
	var mod *BAPIBankModule
	_, err := mod.SupplyTotal(context.Background(), "stake")
	require.Error(t, err)
}
