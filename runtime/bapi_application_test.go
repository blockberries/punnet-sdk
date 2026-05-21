package runtime

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
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

	// Pin the "historical queries surfaced as an error" semantic.
	// Previously the height was passed to handlers but silently ignored
	// (handlers read from a.stateStore which is always at current
	// version) — clients would think they got historical state. The
	// runtime now rejects mismatched heights explicitly.
	t.Run("rejects historical query for non-current height", func(t *testing.T) {
		// Bump the state store version so we can ask for a past one.
		// First commit advances version to 1; the test setup leaves us
		// at version 0, so request height 999 — guaranteed to differ.
		var futureHeight uint64 = 999
		result, err := app.Query(context.Background(), types.StateQuery{
			Path:   "/test/query",
			Height: &futureHeight,
		})
		require.NoError(t, err)
		assert.Equal(t, uint32(5), result.Code,
			"historical query should return code 5 (unsupported)")
		assert.Contains(t, result.Info, "historical queries not supported")
	})

	t.Run("accepts query with height == current version", func(t *testing.T) {
		current := uint64(ss.Version())
		result, err := app.Query(context.Background(), types.StateQuery{
			Path:   "/test/query",
			Height: &current,
		})
		require.NoError(t, err)
		assert.Equal(t, uint32(0), result.Code,
			"query at current height must succeed; info=%s", result.Info)
	})

	t.Run("accepts query with height == 0 (defaulted)", func(t *testing.T) {
		var zero uint64
		result, err := app.Query(context.Background(), types.StateQuery{
			Path:   "/test/query",
			Height: &zero,
		})
		require.NoError(t, err)
		assert.Equal(t, uint32(0), result.Code,
			"height=0 should be treated as 'use current'; info=%s", result.Info)
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

// --- Pending nonce tests ---

// testMsg is a minimal Message implementation for testing CheckTx.
type testMsg struct {
	Sender ptypes.AccountName `json:"sender"`
}

func (m *testMsg) Type() string                       { return "test/msg" }
func (m *testMsg) ValidateBasic() error                { return nil }
func (m *testMsg) GetSigners() []ptypes.AccountName    { return []ptypes.AccountName{m.Sender} }

func init() {
	ptypes.RegisterMessage("test/msg", func() ptypes.Message { return &testMsg{} })
}

// buildSignedTx creates a properly signed transaction for testing.
func buildSignedTx(account ptypes.AccountName, nonce uint64, privKey ed25519.PrivateKey) []byte {
	msg := &testMsg{Sender: account}
	tx := ptypes.NewTransaction(account, nonce, []ptypes.Message{msg}, nil)

	signBytes := tx.GetSignBytes()
	sig := ed25519.Sign(privKey, signBytes)
	pubKey := privKey.Public().(ed25519.PublicKey)

	tx.Authorization = &ptypes.Authorization{
		Signatures: []ptypes.Signature{
			{PubKey: []byte(pubKey), Signature: sig},
		},
	}

	data, err := json.Marshal(tx)
	if err != nil {
		panic(err)
	}
	return data
}

// setupAppWithAccount creates a BAPIApplication with a genesis account for testing.
func setupAppWithAccount(t *testing.T) (*BAPIApplication, ed25519.PrivateKey) {
	t.Helper()
	ss := createTestStateStore(t)

	// Generate key
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Register a handler for test/msg (no-op)
	mod := &testBAPIModule{
		name: "test",
		msgHandlers: map[string]BAPIMsgHandler{
			"test/msg": func(ctx context.Context, txCtx *BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
				return nil, nil
			},
		},
	}

	app, err := NewBAPIApplication(BAPIApplicationConfig{
		ChainID:    "test-chain",
		StateStore: ss,
		Modules:    []BAPIModule{mod},
	})
	require.NoError(t, err)

	// Genesis handshake
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
				PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: make([]byte, 32)},
				Power:  100,
			},
		},
		AppState: appState,
	}
	_, err = app.Handshake(context.Background(), types.HandshakeRequest{
		LastCommitted: nil,
		Genesis:       genesis,
	})
	require.NoError(t, err)

	// Create account directly via store
	account := &ptypes.Account{
		Name: "alice",
		Authority: ptypes.NewAuthority(1, pubKey, 1),
		Nonce: 0,
	}
	err = app.accountStore.Set(context.Background(), account)
	require.NoError(t, err)

	return app, privKey
}

// TestAnteHandler_Chain pins the executeTx wiring for the AnteHandler
// chain added in PLAN §7 Phase 0.4. Three cases:
//
//   1. Empty chain — handler doesn't run, behaviour is unchanged.
//   2. Single AnteHandler returning nil — runs exactly once per tx and
//      sees the right ptx.Account / ptx.Messages.
//   3. AnteHandler returning an error — tx fails with code 20 ("ante:
//      ..." prefix on Info) and no effects from later handlers commit.
//      Today this also rolls back any AnteHandler effects; Phase 1.5
//      will split the commit so fees persist.
func TestAnteHandler_Chain(t *testing.T) {
	ctx := context.Background()

	t.Run("empty chain — tx executes normally", func(t *testing.T) {
		app, privKey := setupAppWithAccount(t)
		// No AnteHandler registered; should execute as before.
		txBytes := buildSignedTx("alice", 0, privKey)
		outcome, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
			Height: 1,
			Txs:    []types.Tx{txBytes},
		})
		require.NoError(t, err)
		require.Len(t, outcome.TxOutcomes, 1)
		assert.Equal(t, uint32(0), outcome.TxOutcomes[0].Code,
			"empty AnteHandler chain should not affect tx outcome; got code %d info=%q",
			outcome.TxOutcomes[0].Code, outcome.TxOutcomes[0].Info)
	})

	t.Run("single AnteHandler runs once per tx", func(t *testing.T) {
		app, privKey := setupAppWithAccount(t)
		var fires int
		var sawAccount ptypes.AccountName
		app.RegisterAnteHandler(func(_ context.Context, txCtx *BAPITxContext, tx *ptypes.Transaction) ([]effects.Effect, error) {
			fires++
			sawAccount = tx.Account
			return nil, nil
		})
		txBytes := buildSignedTx("alice", 0, privKey)
		outcome, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
			Height: 1,
			Txs:    []types.Tx{txBytes},
		})
		require.NoError(t, err)
		require.Len(t, outcome.TxOutcomes, 1)
		assert.Equal(t, uint32(0), outcome.TxOutcomes[0].Code)
		assert.Equal(t, 1, fires, "AnteHandler should fire exactly once per tx")
		assert.Equal(t, ptypes.AccountName("alice"), sawAccount,
			"AnteHandler should see ptx.Account = signing account")
	})

	t.Run("AnteHandler error aborts tx with code 20", func(t *testing.T) {
		app, privKey := setupAppWithAccount(t)
		app.RegisterAnteHandler(func(_ context.Context, _ *BAPITxContext, _ *ptypes.Transaction) ([]effects.Effect, error) {
			return nil, fmt.Errorf("simulated ante rejection")
		})
		txBytes := buildSignedTx("alice", 0, privKey)
		outcome, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
			Height: 1,
			Txs:    []types.Tx{txBytes},
		})
		require.NoError(t, err)
		require.Len(t, outcome.TxOutcomes, 1)
		assert.Equal(t, uint32(20), outcome.TxOutcomes[0].Code,
			"AnteHandler error should produce code 20; got %d info=%q",
			outcome.TxOutcomes[0].Code, outcome.TxOutcomes[0].Info)
		assert.Contains(t, outcome.TxOutcomes[0].Info, "ante:",
			"info should be prefixed with 'ante:'")
		assert.Contains(t, outcome.TxOutcomes[0].Info, "simulated ante rejection")
	})

	t.Run("chain runs in registration order, first error short-circuits", func(t *testing.T) {
		app, privKey := setupAppWithAccount(t)
		var calls []string
		app.RegisterAnteHandler(func(_ context.Context, _ *BAPITxContext, _ *ptypes.Transaction) ([]effects.Effect, error) {
			calls = append(calls, "first")
			return nil, fmt.Errorf("stop here")
		})
		app.RegisterAnteHandler(func(_ context.Context, _ *BAPITxContext, _ *ptypes.Transaction) ([]effects.Effect, error) {
			calls = append(calls, "second")
			return nil, nil
		})
		txBytes := buildSignedTx("alice", 0, privKey)
		outcome, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
			Height: 1,
			Txs:    []types.Tx{txBytes},
		})
		require.NoError(t, err)
		require.Len(t, outcome.TxOutcomes, 1)
		assert.Equal(t, uint32(20), outcome.TxOutcomes[0].Code)
		assert.Equal(t, []string{"first"}, calls,
			"second AnteHandler should NOT run when first errors")
	})
}

func TestCheckTx_PendingNonces(t *testing.T) {
	app, privKey := setupAppWithAccount(t)
	ctx := context.Background()

	t.Run("accepts first tx with nonce 0", func(t *testing.T) {
		tx := buildSignedTx("alice", 0, privKey)
		verdict, err := app.CheckTx(ctx, tx, types.MempoolFirstSeen)
		require.NoError(t, err)
		assert.True(t, verdict.Accepted(), "nonce=0 should be accepted: %s", verdict.Info)
	})

	t.Run("accepts second tx with nonce 1", func(t *testing.T) {
		tx := buildSignedTx("alice", 1, privKey)
		verdict, err := app.CheckTx(ctx, tx, types.MempoolFirstSeen)
		require.NoError(t, err)
		assert.True(t, verdict.Accepted(), "nonce=1 should be accepted: %s", verdict.Info)
	})

	t.Run("accepts third tx with nonce 2", func(t *testing.T) {
		tx := buildSignedTx("alice", 2, privKey)
		verdict, err := app.CheckTx(ctx, tx, types.MempoolFirstSeen)
		require.NoError(t, err)
		assert.True(t, verdict.Accepted(), "nonce=2 should be accepted: %s", verdict.Info)
	})

	t.Run("rejects duplicate nonce", func(t *testing.T) {
		tx := buildSignedTx("alice", 1, privKey)
		verdict, err := app.CheckTx(ctx, tx, types.MempoolFirstSeen)
		require.NoError(t, err)
		assert.False(t, verdict.Accepted(), "duplicate nonce=1 should be rejected")
		assert.Contains(t, verdict.Info, "nonce mismatch")
	})

	t.Run("rejects gap nonce", func(t *testing.T) {
		tx := buildSignedTx("alice", 5, privKey)
		verdict, err := app.CheckTx(ctx, tx, types.MempoolFirstSeen)
		require.NoError(t, err)
		assert.False(t, verdict.Accepted(), "gap nonce=5 should be rejected")
		assert.Contains(t, verdict.Info, "nonce mismatch")
	})
}

func TestCheckTx_PendingNonces_CommitResets(t *testing.T) {
	app, privKey := setupAppWithAccount(t)
	ctx := context.Background()

	// Accept nonce 0 and 1
	tx0 := buildSignedTx("alice", 0, privKey)
	verdict, err := app.CheckTx(ctx, tx0, types.MempoolFirstSeen)
	require.NoError(t, err)
	require.True(t, verdict.Accepted())

	tx1 := buildSignedTx("alice", 1, privKey)
	verdict, err = app.CheckTx(ctx, tx1, types.MempoolFirstSeen)
	require.NoError(t, err)
	require.True(t, verdict.Accepted())

	// Commit (no block was executed, so committed nonce stays at 0)
	_, err = app.Commit(ctx)
	require.NoError(t, err)

	// After commit, pending nonces are cleared.
	// Committed nonce is 0, so nonce=0 should be accepted again.
	tx0Again := buildSignedTx("alice", 0, privKey)
	verdict, err = app.CheckTx(ctx, tx0Again, types.MempoolFirstSeen)
	require.NoError(t, err)
	assert.True(t, verdict.Accepted(), "nonce=0 should be accepted after commit reset: %s", verdict.Info)
}

// TestCheckTx_RevalidationDoesNotBumpPending pins the new behavior of
// checkTxAuthorization: a TX seen with MempoolRevalidation must NOT
// bump the pending nonce. The previous implementation bumped on every
// call, so a TX that survived multiple revalidation rounds would drift
// the per-account pending nonce upward and eventually reject legitimate
// sequential TXs as "nonce mismatch."
func TestCheckTx_RevalidationDoesNotBumpPending(t *testing.T) {
	app, privKey := setupAppWithAccount(t)
	ctx := context.Background()

	// First-seen at nonce 0 — bumps pending to 1.
	tx0 := buildSignedTx("alice", 0, privKey)
	v0, err := app.CheckTx(ctx, tx0, types.MempoolFirstSeen)
	require.NoError(t, err)
	require.True(t, v0.Accepted())

	// Revalidate the same TX several times. Each revalidation must
	// validate against the existing pending nonce but not advance it.
	for i := 0; i < 5; i++ {
		v, err := app.CheckTx(ctx, tx0, types.MempoolRevalidation)
		require.NoError(t, err)
		assert.True(t, v.Accepted(), "revalidation %d should re-accept: %s", i, v.Info)
	}

	// After 5 revalidations the pending nonce is still 1 (would be 6
	// under the old bump-every-call behavior). The next first-seen
	// TX at nonce 1 must therefore be accepted.
	tx1 := buildSignedTx("alice", 1, privKey)
	v1, err := app.CheckTx(ctx, tx1, types.MempoolFirstSeen)
	require.NoError(t, err)
	assert.True(t, v1.Accepted(),
		"nonce=1 must be accepted; pending nonce should still be 1 after revalidations (got: %s)",
		v1.Info)
}

// TestCheckTx_RevalidationAfterCommitAccepts pins the post-commit
// revalidation case: when a TX is still in the mempool at the moment
// of Commit (e.g. it didn't fit in the just-committed block), the
// next revalidation must accept it. Without this, the previous
// "tx.Nonce ∈ [committed, pending)" check would reject because
// pending was wiped on Commit to 0 (so the window is empty).
//
// The current semantic for revalidation is simply "tx.Nonce >=
// committed": the TX hasn't been executed yet, so it's still
// applicable. The mempool layer is the authoritative source for
// "which TXs are in flight"; the app just gates on freshness.
func TestCheckTx_RevalidationAfterCommitAccepts(t *testing.T) {
	app, privKey := setupAppWithAccount(t)
	ctx := context.Background()

	tx0 := buildSignedTx("alice", 0, privKey)
	v, err := app.CheckTx(ctx, tx0, types.MempoolFirstSeen)
	require.NoError(t, err)
	require.True(t, v.Accepted())

	// Simulate a block-commit cycle without actually executing tx0
	// (mimicking the case where tx0 didn't fit in the produced block
	// and is still pending in the mempool). Committed nonce stays 0.
	_, err = app.Commit(ctx)
	require.NoError(t, err)

	// The mempool revalidates tx0 — it must still be accepted
	// because committed nonce is 0 and tx0.Nonce is 0.
	v, err = app.CheckTx(ctx, tx0, types.MempoolRevalidation)
	require.NoError(t, err)
	assert.True(t, v.Accepted(),
		"revalidation post-commit should keep an unexecuted tx alive: %s", v.Info)
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
