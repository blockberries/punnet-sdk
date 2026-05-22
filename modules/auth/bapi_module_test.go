package auth

import (
	"context"
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

func createTestModule(t *testing.T) (*BAPIAuthModule, *store.BAPIAccountStore) {
	ss := createTestStateStore(t)
	accountStore := store.NewBAPIAccountStore(ss)
	mod, err := NewBAPIAuthModule(accountStore)
	require.NoError(t, err)
	return mod, accountStore
}

func TestNewBAPIAuthModule(t *testing.T) {
	t.Run("creates module with valid store", func(t *testing.T) {
		ss := createTestStateStore(t)
		accountStore := store.NewBAPIAccountStore(ss)

		mod, err := NewBAPIAuthModule(accountStore)
		require.NoError(t, err)
		assert.NotNil(t, mod)
		assert.Equal(t, ModuleName, mod.Name())
	})

	t.Run("fails with nil store", func(t *testing.T) {
		_, err := NewBAPIAuthModule(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "account store cannot be nil")
	})
}

func TestBAPIAuthModule_RegisterHandlers(t *testing.T) {
	mod, _ := createTestModule(t)

	t.Run("registers message handlers", func(t *testing.T) {
		handlers := mod.RegisterMsgHandlers()
		assert.Len(t, handlers, 3)
		assert.Contains(t, handlers, TypeMsgCreateAccount)
		assert.Contains(t, handlers, TypeMsgUpdateAuthority)
		assert.Contains(t, handlers, TypeMsgDeleteAccount)
	})

	t.Run("registers query handlers", func(t *testing.T) {
		handlers := mod.RegisterQueryHandlers()
		assert.Len(t, handlers, 2)
		assert.Contains(t, handlers, "/auth/account")
		assert.Contains(t, handlers, "/auth/nonce")
	})
}

func TestBAPIAuthModule_HandleCreateAccount(t *testing.T) {
	mod, accountStore := createTestModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	t.Run("creates account successfully", func(t *testing.T) {
		accountName := ptypes.AccountName("alice")

		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          accountName,
			TxIndex:          0,
		}

		msg := &MsgCreateAccount{
			Name:   accountName,
			PubKey: []byte("testpubkey12345678901234567890"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"testpubkey12345678901234567890": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
		}

		effs, err := mod.handleCreateAccount(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		// Check it's a write effect
		writeEff, ok := effs[0].(*effects.WriteEffect[*ptypes.Account])
		assert.True(t, ok)
		assert.Equal(t, "accounts", writeEff.Store)
		assert.Equal(t, []byte(accountName), writeEff.StoreKey)
		assert.Equal(t, accountName, writeEff.Value.Name)

		// Check event effect
		eventEff, ok := effs[1].(effects.EventEffect)
		assert.True(t, ok)
		assert.Equal(t, "account.created", eventEff.EventType)
	})

	t.Run("allows different account to create another", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		msg := &MsgCreateAccount{
			Creator: ptypes.AccountName("alice"),
			Name:    ptypes.AccountName("bob"),
			PubKey:  []byte("testpubkey12345678901234567890"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"testpubkey12345678901234567890": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
		}

		effs, err := mod.handleCreateAccount(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		writeEff, ok := effs[0].(*effects.WriteEffect[*ptypes.Account])
		assert.True(t, ok)
		assert.Equal(t, ptypes.AccountName("bob"), writeEff.Value.Name)
	})

	t.Run("fails when account already exists", func(t *testing.T) {
		accountName := ptypes.AccountName("existing")

		// Create the account first
		existingAccount := &ptypes.Account{
			Name: accountName,
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"key": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
			Nonce:     0,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := accountStore.Set(ctx, existingAccount)
		require.NoError(t, err)

		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          accountName,
			TxIndex:          0,
		}

		msg := &MsgCreateAccount{
			Name:   accountName,
			PubKey: []byte("testpubkey12345678901234567890"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"testpubkey12345678901234567890": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
		}

		_, err = mod.handleCreateAccount(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("fails with invalid message type", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		}

		_, err := mod.handleCreateAccount(ctx, txCtx, &MsgDeleteAccount{Name: "alice"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid message type")
	})

	t.Run("fails with nil context", func(t *testing.T) {
		msg := &MsgCreateAccount{
			Name:   ptypes.AccountName("alice"),
			PubKey: []byte("testpubkey12345678901234567890"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"key": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
		}

		_, err := mod.handleCreateAccount(ctx, nil, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction context is nil")
	})
}

func TestBAPIAuthModule_HandleUpdateAuthority(t *testing.T) {
	mod, accountStore := createTestModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	// Create an account to update
	accountName := ptypes.AccountName("alice")
	existingAccount := &ptypes.Account{
		Name: accountName,
		Authority: ptypes.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"oldkey": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		},
		Nonce:     5,
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	err := accountStore.Set(ctx, existingAccount)
	require.NoError(t, err)

	t.Run("updates authority successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          accountName,
			TxIndex:          0,
		}

		newAuthority := ptypes.Authority{
			Threshold:      2,
			KeyWeights:     map[string]uint64{"newkey1": 1, "newkey2": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		}

		msg := &MsgUpdateAuthority{
			Name:         accountName,
			NewAuthority: newAuthority,
		}

		effs, err := mod.handleUpdateAuthority(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		// Check the write effect contains the new authority
		writeEff := effs[0].(*effects.WriteEffect[*ptypes.Account])
		assert.Equal(t, uint64(2), writeEff.Value.Authority.Threshold)
		assert.Len(t, writeEff.Value.Authority.KeyWeights, 2)
		// Nonce should be preserved
		assert.Equal(t, uint64(5), writeEff.Value.Nonce)

		// Check event
		eventEff := effs[1].(effects.EventEffect)
		assert.Equal(t, "account.authority_updated", eventEff.EventType)
	})

	t.Run("fails when account doesn't exist", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("nonexistent"),
			TxIndex:          0,
		}

		msg := &MsgUpdateAuthority{
			Name: ptypes.AccountName("nonexistent"),
			NewAuthority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"key": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
		}

		_, err := mod.handleUpdateAuthority(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get account")
	})

	t.Run("fails when account name doesn't match signer", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("bob"),
			TxIndex:          0,
		}

		msg := &MsgUpdateAuthority{
			Name: accountName, // alice, but signed by bob
			NewAuthority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"key": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
		}

		_, err := mod.handleUpdateAuthority(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "account name must match transaction account")
	})
}

func TestBAPIAuthModule_HandleDeleteAccount(t *testing.T) {
	mod, accountStore := createTestModule(t)
	ctx := context.Background()

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}

	// Create an account to delete
	accountName := ptypes.AccountName("todelete")
	existingAccount := &ptypes.Account{
		Name: accountName,
		Authority: ptypes.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"key": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		},
		Nonce:     0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := accountStore.Set(ctx, existingAccount)
	require.NoError(t, err)

	t.Run("deletes account successfully", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          accountName,
			TxIndex:          0,
		}

		msg := &MsgDeleteAccount{
			Name: accountName,
		}

		effs, err := mod.handleDeleteAccount(ctx, txCtx, msg)
		require.NoError(t, err)
		assert.Len(t, effs, 2)

		// Check delete effect
		deleteEff, ok := effs[0].(effects.DeleteEffect[*ptypes.Account])
		assert.True(t, ok)
		assert.Equal(t, "accounts", deleteEff.Store)
		assert.Equal(t, []byte(accountName), deleteEff.StoreKey)

		// Check event
		eventEff := effs[1].(effects.EventEffect)
		assert.Equal(t, "account.deleted", eventEff.EventType)
	})

	t.Run("fails when account doesn't exist", func(t *testing.T) {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("nonexistent"),
			TxIndex:          0,
		}

		msg := &MsgDeleteAccount{
			Name: ptypes.AccountName("nonexistent"),
		}

		_, err := mod.handleDeleteAccount(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("fails when account name doesn't match signer", func(t *testing.T) {
		// Create another account
		otherAccount := &ptypes.Account{
			Name: ptypes.AccountName("other"),
			Authority: ptypes.Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{"key": 1},
				AccountWeights: make(map[ptypes.AccountName]uint64),
			},
			Nonce:     0,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		accountStore.Set(ctx, otherAccount)

		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("attacker"),
			TxIndex:          0,
		}

		msg := &MsgDeleteAccount{
			Name: ptypes.AccountName("other"),
		}

		_, err := mod.handleDeleteAccount(ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "account name must match transaction account")
	})
}

func TestBAPIAuthModule_HandleQueryAccount(t *testing.T) {
	mod, accountStore := createTestModule(t)
	ctx := context.Background()

	// Create an account
	accountName := ptypes.AccountName("querytest")
	existingAccount := &ptypes.Account{
		Name: accountName,
		Authority: ptypes.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"key": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		},
		Nonce:     42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := accountStore.Set(ctx, existingAccount)
	require.NoError(t, err)

	t.Run("queries account successfully", func(t *testing.T) {
		result, err := mod.handleQueryAccount(ctx, []byte(accountName), 0)
		require.NoError(t, err)
		assert.NotNil(t, result)
		// Result is JSON encoded
		assert.Contains(t, string(result), "querytest")
		assert.Contains(t, string(result), "42") // nonce
	})

	t.Run("fails with invalid account name", func(t *testing.T) {
		_, err := mod.handleQueryAccount(ctx, []byte(""), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid account name")
	})

	t.Run("fails when account not found", func(t *testing.T) {
		_, err := mod.handleQueryAccount(ctx, []byte("nonexistent"), 0)
		assert.Error(t, err)
	})
}

func TestBAPIAuthModule_HandleQueryNonce(t *testing.T) {
	mod, accountStore := createTestModule(t)
	ctx := context.Background()

	// Create an account
	accountName := ptypes.AccountName("noncetest")
	existingAccount := &ptypes.Account{
		Name: accountName,
		Authority: ptypes.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"key": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		},
		Nonce:     99,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := accountStore.Set(ctx, existingAccount)
	require.NoError(t, err)

	t.Run("queries nonce successfully", func(t *testing.T) {
		result, err := mod.handleQueryNonce(ctx, []byte(accountName), 0)
		require.NoError(t, err)
		assert.Equal(t, "99", string(result))
	})

	t.Run("fails with invalid account name", func(t *testing.T) {
		_, err := mod.handleQueryNonce(ctx, []byte(""), 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid account name")
	})

	t.Run("fails when account not found", func(t *testing.T) {
		_, err := mod.handleQueryNonce(ctx, []byte("nonexistent"), 0)
		assert.Error(t, err)
	})
}

func TestBAPIAuthModule_InitGenesis(t *testing.T) {
	mod, accountStore := createTestModule(t)
	ctx := context.Background()

	t.Run("initializes empty genesis", func(t *testing.T) {
		err := mod.InitGenesis(ctx, nil)
		assert.NoError(t, err)

		err = mod.InitGenesis(ctx, []byte{})
		assert.NoError(t, err)
	})

	t.Run("initializes genesis accounts", func(t *testing.T) {
		genesisData := `{
			"accounts": [
				{
					"name": "genesis1",
					"authority": {
						"threshold": 1,
						"key_weights": {"key1": 1},
						"account_weights": {}
					},
					"nonce": 0,
					"created_at": "2024-01-01T00:00:00Z",
					"updated_at": "2024-01-01T00:00:00Z"
				}
			]
		}`

		err := mod.InitGenesis(ctx, []byte(genesisData))
		require.NoError(t, err)

		// Verify account was created
		account, err := accountStore.Get(ctx, "genesis1")
		require.NoError(t, err)
		assert.Equal(t, ptypes.AccountName("genesis1"), account.Name)
	})

	t.Run("fails with invalid JSON", func(t *testing.T) {
		err := mod.InitGenesis(ctx, []byte("invalid json"))
		assert.Error(t, err)
	})
}

func TestBAPIAuthModule_ExportGenesis(t *testing.T) {
	mod, _ := createTestModule(t)
	ctx := context.Background()

	t.Run("exports genesis", func(t *testing.T) {
		data, err := mod.ExportGenesis(ctx)
		require.NoError(t, err)
		assert.NotNil(t, data)
		// Should be valid JSON
		assert.Contains(t, string(data), "accounts")
	})
}

func TestBAPIAuthModule_InterfaceCompliance(t *testing.T) {
	mod, _ := createTestModule(t)

	// Verify the module implements all required interfaces
	var _ runtime.BAPIModule = mod
	var _ runtime.BAPIGenesisInitializer = mod
	var _ runtime.BAPIGenesisExporter = mod
}
