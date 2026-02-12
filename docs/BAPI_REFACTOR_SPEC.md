# Punnet SDK BAPI Refactor Specification

## Executive Summary

This specification details the refactoring of Punnet SDK to conform to the new BAPI (Block Application Programming Interface) and integrate with the updated Blockberry/Raspberry ecosystem. The refactor replaces the legacy IAVL-based storage with Blockberry's avlberry-backed StateStore, updates the application lifecycle to match BAPI's interface, and modifies Raspberry to use Punnet SDK modules instead of NoopApp.

**Key Changes:**
- Full lifecycle refactor from BeginBlock/ExecuteTx/EndBlock to BAPI's ExecuteBlock paradigm
- Direct use of Blockberry's StateStore (no custom IAVL wrapper)
- Full dependency injection for all components including capabilities
- Import and use BAPI types directly (no conversion layer)
- Modules interact only with typed stores (AccountStore, BalanceStore, ValidatorStore)
- Raspberry configures and registers modules explicitly
- Support for both in-process and gRPC deployment modes

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [BAPI Interface Implementation](#2-bapi-interface-implementation)
3. [Dependency Injection System](#3-dependency-injection-system)
4. [Storage Layer Refactor](#4-storage-layer-refactor)
5. [Module System Refactor](#5-module-system-refactor)
6. [Effect System Completion](#6-effect-system-completion)
7. [Core Modules Refactor](#7-core-modules-refactor)
8. [Query System](#8-query-system)
9. [Raspberry Integration](#9-raspberry-integration)
10. [Configuration](#10-configuration)
11. [Testing Strategy](#11-testing-strategy)
12. [Migration Plan](#12-migration-plan)
13. [Implementation Phases](#13-implementation-phases)

---

## 1. Architecture Overview

### 1.1 New Component Stack

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         RASPBERRY NODE                                   │
│  main.go → node.New() → WithApplication(punnetApp)                      │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      PUNNET SDK APPLICATION                              │
│  Implements: bapi.Lifecycle (required)                                   │
│  Optional: ProposalControl, VoteExtender, StateSync, Simulator          │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
          ┌────────────────────────┼────────────────────────┐
          ▼                        ▼                        ▼
┌─────────────────┐    ┌─────────────────────┐    ┌────────────────────┐
│  DI Container   │    │   Module Registry   │    │  Effect Executor   │
│  (punnet/di)    │    │   (punnet/module)   │    │  (punnet/effects)  │
└─────────────────┘    └─────────────────────┘    └────────────────────┘
          │                        │                        │
          ▼                        ▼                        ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         TYPED STORES LAYER                               │
│  AccountStore │ BalanceStore │ ValidatorStore │ ParamsStore             │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    BLOCKBERRY StateStore (avlberry)                      │
│                    Injected via DI Container                             │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.2 Key Design Principles

1. **Zero Conversion**: Import BAPI types directly. No wrapper types or conversion functions.
2. **Full DI**: All dependencies injected via interface markers. No global state.
3. **Typed Stores Only**: Modules never access raw StateStore. Only typed stores.
4. **Effect-Based Mutation**: All state changes go through the effect system.
5. **Collect-Then-Execute**: Effects collected during handler execution, validated, then executed atomically.
6. **Auditability**: All state access traceable through typed stores and effect logs.
7. **Zero-Copy**: Minimize allocations, use slice views where possible.
8. **Extensibility**: Easy to add new modules, new effect types, new capabilities.

### 1.3 Import Structure

```go
import (
    "github.com/blockberries/bapi"
    "github.com/blockberries/bapi/types"
    "github.com/blockberries/blockberry/pkg/statestore"
)
```

---

## 2. BAPI Interface Implementation

### 2.1 Lifecycle Interface (Required)

Punnet SDK's Application must implement `bapi.Lifecycle`:

```go
type Application struct {
    // Injected dependencies
    stateStore  statestore.StateStore

    // Internal components
    di          *di.Container
    modules     *module.Registry
    effects     *effects.Executor
    router      *Router

    // State
    currentBlock *types.FinalizedBlock
    appHash      types.AppHash

    // Stores (created from StateStore)
    accountStore   *store.AccountStore
    balanceStore   *store.BalanceStore
    validatorStore *store.ValidatorStore
    paramsStore    *store.ParamsStore
}

func (a *Application) Handshake(ctx context.Context, req types.HandshakeRequest) (types.HandshakeResponse, error)
func (a *Application) CheckTx(ctx context.Context, tx types.Tx, mctx types.MempoolContext) (types.GateVerdict, error)
func (a *Application) ExecuteBlock(ctx context.Context, block types.FinalizedBlock) (types.BlockOutcome, error)
func (a *Application) Commit(ctx context.Context) (types.CommitResult, error)
func (a *Application) Query(ctx context.Context, req types.StateQuery) (types.StateQueryResult, error)
```

### 2.2 Handshake Implementation

```go
func (a *Application) Handshake(ctx context.Context, req types.HandshakeRequest) (types.HandshakeResponse, error) {
    if req.LastCommitted == nil {
        // Genesis initialization
        if req.Genesis == nil {
            return types.HandshakeResponse{}, errors.New("genesis required for fresh start")
        }

        // Parse AppState JSON and dispatch to modules
        var genesisState GenesisState
        if err := json.Unmarshal(req.Genesis.AppState, &genesisState); err != nil {
            return types.HandshakeResponse{}, fmt.Errorf("parse genesis app state: %w", err)
        }

        // Initialize each module with its genesis data
        for _, mod := range a.modules.All() {
            if initializer, ok := mod.(module.GenesisInitializer); ok {
                if err := initializer.InitGenesis(ctx, genesisState.ModuleData[mod.Name()]); err != nil {
                    return types.HandshakeResponse{}, fmt.Errorf("init genesis for %s: %w", mod.Name(), err)
                }
            }
        }

        // Commit genesis state
        hash, _, err := a.stateStore.Commit()
        if err != nil {
            return types.HandshakeResponse{}, fmt.Errorf("commit genesis: %w", err)
        }
        copy(a.appHash[:], hash)

        return types.HandshakeResponse{
            LastBlock:    nil,
            AppHash:      &a.appHash,
            Capabilities: 0, // Lifecycle only for now
        }, nil
    }

    // Restart with existing state
    appVersion := a.stateStore.Version()
    requestedHeight := int64(req.LastCommitted.Height)

    if appVersion != requestedHeight {
        // Try to load the requested version
        if err := a.stateStore.LoadVersion(requestedHeight); err != nil {
            return types.HandshakeResponse{}, types.NewHaltError(
                uint64(appVersion),
                fmt.Sprintf("cannot load version %d, have %d: %v", requestedHeight, appVersion, err),
            )
        }
    }

    rootHash := a.stateStore.RootHash()
    copy(a.appHash[:], rootHash)

    return types.HandshakeResponse{
        LastBlock: &types.BlockID{
            Height: uint64(a.stateStore.Version()),
            Hash:   types.Hash(rootHash),
        },
        AppHash:      &a.appHash,
        Capabilities: 0,
    }, nil
}
```

### 2.3 CheckTx Implementation

```go
func (a *Application) CheckTx(ctx context.Context, tx types.Tx, mctx types.MempoolContext) (types.GateVerdict, error) {
    // Decode transaction
    ptx, err := a.decodeTx(tx)
    if err != nil {
        return types.GateVerdict{
            Code: 1,
            Info: fmt.Sprintf("decode error: %v", err),
        }, nil
    }

    // Validate authorization (signatures, nonces)
    if err := a.validateAuthorization(ctx, ptx); err != nil {
        return types.GateVerdict{
            Code: 2,
            Info: fmt.Sprintf("authorization failed: %v", err),
        }, nil
    }

    // Route to module for message-specific validation
    handler := a.router.GetMsgHandler(ptx.Message.Type())
    if handler == nil {
        return types.GateVerdict{
            Code: 3,
            Info: fmt.Sprintf("unknown message type: %s", ptx.Message.Type()),
        }, nil
    }

    // Validate without executing (no effects)
    if err := handler.Validate(ctx, ptx.Message); err != nil {
        return types.GateVerdict{
            Code: 4,
            Info: fmt.Sprintf("validation failed: %v", err),
        }, nil
    }

    return types.GateVerdict{
        Code:     0,
        Priority: 0, // Fixed priority
        Sender:   ptx.Account, // Account name for same-sender sequencing
    }, nil
}
```

### 2.4 ExecuteBlock Implementation

```go
func (a *Application) ExecuteBlock(ctx context.Context, block types.FinalizedBlock) (types.BlockOutcome, error) {
    a.currentBlock = &block

    // Create block context
    blockCtx := &BlockContext{
        Height:    block.Height,
        Time:      block.Time.ToTime(),
        Proposer:  block.Proposer,
    }

    // Process evidence (slashing)
    for _, evidence := range block.Evidence {
        if err := a.processEvidence(ctx, blockCtx, evidence); err != nil {
            return types.BlockOutcome{}, types.NewHaltError(block.Height,
                fmt.Sprintf("process evidence: %v", err))
        }
    }

    // Execute all transactions
    txOutcomes := make([]types.TxOutcome, len(block.Txs))
    blockEvents := []types.Event{}

    for i, tx := range block.Txs {
        outcome, events := a.executeTx(ctx, blockCtx, tx, uint32(i))
        txOutcomes[i] = outcome
        blockEvents = append(blockEvents, events...)

        // Continue on failure - record outcome but keep processing
        if outcome.Code != 0 {
            // Log failed transaction for debugging
            a.logFailedTx(tx, outcome)
        }
    }

    // Run module end-block hooks (validator updates, etc.)
    validatorUpdates, paramsUpdate, endBlockEvents, err := a.runEndBlockHooks(ctx, blockCtx)
    if err != nil {
        return types.BlockOutcome{}, types.NewHaltError(block.Height,
            fmt.Sprintf("end block hooks: %v", err))
    }
    blockEvents = append(blockEvents, endBlockEvents...)

    // Compute new app hash (but don't commit yet)
    // This is a "working" hash that will be finalized on Commit
    workingHash := a.computeWorkingHash()
    copy(a.appHash[:], workingHash)

    return types.BlockOutcome{
        TxOutcomes:       txOutcomes,
        BlockEvents:      blockEvents,
        AppHash:          a.appHash,
        ValidatorUpdates: validatorUpdates,
        ParamsUpdate:     paramsUpdate,
    }, nil
}

func (a *Application) executeTx(ctx context.Context, blockCtx *BlockContext, tx types.Tx, index uint32) (types.TxOutcome, []types.Event) {
    // Decode
    ptx, err := a.decodeTx(tx)
    if err != nil {
        return types.TxOutcome{
            Index: index,
            Code:  1,
            Info:  fmt.Sprintf("decode error: %v", err),
        }, nil
    }

    // Create transaction context
    txCtx := &TxContext{
        BlockContext: blockCtx,
        Account:      ptx.Account,
        TxIndex:      index,
    }

    // Get handler
    handler := a.router.GetMsgHandler(ptx.Message.Type())
    if handler == nil {
        return types.TxOutcome{
            Index: index,
            Code:  2,
            Info:  fmt.Sprintf("unknown message type: %s", ptx.Message.Type()),
        }, nil
    }

    // Execute handler - collect effects
    effects, err := handler.Execute(ctx, txCtx, ptx.Message)
    if err != nil {
        return types.TxOutcome{
            Index: index,
            Code:  3,
            Info:  fmt.Sprintf("handler error: %v", err),
        }, nil
    }

    // Validate all effects
    if err := a.effects.ValidateAll(effects); err != nil {
        return types.TxOutcome{
            Index: index,
            Code:  4,
            Info:  fmt.Sprintf("effect validation: %v", err),
        }, nil
    }

    // Check for critical invariant violations
    if violation := a.checkInvariants(effects); violation != nil {
        // This is a HaltError condition - should not happen in normal operation
        panic(fmt.Sprintf("critical invariant violation at height %d: %v", blockCtx.Height, violation))
    }

    // Execute all effects atomically
    events, err := a.effects.ExecuteAll(ctx, effects)
    if err != nil {
        return types.TxOutcome{
            Index: index,
            Code:  5,
            Info:  fmt.Sprintf("effect execution: %v", err),
        }, nil
    }

    return types.TxOutcome{
        Index:  index,
        Code:   0,
        Events: events,
    }, events
}
```

### 2.5 Commit Implementation

```go
func (a *Application) Commit(ctx context.Context) (types.CommitResult, error) {
    // Persist all state changes
    hash, version, err := a.stateStore.Commit()
    if err != nil {
        return types.CommitResult{}, fmt.Errorf("commit state: %w", err)
    }

    // Update app hash
    copy(a.appHash[:], hash)

    // Clear current block
    a.currentBlock = nil

    // Return retain height (keep last 100 blocks for queries)
    retainHeight := uint64(0)
    if version > 100 {
        retainHeight = uint64(version - 100)
    }

    return types.CommitResult{
        RetainHeight: retainHeight,
    }, nil
}
```

### 2.6 Query Implementation

See [Section 8: Query System](#8-query-system) for detailed query implementation.

---

## 3. Dependency Injection System

### 3.1 DI Container Design

Location: `punnet-sdk/di/`

```go
// Container is the dependency injection container
type Container struct {
    mu       sync.RWMutex
    services map[reflect.Type]any
    factories map[reflect.Type]func(*Container) (any, error)
}

// Register registers a service instance
func (c *Container) Register(service any) error

// RegisterFactory registers a factory function for lazy instantiation
func (c *Container) RegisterFactory(serviceType reflect.Type, factory func(*Container) (any, error)) error

// Resolve resolves a dependency by type
func (c *Container) Resolve(target any) error

// MustResolve resolves or panics
func (c *Container) MustResolve(target any)
```

### 3.2 Interface Markers

Modules declare dependencies via interface markers:

```go
// NeedsAccountStore is implemented by modules that need account access
type NeedsAccountStore interface {
    SetAccountStore(*store.AccountStore)
}

// NeedsBalanceStore is implemented by modules that need balance access
type NeedsBalanceStore interface {
    SetBalanceStore(*store.BalanceStore)
}

// NeedsValidatorStore is implemented by modules that need validator access
type NeedsValidatorStore interface {
    SetValidatorStore(*store.ValidatorStore)
}

// NeedsParamsStore is implemented by modules that need params access
type NeedsParamsStore interface {
    SetParamsStore(*store.ParamsStore)
}

// NeedsStateStore is implemented by modules that need raw state access (rare)
type NeedsStateStore interface {
    SetStateStore(statestore.StateStore)
}
```

### 3.3 Automatic Injection

```go
func (c *Container) InjectDependencies(module module.Module) error {
    if m, ok := module.(NeedsAccountStore); ok {
        var store *store.AccountStore
        if err := c.Resolve(&store); err != nil {
            return fmt.Errorf("resolve AccountStore: %w", err)
        }
        m.SetAccountStore(store)
    }

    if m, ok := module.(NeedsBalanceStore); ok {
        var store *store.BalanceStore
        if err := c.Resolve(&store); err != nil {
            return fmt.Errorf("resolve BalanceStore: %w", err)
        }
        m.SetBalanceStore(store)
    }

    // ... similar for other stores

    return nil
}
```

### 3.4 Container Setup

```go
func NewContainer(stateStore statestore.StateStore) *Container {
    c := &Container{
        services:  make(map[reflect.Type]any),
        factories: make(map[reflect.Type]func(*Container) (any, error)),
    }

    // Register the base state store
    c.Register(stateStore)

    // Register typed store factories
    c.RegisterFactory(reflect.TypeOf((*store.AccountStore)(nil)), func(c *Container) (any, error) {
        var ss statestore.StateStore
        if err := c.Resolve(&ss); err != nil {
            return nil, err
        }
        return store.NewAccountStore(ss, "accounts/"), nil
    })

    c.RegisterFactory(reflect.TypeOf((*store.BalanceStore)(nil)), func(c *Container) (any, error) {
        var ss statestore.StateStore
        if err := c.Resolve(&ss); err != nil {
            return nil, err
        }
        return store.NewBalanceStore(ss, "balances/"), nil
    })

    // ... similar for other stores

    return c
}
```

---

## 4. Storage Layer Refactor

### 4.1 Remove Custom IAVL Wrapper

**Delete:**
- `store/iavl_store.go`
- `store/memory_store.go` (replace with blockberry's in-memory store for testing)

**Keep (refactored):**
- `store/account_store.go`
- `store/balance_store.go`
- `store/validator_store.go`
- `store/serializer.go`

**Add:**
- `store/params_store.go`

### 4.2 Typed Store Interface

Each typed store wraps blockberry's StateStore with a key prefix:

```go
// AccountStore provides typed access to account data
type AccountStore struct {
    store  statestore.StateStore
    prefix string
    codec  Codec[Account]
}

func NewAccountStore(store statestore.StateStore, prefix string) *AccountStore {
    return &AccountStore{
        store:  store,
        prefix: prefix,
        codec:  CramberryCodec[Account]{},
    }
}

func (s *AccountStore) Get(ctx context.Context, name string) (*Account, error) {
    key := []byte(s.prefix + name)
    value, err := s.store.Get(key)
    if err != nil {
        return nil, fmt.Errorf("get account %s: %w", name, err)
    }
    if value == nil {
        return nil, ErrNotFound
    }
    return s.codec.Decode(value)
}

func (s *AccountStore) Set(ctx context.Context, name string, account *Account) error {
    key := []byte(s.prefix + name)
    value, err := s.codec.Encode(account)
    if err != nil {
        return fmt.Errorf("encode account: %w", err)
    }
    return s.store.Set(key, value)
}

func (s *AccountStore) Delete(ctx context.Context, name string) error {
    key := []byte(s.prefix + name)
    return s.store.Delete(key)
}

func (s *AccountStore) Has(ctx context.Context, name string) (bool, error) {
    key := []byte(s.prefix + name)
    return s.store.Has(key)
}

// GetWithProof returns the account with a Merkle proof (for key-value queries only)
func (s *AccountStore) GetWithProof(ctx context.Context, name string) (*Account, *statestore.Proof, error) {
    key := []byte(s.prefix + name)
    proof, err := s.store.GetProof(key)
    if err != nil {
        return nil, nil, fmt.Errorf("get proof: %w", err)
    }
    if !proof.Exists {
        return nil, proof, ErrNotFound
    }
    account, err := s.codec.Decode(proof.Value)
    if err != nil {
        return nil, nil, fmt.Errorf("decode account: %w", err)
    }
    return account, proof, nil
}

// GetAtHeight returns account at a specific block height (for historical queries)
func (s *AccountStore) GetAtHeight(ctx context.Context, name string, height int64) (*Account, error) {
    // This requires loading a specific version of the state store
    // Implementation depends on blockberry's version loading API
    if err := s.store.LoadVersion(height); err != nil {
        return nil, fmt.Errorf("load version %d: %w", height, err)
    }
    defer s.store.LoadVersion(s.store.Version()) // Restore current version

    return s.Get(ctx, name)
}
```

### 4.3 Codec Interface

```go
// Codec handles serialization/deserialization
type Codec[T any] interface {
    Encode(value *T) ([]byte, error)
    Decode(data []byte) (*T, error)
}

// CramberryCodec uses cramberry for serialization
type CramberryCodec[T any] struct{}

func (c CramberryCodec[T]) Encode(value *T) ([]byte, error) {
    return cramberry.Marshal(value)
}

func (c CramberryCodec[T]) Decode(data []byte) (*T, error) {
    var result T
    if err := cramberry.Unmarshal(data, &result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

### 4.4 Params Store

```go
// ParamsStore manages consensus parameters
type ParamsStore struct {
    store  statestore.StateStore
    prefix string
    codec  Codec[types.ConsensusParams]
}

func NewParamsStore(store statestore.StateStore, prefix string) *ParamsStore {
    return &ParamsStore{
        store:  store,
        prefix: prefix,
        codec:  CramberryCodec[types.ConsensusParams]{},
    }
}

func (s *ParamsStore) Get(ctx context.Context) (*types.ConsensusParams, error) {
    key := []byte(s.prefix + "current")
    value, err := s.store.Get(key)
    if err != nil {
        return nil, err
    }
    if value == nil {
        return nil, ErrNotFound
    }
    return s.codec.Decode(value)
}

func (s *ParamsStore) Set(ctx context.Context, params *types.ConsensusParams) error {
    key := []byte(s.prefix + "current")
    value, err := s.codec.Encode(params)
    if err != nil {
        return err
    }
    return s.store.Set(key, value)
}
```

---

## 5. Module System Refactor

### 5.1 New Module Interface

```go
// Module is the core module interface
type Module interface {
    Name() string
}

// MessageHandler handles a specific message type
type MessageHandler interface {
    // Type returns the message type this handler processes
    Type() string

    // Validate performs stateless validation (for CheckTx)
    Validate(ctx context.Context, msg Message) error

    // Execute processes the message and returns effects
    Execute(ctx context.Context, txCtx *TxContext, msg Message) ([]effects.Effect, error)
}

// QueryHandler handles a specific query type
type QueryHandler interface {
    // Type returns the query type this handler processes
    Type() string

    // Query executes the query and returns a response
    Query(ctx context.Context, req QueryRequest) (QueryResponse, error)
}

// BlockProcessor processes block-level events
type BlockProcessor interface {
    // ProcessBlock is called during ExecuteBlock with the full block
    ProcessBlock(ctx context.Context, block *types.FinalizedBlock) ([]effects.Effect, error)

    // EndBlock is called after all transactions are processed
    // Returns validator updates and any parameter changes
    EndBlock(ctx context.Context) ([]types.ValidatorUpdate, *types.ConsensusParams, []types.Event, error)
}

// GenesisInitializer handles genesis state initialization
type GenesisInitializer interface {
    // InitGenesis initializes module state from genesis data
    InitGenesis(ctx context.Context, data json.RawMessage) error

    // ExportGenesis exports current state for genesis
    ExportGenesis(ctx context.Context) (json.RawMessage, error)
}
```

### 5.2 ModuleBuilder (Retained and Extended)

```go
type ModuleBuilder struct {
    name            string
    msgHandlers     map[string]MessageHandler
    queryHandlers   map[string]QueryHandler
    blockProcessor  BlockProcessor
    genesisInit     GenesisInitializer

    // Dependency markers
    needsAccountStore   bool
    needsBalanceStore   bool
    needsValidatorStore bool
    needsParamsStore    bool
}

func NewModuleBuilder(name string) *ModuleBuilder {
    return &ModuleBuilder{
        name:          name,
        msgHandlers:   make(map[string]MessageHandler),
        queryHandlers: make(map[string]QueryHandler),
    }
}

func (b *ModuleBuilder) WithMsgHandler(handler MessageHandler) *ModuleBuilder {
    b.msgHandlers[handler.Type()] = handler
    return b
}

func (b *ModuleBuilder) WithQueryHandler(handler QueryHandler) *ModuleBuilder {
    b.queryHandlers[handler.Type()] = handler
    return b
}

func (b *ModuleBuilder) WithBlockProcessor(processor BlockProcessor) *ModuleBuilder {
    b.blockProcessor = processor
    return b
}

func (b *ModuleBuilder) WithGenesisInitializer(init GenesisInitializer) *ModuleBuilder {
    b.genesisInit = init
    return b
}

func (b *ModuleBuilder) RequiresAccountStore() *ModuleBuilder {
    b.needsAccountStore = true
    return b
}

func (b *ModuleBuilder) RequiresBalanceStore() *ModuleBuilder {
    b.needsBalanceStore = true
    return b
}

func (b *ModuleBuilder) RequiresValidatorStore() *ModuleBuilder {
    b.needsValidatorStore = true
    return b
}

func (b *ModuleBuilder) RequiresParamsStore() *ModuleBuilder {
    b.needsParamsStore = true
    return b
}

func (b *ModuleBuilder) Build() *BuiltModule {
    return &BuiltModule{
        name:                b.name,
        msgHandlers:         b.msgHandlers,
        queryHandlers:       b.queryHandlers,
        blockProcessor:      b.blockProcessor,
        genesisInit:         b.genesisInit,
        needsAccountStore:   b.needsAccountStore,
        needsBalanceStore:   b.needsBalanceStore,
        needsValidatorStore: b.needsValidatorStore,
        needsParamsStore:    b.needsParamsStore,
    }
}
```

### 5.3 Module Registry

```go
type Registry struct {
    modules       map[string]*BuiltModule
    orderedNames  []string // Topologically sorted
}

func NewRegistry() *Registry {
    return &Registry{
        modules: make(map[string]*BuiltModule),
    }
}

func (r *Registry) Register(module *BuiltModule) error {
    if _, exists := r.modules[module.Name()]; exists {
        return fmt.Errorf("module %s already registered", module.Name())
    }
    r.modules[module.Name()] = module
    r.orderedNames = append(r.orderedNames, module.Name())
    return nil
}

func (r *Registry) Get(name string) (*BuiltModule, bool) {
    m, ok := r.modules[name]
    return m, ok
}

func (r *Registry) All() []*BuiltModule {
    result := make([]*BuiltModule, 0, len(r.orderedNames))
    for _, name := range r.orderedNames {
        result = append(result, r.modules[name])
    }
    return result
}

// InjectDependencies injects stores into all modules
func (r *Registry) InjectDependencies(container *di.Container) error {
    for _, module := range r.modules {
        if err := container.InjectDependencies(module); err != nil {
            return fmt.Errorf("inject dependencies for %s: %w", module.Name(), err)
        }
    }
    return nil
}
```

---

## 6. Effect System Completion

### 6.1 Effect Interface (Refined)

```go
// Effect represents an intent to modify state
type Effect interface {
    // Type returns the effect type for routing
    Type() EffectType

    // Validate checks effect validity without execution
    Validate() error

    // Dependencies returns state keys this effect depends on
    Dependencies() []Dependency

    // Execute applies the effect to state
    Execute(ctx context.Context) ([]types.Event, error)
}

type EffectType uint8

const (
    EffectTypeRead EffectType = iota
    EffectTypeWrite
    EffectTypeDelete
    EffectTypeTransfer
    EffectTypeEvent
    EffectTypeValidatorUpdate
    EffectTypeParamsUpdate
)

type Dependency struct {
    Key       []byte
    WriteMode bool // true = write dependency, false = read dependency
}
```

### 6.2 WriteEffect (Fixed Implementation)

```go
// WriteEffect writes a typed value to state
type WriteEffect[T any] struct {
    store  TypedStore[T]
    key    string
    value  *T
}

func NewWriteEffect[T any](store TypedStore[T], key string, value *T) *WriteEffect[T] {
    return &WriteEffect[T]{
        store: store,
        key:   key,
        value: value,
    }
}

func (e *WriteEffect[T]) Type() EffectType {
    return EffectTypeWrite
}

func (e *WriteEffect[T]) Validate() error {
    if e.key == "" {
        return errors.New("empty key")
    }
    if e.value == nil {
        return errors.New("nil value")
    }
    return nil
}

func (e *WriteEffect[T]) Dependencies() []Dependency {
    return []Dependency{{Key: []byte(e.key), WriteMode: true}}
}

func (e *WriteEffect[T]) Execute(ctx context.Context) ([]types.Event, error) {
    if err := e.store.Set(ctx, e.key, e.value); err != nil {
        return nil, fmt.Errorf("write %s: %w", e.key, err)
    }
    return nil, nil // No events from write
}
```

### 6.3 TransferEffect (Fixed Implementation)

```go
// TransferEffect transfers tokens between accounts
type TransferEffect struct {
    balanceStore *store.BalanceStore
    from         string
    to           string
    amount       Coin
}

func NewTransferEffect(store *store.BalanceStore, from, to string, amount Coin) *TransferEffect {
    return &TransferEffect{
        balanceStore: store,
        from:         from,
        to:           to,
        amount:       amount,
    }
}

func (e *TransferEffect) Type() EffectType {
    return EffectTypeTransfer
}

func (e *TransferEffect) Validate() error {
    if e.from == "" || e.to == "" {
        return errors.New("empty account")
    }
    if e.amount.Amount.IsNegative() || e.amount.Amount.IsZero() {
        return errors.New("invalid amount")
    }
    return nil
}

func (e *TransferEffect) Dependencies() []Dependency {
    return []Dependency{
        {Key: []byte("balance/" + e.from + "/" + e.amount.Denom), WriteMode: true},
        {Key: []byte("balance/" + e.to + "/" + e.amount.Denom), WriteMode: true},
    }
}

func (e *TransferEffect) Execute(ctx context.Context) ([]types.Event, error) {
    // Debit from sender
    fromBalance, err := e.balanceStore.Get(ctx, e.from, e.amount.Denom)
    if err != nil && !errors.Is(err, store.ErrNotFound) {
        return nil, fmt.Errorf("get sender balance: %w", err)
    }
    if fromBalance == nil {
        fromBalance = &Balance{Account: e.from, Denom: e.amount.Denom, Amount: math.ZeroInt()}
    }

    newFromAmount := fromBalance.Amount.Sub(e.amount.Amount)
    if newFromAmount.IsNegative() {
        return nil, fmt.Errorf("insufficient balance: have %s, need %s", fromBalance.Amount, e.amount.Amount)
    }
    fromBalance.Amount = newFromAmount

    // Credit to recipient
    toBalance, err := e.balanceStore.Get(ctx, e.to, e.amount.Denom)
    if err != nil && !errors.Is(err, store.ErrNotFound) {
        return nil, fmt.Errorf("get recipient balance: %w", err)
    }
    if toBalance == nil {
        toBalance = &Balance{Account: e.to, Denom: e.amount.Denom, Amount: math.ZeroInt()}
    }

    newToAmount := toBalance.Amount.Add(e.amount.Amount)
    // Check for overflow
    if newToAmount.LT(toBalance.Amount) {
        return nil, errors.New("balance overflow")
    }
    toBalance.Amount = newToAmount

    // Persist changes
    if err := e.balanceStore.Set(ctx, e.from, e.amount.Denom, fromBalance); err != nil {
        return nil, fmt.Errorf("set sender balance: %w", err)
    }
    if err := e.balanceStore.Set(ctx, e.to, e.amount.Denom, toBalance); err != nil {
        return nil, fmt.Errorf("set recipient balance: %w", err)
    }

    // Return transfer event
    return []types.Event{{
        Kind: "transfer",
        Attributes: []types.EventAttribute{
            {Key: "from", Value: e.from, Index: true},
            {Key: "to", Value: e.to, Index: true},
            {Key: "amount", Value: e.amount.String(), Index: false},
        },
    }}, nil
}
```

### 6.4 EventEffect

```go
// EventEffect emits an event
type EventEffect struct {
    event types.Event
}

func NewEventEffect(kind string, attrs ...types.EventAttribute) *EventEffect {
    return &EventEffect{
        event: types.Event{
            Kind:       kind,
            Attributes: attrs,
        },
    }
}

func (e *EventEffect) Type() EffectType {
    return EffectTypeEvent
}

func (e *EventEffect) Validate() error {
    if e.event.Kind == "" {
        return errors.New("empty event kind")
    }
    return nil
}

func (e *EventEffect) Dependencies() []Dependency {
    return nil // Events have no state dependencies
}

func (e *EventEffect) Execute(ctx context.Context) ([]types.Event, error) {
    return []types.Event{e.event}, nil
}
```

### 6.5 Effect Executor

```go
type Executor struct {
    // For conflict detection
    accessLog map[string]bool // key -> was written?
}

func NewExecutor() *Executor {
    return &Executor{
        accessLog: make(map[string]bool),
    }
}

func (e *Executor) ValidateAll(effects []Effect) error {
    // Reset access log
    e.accessLog = make(map[string]bool)

    for i, effect := range effects {
        // Individual validation
        if err := effect.Validate(); err != nil {
            return fmt.Errorf("effect %d validation: %w", i, err)
        }

        // Conflict detection
        for _, dep := range effect.Dependencies() {
            key := string(dep.Key)

            if dep.WriteMode {
                // Check for write-write conflict
                if wasWritten, exists := e.accessLog[key]; exists && wasWritten {
                    // Allow - last write wins (for now)
                }
                e.accessLog[key] = true
            } else {
                // Read dependency - check if key was already written
                // (read-after-write in same tx is allowed)
            }
        }
    }

    return nil
}

func (e *Executor) ExecuteAll(ctx context.Context, effects []Effect) ([]types.Event, error) {
    var allEvents []types.Event

    for i, effect := range effects {
        events, err := effect.Execute(ctx)
        if err != nil {
            return nil, fmt.Errorf("effect %d execution: %w", i, err)
        }
        allEvents = append(allEvents, events...)
    }

    return allEvents, nil
}
```

---

## 7. Core Modules Refactor

### 7.1 Auth Module

```go
package auth

type Module struct {
    accountStore *store.AccountStore
}

func New() *Module {
    return &Module{}
}

// Implement NeedsAccountStore
func (m *Module) SetAccountStore(store *store.AccountStore) {
    m.accountStore = store
}

func (m *Module) Name() string {
    return "auth"
}

func (m *Module) Build() *module.BuiltModule {
    return module.NewModuleBuilder("auth").
        RequiresAccountStore().
        WithMsgHandler(&CreateAccountHandler{module: m}).
        WithMsgHandler(&UpdateAccountHandler{module: m}).
        WithQueryHandler(&AccountQueryHandler{module: m}).
        WithGenesisInitializer(m).
        Build()
}

// CreateAccountHandler
type CreateAccountHandler struct {
    module *Module
}

func (h *CreateAccountHandler) Type() string {
    return "auth/CreateAccount"
}

func (h *CreateAccountHandler) Validate(ctx context.Context, msg module.Message) error {
    m := msg.(*MsgCreateAccount)
    if m.Name == "" {
        return errors.New("empty account name")
    }
    if m.Authority == nil {
        return errors.New("nil authority")
    }
    return nil
}

func (h *CreateAccountHandler) Execute(ctx context.Context, txCtx *module.TxContext, msg module.Message) ([]effects.Effect, error) {
    m := msg.(*MsgCreateAccount)

    // Check account doesn't exist
    exists, err := h.module.accountStore.Has(ctx, m.Name)
    if err != nil {
        return nil, fmt.Errorf("check account exists: %w", err)
    }
    if exists {
        return nil, fmt.Errorf("account %s already exists", m.Name)
    }

    account := &Account{
        Name:      m.Name,
        Authority: m.Authority,
        Nonce:     0,
    }

    return []effects.Effect{
        effects.NewWriteEffect(h.module.accountStore, m.Name, account),
        effects.NewEventEffect("account_created",
            types.EventAttribute{Key: "name", Value: m.Name, Index: true},
        ),
    }, nil
}
```

### 7.2 Bank Module

```go
package bank

type Module struct {
    accountStore *store.AccountStore
    balanceStore *store.BalanceStore
}

func New() *Module {
    return &Module{}
}

func (m *Module) SetAccountStore(store *store.AccountStore) {
    m.accountStore = store
}

func (m *Module) SetBalanceStore(store *store.BalanceStore) {
    m.balanceStore = store
}

func (m *Module) Name() string {
    return "bank"
}

func (m *Module) Build() *module.BuiltModule {
    return module.NewModuleBuilder("bank").
        RequiresAccountStore().
        RequiresBalanceStore().
        WithMsgHandler(&SendHandler{module: m}).
        WithQueryHandler(&BalanceQueryHandler{module: m}).
        WithQueryHandler(&AllBalancesQueryHandler{module: m}).
        WithGenesisInitializer(m).
        Build()
}

// SendHandler
type SendHandler struct {
    module *Module
}

func (h *SendHandler) Type() string {
    return "bank/Send"
}

func (h *SendHandler) Validate(ctx context.Context, msg module.Message) error {
    m := msg.(*MsgSend)
    if m.From == "" || m.To == "" {
        return errors.New("empty account")
    }
    if len(m.Amount) == 0 {
        return errors.New("empty amount")
    }
    for _, coin := range m.Amount {
        if coin.Amount.IsNegative() || coin.Amount.IsZero() {
            return errors.New("invalid coin amount")
        }
    }
    return nil
}

func (h *SendHandler) Execute(ctx context.Context, txCtx *module.TxContext, msg module.Message) ([]effects.Effect, error) {
    m := msg.(*MsgSend)

    // Verify sender is the transaction account
    if m.From != txCtx.Account {
        return nil, errors.New("sender must be transaction account")
    }

    // Verify recipient account exists
    exists, err := h.module.accountStore.Has(ctx, m.To)
    if err != nil {
        return nil, fmt.Errorf("check recipient: %w", err)
    }
    if !exists {
        return nil, fmt.Errorf("recipient account %s does not exist", m.To)
    }

    // Create transfer effects for each coin
    var effs []effects.Effect
    for _, coin := range m.Amount {
        effs = append(effs, effects.NewTransferEffect(h.module.balanceStore, m.From, m.To, coin))
    }

    return effs, nil
}
```

### 7.3 Staking Module

```go
package staking

type Module struct {
    accountStore   *store.AccountStore
    balanceStore   *store.BalanceStore
    validatorStore *store.ValidatorStore

    // Accumulated validator updates during block
    pendingUpdates []types.ValidatorUpdate
}

func New() *Module {
    return &Module{}
}

func (m *Module) SetAccountStore(store *store.AccountStore)     { m.accountStore = store }
func (m *Module) SetBalanceStore(store *store.BalanceStore)     { m.balanceStore = store }
func (m *Module) SetValidatorStore(store *store.ValidatorStore) { m.validatorStore = store }

func (m *Module) Name() string {
    return "staking"
}

func (m *Module) Build() *module.BuiltModule {
    return module.NewModuleBuilder("staking").
        RequiresAccountStore().
        RequiresBalanceStore().
        RequiresValidatorStore().
        WithMsgHandler(&CreateValidatorHandler{module: m}).
        WithMsgHandler(&DelegateHandler{module: m}).
        WithMsgHandler(&UndelegateHandler{module: m}).
        WithBlockProcessor(m).
        WithQueryHandler(&ValidatorQueryHandler{module: m}).
        WithGenesisInitializer(m).
        Build()
}

// BlockProcessor implementation
func (m *Module) ProcessBlock(ctx context.Context, block *types.FinalizedBlock) ([]effects.Effect, error) {
    // Reset pending updates
    m.pendingUpdates = nil

    // Process evidence (slashing)
    for _, evidence := range block.Evidence {
        if evidence.Type == types.EvidenceTypeDuplicateVote {
            effs, err := m.slashValidator(ctx, evidence)
            if err != nil {
                return nil, fmt.Errorf("slash validator: %w", err)
            }
            // Note: slashing may add to pendingUpdates
            _ = effs
        }
    }

    return nil, nil
}

func (m *Module) EndBlock(ctx context.Context) ([]types.ValidatorUpdate, *types.ConsensusParams, []types.Event, error) {
    // Return accumulated validator updates
    updates := m.pendingUpdates
    m.pendingUpdates = nil

    var events []types.Event
    for _, update := range updates {
        events = append(events, types.Event{
            Kind: "validator_update",
            Attributes: []types.EventAttribute{
                {Key: "pubkey", Value: hex.EncodeToString(update.PubKey.Data), Index: true},
                {Key: "power", Value: fmt.Sprintf("%d", update.Power), Index: false},
            },
        })
    }

    return updates, nil, events, nil
}
```

### 7.4 Governance Module (New)

```go
package gov

type Module struct {
    accountStore *store.AccountStore
    paramsStore  *store.ParamsStore

    // Proposals stored inline for simplicity
    proposals map[uint64]*Proposal
    nextID    uint64
}

type Proposal struct {
    ID          uint64
    Title       string
    Description string
    Type        ProposalType
    Status      ProposalStatus
    ParamChange *ParamChangeProposal
    Votes       map[string]VoteOption // account -> vote
    SubmitTime  time.Time
    VotingEnd   time.Time
}

type ProposalType uint8

const (
    ProposalTypeParamChange ProposalType = iota
    ProposalTypeTextProposal
)

type ProposalStatus uint8

const (
    ProposalStatusVoting ProposalStatus = iota
    ProposalStatusPassed
    ProposalStatusRejected
)

func New() *Module {
    return &Module{
        proposals: make(map[uint64]*Proposal),
        nextID:    1,
    }
}

func (m *Module) SetAccountStore(store *store.AccountStore) { m.accountStore = store }
func (m *Module) SetParamsStore(store *store.ParamsStore)   { m.paramsStore = store }

func (m *Module) Name() string {
    return "gov"
}

func (m *Module) Build() *module.BuiltModule {
    return module.NewModuleBuilder("gov").
        RequiresAccountStore().
        RequiresParamsStore().
        WithMsgHandler(&SubmitProposalHandler{module: m}).
        WithMsgHandler(&VoteHandler{module: m}).
        WithBlockProcessor(m).
        WithQueryHandler(&ProposalQueryHandler{module: m}).
        WithGenesisInitializer(m).
        Build()
}

func (m *Module) EndBlock(ctx context.Context) ([]types.ValidatorUpdate, *types.ConsensusParams, []types.Event, error) {
    var paramsUpdate *types.ConsensusParams
    var events []types.Event

    // Check for passed proposals
    for _, proposal := range m.proposals {
        if proposal.Status == ProposalStatusVoting && time.Now().After(proposal.VotingEnd) {
            // Tally votes (simplified: majority wins)
            yesVotes, noVotes := 0, 0
            for _, vote := range proposal.Votes {
                if vote == VoteOptionYes {
                    yesVotes++
                } else {
                    noVotes++
                }
            }

            if yesVotes > noVotes {
                proposal.Status = ProposalStatusPassed

                // Apply param change if applicable
                if proposal.Type == ProposalTypeParamChange && proposal.ParamChange != nil {
                    paramsUpdate = proposal.ParamChange.NewParams
                }

                events = append(events, types.Event{
                    Kind: "proposal_passed",
                    Attributes: []types.EventAttribute{
                        {Key: "proposal_id", Value: fmt.Sprintf("%d", proposal.ID), Index: true},
                    },
                })
            } else {
                proposal.Status = ProposalStatusRejected
                events = append(events, types.Event{
                    Kind: "proposal_rejected",
                    Attributes: []types.EventAttribute{
                        {Key: "proposal_id", Value: fmt.Sprintf("%d", proposal.ID), Index: true},
                    },
                })
            }
        }
    }

    return nil, paramsUpdate, events, nil
}
```

---

## 8. Query System

### 8.1 Structured Query Types

```go
// QueryRequest is the base query request
type QueryRequest struct {
    Path   string          // Module/method path
    Data   json.RawMessage // Request-specific data
    Height *uint64         // Optional height for historical queries
    Prove  bool            // Include Merkle proof?
}

// QueryResponse is the base query response
type QueryResponse struct {
    Code   uint32
    Data   json.RawMessage
    Proof  *types.MerkleProof // Only for key-value queries with Prove=true
    Height uint64
    Info   string
}
```

### 8.2 Module Query Handlers

```go
// Auth module queries
type AccountQueryRequest struct {
    Name string `json:"name"`
}

type AccountQueryResponse struct {
    Account *Account `json:"account"`
}

type AccountQueryHandler struct {
    module *Module
}

func (h *AccountQueryHandler) Type() string {
    return "auth/account"
}

func (h *AccountQueryHandler) Query(ctx context.Context, req module.QueryRequest) (module.QueryResponse, error) {
    var qr AccountQueryRequest
    if err := json.Unmarshal(req.Data, &qr); err != nil {
        return module.QueryResponse{Code: 1, Info: "invalid request"}, nil
    }

    // Handle historical queries
    if req.Height != nil {
        account, err := h.module.accountStore.GetAtHeight(ctx, qr.Name, int64(*req.Height))
        if err != nil {
            return module.QueryResponse{Code: 2, Info: err.Error()}, nil
        }

        data, _ := json.Marshal(AccountQueryResponse{Account: account})
        return module.QueryResponse{
            Code:   0,
            Data:   data,
            Height: *req.Height,
        }, nil
    }

    // Handle proof requests
    if req.Prove {
        account, proof, err := h.module.accountStore.GetWithProof(ctx, qr.Name)
        if err != nil {
            return module.QueryResponse{Code: 2, Info: err.Error()}, nil
        }

        data, _ := json.Marshal(AccountQueryResponse{Account: account})
        return module.QueryResponse{
            Code:  0,
            Data:  data,
            Proof: convertProof(proof),
        }, nil
    }

    // Standard query
    account, err := h.module.accountStore.Get(ctx, qr.Name)
    if err != nil {
        return module.QueryResponse{Code: 2, Info: err.Error()}, nil
    }

    data, _ := json.Marshal(AccountQueryResponse{Account: account})
    return module.QueryResponse{
        Code: 0,
        Data: data,
    }, nil
}
```

### 8.3 Application Query Router

```go
func (a *Application) Query(ctx context.Context, req types.StateQuery) (types.StateQueryResult, error) {
    // Parse path: module/method
    parts := strings.SplitN(string(req.Path), "/", 2)
    if len(parts) != 2 {
        return types.StateQueryResult{
            Code: 1,
            Info: "invalid query path format, expected 'module/method'",
        }, nil
    }

    moduleName, method := parts[0], parts[1]

    // Find module
    mod, ok := a.modules.Get(moduleName)
    if !ok {
        return types.StateQueryResult{
            Code: 2,
            Info: fmt.Sprintf("module %s not found", moduleName),
        }, nil
    }

    // Find query handler
    handler := mod.GetQueryHandler(method)
    if handler == nil {
        return types.StateQueryResult{
            Code: 3,
            Info: fmt.Sprintf("query %s not found in module %s", method, moduleName),
        }, nil
    }

    // Execute query
    queryReq := module.QueryRequest{
        Path:   string(req.Path),
        Data:   req.Data,
        Height: req.Height,
        Prove:  req.Prove,
    }

    resp, err := handler.Query(ctx, queryReq)
    if err != nil {
        return types.StateQueryResult{
            Code: 4,
            Info: fmt.Sprintf("query error: %v", err),
        }, nil
    }

    return types.StateQueryResult{
        Code:   resp.Code,
        Value:  resp.Data,
        Height: a.stateStore.Version(),
        Proof:  resp.Proof,
        Info:   resp.Info,
    }, nil
}
```

---

## 9. Raspberry Integration

### 9.1 main.go Changes

```go
// raspberry/cmd/raspberry/main.go

package main

import (
    "github.com/blockberries/raspberry/internal/config"
    "github.com/blockberries/raspberry/internal/node"
    "github.com/blockberries/punnet-sdk"
    "github.com/blockberries/punnet-sdk/modules/auth"
    "github.com/blockberries/punnet-sdk/modules/bank"
    "github.com/blockberries/punnet-sdk/modules/staking"
    "github.com/blockberries/punnet-sdk/modules/gov"
)

func main() {
    // Load configuration
    cfg, err := config.LoadConfigFile("config.toml")
    if err != nil {
        log.Fatalf("load config: %v", err)
    }

    // Load punnet-sdk app configuration
    appCfg, err := punnet.LoadAppConfig("app.toml")
    if err != nil {
        log.Fatalf("load app config: %v", err)
    }

    // Create punnet application
    punnetApp, err := createPunnetApp(cfg, appCfg)
    if err != nil {
        log.Fatalf("create punnet app: %v", err)
    }

    // Create node with punnet application
    n, err := node.New(cfg, node.WithApplication(punnetApp))
    if err != nil {
        log.Fatalf("create node: %v", err)
    }

    // Run node
    if err := n.Start(context.Background()); err != nil {
        log.Fatalf("start node: %v", err)
    }
}

func createPunnetApp(nodeCfg *config.Config, appCfg *punnet.AppConfig) (*punnet.Application, error) {
    // Create state store from blockberry
    stateStore, err := statestore.NewIAVLStore(nodeCfg.Storage.StatePath)
    if err != nil {
        return nil, fmt.Errorf("create state store: %w", err)
    }

    // Create DI container with state store
    container := punnet.NewDIContainer(stateStore)

    // Create module registry
    registry := punnet.NewModuleRegistry()

    // Register core modules (order matters for genesis)
    registry.Register(auth.New().Build())
    registry.Register(bank.New().Build())
    registry.Register(staking.New().Build())
    registry.Register(gov.New().Build())

    // Inject dependencies into all modules
    if err := registry.InjectDependencies(container); err != nil {
        return nil, fmt.Errorf("inject dependencies: %w", err)
    }

    // Create application
    app, err := punnet.NewApplication(
        punnet.WithDIContainer(container),
        punnet.WithModuleRegistry(registry),
        punnet.WithConfig(appCfg),
    )
    if err != nil {
        return nil, fmt.Errorf("create application: %w", err)
    }

    return app, nil
}
```

### 9.2 RPC Endpoint Registration

```go
// raspberry/internal/rpc/punnet.go

package rpc

import (
    "encoding/json"
    "net/http"

    "github.com/blockberries/bapi/types"
)

// RegisterPunnetEndpoints adds module-specific RPC endpoints
func (s *Server) RegisterPunnetEndpoints() {
    // Auth endpoints
    s.mux.HandleFunc("/auth/account/{name}", s.handleAccountQuery)
    s.mux.HandleFunc("/auth/accounts", s.handleAccountsQuery)

    // Bank endpoints
    s.mux.HandleFunc("/bank/balances/{account}", s.handleBalancesQuery)
    s.mux.HandleFunc("/bank/balance/{account}/{denom}", s.handleBalanceQuery)

    // Staking endpoints
    s.mux.HandleFunc("/staking/validators", s.handleValidatorsQuery)
    s.mux.HandleFunc("/staking/validator/{address}", s.handleValidatorQuery)
    s.mux.HandleFunc("/staking/delegations/{delegator}", s.handleDelegationsQuery)

    // Gov endpoints
    s.mux.HandleFunc("/gov/proposals", s.handleProposalsQuery)
    s.mux.HandleFunc("/gov/proposal/{id}", s.handleProposalQuery)
}

func (s *Server) handleAccountQuery(w http.ResponseWriter, r *http.Request) {
    name := r.PathValue("name")

    req := types.StateQuery{
        Path: types.QueryPath("auth/account"),
        Data: json.RawMessage(fmt.Sprintf(`{"name":"%s"}`, name)),
    }

    result, err := s.node.Query(r.Context(), req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// ... similar handlers for other endpoints
```

---

## 10. Configuration

### 10.1 App Config File (app.toml)

```toml
# Punnet SDK Application Configuration

[app]
# Minimum gas prices for transaction acceptance
min_gas_prices = "0.001stake"

# Enable indexing of events
index_events = true

[modules]
# Module-specific configuration

[modules.auth]
# Account creation fee
account_creation_fee = "1000stake"

[modules.bank]
# Send enabled denoms (empty = all enabled)
send_enabled_denoms = []

[modules.staking]
# Unbonding time in seconds
unbonding_time = 1814400  # 21 days

# Maximum validators
max_validators = 100

# Bond denom
bond_denom = "stake"

[modules.gov]
# Minimum deposit for proposal
min_deposit = "10000stake"

# Voting period in seconds
voting_period = 604800  # 7 days

# Quorum percentage
quorum = 0.334

# Threshold percentage
threshold = 0.5
```

### 10.2 AppConfig Type

```go
package punnet

type AppConfig struct {
    App     AppSettings     `toml:"app"`
    Modules ModulesConfig   `toml:"modules"`
}

type AppSettings struct {
    MinGasPrices string `toml:"min_gas_prices"`
    IndexEvents  bool   `toml:"index_events"`
}

type ModulesConfig struct {
    Auth    AuthConfig    `toml:"auth"`
    Bank    BankConfig    `toml:"bank"`
    Staking StakingConfig `toml:"staking"`
    Gov     GovConfig     `toml:"gov"`
}

type AuthConfig struct {
    AccountCreationFee string `toml:"account_creation_fee"`
}

type BankConfig struct {
    SendEnabledDenoms []string `toml:"send_enabled_denoms"`
}

type StakingConfig struct {
    UnbondingTime int    `toml:"unbonding_time"`
    MaxValidators int    `toml:"max_validators"`
    BondDenom     string `toml:"bond_denom"`
}

type GovConfig struct {
    MinDeposit   string  `toml:"min_deposit"`
    VotingPeriod int     `toml:"voting_period"`
    Quorum       float64 `toml:"quorum"`
    Threshold    float64 `toml:"threshold"`
}

func LoadAppConfig(path string) (*AppConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config file: %w", err)
    }

    var cfg AppConfig
    if err := toml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }

    return &cfg, nil
}
```

---

## 11. Testing Strategy

### 11.1 Unit Tests

Each component should have comprehensive unit tests:

```go
// store/account_store_test.go
func TestAccountStore_CRUD(t *testing.T) {
    store := statestore.NewMemoryIAVLStore()
    accountStore := NewAccountStore(store, "accounts/")

    ctx := context.Background()

    // Create
    account := &Account{Name: "alice", Nonce: 0}
    err := accountStore.Set(ctx, "alice", account)
    require.NoError(t, err)

    // Read
    got, err := accountStore.Get(ctx, "alice")
    require.NoError(t, err)
    assert.Equal(t, account, got)

    // Update
    account.Nonce = 1
    err = accountStore.Set(ctx, "alice", account)
    require.NoError(t, err)

    got, err = accountStore.Get(ctx, "alice")
    require.NoError(t, err)
    assert.Equal(t, uint64(1), got.Nonce)

    // Delete
    err = accountStore.Delete(ctx, "alice")
    require.NoError(t, err)

    _, err = accountStore.Get(ctx, "alice")
    assert.ErrorIs(t, err, ErrNotFound)
}
```

### 11.2 BAPI Compliance Suite

```go
// tests/compliance_test.go
func TestBAPICompliance(t *testing.T) {
    bapitest.RunComplianceSuite(t, func() bapi.Lifecycle {
        store := statestore.NewMemoryIAVLStore()
        container := punnet.NewDIContainer(store)
        registry := punnet.NewModuleRegistry()

        registry.Register(auth.New().Build())
        registry.Register(bank.New().Build())
        registry.InjectDependencies(container)

        app, _ := punnet.NewApplication(
            punnet.WithDIContainer(container),
            punnet.WithModuleRegistry(registry),
        )

        return app
    })
}
```

### 11.3 Integration Tests

```go
// tests/integration/full_node_test.go
func TestFullNodeIntegration(t *testing.T) {
    // Create temp directories
    tmpDir := t.TempDir()

    // Create config
    cfg := createTestConfig(tmpDir)
    appCfg := createTestAppConfig()

    // Create app
    app := createTestApp(cfg, appCfg)

    // Create node
    n, err := node.New(cfg, node.WithApplication(app))
    require.NoError(t, err)

    // Start node
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    go n.Start(ctx)

    // Wait for node to start
    time.Sleep(2 * time.Second)

    // Test transaction submission
    tx := createTestTransaction(t)
    result, err := n.BroadcastTxSync(ctx, tx)
    require.NoError(t, err)
    assert.Equal(t, uint32(0), result.Code)

    // Wait for block
    time.Sleep(3 * time.Second)

    // Query result
    queryResult, err := n.Query(ctx, types.StateQuery{
        Path: "auth/account",
        Data: []byte(`{"name":"test"}`),
    })
    require.NoError(t, err)
    assert.Equal(t, uint32(0), queryResult.Code)
}
```

### 11.4 Benchmark Tests

```go
// tests/benchmark/checktx_test.go
func BenchmarkCheckTx(b *testing.B) {
    app := createBenchmarkApp()

    // Pre-create accounts
    setupBenchmarkState(app)

    tx := createBenchmarkTransaction()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := app.CheckTx(context.Background(), tx, types.MempoolFirstSeen)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkExecuteBlock(b *testing.B) {
    app := createBenchmarkApp()
    setupBenchmarkState(app)

    // Create block with 1000 transactions
    block := createBenchmarkBlock(1000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := app.ExecuteBlock(context.Background(), block)
        if err != nil {
            b.Fatal(err)
        }

        // Commit to reset state
        app.Commit(context.Background())

        // Increment block height for next iteration
        block.Height++
    }
}
```

---

## 12. Migration Plan

### 12.1 Files to Delete

```
punnet-sdk/store/iavl_store.go
punnet-sdk/store/memory_store.go
punnet-sdk/capability/capability.go (replace with DI)
punnet-sdk/capability/account.go
punnet-sdk/capability/balance.go
punnet-sdk/capability/validator.go
```

### 12.2 Files to Create

```
punnet-sdk/di/container.go
punnet-sdk/di/markers.go
punnet-sdk/store/params_store.go
punnet-sdk/modules/gov/module.go
punnet-sdk/modules/gov/handlers.go
punnet-sdk/modules/gov/types.go
raspberry/internal/rpc/punnet.go
```

### 12.3 Files to Heavily Modify

```
punnet-sdk/runtime/application.go (complete rewrite for bapi)
punnet-sdk/module/module.go (new interface)
punnet-sdk/module/builder.go (extend for new interfaces)
punnet-sdk/effects/executor.go (fix execution)
punnet-sdk/effects/effect.go (add new effect types)
punnet-sdk/store/account_store.go (use blockberry StateStore)
punnet-sdk/store/balance_store.go (use blockberry StateStore)
punnet-sdk/store/validator_store.go (use blockberry StateStore)
punnet-sdk/modules/auth/*.go (adapt to new interfaces)
punnet-sdk/modules/bank/*.go (adapt to new interfaces)
punnet-sdk/modules/staking/*.go (adapt to new interfaces)
raspberry/cmd/raspberry/main.go (punnet integration)
raspberry/internal/rpc/server.go (add punnet endpoints)
```

---

## 13. Implementation Phases

### Phase 1: Foundation

**Goal:** Get basic BAPI lifecycle working

1. Create DI container (`punnet-sdk/di/`)
2. Refactor storage layer to use blockberry StateStore
3. Implement basic BAPI Lifecycle interface
4. Create minimal auth module with new interfaces
5. Test with BAPI compliance suite

**Deliverables:**
- DI container with typed store factories
- AccountStore backed by blockberry StateStore
- Application implementing Handshake, CheckTx, ExecuteBlock, Commit, Query
- Auth module with CreateAccount handler
- Passing BAPI compliance tests

### Phase 2: Effect System

**Goal:** Complete effect system implementation

1. Fix WriteEffect to properly serialize values
2. Implement TransferEffect with overflow checking
3. Add effect validation and conflict detection
4. Implement EventEffect
5. Add ValidatorUpdateEffect for staking

**Deliverables:**
- Complete effect executor with validation
- All effect types properly implemented
- Effect dependency tracking
- Effect audit logging

### Phase 3: Core Modules

**Goal:** Refactor all core modules to new interfaces

1. Complete auth module (all handlers)
2. Complete bank module (all handlers)
3. Complete staking module (all handlers)
4. Create governance module
5. Add ParamsStore and params management

**Deliverables:**
- All four modules fully functional
- Genesis initialization for each module
- Query handlers for each module
- Validator updates from staking
- Param updates from governance

### Phase 4: Raspberry Integration

**Goal:** Full integration with Raspberry

1. Modify raspberry main.go to use punnet app
2. Add module-specific RPC endpoints
3. Create app.toml configuration
4. Write integration tests

**Deliverables:**
- Raspberry node running with punnet-sdk
- Module-specific RPC endpoints working
- Configuration system working
- Full integration test suite passing

### Phase 5: Production Hardening

**Goal:** Performance and reliability

1. Add benchmark tests
2. Optimize hot paths
3. Add comprehensive logging
4. Add metrics collection
5. Documentation

**Deliverables:**
- Benchmark results meeting targets
- Production-ready logging
- Prometheus metrics
- Complete documentation

---

## Appendix A: Type Mappings

| Punnet SDK Type | BAPI Type |
|-----------------|-----------|
| `types.TxResult` | `types.TxOutcome` |
| `types.BlockHeader` | `types.FinalizedBlock` |
| `types.CommitResult` | `types.CommitResult` |
| `types.QueryResult` | `types.StateQueryResult` |
| `types.Validator` | `types.ValidatorUpdate` |
| `types.Event` | `types.Event` |
| `types.EventAttribute` | `types.EventAttribute` |

## Appendix B: Import Changes

**Old imports:**
```go
import (
    "github.com/cosmos/iavl"
    "github.com/blockberries/punnet-sdk/types"
)
```

**New imports:**
```go
import (
    "github.com/blockberries/bapi"
    "github.com/blockberries/bapi/types"
    "github.com/blockberries/blockberry/pkg/statestore"
)
```

---

*End of Specification*
