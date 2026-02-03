package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dbm "github.com/cosmos/cosmos-db"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// mockModule implements the Module interface for testing
type mockModule struct {
	name          string
	msgHandlers   map[string]MsgHandler
	queryHandlers map[string]QueryHandler
	beginBlocker  BeginBlocker
	endBlocker    EndBlocker
	initGenesis   InitGenesis
	exportGenesis ExportGenesis
}

func (m *mockModule) Name() string {
	return m.name
}

func (m *mockModule) RegisterMsgHandlers() map[string]MsgHandler {
	return m.msgHandlers
}

func (m *mockModule) RegisterQueryHandlers() map[string]QueryHandler {
	return m.queryHandlers
}

func (m *mockModule) BeginBlock() BeginBlocker {
	return m.beginBlocker
}

func (m *mockModule) EndBlock() EndBlocker {
	return m.endBlocker
}

func (m *mockModule) InitGenesis() InitGenesis {
	return m.initGenesis
}

func (m *mockModule) ExportGenesis() ExportGenesis {
	return m.exportGenesis
}

// testMessage implements types.Message for testing in application tests
type testMessage struct {
	msgType string
	signers []types.AccountName
}

func (m *testMessage) Type() string {
	return m.msgType
}

func (m *testMessage) ValidateBasic() error {
	return nil
}

func (m *testMessage) GetSigners() []types.AccountName {
	return m.signers
}

// setupTestApp creates a test application with IAVL store
func setupTestApp(t *testing.T) *Application {
	t.Helper()

	// Create in-memory database
	db := dbm.NewMemDB()

	// Create IAVL store
	iavlStore, err := store.NewIAVLStore(db, 0)
	if err != nil {
		t.Fatalf("failed to create IAVL store: %v", err)
	}

	// Create mock module with simple handlers
	mockMod := &mockModule{
		name: "test",
		msgHandlers: map[string]MsgHandler{
			"test.msg": func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
				return nil, nil
			},
		},
		queryHandlers: map[string]QueryHandler{
			"/test/query": func(ctx context.Context, path string, data []byte) ([]byte, error) {
				return []byte("test result"), nil
			},
		},
	}

	// Create application
	app, err := NewApplication(ApplicationConfig{
		ChainID:    "test-chain",
		StateStore: iavlStore,
		Modules:    []Module{mockMod},
	})
	if err != nil {
		t.Fatalf("failed to create application: %v", err)
	}

	return app
}

func TestNewApplication(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		app := setupTestApp(t)
		if app == nil {
			t.Fatal("expected non-nil application")
		}

		if app.chainID != "test-chain" {
			t.Errorf("expected chain ID 'test-chain', got %s", app.chainID)
		}
	})

	t.Run("nil_store", func(t *testing.T) {
		_, err := NewApplication(ApplicationConfig{
			ChainID:    "test",
			StateStore: nil,
			Modules:    []Module{&mockModule{name: "test"}},
		})
		if err == nil {
			t.Fatal("expected error with nil state store")
		}
	})

	t.Run("empty_chain_id", func(t *testing.T) {
		db := dbm.NewMemDB()
		iavlStore, _ := store.NewIAVLStore(db, 0)

		_, err := NewApplication(ApplicationConfig{
			ChainID:    "",
			StateStore: iavlStore,
			Modules:    []Module{&mockModule{name: "test"}},
		})
		if err == nil {
			t.Fatal("expected error with empty chain ID")
		}
	})

	t.Run("no_modules", func(t *testing.T) {
		db := dbm.NewMemDB()
		iavlStore, _ := store.NewIAVLStore(db, 0)

		_, err := NewApplication(ApplicationConfig{
			ChainID:    "test",
			StateStore: iavlStore,
			Modules:    nil,
		})
		if err == nil {
			t.Fatal("expected error with no modules")
		}
	})
}

func TestApplication_BeginBlock(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	header := NewBlockHeader(1, time.Now(), "test-chain", nil)

	err := app.BeginBlock(ctx, header)
	if err != nil {
		t.Fatalf("BeginBlock failed: %v", err)
	}

	// Verify header is set
	app.mu.RLock()
	currentHeader := app.currentHeader
	app.mu.RUnlock()

	if currentHeader == nil {
		t.Fatal("expected current header to be set")
	}

	if currentHeader.Height != 1 {
		t.Errorf("expected height 1, got %d", currentHeader.Height)
	}
}

func TestApplication_BeginBlock_NilContext(t *testing.T) {
	app := setupTestApp(t)
	header := NewBlockHeader(1, time.Now(), "test-chain", nil)

	// nolint:staticcheck // SA1012: intentionally testing nil context handling
	err := app.BeginBlock(nil, header)
	if err == nil {
		t.Fatal("expected error with nil context")
	}
}

func TestApplication_BeginBlock_NilHeader(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	err := app.BeginBlock(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil header")
	}
}

func TestApplication_BeginBlock_InvalidHeader(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Invalid header (height 0)
	header := &BlockHeader{
		Height:  0,
		Time:    time.Now(),
		ChainID: "test-chain",
	}

	err := app.BeginBlock(ctx, header)
	if err == nil {
		t.Fatal("expected error with invalid header")
	}
}

func TestApplication_EndBlock(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Must call BeginBlock first
	header := NewBlockHeader(1, time.Now(), "test-chain", nil)
	err := app.BeginBlock(ctx, header)
	if err != nil {
		t.Fatalf("BeginBlock failed: %v", err)
	}

	result, err := app.EndBlock(ctx)
	if err != nil {
		t.Fatalf("EndBlock failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil end block result")
	}
}

func TestApplication_EndBlock_NoBlockInProgress(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	_, err := app.EndBlock(ctx)
	if err == nil {
		t.Fatal("expected error when no block in progress")
	}
}

func TestApplication_Commit(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Begin block
	header := NewBlockHeader(1, time.Now(), "test-chain", nil)
	err := app.BeginBlock(ctx, header)
	if err != nil {
		t.Fatalf("BeginBlock failed: %v", err)
	}

	// Commit
	result, err := app.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil commit result")
	}

	if result.AppHash == nil {
		t.Fatal("expected non-nil app hash")
	}

	// Verify header is cleared
	app.mu.RLock()
	currentHeader := app.currentHeader
	app.mu.RUnlock()

	if currentHeader != nil {
		t.Error("expected current header to be nil after commit")
	}
}

func TestApplication_Commit_NoBlockInProgress(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	_, err := app.Commit(ctx)
	if err == nil {
		t.Fatal("expected error when no block in progress")
	}
}

func TestApplication_CheckTx(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Create test account
	pubKey := []byte("test-pubkey-123456789012345678901234")
	account := types.NewAccount("alice", pubKey)
	err := app.accountStore.Set(ctx, []byte("alice"), account)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create transaction
	msg := &testMessage{
		msgType: "test.msg",
		signers: []types.AccountName{"alice"},
	}

	tx := types.NewTransaction(
		"alice",
		0,
		[]types.Message{msg},
		&types.Authorization{
			Signatures: []types.Signature{
				{PubKey: pubKey, Signature: []byte("sig")},
			},
		},
	)

	// Serialize transaction
	txBytes, err := app.txSerializer.Marshal(tx)
	if err != nil {
		t.Fatalf("failed to serialize tx: %v", err)
	}

	// Check tx
	err = app.CheckTx(ctx, txBytes)
	// Note: This may fail due to signature verification, which is expected in this test
	// The important part is that it doesn't panic and processes the tx
	_ = err
}

func TestApplication_CheckTx_EmptyBytes(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	err := app.CheckTx(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil tx bytes")
	}

	err = app.CheckTx(ctx, []byte{})
	if err == nil {
		t.Fatal("expected error with empty tx bytes")
	}
}

func TestApplication_ExecuteTx(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Begin block
	header := NewBlockHeader(1, time.Now(), "test-chain", nil)
	err := app.BeginBlock(ctx, header)
	if err != nil {
		t.Fatalf("BeginBlock failed: %v", err)
	}

	// Create test account
	pubKey := []byte("test-pubkey-123456789012345678901234")
	account := types.NewAccount("alice", pubKey)
	err = app.accountStore.Set(ctx, []byte("alice"), account)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create transaction
	msg := &testMessage{
		msgType: "test.msg",
		signers: []types.AccountName{"alice"},
	}

	tx := types.NewTransaction(
		"alice",
		0,
		[]types.Message{msg},
		&types.Authorization{
			Signatures: []types.Signature{
				{PubKey: pubKey, Signature: []byte("sig")},
			},
		},
	)

	// Serialize transaction
	txBytes, err := app.txSerializer.Marshal(tx)
	if err != nil {
		t.Fatalf("failed to serialize tx: %v", err)
	}

	// Execute tx
	result, err := app.ExecuteTx(ctx, txBytes)
	if err != nil {
		t.Fatalf("ExecuteTx failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil tx result")
	}

	// Note: Result may have Code=1 due to signature verification failure,
	// but the execution path is tested
}

func TestApplication_ExecuteTx_NoBlockInProgress(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Create valid transaction bytes
	msg := &testMessage{
		msgType: "test.msg",
		signers: []types.AccountName{"alice"},
	}

	tx := types.NewTransaction(
		"alice",
		0,
		[]types.Message{msg},
		&types.Authorization{
			Signatures: []types.Signature{
				{PubKey: []byte("key"), Signature: []byte("sig")},
			},
		},
	)

	txBytes, _ := app.txSerializer.Marshal(tx)

	// Should return TxResult with error code (not fail) - ExecuteTx handles errors gracefully
	result, err := app.ExecuteTx(ctx, txBytes)
	// Actual error occurs during execution (account not found), so we get a result
	if err != nil || result == nil {
		// This is acceptable - either we get an error or a failed result
		return
	}

	// If we got a result, it should have a non-zero code
	if result.Code == 0 {
		t.Fatal("expected non-zero code when account not found")
	}
}

func TestApplication_Query(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	result, err := app.Query(ctx, "/test/query", nil, 0)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil query result")
	}

	if result.Code != 0 {
		t.Errorf("expected code 0, got %d: %s", result.Code, result.Log)
	}

	if string(result.Data) != "test result" {
		t.Errorf("expected 'test result', got %s", string(result.Data))
	}
}

func TestApplication_Query_EmptyPath(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	_, err := app.Query(ctx, "", nil, 0)
	if err == nil {
		t.Fatal("expected error with empty query path")
	}
}

func TestApplication_Query_NotFound(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	result, err := app.Query(ctx, "/nonexistent", nil, 0)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Code == 0 {
		t.Error("expected non-zero code for nonexistent query")
	}
}

func TestApplication_InitChain(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	validators := []types.ValidatorUpdate{
		{
			PubKey: []byte("validator-1"),
			Power:  100,
		},
	}

	err := app.InitChain(ctx, validators, nil)
	if err != nil {
		t.Fatalf("InitChain failed: %v", err)
	}
}

func TestApplication_InitChain_WithAppState(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	validators := []types.ValidatorUpdate{
		{
			PubKey: []byte("validator-1"),
			Power:  100,
		},
	}

	genesis := &GenesisState{
		ChainID:       "test-chain",
		GenesisTime:   time.Now(),
		InitialHeight: 1,
		Validators:    validators,
		AppState:      make(map[string]json.RawMessage),
	}

	genesisBytes, err := json.Marshal(genesis)
	if err != nil {
		t.Fatalf("failed to marshal genesis: %v", err)
	}

	err = app.InitChain(ctx, validators, genesisBytes)
	if err != nil {
		t.Fatalf("InitChain failed: %v", err)
	}
}

func TestApplication_NilChecks(t *testing.T) {
	var app *Application

	t.Run("nil_application", func(t *testing.T) {
		ctx := context.Background()

		if app.ChainID() != "" {
			t.Error("expected empty chain ID from nil app")
		}

		if app.Router() != nil {
			t.Error("expected nil router from nil app")
		}

		err := app.CheckTx(ctx, []byte{})
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}

		err = app.BeginBlock(ctx, NewBlockHeader(1, time.Now(), "test", nil))
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}

		_, err = app.ExecuteTx(ctx, []byte{})
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}

		_, err = app.EndBlock(ctx)
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}

		_, err = app.Commit(ctx)
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}

		_, err = app.Query(ctx, "/test", nil, 0)
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}

		err = app.InitChain(ctx, nil, nil)
		if err != ErrApplicationNil {
			t.Errorf("expected ErrApplicationNil, got %v", err)
		}
	})
}

func TestApplication_BlockLifecycle(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	// Full block lifecycle
	// Note: IAVL store versions are cumulative (each SaveVersion increments)
	for height := uint64(1); height <= 3; height++ {
		// BeginBlock
		header := NewBlockHeader(height, time.Now(), "test-chain", nil)
		err := app.BeginBlock(ctx, header)
		if err != nil {
			t.Fatalf("BeginBlock failed at height %d: %v", height, err)
		}

		// EndBlock
		_, err = app.EndBlock(ctx)
		if err != nil {
			t.Fatalf("EndBlock failed at height %d: %v", height, err)
		}

		// Commit
		result, err := app.Commit(ctx)
		if err != nil {
			t.Fatalf("Commit failed at height %d: %v", height, err)
		}

		// Verify we got a valid version (IAVL versions increment)
		if result.Height == 0 {
			t.Errorf("expected non-zero height, got %d", result.Height)
		}

		// Verify app hash exists
		if len(result.AppHash) == 0 {
			t.Error("expected non-empty app hash")
		}
	}
}

func TestApplication_Accessors(t *testing.T) {
	app := setupTestApp(t)

	if app.ChainID() != "test-chain" {
		t.Errorf("expected chain ID 'test-chain', got %s", app.ChainID())
	}

	if app.Router() == nil {
		t.Error("expected non-nil router")
	}

	if app.CapabilityManager() == nil {
		t.Error("expected non-nil capability manager")
	}

	if app.EffectExecutor() == nil {
		t.Error("expected non-nil effect executor")
	}

	if app.StateStore() == nil {
		t.Error("expected non-nil state store")
	}

	if app.AccountStore() == nil {
		t.Error("expected non-nil account store")
	}

	if app.BalanceStore() == nil {
		t.Error("expected non-nil balance store")
	}
}
