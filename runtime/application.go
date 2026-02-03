package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

var (
	// ErrApplicationNil is returned when the application is nil
	ErrApplicationNil = fmt.Errorf("application is nil")

	// ErrNoModules is returned when no modules are registered
	ErrNoModules = fmt.Errorf("no modules registered")

	// ErrInvalidHeight is returned when an invalid height is provided
	ErrInvalidHeight = fmt.Errorf("invalid height")
)

// Application implements the Blockberry Application interface
// It coordinates transaction execution, module lifecycle, and state management
type Application struct {
	mu sync.RWMutex

	// router routes messages and queries to handlers
	router *Router

	// capabilityManager manages capabilities granted to modules
	capabilityManager *capability.CapabilityManager

	// effectExecutor executes effects against state stores
	effectExecutor *effects.Executor

	// stateStore is the underlying IAVL state storage
	stateStore *store.IAVLStore

	// accountStore provides typed account storage
	accountStore store.ObjectStore[*types.Account]

	// balanceStore provides balance operations
	balanceStore *store.BalanceStore

	// currentHeader is the current block header (nil between blocks)
	currentHeader *BlockHeader

	// chainID is the blockchain identifier
	chainID string

	// txSerializer handles transaction serialization
	txSerializer *store.JSONSerializer[*types.Transaction]

	// accountGetter adapts accountStore for authorization verification
	accountGetter types.AccountGetter
}

// iavlStoreAdapter adapts IAVLStore to effects.Store interface
type iavlStoreAdapter struct {
	store *store.IAVLStore
}

func (a *iavlStoreAdapter) Get(key []byte) ([]byte, error) {
	return a.store.Get(key)
}

func (a *iavlStoreAdapter) Set(key []byte, value []byte) error {
	return a.store.Set(key, value)
}

func (a *iavlStoreAdapter) Delete(key []byte) error {
	return a.store.Delete(key)
}

func (a *iavlStoreAdapter) Has(key []byte) bool {
	has, _ := a.store.Has(key)
	return has
}

// balanceStoreAdapter adapts store.BalanceStore to effects.BalanceStore interface
type balanceStoreAdapter struct {
	store *store.BalanceStore
}

func (a *balanceStoreAdapter) GetBalance(account types.AccountName, denom string) (uint64, error) {
	balance, err := a.store.Get(context.Background(), account, denom)
	if err != nil {
		return 0, err
	}
	return balance.Amount, nil
}

func (a *balanceStoreAdapter) SetBalance(account types.AccountName, denom string, amount uint64) error {
	balance := store.NewBalance(account, denom, amount)
	return a.store.Set(context.Background(), balance)
}

func (a *balanceStoreAdapter) SubBalance(account types.AccountName, denom string, amount uint64) error {
	return a.store.SubAmount(context.Background(), account, denom, amount)
}

func (a *balanceStoreAdapter) AddBalance(account types.AccountName, denom string, amount uint64) error {
	return a.store.AddAmount(context.Background(), account, denom, amount)
}

// accountGetterAdapter adapts ObjectStore to types.AccountGetter interface
type accountGetterAdapter struct {
	store store.ObjectStore[*types.Account]
}

func (a *accountGetterAdapter) GetAccount(name types.AccountName) (*types.Account, error) {
	// Use background context for authorization verification
	return a.store.Get(context.Background(), []byte(name))
}

// ApplicationConfig configures the application
type ApplicationConfig struct {
	// ChainID is the blockchain identifier
	ChainID string

	// StateStore is the backing IAVL store
	StateStore *store.IAVLStore

	// Modules are the modules to register
	Modules []Module
}

// NewApplication creates a new application
func NewApplication(config ApplicationConfig) (*Application, error) {
	if config.ChainID == "" {
		return nil, fmt.Errorf("chain ID cannot be empty")
	}

	if config.StateStore == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}

	if len(config.Modules) == 0 {
		return nil, ErrNoModules
	}

	// Create router
	router := NewRouter()

	// Create account store with JSON serializer
	// L1 cache: 1000 entries, L2 cache: 10000 entries
	accountStore := store.NewCachedObjectStore[*types.Account](
		config.StateStore,
		store.NewJSONSerializer[*types.Account](),
		1000,  // L1 cache size
		10000, // L2 cache size
	)

	// Create balance store
	balanceStore := store.NewBalanceStore(config.StateStore)

	// Create capability manager
	capMgr := capability.NewCapabilityManager(config.StateStore)

	// Create effect executor (wrapping IAVL store to match effects.Store interface)
	storeAdapter := &iavlStoreAdapter{store: config.StateStore}
	balanceStoreAdapter := &balanceStoreAdapter{store: balanceStore}
	executor, err := effects.NewExecutor(storeAdapter, balanceStoreAdapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create effect executor: %w", err)
	}

	// Register all modules
	for _, mod := range config.Modules {
		if err := router.RegisterModule(mod); err != nil {
			return nil, fmt.Errorf("failed to register module %s: %w", mod.Name(), err)
		}
	}

	// Create account getter adapter
	accountGetter := &accountGetterAdapter{store: accountStore}

	app := &Application{
		router:            router,
		capabilityManager: capMgr,
		effectExecutor:    executor,
		stateStore:        config.StateStore,
		accountStore:      accountStore,
		balanceStore:      balanceStore,
		chainID:           config.ChainID,
		txSerializer:      store.NewJSONSerializer[*types.Transaction](),
		accountGetter:     accountGetter,
	}

	return app, nil
}

// CheckTx performs lightweight validation of a transaction
// This is called during mempool admission and does not modify state
func (app *Application) CheckTx(ctx context.Context, txBytes []byte) error {
	if app == nil {
		return ErrApplicationNil
	}

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if len(txBytes) == 0 {
		return fmt.Errorf("transaction bytes cannot be empty")
	}

	// Deserialize transaction
	tx, err := app.txSerializer.Unmarshal(txBytes)
	if err != nil {
		return fmt.Errorf("failed to deserialize transaction: %w", err)
	}

	// Basic validation
	if err := tx.ValidateBasic(); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Get account for authorization check
	accountKey := []byte(tx.Account)
	account, err := app.accountStore.Get(ctx, accountKey)
	if err != nil {
		if err == store.ErrNotFound {
			return fmt.Errorf("account not found: %s", tx.Account)
		}
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Verify authorization using SignDoc-based verification
	// SECURITY: chainID binding prevents cross-chain replay attacks
	if err := tx.VerifyAuthorization(app.chainID, account, app.accountGetter); err != nil {
		return fmt.Errorf("authorization verification failed: %w", err)
	}

	// Create read-only context for message validation
	app.mu.RLock()
	header := app.currentHeader
	app.mu.RUnlock()

	if header == nil {
		// Use dummy header if not in block context
		header = NewBlockHeader(1, time.Now(), app.chainID, nil)
	}

	readOnlyCtx, err := NewReadOnlyContext(ctx, header, tx.Account)
	if err != nil {
		return fmt.Errorf("failed to create read-only context: %w", err)
	}

	// Validate all messages by routing them (handlers should validate)
	for _, msg := range tx.Messages {
		// Route message to handler (read-only, no effects)
		_, err := app.router.RouteMsg(readOnlyCtx, msg)
		if err != nil {
			return fmt.Errorf("message validation failed: %w", err)
		}
	}

	return nil
}

// BeginBlock is called at the beginning of each block
func (app *Application) BeginBlock(ctx context.Context, header *BlockHeader) error {
	if app == nil {
		return ErrApplicationNil
	}

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if header == nil {
		return fmt.Errorf("block header cannot be nil")
	}

	if err := header.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid block header: %w", err)
	}

	app.mu.Lock()
	app.currentHeader = header
	app.mu.Unlock()

	// Call module BeginBlock hooks
	return app.callBeginBlockers(ctx, header)
}

// ExecuteTx executes a transaction and returns the result
func (app *Application) ExecuteTx(ctx context.Context, txBytes []byte) (*types.TxResult, error) {
	if app == nil {
		return nil, ErrApplicationNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if len(txBytes) == 0 {
		return nil, fmt.Errorf("transaction bytes cannot be empty")
	}

	// Deserialize transaction
	tx, err := app.txSerializer.Unmarshal(txBytes)
	if err != nil {
		return &types.TxResult{
			Code: 1,
			Log:  fmt.Sprintf("failed to deserialize transaction: %v", err),
		}, nil
	}

	// Execute transaction
	return app.executeTx(ctx, tx)
}

// EndBlock is called at the end of each block
func (app *Application) EndBlock(ctx context.Context) (*types.EndBlockResult, error) {
	if app == nil {
		return nil, ErrApplicationNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	app.mu.RLock()
	header := app.currentHeader
	app.mu.RUnlock()

	if header == nil {
		return nil, fmt.Errorf("no block in progress")
	}

	// Call module EndBlock hooks
	return app.callEndBlockers(ctx, header)
}

// Commit commits the current state and returns the app hash
func (app *Application) Commit(ctx context.Context) (*types.CommitResult, error) {
	if app == nil {
		return nil, ErrApplicationNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	app.mu.RLock()
	header := app.currentHeader
	app.mu.RUnlock()

	if header == nil {
		return nil, fmt.Errorf("no block in progress")
	}

	// Flush all caches to IAVL
	if err := app.accountStore.Flush(ctx); err != nil {
		return nil, fmt.Errorf("failed to flush account store: %w", err)
	}

	if err := app.balanceStore.Flush(ctx); err != nil {
		return nil, fmt.Errorf("failed to flush balance store: %w", err)
	}

	// Commit IAVL state (save new version)
	hash, version, err := app.stateStore.SaveVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to commit state: %w", err)
	}

	// Clear current header
	app.mu.Lock()
	app.currentHeader = nil
	app.mu.Unlock()

	return &types.CommitResult{
		AppHash: hash,
		Height:  uint64(version),
	}, nil
}

// Query handles query requests
func (app *Application) Query(ctx context.Context, path string, data []byte, height int64) (*types.QueryResult, error) {
	if app == nil {
		return nil, ErrApplicationNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if path == "" {
		return nil, fmt.Errorf("query path cannot be empty")
	}

	if height < 0 {
		return nil, ErrInvalidHeight
	}

	// TODO: Support historical queries by loading specific IAVL version
	// For now, only support queries at current height

	// Route query to handler
	result, err := app.router.RouteQuery(ctx, path, data)
	if err != nil {
		return &types.QueryResult{
			Code: 1,
			Log:  fmt.Sprintf("query failed: %v", err),
		}, nil
	}

	return &types.QueryResult{
		Code:   0,
		Data:   result,
		Height: uint64(app.stateStore.Version()),
	}, nil
}

// InitChain initializes the blockchain from genesis
func (app *Application) InitChain(ctx context.Context, validators []types.ValidatorUpdate, appState []byte) error {
	if app == nil {
		return ErrApplicationNil
	}

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	// Initialize genesis state
	return app.initGenesis(ctx, validators, appState)
}

// ChainID returns the chain identifier
func (app *Application) ChainID() string {
	if app == nil {
		return ""
	}
	return app.chainID
}

// Router returns the message/query router
func (app *Application) Router() *Router {
	if app == nil {
		return nil
	}
	return app.router
}

// CapabilityManager returns the capability manager
func (app *Application) CapabilityManager() *capability.CapabilityManager {
	if app == nil {
		return nil
	}
	return app.capabilityManager
}

// EffectExecutor returns the effect executor
func (app *Application) EffectExecutor() *effects.Executor {
	if app == nil {
		return nil
	}
	return app.effectExecutor
}

// StateStore returns the underlying IAVL store
func (app *Application) StateStore() *store.IAVLStore {
	if app == nil {
		return nil
	}
	return app.stateStore
}

// AccountStore returns the account store
func (app *Application) AccountStore() store.ObjectStore[*types.Account] {
	if app == nil {
		return nil
	}
	return app.accountStore
}

// BalanceStore returns the balance store
func (app *Application) BalanceStore() *store.BalanceStore {
	if app == nil {
		return nil
	}
	return app.balanceStore
}

// executeTx executes a transaction and returns the result
func (app *Application) executeTx(ctx context.Context, tx *types.Transaction) (*types.TxResult, error) {
	// Validate transaction
	if err := tx.ValidateBasic(); err != nil {
		return &types.TxResult{
			Code: 1,
			Log:  fmt.Sprintf("transaction validation failed: %v", err),
		}, nil
	}

	// Get account
	accountKey := []byte(tx.Account)
	account, err := app.accountStore.Get(ctx, accountKey)
	if err != nil {
		if err == store.ErrNotFound {
			return &types.TxResult{
				Code: 1,
				Log:  fmt.Sprintf("account not found: %s", tx.Account),
			}, nil
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Verify authorization using SignDoc-based verification
	// SECURITY: chainID binding prevents cross-chain replay attacks
	if err := tx.VerifyAuthorization(app.chainID, account, app.accountGetter); err != nil {
		return &types.TxResult{
			Code: 1,
			Log:  fmt.Sprintf("authorization verification failed: %v", err),
		}, nil
	}

	// Create execution context
	app.mu.RLock()
	header := app.currentHeader
	app.mu.RUnlock()

	if header == nil {
		return nil, fmt.Errorf("no block in progress")
	}

	execCtx, err := NewContext(ctx, header, tx.Account)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution context: %w", err)
	}

	// Route all messages and collect effects
	var allEffects []effects.Effect
	for _, msg := range tx.Messages {
		msgEffects, err := app.router.RouteMsg(execCtx, msg)
		if err != nil {
			return &types.TxResult{
				Code: 1,
				Log:  fmt.Sprintf("message execution failed: %v", err),
			}, nil
		}
		allEffects = append(allEffects, msgEffects...)
	}

	// Add any context-emitted effects
	allEffects = append(allEffects, execCtx.CollectEffects()...)

	// Execute all effects
	execResult, err := app.effectExecutor.Execute(allEffects)
	if err != nil {
		return &types.TxResult{
			Code: 1,
			Log:  fmt.Sprintf("effect execution failed: %v", err),
		}, nil
	}

	// Increment account nonce
	account.Nonce++
	if err := app.accountStore.Set(ctx, accountKey, account); err != nil {
		return nil, fmt.Errorf("failed to update account nonce: %w", err)
	}

	// Convert execution events to transaction events
	txEvents := make([]types.Event, len(execResult.Events))
	for i, event := range execResult.Events {
		txEvent := types.NewEvent(event.Type)
		for key, value := range event.Attributes {
			txEvent.AddAttribute(key, value)
		}
		txEvents[i] = txEvent
	}

	return &types.TxResult{
		Code:    0,
		Log:     "transaction executed successfully",
		Events:  txEvents,
		GasUsed: execCtx.GasUsed(),
	}, nil
}
