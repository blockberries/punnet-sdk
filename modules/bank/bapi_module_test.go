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

func createTestStateStore(t *testing.T) statestore.StateStore {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	return ss
}

func createTestBankModule(t *testing.T) (*BAPIBankModule, *store.BAPIBalanceStore) {
	ss := createTestStateStore(t)
	balanceStore := store.NewBAPIBalanceStore(ss)
	mod, err := NewBAPIBankModule(balanceStore)
	require.NoError(t, err)
	return mod, balanceStore
}

func TestNewBAPIBankModule(t *testing.T) {
	t.Run("creates module with valid store", func(t *testing.T) {
		ss := createTestStateStore(t)
		balanceStore := store.NewBAPIBalanceStore(ss)

		mod, err := NewBAPIBankModule(balanceStore)
		require.NoError(t, err)
		assert.NotNil(t, mod)
		assert.Equal(t, ModuleName, mod.Name())
	})

	t.Run("fails with nil store", func(t *testing.T) {
		_, err := NewBAPIBankModule(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "balance store cannot be nil")
	})
}

func TestBAPIBankModule_RegisterHandlers(t *testing.T) {
	mod, _ := createTestBankModule(t)

	t.Run("registers message handlers", func(t *testing.T) {
		handlers := mod.RegisterMsgHandlers()
		assert.Len(t, handlers, 2)
		assert.Contains(t, handlers, TypeMsgSend)
		assert.Contains(t, handlers, TypeMsgMultiSend)
	})

	t.Run("registers query handlers", func(t *testing.T) {
		handlers := mod.RegisterQueryHandlers()
		assert.Len(t, handlers, 2)
		assert.Contains(t, handlers, "/bank/balance")
		assert.Contains(t, handlers, "/bank/all_balances")
	})
}

func TestBAPIBankModule_HandleSend(t *testing.T) {
	mod, balanceStore := createTestBankModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	// Set up initial balance for alice
	err := balanceStore.Set(ctx, "alice", "stake", 1000)
	require.NoError(t, err)

	t.Run("sends tokens successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgSend{
			From:   ptypes.AccountName("alice"),
			To:     ptypes.AccountName("bob"),
			Amount: ptypes.Coin{Denom: "stake", Amount: 100},
		}

		effs, err := mod.handleSend(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		// Check transfer effect
		transferEff, ok := effs[0].(effects.TransferEffect)
		assert.True(t, ok)
		assert.Equal(t, ptypes.AccountName("alice"), transferEff.From)
		assert.Equal(t, ptypes.AccountName("bob"), transferEff.To)
		assert.Len(t, transferEff.Amount, 1)
		assert.Equal(t, "stake", transferEff.Amount[0].Denom)
		assert.Equal(t, uint64(100), transferEff.Amount[0].Amount)

		// Check event effect
		eventEff, ok := effs[1].(effects.EventEffect)
		assert.True(t, ok)
		assert.Equal(t, "bank.send", eventEff.EventType)
	})

	t.Run("fails when sender doesn't match signer", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("bob"), // Different from sender
			TxIndex:          0,
		}

		msg := &MsgSend{
			From:   ptypes.AccountName("alice"),
			To:     ptypes.AccountName("bob"),
			Amount: ptypes.Coin{Denom: "stake", Amount: 100},
		}

		_, err := mod.handleSend(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sender must be transaction account")
	})

	t.Run("fails with insufficient balance", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgSend{
			From:   ptypes.AccountName("alice"),
			To:     ptypes.AccountName("bob"),
			Amount: ptypes.Coin{Denom: "stake", Amount: 99999}, // More than balance
		}

		_, err := mod.handleSend(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient balance")
	})

	t.Run("fails with invalid message type", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		_, err := mod.handleSend(ctx, txCtx, &MsgMultiSend{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid message type")
	})

	t.Run("fails with nil context", func(t *testing.T) {
		msg := &MsgSend{
			From:   ptypes.AccountName("alice"),
			To:     ptypes.AccountName("bob"),
			Amount: ptypes.Coin{Denom: "stake", Amount: 100},
		}

		_, err := mod.handleSend(ctx, nil, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction context is nil")
	})
}

func TestBAPIBankModule_HandleMultiSend(t *testing.T) {
	mod, balanceStore := createTestBankModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	// Set up initial balances
	err := balanceStore.Set(ctx, "alice", "stake", 1000)
	require.NoError(t, err)
	err = balanceStore.Set(ctx, "bob", "stake", 500)
	require.NoError(t, err)

	t.Run("multi-send tokens successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgMultiSend{
			Inputs: []Input{
				{
					Address: ptypes.AccountName("alice"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 200}},
				},
			},
			Outputs: []Output{
				{
					Address: ptypes.AccountName("charlie"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 100}},
				},
				{
					Address: ptypes.AccountName("dave"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 100}},
				},
			},
		}

		effs, err := mod.handleMultiSend(ctx, txCtx, msg)
		require.NoError(t, err)
		// Should have transfer effects plus event
		assert.GreaterOrEqual(t, len(effs), 2)

		// Check last effect is event
		eventEff, ok := effs[len(effs)-1].(effects.EventEffect)
		assert.True(t, ok)
		assert.Equal(t, "bank.multi_send", eventEff.EventType)
	})

	t.Run("fails when sender not authorized", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("charlie"), // Not in inputs
			TxIndex:          0,
		}

		msg := &MsgMultiSend{
			Inputs: []Input{
				{
					Address: ptypes.AccountName("alice"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 100}},
				},
			},
			Outputs: []Output{
				{
					Address: ptypes.AccountName("bob"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 100}},
				},
			},
		}

		_, err := mod.handleMultiSend(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction account must be one of the senders")
	})

	t.Run("fails with insufficient balance", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgMultiSend{
			Inputs: []Input{
				{
					Address: ptypes.AccountName("alice"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 99999}}, // More than balance
				},
			},
			Outputs: []Output{
				{
					Address: ptypes.AccountName("bob"),
					Coins:   ptypes.Coins{{Denom: "stake", Amount: 99999}},
				},
			},
		}

		_, err := mod.handleMultiSend(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient balance")
	})
}

func TestBAPIBankModule_HandleQueryBalance(t *testing.T) {
	mod, balanceStore := createTestBankModule(t)
	ctx := context.Background()

	// Set up balance
	err := balanceStore.Set(ctx, "alice", "stake", 500)
	require.NoError(t, err)

	t.Run("queries balance successfully", func(t *testing.T) {
		result, err := mod.handleQueryBalance(ctx, []byte("alice/stake"), 0)
		require.NoError(t, err)
		assert.Equal(t, "500", string(result))
	})

	t.Run("returns zero for missing balance", func(t *testing.T) {
		result, err := mod.handleQueryBalance(ctx, []byte("bob/stake"), 0)
		require.NoError(t, err)
		assert.Equal(t, "0", string(result))
	})

	t.Run("fails with invalid format", func(t *testing.T) {
		_, err := mod.handleQueryBalance(ctx, []byte("invalid"), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid query format")
	})

	t.Run("fails with invalid account name", func(t *testing.T) {
		_, err := mod.handleQueryBalance(ctx, []byte("/stake"), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid account name")
	})

	t.Run("fails with empty denom", func(t *testing.T) {
		_, err := mod.handleQueryBalance(ctx, []byte("alice/"), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denom cannot be empty")
	})
}

func TestBAPIBankModule_HandleQueryAllBalances(t *testing.T) {
	mod, balanceStore := createTestBankModule(t)
	ctx := context.Background()

	// Seed three balances so the tests below have something to enumerate.
	// Two accounts with two distinct denoms apiece — enough to exercise both
	// the all-accounts and filtered code paths (PLAN B2-4).
	require.NoError(t, balanceStore.Set(ctx, "alice", "stake", 100))
	require.NoError(t, balanceStore.Set(ctx, "alice", "atom", 250))
	require.NoError(t, balanceStore.Set(ctx, "bob", "stake", 50))

	t.Run("returns all balances for a specific account", func(t *testing.T) {
		result, err := mod.handleQueryAllBalances(ctx, []byte("alice"), 0)
		require.NoError(t, err)

		var resp AllBalancesResponse
		require.NoError(t, json.Unmarshal(result, &resp))
		assert.Equal(t, "alice", resp.Account)
		require.Len(t, resp.Balances, 2)
		// Sorted by "account/denom" key — alice/atom comes before alice/stake.
		assert.Equal(t, "atom", resp.Balances[0].Denom)
		assert.Equal(t, uint64(250), resp.Balances[0].Amount)
		assert.Equal(t, "stake", resp.Balances[1].Denom)
		assert.Equal(t, uint64(100), resp.Balances[1].Amount)
	})

	t.Run("returns balances across every account when data is empty", func(t *testing.T) {
		result, err := mod.handleQueryAllBalances(ctx, nil, 0)
		require.NoError(t, err)

		var resp AllBalancesResponse
		require.NoError(t, json.Unmarshal(result, &resp))
		assert.Equal(t, "", resp.Account)
		require.Len(t, resp.Balances, 3)
		// Deterministic ascending order on "account/denom".
		assert.Equal(t, "alice", resp.Balances[0].Account)
		assert.Equal(t, "atom", resp.Balances[0].Denom)
		assert.Equal(t, "alice", resp.Balances[1].Account)
		assert.Equal(t, "stake", resp.Balances[1].Denom)
		assert.Equal(t, "bob", resp.Balances[2].Account)
		assert.Equal(t, "stake", resp.Balances[2].Denom)
	})

	t.Run("rejects an invalid (non-empty) account name", func(t *testing.T) {
		_, err := mod.handleQueryAllBalances(ctx, []byte("INVALID NAME WITH SPACE"), 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid account name")
	})
}

func TestBAPIBankModule_InitGenesis(t *testing.T) {
	mod, balanceStore := createTestBankModule(t)
	ctx := context.Background()

	t.Run("initializes empty genesis", func(t *testing.T) {
		err := mod.InitGenesis(ctx, nil)
		assert.NoError(t, err)

		err = mod.InitGenesis(ctx, []byte{})
		assert.NoError(t, err)
	})

	t.Run("initializes genesis balances", func(t *testing.T) {
		genesisData := `{
			"balances": [
				{"account": "genesis1", "denom": "stake", "amount": 1000},
				{"account": "genesis1", "denom": "token", "amount": 500},
				{"account": "genesis2", "denom": "stake", "amount": 2000}
			]
		}`

		err := mod.InitGenesis(ctx, []byte(genesisData))
		require.NoError(t, err)

		// Verify balances were created
		balance, err := balanceStore.GetAmount(ctx, "genesis1", "stake")
		require.NoError(t, err)
		assert.Equal(t, uint64(1000), balance)

		balance, err = balanceStore.GetAmount(ctx, "genesis1", "token")
		require.NoError(t, err)
		assert.Equal(t, uint64(500), balance)

		balance, err = balanceStore.GetAmount(ctx, "genesis2", "stake")
		require.NoError(t, err)
		assert.Equal(t, uint64(2000), balance)
	})

	t.Run("fails with invalid JSON", func(t *testing.T) {
		err := mod.InitGenesis(ctx, []byte("invalid json"))
		assert.Error(t, err)
	})
}

func TestBAPIBankModule_ExportGenesis(t *testing.T) {
	mod, _ := createTestBankModule(t)
	ctx := context.Background()

	t.Run("exports genesis", func(t *testing.T) {
		data, err := mod.ExportGenesis(ctx)
		require.NoError(t, err)
		assert.NotNil(t, data)
		// Should be valid JSON
		assert.Contains(t, string(data), "balances")
	})
}

func TestBAPIBankModule_InterfaceCompliance(t *testing.T) {
	mod, _ := createTestBankModule(t)

	// Verify the module implements all required interfaces
	var _ runtime.BAPIModule = mod
	var _ runtime.BAPIGenesisInitializer = mod
	var _ runtime.BAPIGenesisExporter = mod
}
