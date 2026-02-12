package staking

import (
	"context"
	"encoding/hex"
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

func createTestStakingModule(t *testing.T) (*BAPIStakingModule, *store.BAPIValidatorStore, *store.BAPIBalanceStore) {
	ss := createTestStateStore(t)
	validatorStore := store.NewBAPIValidatorStore(ss)
	balanceStore := store.NewBAPIBalanceStore(ss)
	mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
	require.NoError(t, err)
	return mod, validatorStore, balanceStore
}

func TestNewBAPIStakingModule(t *testing.T) {
	t.Run("creates module with valid stores", func(t *testing.T) {
		ss := createTestStateStore(t)
		validatorStore := store.NewBAPIValidatorStore(ss)
		balanceStore := store.NewBAPIBalanceStore(ss)

		mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
		require.NoError(t, err)
		assert.NotNil(t, mod)
		assert.Equal(t, ModuleName, mod.Name())
	})

	t.Run("fails with nil validator store", func(t *testing.T) {
		ss := createTestStateStore(t)
		balanceStore := store.NewBAPIBalanceStore(ss)

		_, err := NewBAPIStakingModule(nil, balanceStore)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validator store cannot be nil")
	})

	t.Run("fails with nil balance store", func(t *testing.T) {
		ss := createTestStateStore(t)
		validatorStore := store.NewBAPIValidatorStore(ss)

		_, err := NewBAPIStakingModule(validatorStore, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "balance store cannot be nil")
	})
}

func TestBAPIStakingModule_RegisterHandlers(t *testing.T) {
	mod, _, _ := createTestStakingModule(t)

	t.Run("registers message handlers", func(t *testing.T) {
		handlers := mod.RegisterMsgHandlers()
		assert.Len(t, handlers, 3)
		assert.Contains(t, handlers, TypeMsgCreateValidator)
		assert.Contains(t, handlers, TypeMsgDelegate)
		assert.Contains(t, handlers, TypeMsgUndelegate)
	})

	t.Run("registers query handlers", func(t *testing.T) {
		handlers := mod.RegisterQueryHandlers()
		assert.Len(t, handlers, 2)
		assert.Contains(t, handlers, "/staking/validator")
		assert.Contains(t, handlers, "/staking/delegation")
	})
}

func TestBAPIStakingModule_HandleCreateValidator(t *testing.T) {
	mod, validatorStore, _ := createTestStakingModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	pubKey := []byte("validator-pubkey-123456789012")

	t.Run("creates validator successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgCreateValidator{
			Delegator:    ptypes.AccountName("alice"),
			PubKey:       pubKey,
			InitialPower: 100,
			Commission:   500, // 5%
		}

		effs, err := mod.handleCreateValidator(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 1)

		// Check event effect
		eventEff, ok := effs[0].(effects.EventEffect)
		assert.True(t, ok)
		assert.Equal(t, "staking.validator_created", eventEff.EventType)

		// Verify validator was created
		validator, err := validatorStore.GetValidator(ctx, pubKey)
		require.NoError(t, err)
		assert.Equal(t, uint64(100), validator.Power)
		assert.Equal(t, uint32(500), validator.Commission)
	})

	t.Run("fails when delegator doesn't match signer", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("bob"), // Different from delegator
			TxIndex:          0,
		}

		msg := &MsgCreateValidator{
			Delegator:    ptypes.AccountName("alice"),
			PubKey:       []byte("another-pubkey-123456789012"),
			InitialPower: 100,
			Commission:   500,
		}

		_, err := mod.handleCreateValidator(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delegator must be transaction account")
	})

	t.Run("fails when validator already exists", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgCreateValidator{
			Delegator:    ptypes.AccountName("alice"),
			PubKey:       pubKey, // Same pubkey as before
			InitialPower: 200,
			Commission:   500,
		}

		_, err := mod.handleCreateValidator(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("fails with invalid message type", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		_, err := mod.handleCreateValidator(ctx, txCtx, &MsgDelegate{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid message type")
	})
}

func TestBAPIStakingModule_HandleDelegate(t *testing.T) {
	mod, validatorStore, balanceStore := createTestStakingModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	// Create a validator
	pubKey := []byte("validator-for-delegation-test1")
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           100,
		Jailed:          false,
		Commission:      500,
		TotalDelegation: 0,
	}
	err := validatorStore.SetValidator(ctx, validator)
	require.NoError(t, err)

	// Set up balance for alice
	err = balanceStore.Set(ctx, "alice", "stake", 1000)
	require.NoError(t, err)

	t.Run("delegates successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgDelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
		}

		effs, err := mod.handleDelegate(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		// Check transfer effect
		transferEff, ok := effs[0].(effects.TransferEffect)
		assert.True(t, ok)
		assert.Equal(t, ptypes.AccountName("alice"), transferEff.From)
		assert.Equal(t, ptypes.AccountName("staking.pool"), transferEff.To)

		// Check event effect
		eventEff, ok := effs[1].(effects.EventEffect)
		assert.True(t, ok)
		assert.Equal(t, "staking.delegated", eventEff.EventType)
	})

	t.Run("fails when validator doesn't exist", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgDelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: []byte("nonexistent-validator-key123"),
			Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
		}

		_, err := mod.handleDelegate(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validator not found")
	})

	t.Run("fails with insufficient balance", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgDelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 99999}, // More than balance
		}

		_, err := mod.handleDelegate(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient balance")
	})

	t.Run("fails when delegator doesn't match signer", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("bob"),
			TxIndex:          0,
		}

		msg := &MsgDelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
		}

		_, err := mod.handleDelegate(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delegator must be transaction account")
	})
}

func TestBAPIStakingModule_HandleUndelegate(t *testing.T) {
	mod, validatorStore, balanceStore := createTestStakingModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	// Create a validator
	pubKey := []byte("validator-for-undelegate-test")
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           100,
		Jailed:          false,
		Commission:      500,
		TotalDelegation: 500,
	}
	err := validatorStore.SetValidator(ctx, validator)
	require.NoError(t, err)

	// Set up balance and delegation
	err = balanceStore.Set(ctx, "staking.pool", "stake", 1000)
	require.NoError(t, err)
	err = validatorStore.Delegate(ctx, "alice", pubKey, 500)
	require.NoError(t, err)

	t.Run("undelegates successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgUndelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
		}

		effs, err := mod.handleUndelegate(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		// Check transfer effect
		transferEff, ok := effs[0].(effects.TransferEffect)
		assert.True(t, ok)
		assert.Equal(t, ptypes.AccountName("staking.pool"), transferEff.From)
		assert.Equal(t, ptypes.AccountName("alice"), transferEff.To)

		// Check event effect
		eventEff, ok := effs[1].(effects.EventEffect)
		assert.True(t, ok)
		assert.Equal(t, "staking.undelegated", eventEff.EventType)
	})

	t.Run("fails when delegation doesn't exist", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("bob"),
			TxIndex:          0,
		}

		msg := &MsgUndelegate{
			Delegator: ptypes.AccountName("bob"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
		}

		_, err := mod.handleUndelegate(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delegation not found")
	})

	t.Run("fails with insufficient delegation", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgUndelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 99999}, // More than delegated
		}

		_, err := mod.handleUndelegate(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient delegation")
	})
}

func TestBAPIStakingModule_HandleQueryValidator(t *testing.T) {
	mod, validatorStore, _ := createTestStakingModule(t)
	ctx := context.Background()

	// Create a validator
	pubKey := []byte("validator-for-query-test12345")
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           100,
		Jailed:          false,
		Description:     "Test validator",
		Commission:      500,
		TotalDelegation: 1000,
	}
	err := validatorStore.SetValidator(ctx, validator)
	require.NoError(t, err)

	t.Run("queries validator successfully", func(t *testing.T) {
		result, err := mod.handleQueryValidator(ctx, []byte(hex.EncodeToString(pubKey)), 0)
		require.NoError(t, err)
		assert.Contains(t, string(result), "100")  // power
		assert.Contains(t, string(result), "500")  // commission
		assert.Contains(t, string(result), "1000") // total_delegation
	})

	t.Run("fails with empty pubkey", func(t *testing.T) {
		_, err := mod.handleQueryValidator(ctx, []byte{}, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "public key cannot be empty")
	})

	t.Run("fails when validator not found", func(t *testing.T) {
		_, err := mod.handleQueryValidator(ctx, []byte(hex.EncodeToString([]byte("nonexistent"))), 0)
		assert.Error(t, err)
	})
}

func TestBAPIStakingModule_HandleQueryDelegation(t *testing.T) {
	mod, validatorStore, _ := createTestStakingModule(t)
	ctx := context.Background()

	// Create a validator and delegation
	pubKey := []byte("validator-for-del-query-test")
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           100,
		TotalDelegation: 500,
	}
	err := validatorStore.SetValidator(ctx, validator)
	require.NoError(t, err)
	err = validatorStore.Delegate(ctx, "alice", pubKey, 500)
	require.NoError(t, err)

	t.Run("queries delegation successfully", func(t *testing.T) {
		query := "alice/" + hex.EncodeToString(pubKey)
		result, err := mod.handleQueryDelegation(ctx, []byte(query), 0)
		require.NoError(t, err)
		assert.Contains(t, string(result), "alice")
		assert.Contains(t, string(result), "500") // amount
	})

	t.Run("fails with invalid format", func(t *testing.T) {
		_, err := mod.handleQueryDelegation(ctx, []byte("invalid"), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid query format")
	})

	t.Run("fails with invalid account name", func(t *testing.T) {
		_, err := mod.handleQueryDelegation(ctx, []byte("/"+hex.EncodeToString(pubKey)), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid delegator account")
	})
}

func TestBAPIStakingModule_InitGenesis(t *testing.T) {
	mod, validatorStore, _ := createTestStakingModule(t)
	ctx := context.Background()

	t.Run("initializes empty genesis", func(t *testing.T) {
		err := mod.InitGenesis(ctx, nil)
		assert.NoError(t, err)

		err = mod.InitGenesis(ctx, []byte{})
		assert.NoError(t, err)
	})

	t.Run("initializes genesis validators", func(t *testing.T) {
		pubKey := hex.EncodeToString([]byte("genesis-validator-pubkey123"))
		genesisData := `{
			"validators": [
				{
					"pub_key": "` + pubKey + `",
					"power": 100,
					"description": "Genesis validator",
					"commission": 500
				}
			]
		}`

		err := mod.InitGenesis(ctx, []byte(genesisData))
		require.NoError(t, err)

		// Verify validator was created
		pubKeyBytes, _ := hex.DecodeString(pubKey)
		validator, err := validatorStore.GetValidator(ctx, pubKeyBytes)
		require.NoError(t, err)
		assert.Equal(t, uint64(100), validator.Power)
		assert.Equal(t, "Genesis validator", validator.Description)
	})

	t.Run("fails with invalid JSON", func(t *testing.T) {
		err := mod.InitGenesis(ctx, []byte("invalid json"))
		assert.Error(t, err)
	})
}

func TestBAPIStakingModule_ExportGenesis(t *testing.T) {
	mod, _, _ := createTestStakingModule(t)
	ctx := context.Background()

	t.Run("exports genesis", func(t *testing.T) {
		data, err := mod.ExportGenesis(ctx)
		require.NoError(t, err)
		assert.NotNil(t, data)
		// Should be valid JSON
		assert.Contains(t, string(data), "validators")
	})
}

func TestBAPIStakingModule_BlockLifecycle(t *testing.T) {
	mod, _, _ := createTestStakingModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	t.Run("BeginBlock returns no effects", func(t *testing.T) {
		effs, err := mod.BeginBlock(ctx, blockCtx)
		require.NoError(t, err)
		assert.Nil(t, effs)
	})

	t.Run("EndBlock returns no updates", func(t *testing.T) {
		effs, updates, err := mod.EndBlock(ctx, blockCtx)
		require.NoError(t, err)
		assert.Nil(t, effs)
		assert.Nil(t, updates)
	})
}

func TestBAPIStakingModule_InterfaceCompliance(t *testing.T) {
	mod, _, _ := createTestStakingModule(t)

	// Verify the module implements all required interfaces
	var _ runtime.BAPIModule = mod
	var _ runtime.BAPIBlockProcessor = mod
	var _ runtime.BAPIGenesisInitializer = mod
	var _ runtime.BAPIGenesisExporter = mod
}
