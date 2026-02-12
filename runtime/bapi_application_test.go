package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestStateStore creates an in-memory state store for testing.
func createTestStateStore(t *testing.T) statestore.StateStore {
	store, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	return store
}

// testBAPIModule is a minimal module for testing.
type testBAPIModule struct {
	name         string
	msgHandlers  map[string]BAPIMsgHandler
	queryHandlers map[string]BAPIQueryHandler
}

func (m *testBAPIModule) Name() string {
	return m.name
}

func (m *testBAPIModule) RegisterMsgHandlers() map[string]BAPIMsgHandler {
	return m.msgHandlers
}

func (m *testBAPIModule) RegisterQueryHandlers() map[string]BAPIQueryHandler {
	return m.queryHandlers
}

func TestNewBAPIApplication(t *testing.T) {
	t.Run("creates application with valid config", func(t *testing.T) {
		ss := createTestStateStore(t)

		app, err := NewBAPIApplication(BAPIApplicationConfig{
			ChainID:    "test-chain",
			StateStore: ss,
			Modules:    []BAPIModule{},
		})

		require.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "test-chain", app.ChainID())
	})

	t.Run("fails with empty chain ID", func(t *testing.T) {
		ss := createTestStateStore(t)

		_, err := NewBAPIApplication(BAPIApplicationConfig{
			ChainID:    "",
			StateStore: ss,
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "chain ID")
	})

	t.Run("fails with nil state store", func(t *testing.T) {
		_, err := NewBAPIApplication(BAPIApplicationConfig{
			ChainID:    "test-chain",
			StateStore: nil,
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "state store")
	})

	t.Run("registers modules", func(t *testing.T) {
		ss := createTestStateStore(t)

		mod := &testBAPIModule{
			name:         "test",
			msgHandlers:  map[string]BAPIMsgHandler{},
			queryHandlers: map[string]BAPIQueryHandler{},
		}

		app, err := NewBAPIApplication(BAPIApplicationConfig{
			ChainID:    "test-chain",
			StateStore: ss,
			Modules:    []BAPIModule{mod},
		})

		require.NoError(t, err)
		assert.Len(t, app.Router().Modules(), 1)
	})
}

func TestBAPIApplication_Handshake_Genesis(t *testing.T) {
	ss := createTestStateStore(t)

	app, err := NewBAPIApplication(BAPIApplicationConfig{
		ChainID:    "test-chain",
		StateStore: ss,
		Modules:    []BAPIModule{},
	})
	require.NoError(t, err)

	// Create genesis with minimal app state
	appState, _ := json.Marshal(BAPIGenesisState{
		Modules: make(map[string]json.RawMessage),
	})

	genesis := &types.GenesisDoc{
		ChainID:       "test-chain",
		InitialHeight: 1,
		ConsensusParams: types.ConsensusParams{
			MaxBlockBytes: 22020096,
			MaxTxBytes:    1048576,
		},
		Validators: []types.ValidatorUpdate{
			{
				PubKey: types.PublicKey{
					Type: types.KeyTypeEd25519,
					Data: make([]byte, 32),
				},
				Power: 100,
			},
		},
		AppState: appState,
	}

	resp, err := app.Handshake(context.Background(), types.HandshakeRequest{
		LastCommitted: nil,
		Genesis:       genesis,
	})

	require.NoError(t, err)
	assert.Nil(t, resp.LastBlock) // Genesis returns nil LastBlock
	assert.NotNil(t, resp.AppHash)
}

func TestBAPIApplication_Handshake_Restart(t *testing.T) {
	ss := createTestStateStore(t)

	app, err := NewBAPIApplication(BAPIApplicationConfig{
		ChainID:    "test-chain",
		StateStore: ss,
		Modules:    []BAPIModule{},
	})
	require.NoError(t, err)

	// First do a genesis handshake
	appState, _ := json.Marshal(BAPIGenesisState{
		Modules: make(map[string]json.RawMessage),
	})

	genesis := &types.GenesisDoc{
		ChainID:       "test-chain",
		InitialHeight: 1,
		ConsensusParams: types.ConsensusParams{
			MaxBlockBytes: 22020096,
			MaxTxBytes:    1048576,
		},
		Validators: []types.ValidatorUpdate{
			{
				PubKey: types.PublicKey{
					Type: types.KeyTypeEd25519,
					Data: make([]byte, 32),
				},
				Power: 100,
			},
		},
		AppState: appState,
	}

	_, err = app.Handshake(context.Background(), types.HandshakeRequest{
		LastCommitted: nil,
		Genesis:       genesis,
	})
	require.NoError(t, err)

	// Now do a restart handshake
	resp, err := app.Handshake(context.Background(), types.HandshakeRequest{
		LastCommitted: &types.BlockID{
			Height: 1,
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp.LastBlock)
	assert.Equal(t, uint64(1), resp.LastBlock.Height)
}

func TestBAPIApplication_Query(t *testing.T) {
	ss := createTestStateStore(t)

	// Create a module with a query handler
	mod := &testBAPIModule{
		name: "test",
		queryHandlers: map[string]BAPIQueryHandler{
			"/test/query": func(ctx context.Context, data []byte, height int64) ([]byte, error) {
				return []byte("query result"), nil
			},
		},
	}

	app, err := NewBAPIApplication(BAPIApplicationConfig{
		ChainID:    "test-chain",
		StateStore: ss,
		Modules:    []BAPIModule{mod},
	})
	require.NoError(t, err)

	t.Run("handles valid query", func(t *testing.T) {
		result, err := app.Query(context.Background(), types.StateQuery{
			Path:  "/test/query",
			Data:  []byte("test data"),
			Prove: false,
		})

		require.NoError(t, err)
		assert.Equal(t, uint32(0), result.Code)
		assert.Equal(t, []byte("query result"), result.Value)
	})

	t.Run("returns error for unknown path", func(t *testing.T) {
		result, err := app.Query(context.Background(), types.StateQuery{
			Path: "/unknown/path",
		})

		require.NoError(t, err)
		assert.NotEqual(t, uint32(0), result.Code)
		assert.Contains(t, result.Info, "unknown query path")
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		result, err := app.Query(context.Background(), types.StateQuery{
			Path: "",
		})

		require.NoError(t, err)
		assert.NotEqual(t, uint32(0), result.Code)
		assert.Contains(t, result.Info, "empty query path")
	})
}

func TestBAPIRouter(t *testing.T) {
	t.Run("registers and retrieves msg handlers", func(t *testing.T) {
		router := NewBAPIRouter()

		handler := func(ctx context.Context, txCtx *BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
			return nil, nil
		}

		mod := &testBAPIModule{
			name: "test",
			msgHandlers: map[string]BAPIMsgHandler{
				"test_msg": handler,
			},
		}

		err := router.RegisterModule(mod)
		require.NoError(t, err)

		retrieved := router.GetMsgHandler("test_msg")
		assert.NotNil(t, retrieved)
	})

	t.Run("registers and retrieves query handlers", func(t *testing.T) {
		router := NewBAPIRouter()

		handler := func(ctx context.Context, data []byte, height int64) ([]byte, error) {
			return []byte("result"), nil
		}

		mod := &testBAPIModule{
			name: "test",
			queryHandlers: map[string]BAPIQueryHandler{
				"/test/path": handler,
			},
		}

		err := router.RegisterModule(mod)
		require.NoError(t, err)

		retrieved := router.GetQueryHandler("/test/path")
		assert.NotNil(t, retrieved)
	})

	t.Run("returns nil for unknown handlers", func(t *testing.T) {
		router := NewBAPIRouter()

		assert.Nil(t, router.GetMsgHandler("unknown"))
		assert.Nil(t, router.GetQueryHandler("/unknown"))
	})

	t.Run("returns modules list", func(t *testing.T) {
		router := NewBAPIRouter()

		mod1 := &testBAPIModule{name: "mod1"}
		mod2 := &testBAPIModule{name: "mod2"}

		router.RegisterModule(mod1)
		router.RegisterModule(mod2)

		modules := router.Modules()
		assert.Len(t, modules, 2)
	})
}

func TestBAPIBlockContext(t *testing.T) {
	ctx := &BAPIBlockContext{
		Height:  100,
		ChainID: "test-chain",
	}

	assert.Equal(t, uint64(100), ctx.Height)
	assert.Equal(t, "test-chain", ctx.ChainID)
}

func TestBAPITxContext(t *testing.T) {
	blockCtx := &BAPIBlockContext{
		Height:  100,
		ChainID: "test-chain",
	}

	txCtx := &BAPITxContext{
		BAPIBlockContext: blockCtx,
		Account:          "alice",
		TxIndex:          5,
	}

	assert.Equal(t, uint64(100), txCtx.Height)
	assert.Equal(t, "test-chain", txCtx.ChainID)
	assert.Equal(t, "alice", string(txCtx.Account))
	assert.Equal(t, uint32(5), txCtx.TxIndex)
}

func TestDeduplicateBAPIValidatorUpdates(t *testing.T) {
	t.Run("empty slice returns nil", func(t *testing.T) {
		result := deduplicateBAPIValidatorUpdates(nil)
		assert.Nil(t, result)

		result = deduplicateBAPIValidatorUpdates([]types.ValidatorUpdate{})
		assert.Nil(t, result)
	})

	t.Run("deduplicates by pubkey", func(t *testing.T) {
		updates := []types.ValidatorUpdate{
			{
				PubKey: types.PublicKey{Data: []byte("key1")},
				Power:  100,
			},
			{
				PubKey: types.PublicKey{Data: []byte("key1")},
				Power:  200, // Same key, different power
			},
			{
				PubKey: types.PublicKey{Data: []byte("key2")},
				Power:  150,
			},
		}

		result := deduplicateBAPIValidatorUpdates(updates)

		assert.Len(t, result, 2)
		// Last update for key1 wins
		assert.Equal(t, uint64(200), result[0].Power)
		assert.Equal(t, uint64(150), result[1].Power)
	})
}

func TestConvertToBAPIEvent(t *testing.T) {
	event := effects.Event{
		Type: "transfer",
		Attributes: map[string][]byte{
			"from":   []byte("alice"),
			"to":     []byte("bob"),
			"amount": []byte("100"),
		},
	}

	bapiEvent := convertToBAPIEvent(event)

	assert.Equal(t, "transfer", bapiEvent.Kind)
	assert.Len(t, bapiEvent.Attributes, 3)

	// Attributes should be sorted
	assert.Equal(t, "amount", bapiEvent.Attributes[0].Key)
	assert.Equal(t, "from", bapiEvent.Attributes[1].Key)
	assert.Equal(t, "to", bapiEvent.Attributes[2].Key)
}
