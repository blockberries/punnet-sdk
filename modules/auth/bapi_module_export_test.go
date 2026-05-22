package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportGenesis_ReturnsAllAccounts is the PLAN B2-5 regression: previously
// ExportGenesis returned a hard-coded empty list; it must now walk the auth
// account store and return every known account.
func TestExportGenesis_ReturnsAllAccounts(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	accountStore := provider.GetAccountStore()
	mod, err := NewBAPIAuthModule(accountStore)
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now()
	want := []*ptypes.Account{
		{
			Name: ptypes.AccountName("alice"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"pkalice": 1},
				AccountWeights: map[ptypes.AccountName]uint64{},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Name: ptypes.AccountName("bob"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"pkbob": 1},
				AccountWeights: map[ptypes.AccountName]uint64{},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	for _, a := range want {
		require.NoError(t, accountStore.Set(ctx, a))
	}

	raw, err := mod.ExportGenesis(ctx)
	require.NoError(t, err)

	var gen AuthGenesisState
	require.NoError(t, json.Unmarshal(raw, &gen))
	require.Len(t, gen.Accounts, 2)
	// Returned in sorted name order (alice < bob).
	assert.Equal(t, ptypes.AccountName("alice"), gen.Accounts[0].Name)
	assert.Equal(t, ptypes.AccountName("bob"), gen.Accounts[1].Name)
}

// TestExportGenesis_TracksAccountsCreatedViaEffects pins down the executor →
// store index notification path. When MsgCreateAccount is handled and its
// WriteEffect is executed, the resulting raw write must update the typed
// store's in-memory index so ExportGenesis sees the account.
func TestExportGenesis_TracksAccountsCreatedViaEffects(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	accountStore := provider.GetAccountStore()
	mod, err := NewBAPIAuthModule(accountStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()

	now := time.Now()
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height:  1,
			Time:    types.TimeToTimestamp(now),
			ChainID: "test-chain",
		},
		Account: ptypes.AccountName("alice"),
		TxIndex: 0,
	}

	effs, err := mod.handleCreateAccount(ctx, txCtx, &MsgCreateAccount{
		Name:   ptypes.AccountName("alice"),
		PubKey: []byte("alicepubkey-1234567890123456789"),
		Authority: ptypes.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"alicepubkey-1234567890123456789": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		},
	})
	require.NoError(t, err)

	// Pre-execute: tracker empty.
	assert.Empty(t, accountStore.TrackedKeys(), "no writes yet, tracker must be empty")

	_, err = exec.Execute(effs)
	require.NoError(t, err)

	// Post-execute: tracker should know about alice.
	assert.Equal(t, []string{"alice"}, accountStore.TrackedKeys())

	raw, err := mod.ExportGenesis(ctx)
	require.NoError(t, err)
	var gen AuthGenesisState
	require.NoError(t, json.Unmarshal(raw, &gen))
	require.Len(t, gen.Accounts, 1)
	assert.Equal(t, ptypes.AccountName("alice"), gen.Accounts[0].Name)
}
