package bank

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

// TestHandleQueryAllBalances_TracksTransferEffects covers the end-to-end path
// for PLAN B2-4: a MsgSend executed through the effect executor updates the
// balance via TransferEffect (which goes through BAPIBalanceStore.Transfer),
// and handleQueryAllBalances surfaces the resulting balances. The recipient
// account, which had no prior balance, must be discoverable after the send.
func TestHandleQueryAllBalances_TracksTransferEffects(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	balanceStore := provider.GetBalanceStore()
	mod, err := NewBAPIBankModule(balanceStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()

	// Seed alice with funds via the typed store (this is the genesis pattern).
	require.NoError(t, balanceStore.Set(ctx, "alice", "stake", 1000))

	// Build a MsgSend alice -> bob and run it through the handler / executor.
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height:  1,
			Time:    types.TimeToTimestamp(time.Now()),
			ChainID: "test-chain",
		},
		Account: ptypes.AccountName("alice"),
		TxIndex: 0,
	}
	effs, err := mod.handleSend(ctx, txCtx, &MsgSend{
		From:   ptypes.AccountName("alice"),
		To:     ptypes.AccountName("bob"),
		Amount: ptypes.Coin{Denom: "stake", Amount: 250},
	})
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	raw, err := mod.handleQueryAllBalances(ctx, nil, 0)
	require.NoError(t, err)

	var resp AllBalancesResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	// alice/stake (= 750) and bob/stake (= 250) — sorted ascending by key.
	require.Len(t, resp.Balances, 2)
	assert.Equal(t, "alice", resp.Balances[0].Account)
	assert.Equal(t, uint64(750), resp.Balances[0].Amount)
	assert.Equal(t, "bob", resp.Balances[1].Account)
	assert.Equal(t, uint64(250), resp.Balances[1].Amount)
}
