# Punnet SDK Architecture

## Executive Summary

Punnet SDK is a next-generation blockchain application framework built on the Blockberries ecosystem. It introduces a novel **effect-based module system** with **capability security**, **zero-copy operations**, and **parallel transaction execution**. Unlike traditional blockchain frameworks, Punnet SDK uses modern patterns from game engines (ECS), functional programming (effect systems), and high-performance computing (object pooling, cache-first design).

**Core Innovations:**
1. **Effect System**: Explicit, composable side effects with automatic parallelization
2. **Capability Security**: Modules receive limited capabilities, not global store access
3. **Object Stores**: Typed, cached storage with automatic serialization
4. **Declarative Modules**: Builder pattern for ergonomic module creation
5. **Zero-Copy**: Memory-efficient operations with object pooling
6. **Multi-Level Caching**: Account, balance, and validator caches

---

## Design Philosophy

### Core Principles

1. **Composition over Inheritance**: Modules compose behaviors through traits and capabilities
2. **Explicit over Implicit**: All dependencies and side effects are traceable
3. **Performance by Default**: Zero-copy, parallel execution, cache-friendly structures
4. **Developer Ergonomics**: Module creation in minutes with declarative builders
5. **Type Safety**: Compile-time guarantees via generics and strict interfaces

### Novel Concepts

| Concept | Traditional Approach | Punnet SDK Approach |
|---------|---------------------|---------------------|
| **State Access** | Direct KVStore | Capability-based object stores |
| **Side Effects** | Implicit in function calls | Explicit effect collection |
| **Parallelization** | Sequential execution | Automatic dependency analysis |
| **Caching** | Manual caching | Automatic multi-level caching |
| **Module Composition** | Keeper passing | Trait composition |
| **Memory Management** | Allocate-per-request | Object pooling, zero-copy |

---

## Architecture Overview

### System Layers

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          MODULE LAYER                                    │
│  (User-defined modules using SDK primitives)                            │
│                                                                          │
│  module := NewModuleBuilder("mymodule").                                │
│      WithDependency("auth", "bank").                                    │
│      WithMsgHandler("Send", handleSend).                                │
│      WithQueryHandler("Balance", queryBalance).                         │
│      WithBeginBlocker(onBeginBlock).                                    │
│      Build()                                                            │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────┐
│                         RUNTIME LAYER                                    │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                      Effect Executor                                │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │ │
│  │  │ Dependency   │  │  Parallel    │  │   Effect     │             │ │
│  │  │  Analysis    │  │  Scheduler   │  │  Validation  │             │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘             │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                     Capability Manager                              │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │ │
│  │  │  AccountCap  │  │  BalanceCap  │  │ ValidatorCap │             │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘             │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                     Message Router                                  │ │
│  │  ┌──────────────────────────────────────────────────────────────┐  │ │
│  │  │ Type-Safe Routing with Generics                               │  │ │
│  │  └──────────────────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────┐
│                         STORAGE LAYER                                    │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                      Object Stores                                  │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │ │
│  │  │  AccountStore│  │ BalanceStore │  │ValidatorStore│             │ │
│  │  │  (Typed)     │  │  (Typed)     │  │  (Typed)     │             │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘             │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                      Cache Layer                                    │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐             │ │
│  │  │ L1: Hot Cache│  │ L2: Warm     │  │ L3: Disk     │             │ │
│  │  │ (LRU 10k)    │  │ (LRU 100k)   │  │ (IAVL)       │             │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘             │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                      Object Pool                                    │ │
│  │  Pre-allocated objects for zero-copy operations                    │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────┐
│                      BLOCKBERRY (via ABI)                                │
│               StateStore (IAVL), BlockStore, Mempool                    │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Effect System

### Core Concept

Instead of directly mutating state, modules declare **effects** that describe their intent. The runtime collects, validates, and executes effects with automatic:
- Dependency detection for parallel execution
- Conflict detection
- Caching
- Batched writes

### Effect Types

```go
// Effect is the base interface for all side effects
type Effect interface {
    Type() EffectType
    Validate() error
    Dependencies() []Dependency
}

type EffectType uint8

const (
    EffectTypeRead EffectType = iota
    EffectTypeWrite
    EffectTypeTransfer
    EffectTypeEvent
    EffectTypeDelegate
    EffectTypeSlash
)

// ReadEffect declares intent to read state
type ReadEffect[T any] struct {
    Store string      // e.g., "account", "balance"
    Key   []byte
    Dest  *T          // Where to store result
}

func (e ReadEffect[T]) Type() EffectType { return EffectTypeRead }

// WriteEffect declares intent to write state
type WriteEffect[T any] struct {
    Store string
    Key   []byte
    Value T
}

func (e WriteEffect[T]) Type() EffectType { return EffectTypeWrite }

// TransferEffect is a high-level effect for token transfers
type TransferEffect struct {
    From   AccountName
    To     AccountName
    Amount Coins
}

func (e TransferEffect) Type() EffectType { return EffectTypeTransfer }

func (e TransferEffect) Dependencies() []Dependency {
    return []Dependency{
        {Type: DependencyTypeAccount, Key: []byte(e.From)},
        {Type: DependencyTypeAccount, Key: []byte(e.To)},
        {Type: DependencyTypeBalance, Key: balanceKey(e.From, e.Amount.Denom)},
    }
}

// EventEffect declares intent to emit an event
type EventEffect struct {
    Type       string
    Attributes map[string][]byte
}

func (e EventEffect) Type() EffectType { return EffectTypeEvent }
```

### Effect Collection

```go
type EffectCollector struct {
    effects []Effect
    mu      sync.Mutex
}

func (c *EffectCollector) Add(effect Effect) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.effects = append(c.effects, effect)
}

func (c *EffectCollector) Collect() []Effect {
    c.mu.Lock()
    defer c.mu.Unlock()
    result := c.effects
    c.effects = nil
    return result
}
```

### Effect Execution

```go
type EffectExecutor struct {
    stores    map[string]ObjectStore
    cache     *CacheManager
    scheduler *ParallelScheduler
}

func (e *EffectExecutor) Execute(effects []Effect) error {
    // 1. Validate all effects
    for _, eff := range effects {
        if err := eff.Validate(); err != nil {
            return err
        }
    }

    // 2. Build dependency graph
    graph := e.buildDependencyGraph(effects)

    // 3. Detect conflicts (read-write, write-write)
    if conflicts := graph.DetectConflicts(); len(conflicts) > 0 {
        return fmt.Errorf("conflicting effects: %v", conflicts)
    }

    // 4. Schedule parallel execution
    groups := e.scheduler.Schedule(graph)

    // 5. Execute effect groups in parallel
    for _, group := range groups {
        if err := e.executeGroup(group); err != nil {
            return err
        }
    }

    return nil
}
```

---

## Capability System

### Core Concept

Modules don't get direct store access. Instead, they receive **capabilities** that limit what they can do. This provides:
- **Security**: Modules can't access state they shouldn't
- **Auditability**: All state access is traceable
- **Performance**: Capabilities cache aggressively

### Capability Types

```go
// Capability is granted to modules for specific operations
type Capability[T any] interface {
    Read(key []byte) (*T, error)
    Write(key []byte, value T) error
    Delete(key []byte) error
    Iterate(start, end []byte) Iterator[T]
}

// AccountCapability for account operations
type AccountCapability interface {
    Capability[Account]

    // High-level operations
    GetAccount(name AccountName) (*Account, error)
    CreateAccount(name AccountName, authority Authority) error
    UpdateAuthority(name AccountName, authority Authority) error
    VerifyAuth(name AccountName, auth Authorization) error
}

// BalanceCapability for balance operations
type BalanceCapability interface {
    Capability[Balance]

    // High-level operations
    GetBalance(account AccountName, denom string) (uint64, error)
    AddBalance(account AccountName, coin Coin) error
    SubBalance(account AccountName, coin Coin) error
    Transfer(from, to AccountName, amount Coins) error
}

// ValidatorCapability for validator operations
type ValidatorCapability interface {
    Capability[Validator]

    // High-level operations
    GetValidator(name AccountName) (*Validator, error)
    SetValidator(val Validator) error
    IterateValidators(fn func(Validator) bool) error
    GetValidatorSet() ([]Validator, error)
}
```

### Capability Granting

```go
type CapabilityManager struct {
    stores map[string]ObjectStore
    cache  *CacheManager
}

func (cm *CapabilityManager) GrantAccountCap(module string) AccountCapability {
    return &accountCap{
        module: module,
        store:  cm.stores["account"],
        cache:  cm.cache.GetCache("account"),
    }
}

func (cm *CapabilityManager) GrantBalanceCap(module string) BalanceCapability {
    return &balanceCap{
        module: module,
        store:  cm.stores["balance"],
        cache:  cm.cache.GetCache("balance"),
    }
}

// Capabilities automatically:
// - Cache reads
// - Batch writes
// - Track access for auditing
// - Enforce module-specific permissions
```

---

## Object Store System

### Typed Object Stores

Instead of raw KVStore, use typed object stores with automatic serialization:

```go
type ObjectStore[T any] interface {
    Get(key []byte) (*T, error)
    Set(key []byte, value T) error
    Delete(key []byte) error
    Has(key []byte) bool
    Iterator(start, end []byte) Iterator[T]

    // Batch operations for efficiency
    GetBatch(keys [][]byte) ([]*T, error)
    SetBatch(items map[string]T) error
}

type Iterator[T any] interface {
    Valid() bool
    Next()
    Key() []byte
    Value() T
    Close()
}

// Implementation with automatic caching
type CachedObjectStore[T any] struct {
    underlying KVStore  // IAVL-backed
    codec      Codec[T] // Cramberry serialization
    cache      Cache[T] // LRU cache

    // Object pool for zero-copy
    pool sync.Pool
}

func (s *CachedObjectStore[T]) Get(key []byte) (*T, error) {
    // 1. Check cache
    if cached, ok := s.cache.Get(string(key)); ok {
        return cached, nil
    }

    // 2. Read from underlying store
    data := s.underlying.Get(key)
    if data == nil {
        return nil, ErrNotFound
    }

    // 3. Get object from pool
    obj := s.pool.Get().(*T)

    // 4. Deserialize in-place (zero-copy where possible)
    if err := s.codec.Unmarshal(data, obj); err != nil {
        s.pool.Put(obj)
        return nil, err
    }

    // 5. Cache and return
    s.cache.Set(string(key), obj)
    return obj, nil
}
```

### Pre-Built Object Stores

```go
// AccountStore for named accounts
type AccountStore = ObjectStore[Account]

// BalanceStore for token balances
type BalanceStore = ObjectStore[Balance]

// ValidatorStore for validator state
type ValidatorStore = ObjectStore[Validator]

// DelegationStore for staking delegations
type DelegationStore = ObjectStore[Delegation]

// Creating an object store
func NewAccountStore(kvstore KVStore) AccountStore {
    return &CachedObjectStore[Account]{
        underlying: kvstore,
        codec:      CramberryCodec[Account]{},
        cache:      NewLRU[Account](10000), // Hot cache
        pool: sync.Pool{
            New: func() any {
                return &Account{}
            },
        },
    }
}
```

---

## Module System

### Declarative Module Builder

```go
// ModuleBuilder provides ergonomic module creation
type ModuleBuilder struct {
    name         string
    dependencies []string
    msgHandlers  map[string]MsgHandler
    queryHandlers map[string]QueryHandler
    beginBlocker BeginBlocker
    endBlocker   EndBlocker
    initGenesis  GenesisInitializer
}

func NewModuleBuilder(name string) *ModuleBuilder {
    return &ModuleBuilder{
        name:          name,
        msgHandlers:   make(map[string]MsgHandler),
        queryHandlers: make(map[string]QueryHandler),
    }
}

func (b *ModuleBuilder) WithDependency(deps ...string) *ModuleBuilder {
    b.dependencies = append(b.dependencies, deps...)
    return b
}

func (b *ModuleBuilder) WithMsgHandler[T Message](
    handler func(Context, T) ([]Effect, error),
) *ModuleBuilder {
    var msg T
    msgType := msg.Type()

    b.msgHandlers[msgType] = &typedMsgHandler[T]{
        handler: handler,
    }
    return b
}

func (b *ModuleBuilder) WithQueryHandler[Req, Resp any](
    path string,
    handler func(Context, Req) (Resp, error),
) *ModuleBuilder {
    b.queryHandlers[path] = &typedQueryHandler[Req, Resp]{
        handler: handler,
    }
    return b
}

func (b *ModuleBuilder) WithBeginBlocker(fn BeginBlocker) *ModuleBuilder {
    b.beginBlocker = fn
    return b
}

func (b *ModuleBuilder) WithEndBlocker(fn EndBlocker) *ModuleBuilder {
    b.endBlocker = fn
    return b
}

func (b *ModuleBuilder) WithGenesisInit(fn GenesisInitializer) *ModuleBuilder {
    b.initGenesis = fn
    return b
}

func (b *ModuleBuilder) Build() Module {
    return &builtModule{
        name:          b.name,
        dependencies:  b.dependencies,
        msgHandlers:   b.msgHandlers,
        queryHandlers: b.queryHandlers,
        beginBlocker:  b.beginBlocker,
        endBlocker:    b.endBlocker,
        initGenesis:   b.initGenesis,
    }
}
```

### Example: Simple Transfer Module

```go
func NewTransferModule(accountCap AccountCapability, balanceCap BalanceCapability) Module {
    return NewModuleBuilder("transfer").
        WithDependency("auth", "bank").
        WithMsgHandler(func(ctx Context, msg *MsgTransfer) ([]Effect, error) {
            // Collect effects instead of mutating state directly
            effects := []Effect{
                // Effect: Transfer tokens
                TransferEffect{
                    From:   msg.From,
                    To:     msg.To,
                    Amount: msg.Amount,
                },
                // Effect: Emit event
                EventEffect{
                    Type: "transfer",
                    Attributes: map[string][]byte{
                        "from":   []byte(msg.From),
                        "to":     []byte(msg.To),
                        "amount": encodeCoin(msg.Amount),
                    },
                },
            }

            return effects, nil
        }).
        Build()
}
```

---

## Trait System

### Core Concept

Modules compose behaviors through **traits** - reusable, composable interfaces for common patterns.

```go
// Trait is a reusable behavior that modules can compose
type Trait interface {
    Name() string
}

// Authorizer trait for modules that verify authorization
type Authorizer interface {
    Trait
    VerifyAuth(ctx Context, account AccountName, auth Authorization) error
}

// Balancer trait for modules that manage balances
type Balancer interface {
    Trait
    GetBalance(ctx Context, account AccountName, denom string) (uint64, error)
    Transfer(ctx Context, from, to AccountName, amount Coins) error
}

// Staker trait for modules that handle staking
type Staker interface {
    Trait
    Delegate(ctx Context, delegator, validator AccountName, amount Coin) error
    Undelegate(ctx Context, delegator, validator AccountName, amount Coin) error
}

// Slasher trait for modules that can slash validators
type Slasher interface {
    Trait
    Slash(ctx Context, validator AccountName, slashFactor float64, reason string) error
}
```

### Trait Composition

```go
// Modules declare which traits they implement
type Module interface {
    Name() string
    Traits() []Trait
}

// Example: Auth module implements Authorizer
type AuthModule struct {
    accountCap AccountCapability
}

func (m *AuthModule) Traits() []Trait {
    return []Trait{m}  // Self as Authorizer
}

func (m *AuthModule) VerifyAuth(
    ctx Context,
    account AccountName,
    auth Authorization,
) error {
    // Implementation
}

// Example: Bank module implements Balancer
type BankModule struct {
    balanceCap BalanceCapability
    authModule Authorizer  // Depends on Authorizer trait
}

func (m *BankModule) Traits() []Trait {
    return []Trait{m}  // Self as Balancer
}

func (m *BankModule) Transfer(
    ctx Context,
    from, to AccountName,
    amount Coins,
) error {
    // Implementation
}

// Trait discovery
func GetTrait[T Trait](app *Application, module string) (T, error) {
    mod := app.GetModule(module)
    for _, trait := range mod.Traits() {
        if t, ok := trait.(T); ok {
            return t, nil
        }
    }
    return nil, fmt.Errorf("module %s does not implement trait", module)
}
```

---

## Context and Execution Model

### Execution Context

```go
type Context struct {
    // Block context (immutable)
    height      uint64
    time        time.Time
    chainID     string
    proposer    AccountName

    // Transaction context
    txBytes     []byte
    txHash      []byte
    account     AccountName  // Transaction signer

    // Effect collection
    collector   *EffectCollector

    // Capability access
    caps        map[string]any  // Type-safe capability retrieval

    // Gas tracking
    gasMeter    GasMeter

    // Event emission
    events      *EventManager

    // Caching
    cache       *ContextCache
}

// Immutable context methods
func (c Context) Height() uint64        { return c.height }
func (c Context) Time() time.Time       { return c.time }
func (c Context) ChainID() string       { return c.chainID }
func (c Context) TxHash() []byte        { return c.txHash }
func (c Context) Account() AccountName  { return c.account }

// Capability access (type-safe)
func GetCap[T any](ctx Context, key string) (T, error) {
    cap, ok := ctx.caps[key]
    if !ok {
        return *new(T), fmt.Errorf("capability not found: %s", key)
    }

    typed, ok := cap.(T)
    if !ok {
        return *new(T), fmt.Errorf("capability type mismatch")
    }

    return typed, nil
}

// Effect emission
func (c Context) EmitEffect(effect Effect) {
    c.collector.Add(effect)
}

func (c Context) EmitEvent(typ string, attrs map[string][]byte) {
    c.collector.Add(EventEffect{Type: typ, Attributes: attrs})
}

// Gas consumption
func (c Context) ConsumeGas(amount uint64, descriptor string) error {
    return c.gasMeter.ConsumeGas(amount, descriptor)
}
```

### Context Caching

```go
type ContextCache struct {
    // Per-transaction cache
    accounts   map[AccountName]*Account
    balances   map[string]uint64  // key: account:denom
    validators map[AccountName]*Validator

    // Dirty tracking
    dirty map[string]struct{}
}

func (c *ContextCache) GetAccount(name AccountName) (*Account, bool) {
    acc, ok := c.accounts[name]
    return acc, ok
}

func (c *ContextCache) SetAccount(acc *Account) {
    c.accounts[acc.Name] = acc
    c.dirty[string(acc.Name)] = struct{}{}
}
```

---

## Core Modules Implementation

### 1. Auth Module

```go
func NewAuthModule() Module {
    var accountCap AccountCapability
    var nonceCap Capability[uint64]

    return NewModuleBuilder("auth").
        WithGenesisInit(func(ctx Context, data []byte) error {
            accountCap = GetCap[AccountCapability](ctx, "account")
            nonceCap = GetCap[Capability[uint64]](ctx, "nonce")

            var genesis AuthGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            for _, acc := range genesis.Accounts {
                if err := accountCap.CreateAccount(acc.Name, acc.Authority); err != nil {
                    return err
                }
            }

            return nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgCreateAccount) ([]Effect, error) {
            // Verify creator has authority
            if err := accountCap.VerifyAuth(msg.Creator, ctx.Authorization()); err != nil {
                return nil, err
            }

            // Return effect to create account
            return []Effect{
                WriteEffect[Account]{
                    Store: "account",
                    Key:   []byte(msg.Name),
                    Value: Account{
                        Name:      msg.Name,
                        Authority: msg.Authority,
                        CreatedAt: ctx.Time(),
                    },
                },
                EventEffect{
                    Type: "account_created",
                    Attributes: map[string][]byte{
                        "name": []byte(msg.Name),
                    },
                },
            }, nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgUpdateAuthority) ([]Effect, error) {
            // Verify current authority allows update
            if err := accountCap.VerifyAuth(msg.Account, ctx.Authorization()); err != nil {
                return nil, err
            }

            // Read current account
            acc, err := accountCap.GetAccount(msg.Account)
            if err != nil {
                return nil, err
            }

            // Update authority
            acc.Authority = msg.NewAuthority
            acc.UpdatedAt = ctx.Time()

            return []Effect{
                WriteEffect[Account]{
                    Store: "account",
                    Key:   []byte(msg.Account),
                    Value: *acc,
                },
                EventEffect{
                    Type: "authority_updated",
                    Attributes: map[string][]byte{
                        "account": []byte(msg.Account),
                    },
                },
            }, nil
        }).
        Build()
}
```

### 2. Bank Module

```go
func NewBankModule(authModule Authorizer) Module {
    var balanceCap BalanceCapability
    var supplyCap Capability[Supply]

    return NewModuleBuilder("bank").
        WithDependency("auth").
        WithGenesisInit(func(ctx Context, data []byte) error {
            balanceCap = GetCap[BalanceCapability](ctx, "balance")
            supplyCap = GetCap[Capability[Supply]](ctx, "supply")

            var genesis BankGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            // Initialize balances
            for _, bal := range genesis.Balances {
                for _, coin := range bal.Coins {
                    if err := balanceCap.AddBalance(bal.Account, coin); err != nil {
                        return err
                    }
                }
            }

            // Initialize supply
            for _, coin := range genesis.Supply {
                if err := supplyCap.Set([]byte(coin.Denom), Supply{Amount: coin.Amount}); err != nil {
                    return err
                }
            }

            return nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgSend) ([]Effect, error) {
            // Verify sender authorization
            if msg.From != ctx.Account() {
                return nil, fmt.Errorf("sender must be transaction account")
            }

            // Return transfer effect
            return []Effect{
                TransferEffect{
                    From:   msg.From,
                    To:     msg.To,
                    Amount: msg.Amount,
                },
                EventEffect{
                    Type: "transfer",
                    Attributes: map[string][]byte{
                        "from":   []byte(msg.From),
                        "to":     []byte(msg.To),
                        "amount": encodeCoins(msg.Amount),
                    },
                },
            }, nil
        }).
        WithQueryHandler("balance", func(ctx Context, req BalanceQuery) (BalanceResponse, error) {
            balance, err := balanceCap.GetBalance(req.Account, req.Denom)
            if err != nil {
                return BalanceResponse{}, err
            }

            return BalanceResponse{
                Account: req.Account,
                Denom:   req.Denom,
                Amount:  balance,
            }, nil
        }).
        Build()
}
```

### 3. Staking Module

```go
func NewStakingModule(authModule Authorizer, bankModule Balancer) Module {
    var validatorCap ValidatorCapability
    var delegationCap Capability[Delegation]
    var balanceCap BalanceCapability

    return NewModuleBuilder("staking").
        WithDependency("auth", "bank").
        WithGenesisInit(func(ctx Context, data []byte) error {
            validatorCap = GetCap[ValidatorCapability](ctx, "validator")
            delegationCap = GetCap[Capability[Delegation]](ctx, "delegation")
            balanceCap = GetCap[BalanceCapability](ctx, "balance")

            var genesis StakingGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            for _, val := range genesis.Validators {
                if err := validatorCap.SetValidator(val); err != nil {
                    return err
                }
            }

            return nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgDelegate) ([]Effect, error) {
            // Verify delegator authorization
            if msg.Delegator != ctx.Account() {
                return nil, fmt.Errorf("delegator must be transaction account")
            }

            // Check validator exists
            val, err := validatorCap.GetValidator(msg.Validator)
            if err != nil {
                return nil, fmt.Errorf("validator not found: %w", err)
            }

            // Check balance
            balance, err := balanceCap.GetBalance(msg.Delegator, msg.Amount.Denom)
            if err != nil {
                return nil, err
            }
            if balance < msg.Amount.Amount {
                return nil, fmt.Errorf("insufficient balance")
            }

            // Compute delegation shares
            shares := computeShares(val, msg.Amount)

            return []Effect{
                // Effect: Transfer tokens to staking pool
                TransferEffect{
                    From:   msg.Delegator,
                    To:     "staking_pool",
                    Amount: Coins{msg.Amount},
                },
                // Effect: Create/update delegation
                WriteEffect[Delegation]{
                    Store: "delegation",
                    Key:   delegationKey(msg.Delegator, msg.Validator),
                    Value: Delegation{
                        Delegator: msg.Delegator,
                        Validator: msg.Validator,
                        Shares:    shares,
                    },
                },
                // Effect: Update validator voting power
                WriteEffect[Validator]{
                    Store: "validator",
                    Key:   []byte(msg.Validator),
                    Value: Validator{
                        Name:        val.Name,
                        PublicKey:   val.PublicKey,
                        VotingPower: val.VotingPower + msg.Amount.Amount,
                        // ... other fields
                    },
                },
                // Effect: Emit event
                EventEffect{
                    Type: "delegate",
                    Attributes: map[string][]byte{
                        "delegator": []byte(msg.Delegator),
                        "validator": []byte(msg.Validator),
                        "amount":    encodeCoin(msg.Amount),
                    },
                },
            }, nil
        }).
        WithEndBlocker(func(ctx Context) ([]ValidatorUpdate, error) {
            // Get all validators
            validators, err := validatorCap.GetValidatorSet()
            if err != nil {
                return nil, err
            }

            // Convert to validator updates for consensus
            updates := make([]ValidatorUpdate, len(validators))
            for i, val := range validators {
                updates[i] = ValidatorUpdate{
                    Name:        val.Name,
                    PublicKey:   val.PublicKey,
                    VotingPower: val.VotingPower,
                }
            }

            return updates, nil
        }).
        Build()
}
```

---

## Parallel Transaction Execution

### Dependency Analysis

```go
type DependencyGraph struct {
    nodes map[TxID]*TxNode
    edges map[TxID][]TxID
}

type TxNode struct {
    ID      TxID
    Effects []Effect
    Deps    []Dependency
}

type Dependency struct {
    Type DependencyType
    Key  []byte
}

type DependencyType uint8

const (
    DependencyTypeAccount DependencyType = iota
    DependencyTypeBalance
    DependencyTypeValidator
    DependencyTypeDelegation
)

func (g *DependencyGraph) DetectConflicts() []Conflict {
    var conflicts []Conflict

    for txID, node := range g.nodes {
        for _, dep := range node.Deps {
            // Check if any other tx writes to the same key
            for otherID, otherNode := range g.nodes {
                if txID == otherID {
                    continue
                }

                for _, effect := range otherNode.Effects {
                    if isConflict(dep, effect) {
                        conflicts = append(conflicts, Conflict{
                            Tx1: txID,
                            Tx2: otherID,
                            Key: dep.Key,
                        })
                    }
                }
            }
        }
    }

    return conflicts
}
```

### Parallel Scheduler

```go
type ParallelScheduler struct {
    maxParallelism int
}

func (s *ParallelScheduler) Schedule(graph *DependencyGraph) [][]TxID {
    // Topological sort with parallelization
    var groups [][]TxID
    remaining := make(map[TxID]*TxNode)
    for id, node := range graph.nodes {
        remaining[id] = node
    }

    for len(remaining) > 0 {
        // Find transactions with no dependencies on remaining txs
        var group []TxID
        for id := range remaining {
            if s.canExecute(id, remaining, graph) {
                group = append(group, id)
            }
        }

        if len(group) == 0 {
            // Circular dependency or conflict
            break
        }

        // Limit parallelism
        if len(group) > s.maxParallelism {
            group = group[:s.maxParallelism]
        }

        groups = append(groups, group)

        // Remove executed transactions
        for _, id := range group {
            delete(remaining, id)
        }
    }

    return groups
}

func (s *ParallelScheduler) ExecuteGroup(
    group []TxID,
    txs map[TxID]*Transaction,
    executor func(Context, *Transaction) error,
) error {
    var wg sync.WaitGroup
    errCh := make(chan error, len(group))

    for _, id := range group {
        wg.Add(1)
        go func(txID TxID) {
            defer wg.Done()

            tx := txs[txID]
            if err := executor(ctx, tx); err != nil {
                errCh <- err
            }
        }(id)
    }

    wg.Wait()
    close(errCh)

    // Return first error if any
    for err := range errCh {
        return err
    }

    return nil
}
```

---

## Caching Strategy

### Multi-Level Cache

```go
type CacheManager struct {
    // L1: Hot cache (LRU 10k entries, ~1MB)
    l1 *LRUCache

    // L2: Warm cache (LRU 100k entries, ~10MB)
    l2 *LRUCache

    // L3: IAVL-backed storage (disk)
    l3 KVStore

    // Statistics
    stats CacheStats
}

func (cm *CacheManager) Get(key []byte) ([]byte, error) {
    keyStr := string(key)

    // Check L1
    if data, ok := cm.l1.Get(keyStr); ok {
        cm.stats.L1Hits++
        return data, nil
    }

    // Check L2
    if data, ok := cm.l2.Get(keyStr); ok {
        cm.stats.L2Hits++
        // Promote to L1
        cm.l1.Set(keyStr, data)
        return data, nil
    }

    // Check L3 (disk)
    data := cm.l3.Get(key)
    if data == nil {
        cm.stats.Misses++
        return nil, ErrNotFound
    }

    cm.stats.L3Hits++

    // Promote to L2 and L1
    cm.l2.Set(keyStr, data)
    cm.l1.Set(keyStr, data)

    return data, nil
}

type CacheStats struct {
    L1Hits uint64
    L2Hits uint64
    L3Hits uint64
    Misses uint64
}

func (s CacheStats) HitRate() float64 {
    total := s.L1Hits + s.L2Hits + s.L3Hits + s.Misses
    if total == 0 {
        return 0
    }
    hits := s.L1Hits + s.L2Hits + s.L3Hits
    return float64(hits) / float64(total)
}
```

### Cache Warming

```go
type CacheWarmer struct {
    stores map[string]ObjectStore
    cache  *CacheManager
}

func (cw *CacheWarmer) WarmAccounts(accounts []AccountName) {
    for _, name := range accounts {
        key := []byte(name)
        if data := cw.stores["account"].Get(key); data != nil {
            cw.cache.Set(key, data)
        }
    }
}

func (cw *CacheWarmer) WarmValidators() {
    iter := cw.stores["validator"].Iterator(nil, nil)
    defer iter.Close()

    for ; iter.Valid(); iter.Next() {
        cw.cache.Set(iter.Key(), iter.Value())
    }
}

// Warm cache before block execution
func (app *Application) BeginBlock(ctx context.Context, header *BlockHeader) error {
    // Predict likely accounts from recent blocks
    likelyAccounts := app.predictor.PredictAccounts(header.Height)

    // Warm caches
    app.cacheWarmer.WarmAccounts(likelyAccounts)
    app.cacheWarmer.WarmValidators()

    // ... continue with BeginBlock
}
```

---

## Zero-Copy Operations

### Object Pooling

```go
type AccountPool struct {
    pool sync.Pool
}

func NewAccountPool() *AccountPool {
    return &AccountPool{
        pool: sync.Pool{
            New: func() any {
                return &Account{}
            },
        },
    }
}

func (p *AccountPool) Get() *Account {
    return p.pool.Get().(*Account)
}

func (p *AccountPool) Put(acc *Account) {
    // Reset account for reuse
    *acc = Account{}
    p.pool.Put(acc)
}

// Usage in object store
func (s *CachedObjectStore[Account]) Get(key []byte) (*Account, error) {
    // ... cache check ...

    // Get from pool
    acc := s.pool.Get().(*Account)

    // Deserialize in-place
    reader := cramberry.NewReader(data)
    if err := acc.UnmarshalFromReader(reader); err != nil {
        s.pool.Put(acc)
        return nil, err
    }

    return acc, nil
}
```

### Zero-Copy Reads

```go
// For read-only operations, return views instead of copies
type AccountView struct {
    data   []byte  // Cramberry-encoded data
    parsed *Account
}

func (v *AccountView) Name() AccountName {
    if v.parsed == nil {
        v.parse()
    }
    return v.parsed.Name
}

func (v *AccountView) Authority() *Authority {
    if v.parsed == nil {
        v.parse()
    }
    return &v.parsed.Authority
}

func (v *AccountView) parse() {
    acc := accountPool.Get()
    cramberry.Unmarshal(v.data, acc)
    v.parsed = acc
}
```

---

## Transaction Format

### Transaction Structure

```go
type Transaction struct {
    // Account taking the action
    Account       AccountName

    // Authorization proof (hierarchical)
    Authorization Authorization

    // Message (operation to perform)
    Message       Message

    // Replay protection
    Nonce         uint64
    Expiration    time.Time

    // Gas and fees
    GasLimit      uint64
    Fee           Coins

    // Memo (optional)
    Memo          string
}

type Authorization struct {
    // Direct signatures
    Signatures []Signature

    // Delegated authorizations (recursive)
    Delegations []DelegatedAuth
}

type Signature struct {
    PublicKey []byte
    Signature []byte
}

type DelegatedAuth struct {
    Account       AccountName
    Authorization Authorization  // Recursive
}
```

### Message Interface

```go
type Message interface {
    // Type information
    Route() string  // Module name
    Type() string   // Message type

    // Validation
    ValidateBasic() error

    // Signer information
    GetSigners() []AccountName

    // Cramberry serialization
    MarshalCramberry() ([]byte, error)
    UnmarshalCramberry([]byte) error
}

// Example: MsgSend
type MsgSend struct {
    From   AccountName
    To     AccountName
    Amount Coins
}

func (m MsgSend) Route() string { return "bank" }
func (m MsgSend) Type() string  { return "send" }

func (m MsgSend) ValidateBasic() error {
    if m.From == "" {
        return fmt.Errorf("from cannot be empty")
    }
    if m.To == "" {
        return fmt.Errorf("to cannot be empty")
    }
    if !m.Amount.IsValid() {
        return fmt.Errorf("invalid amount")
    }
    return nil
}

func (m MsgSend) GetSigners() []AccountName {
    return []AccountName{m.From}
}
```

---

## ABI Integration

### Application Implementation

```go
type Application struct {
    // Module management
    modules      map[string]Module
    moduleOrder  []string
    router       *Router

    // Capability management
    capManager   *CapabilityManager

    // Storage
    stateStore   StateStore    // IAVL from blockberry
    objectStores map[string]ObjectStore

    // Caching
    cacheManager *CacheManager

    // Effect execution
    executor     *EffectExecutor

    // Pools
    accountPool  *ObjectPool[Account]
    balancePool  *ObjectPool[Balance]
    validatorPool *ObjectPool[Validator]

    // Context
    lastHeight   uint64
    lastAppHash  []byte
}

func NewApplication(stateStore StateStore) *Application {
    app := &Application{
        modules:      make(map[string]Module),
        objectStores: make(map[string]ObjectStore),
        stateStore:   stateStore,
        cacheManager: NewCacheManager(stateStore),
        capManager:   NewCapabilityManager(),
        executor:     NewEffectExecutor(),
        accountPool:  NewObjectPool[Account](),
        balancePool:  NewObjectPool[Balance](),
        validatorPool: NewObjectPool[Validator](),
    }

    // Create object stores
    app.objectStores["account"] = NewAccountStore(stateStore)
    app.objectStores["balance"] = NewBalanceStore(stateStore)
    app.objectStores["validator"] = NewValidatorStore(stateStore)
    app.objectStores["delegation"] = NewDelegationStore(stateStore)

    return app
}

// Implement ABI Application interface
func (app *Application) CheckTx(ctx context.Context, txBytes []byte) error {
    // 1. Deserialize (use pool)
    tx := app.txPool.Get()
    defer app.txPool.Put(tx)

    if err := cramberry.Unmarshal(txBytes, tx); err != nil {
        return err
    }

    // 2. Create context with read-only capabilities
    sdkCtx := app.newCheckTxContext(ctx, tx)

    // 3. Verify authorization
    authCap := GetCap[AccountCapability](sdkCtx, "account")
    if err := authCap.VerifyAuth(tx.Account, tx.Authorization); err != nil {
        return err
    }

    // 4. Validate message
    if err := tx.Message.ValidateBasic(); err != nil {
        return err
    }

    // 5. Module-specific validation (collects effects but doesn't apply)
    handler := app.router.GetHandler(tx.Message)
    effects, err := handler(sdkCtx, tx.Message)
    if err != nil {
        return err
    }

    // 6. Validate effects (don't execute)
    for _, effect := range effects {
        if err := effect.Validate(); err != nil {
            return err
        }
    }

    return nil
}

func (app *Application) ExecuteTx(ctx context.Context, txBytes []byte) (*TxResult, error) {
    // 1. Deserialize
    tx := app.txPool.Get()
    defer app.txPool.Put(tx)

    if err := cramberry.Unmarshal(txBytes, tx); err != nil {
        return &TxResult{Code: CodeUnmarshalError}, err
    }

    // 2. Create execution context
    sdkCtx := app.newExecContext(ctx, tx)

    // 3. Execute message (collect effects)
    handler := app.router.GetHandler(tx.Message)
    effects, err := handler(sdkCtx, tx.Message)
    if err != nil {
        return &TxResult{Code: CodeHandlerFailed}, err
    }

    // 4. Execute effects
    if err := app.executor.Execute(sdkCtx, effects); err != nil {
        return &TxResult{Code: CodeEffectFailed}, err
    }

    // 5. Collect events
    events := sdkCtx.Events()

    return &TxResult{
        Code:    CodeOK,
        Events:  events,
        GasUsed: sdkCtx.GasConsumed(),
    }, nil
}
```

### Batch Execution with Parallelization

```go
func (app *Application) ExecuteBlock(ctx context.Context, txs [][]byte) ([]*TxResult, error) {
    // 1. Deserialize all transactions
    transactions := make([]*Transaction, len(txs))
    for i, txBytes := range txs {
        tx := app.txPool.Get()
        if err := cramberry.Unmarshal(txBytes, tx); err != nil {
            return nil, err
        }
        transactions[i] = tx
    }

    // 2. Collect effects from all transactions
    allEffects := make([][]Effect, len(transactions))
    for i, tx := range transactions {
        sdkCtx := app.newExecContext(ctx, tx)
        handler := app.router.GetHandler(tx.Message)
        effects, err := handler(sdkCtx, tx.Message)
        if err != nil {
            return nil, err
        }
        allEffects[i] = effects
    }

    // 3. Build dependency graph
    graph := app.buildDependencyGraph(transactions, allEffects)

    // 4. Schedule parallel execution
    groups := app.scheduler.Schedule(graph)

    // 5. Execute groups in parallel
    results := make([]*TxResult, len(transactions))
    for _, group := range groups {
        if err := app.executeParallelGroup(ctx, group, transactions, results); err != nil {
            return nil, err
        }
    }

    return results, nil
}
```

---

## Message Examples

### Auth Module Messages

```go
type MsgCreateAccount struct {
    Name      AccountName
    Authority Authority
    Creator   AccountName
}

func (m MsgCreateAccount) Route() string                { return "auth" }
func (m MsgCreateAccount) Type() string                 { return "create_account" }
func (m MsgCreateAccount) GetSigners() []AccountName    { return []AccountName{m.Creator} }

type MsgUpdateAuthority struct {
    Account      AccountName
    NewAuthority Authority
}

func (m MsgUpdateAuthority) Route() string              { return "auth" }
func (m MsgUpdateAuthority) Type() string               { return "update_authority" }
func (m MsgUpdateAuthority) GetSigners() []AccountName  { return []AccountName{m.Account} }
```

### Bank Module Messages

```go
type MsgSend struct {
    From   AccountName
    To     AccountName
    Amount Coins
}

func (m MsgSend) Route() string              { return "bank" }
func (m MsgSend) Type() string               { return "send" }
func (m MsgSend) GetSigners() []AccountName  { return []AccountName{m.From} }

type MsgMultiSend struct {
    Inputs  []Input
    Outputs []Output
}

type Input struct {
    Account AccountName
    Coins   Coins
}

type Output struct {
    Account AccountName
    Coins   Coins
}
```

### Staking Module Messages

```go
type MsgCreateValidator struct {
    Validator      AccountName
    PublicKey      []byte
    SelfDelegation Coin
    Commission     Commission
    Description    Description
}

type MsgDelegate struct {
    Delegator AccountName
    Validator AccountName
    Amount    Coin
}

type MsgUndelegate struct {
    Delegator AccountName
    Validator AccountName
    Amount    Coin
}

type MsgRedelegate struct {
    Delegator    AccountName
    SrcValidator AccountName
    DstValidator AccountName
    Amount       Coin
}
```

---

## Integration with Blockberry and Raspberry

Punnet SDK integrates with the Blockberries infrastructure stack through standard interfaces. For complete integration documentation, see **[../ECOSYSTEM.md](../ECOSYSTEM.md)**.

### Application Interface Implementation

Punnet SDK implements the **canonical Application interface** defined in Blockberry (see `github.com/blockberries/blockberry/app/interface.go`):

```go
// Punnet SDK Application implements blockberry's Application interface
type Application struct {
    modules      map[string]Module
    moduleOrder  []string
    capManager   *CapabilityManager
    stateStore   StateStore
    router       *MessageRouter
    effectExec   *EffectExecutor
}

// Implements blockberry.Application
var _ blockberry.Application = (*Application)(nil)

func (app *Application) CheckTx(ctx context.Context, tx []byte) error {
    // Parse and validate transaction
    // Called by Looseberry before batching (validators)
    // Called by TransactionsReactor for gossip validation (full nodes)
}

func (app *Application) BeginBlock(ctx context.Context, header *blockberry.BlockHeader) error {
    // Initialize block context
    // Called by Leaderberry at start of block execution
}

func (app *Application) ExecuteTx(ctx context.Context, tx []byte) (*blockberry.TxResult, error) {
    // Execute transaction via effect system
    // Called by Leaderberry for each tx in committed batches
}

func (app *Application) EndBlock(ctx context.Context) (*blockberry.EndBlockResult, error) {
    // Finalize block, return validator updates
    // Called by Leaderberry at end of block execution
}

func (app *Application) Commit(ctx context.Context) (*blockberry.CommitResult, error) {
    // Persist state, return app hash
    // Called by Leaderberry after EndBlock
}

func (app *Application) Query(ctx context.Context, path string, data []byte, height int64) (*blockberry.QueryResult, error) {
    // Query state at specific height
    // Called by RPC layer
}

func (app *Application) InitChain(ctx context.Context, validators []blockberry.Validator, appState []byte) error {
    // Initialize chain from genesis
    // Called once at chain creation
}
```

### Raspberry Integration

Punnet SDK applications are hosted in Raspberry nodes. The integration flow:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Raspberry Node                              │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    Punnet SDK Application                   │ │
│  │                                                              │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────┐  │ │
│  │  │  Auth    │  │  Bank    │  │ Staking  │  │  Custom   │  │ │
│  │  │  Module  │  │  Module  │  │  Module  │  │  Modules  │  │ │
│  │  └──────────┘  └──────────┘  └──────────┘  └───────────┘  │ │
│  │                       ↓                                     │ │
│  │                 Effect System                               │ │
│  │                       ↓                                     │ │
│  │                 Object Stores (IAVL-backed)                 │ │
│  └───────────────────────┬────────────────────────────────────┘ │
│                          │ implements Application interface      │
│                          ↓                                       │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │              Leaderberry (Consensus)                          ││
│  │  BeginBlock → ExecuteTx* → EndBlock → Commit                 ││
│  └──────────────────────────────────────────────────────────────┘│
│                          ↓ ReapCertifiedBatches()                │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │              Looseberry (DAG Mempool)                         ││
│  │  AddTx → CheckTx validation → Workers → Certificates         ││
│  └──────────────────────────────────────────────────────────────┘│
│                          ↓                                       │
│  ┌──────────────────────────────────────────────────────────────┐│
│  │              Blockberry (Node Framework)                      ││
│  │  BlockStore │ StateStore │ PeerManager │ Reactors            ││
│  └──────────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────────┘
```

### Validator Node Setup

```go
// Create Punnet SDK application
app := punnet.NewApplication(
    punnet.WithModules(
        auth.NewModule(),
        bank.NewModule(),
        staking.NewModule(),
    ),
    punnet.WithStateStore(iavlStore),
)

// Create Raspberry validator node
node := raspberry.NewValidatorNode(config,
    raspberry.WithApplication(app),
    raspberry.WithLooseberry(looseMempool),
)

// Wire CheckTx for Looseberry
looseMempool.SetTxValidator(func(tx []byte) error {
    return app.CheckTx(context.Background(), tx)
})

// Start node
if err := node.Start(); err != nil {
    log.Fatal(err)
}
```

### Validator Set Updates

```go
func (app *Application) EndBlock(ctx context.Context) (*EndBlockResult, error) {
    sdkCtx := app.currentContext()

    var allUpdates []ValidatorUpdate

    // Call EndBlock on all modules
    for _, moduleName := range app.moduleOrder {
        module := app.modules[moduleName]

        updates, err := module.EndBlock(sdkCtx)
        if err != nil {
            return nil, err
        }

        allUpdates = append(allUpdates, updates...)
    }

    return &EndBlockResult{
        ValidatorUpdates: allUpdates,
        Events:          sdkCtx.Events(),
    }, nil
}
```

### Looseberry Integration

Punnet SDK applications work seamlessly with Looseberry's DAG mempool:

```go
// Validator node configuration
node := raspberry.NewValidatorNode(
    config,
    raspberry.WithApplication(punnetApp),
    raspberry.WithLooseberry(looseMempool),
)

// CheckTx called by Looseberry before batching
looseMempool.SetTxValidator(func(txBytes []byte) error {
    return punnetApp.CheckTx(context.Background(), txBytes)
})

// ExecuteTx called by Leaderberry during block execution
// (via Application interface)
```

---

## Developer Experience

### Creating a New Module

```go
// Simple token module in under 50 lines
func NewTokenModule(name string, symbol string) Module {
    var balanceCap BalanceCapability

    return NewModuleBuilder(name).
        WithDependency("auth", "bank").
        WithGenesisInit(func(ctx Context, data []byte) error {
            balanceCap = GetCap[BalanceCapability](ctx, "balance")

            var genesis TokenGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            // Initialize balances
            for _, holder := range genesis.Holders {
                if err := balanceCap.AddBalance(holder.Account, Coin{
                    Denom:  symbol,
                    Amount: holder.Amount,
                }); err != nil {
                    return err
                }
            }

            return nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgMint) ([]Effect, error) {
            // Verify minter authority
            if !isMinter(msg.Minter) {
                return nil, fmt.Errorf("not authorized to mint")
            }

            return []Effect{
                WriteEffect[Balance]{
                    Store: "balance",
                    Key:   balanceKey(msg.To, symbol),
                    Value: Balance{
                        Account: msg.To,
                        Denom:   symbol,
                        Amount:  getCurrentBalance(ctx, msg.To) + msg.Amount,
                    },
                },
                EventEffect{
                    Type: "token_minted",
                    Attributes: map[string][]byte{
                        "to":     []byte(msg.To),
                        "amount": encodeUint64(msg.Amount),
                    },
                },
            }, nil
        }).
        WithQueryHandler("balance", func(ctx Context, req BalanceQuery) (uint64, error) {
            return balanceCap.GetBalance(req.Account, symbol)
        }).
        Build()
}
```

### Module Registration

```go
func NewMyChainApp(stateStore StateStore) *Application {
    app := NewApplication(stateStore)

    // Register core modules
    authModule := auth.NewModule()
    bankModule := bank.NewModule()
    stakingModule := staking.NewModule()

    app.RegisterModule(authModule)
    app.RegisterModule(bankModule)
    app.RegisterModule(stakingModule)

    // Register custom modules
    myTokenModule := NewTokenModule("mytoken", "MTK")
    app.RegisterModule(myTokenModule)

    return app
}
```

---

## Account System

### Named Accounts with Hierarchical Permissions

```go
type Account struct {
    // Identity
    Name      AccountName
    ID        uint64  // Numeric ID for indexing

    // Single authority with hierarchical permissions
    Authority Authority

    // Metadata
    CreatedAt time.Time
    UpdatedAt time.Time
    Metadata  map[string][]byte
}

type Authority struct {
    // Multi-sig threshold
    Threshold uint32

    // Direct key weights
    Keys []KeyWeight

    // Account delegation weights
    Accounts []AccountWeight
}

type KeyWeight struct {
    PublicKey []byte
    Weight    uint32
}

type AccountWeight struct {
    Account AccountName
    Weight  uint32
}
```

### Authorization Verification (with Cycle Detection)

```go
type AuthVerifier struct {
    accountCap AccountCapability
}

func (v *AuthVerifier) Verify(
    account AccountName,
    auth Authorization,
) error {
    visited := make(map[AccountName]bool)
    return v.verifyRecursive(account, auth, visited)
}

func (v *AuthVerifier) verifyRecursive(
    account AccountName,
    auth Authorization,
    visited map[AccountName]bool,
) error {
    // Detect cycles
    if visited[account] {
        return fmt.Errorf("circular delegation detected: %s", account)
    }
    visited[account] = true

    // Get account authority
    acc, err := v.accountCap.GetAccount(account)
    if err != nil {
        return err
    }

    // Compute weight from direct signatures
    var totalWeight uint32
    for _, sig := range auth.Signatures {
        for _, keyWeight := range acc.Authority.Keys {
            if bytes.Equal(sig.PublicKey, keyWeight.PublicKey) {
                // Verify signature
                if !ed25519.Verify(sig.PublicKey, signBytes, sig.Signature) {
                    return fmt.Errorf("invalid signature")
                }
                totalWeight += keyWeight.Weight
                break
            }
        }
    }

    // Compute weight from account delegations
    for _, delegation := range auth.Delegations {
        for _, accWeight := range acc.Authority.Accounts {
            if delegation.Account == accWeight.Account {
                // Recursively verify delegated account
                if err := v.verifyRecursive(
                    delegation.Account,
                    delegation.Authorization,
                    visited,
                ); err != nil {
                    return err
                }
                totalWeight += accWeight.Weight
                break
            }
        }
    }

    // Check threshold
    if totalWeight < acc.Authority.Threshold {
        return fmt.Errorf("insufficient weight: %d < %d", totalWeight, acc.Authority.Threshold)
    }

    return nil
}
```

---

## Core Module Implementations

### 1. Auth Module (Effect-Based)

```go
func NewAuthModule() Module {
    var accountCap AccountCapability
    var nonceCap Capability[uint64]

    return NewModuleBuilder("auth").
        WithGenesisInit(func(ctx Context, data []byte) error {
            accountCap = GetCap[AccountCapability](ctx, "account")
            nonceCap = GetCap[Capability[uint64]](ctx, "nonce")

            var genesis AuthGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            for _, acc := range genesis.Accounts {
                effects := []Effect{
                    WriteEffect[Account]{
                        Store: "account",
                        Key:   []byte(acc.Name),
                        Value: acc,
                    },
                    WriteEffect[uint64]{
                        Store: "nonce",
                        Key:   []byte(acc.Name),
                        Value: 0,
                    },
                }

                if err := ctx.ExecuteEffects(effects); err != nil {
                    return err
                }
            }

            return nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgCreateAccount) ([]Effect, error) {
            // Verify creator authorization
            if err := accountCap.VerifyAuth(msg.Creator, ctx.Authorization()); err != nil {
                return nil, err
            }

            // Check account doesn't exist
            if _, err := accountCap.GetAccount(msg.Name); err == nil {
                return nil, fmt.Errorf("account already exists: %s", msg.Name)
            }

            return []Effect{
                WriteEffect[Account]{
                    Store: "account",
                    Key:   []byte(msg.Name),
                    Value: Account{
                        Name:      msg.Name,
                        ID:        generateAccountID(),
                        Authority: msg.Authority,
                        CreatedAt: ctx.Time(),
                    },
                },
                WriteEffect[uint64]{
                    Store: "nonce",
                    Key:   []byte(msg.Name),
                    Value: 0,
                },
                EventEffect{
                    Type: "account_created",
                    Attributes: map[string][]byte{
                        "name":    []byte(msg.Name),
                        "creator": []byte(msg.Creator),
                    },
                },
            }, nil
        }).
        WithMsgHandler(func(ctx Context, msg *MsgUpdateAuthority) ([]Effect, error) {
            // Verify account authorization
            if err := accountCap.VerifyAuth(msg.Account, ctx.Authorization()); err != nil {
                return nil, err
            }

            // Get current account
            acc, err := accountCap.GetAccount(msg.Account)
            if err != nil {
                return nil, err
            }

            // Update authority
            acc.Authority = msg.NewAuthority
            acc.UpdatedAt = ctx.Time()

            return []Effect{
                WriteEffect[Account]{
                    Store: "account",
                    Key:   []byte(msg.Account),
                    Value: *acc,
                },
                EventEffect{
                    Type: "authority_updated",
                    Attributes: map[string][]byte{
                        "account": []byte(msg.Account),
                    },
                },
            }, nil
        }).
        Build()
}
```

### 2. Bank Module (Effect-Based)

```go
func NewBankModule(authModule Authorizer) Module {
    var balanceCap BalanceCapability
    var supplyCap Capability[Supply]

    return NewModuleBuilder("bank").
        WithDependency("auth").
        WithGenesisInit(func(ctx Context, data []byte) error {
            balanceCap = GetCap[BalanceCapability](ctx, "balance")
            supplyCap = GetCap[Capability[Supply]](ctx, "supply")

            var genesis BankGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            var effects []Effect

            // Initialize balances
            for _, bal := range genesis.Balances {
                for _, coin := range bal.Coins {
                    effects = append(effects, WriteEffect[Balance]{
                        Store: "balance",
                        Key:   balanceKey(bal.Account, coin.Denom),
                        Value: Balance{
                            Account: bal.Account,
                            Denom:   coin.Denom,
                            Amount:  coin.Amount,
                        },
                    })
                }
            }

            // Initialize supply
            for _, coin := range genesis.Supply {
                effects = append(effects, WriteEffect[Supply]{
                    Store: "supply",
                    Key:   []byte(coin.Denom),
                    Value: Supply{
                        Denom:  coin.Denom,
                        Amount: coin.Amount,
                    },
                })
            }

            return ctx.ExecuteEffects(effects)
        }).
        WithMsgHandler(func(ctx Context, msg *MsgSend) ([]Effect, error) {
            // Verify sender is transaction account
            if msg.From != ctx.Account() {
                return nil, fmt.Errorf("sender must be tx account")
            }

            // Verify sufficient balance (read effect)
            for _, coin := range msg.Amount {
                balance, err := balanceCap.GetBalance(msg.From, coin.Denom)
                if err != nil {
                    return nil, err
                }
                if balance < coin.Amount {
                    return nil, fmt.Errorf("insufficient balance: %s", coin.Denom)
                }
            }

            // Return transfer effect
            return []Effect{
                TransferEffect{
                    From:   msg.From,
                    To:     msg.To,
                    Amount: msg.Amount,
                },
                EventEffect{
                    Type: "transfer",
                    Attributes: map[string][]byte{
                        "from":   []byte(msg.From),
                        "to":     []byte(msg.To),
                        "amount": encodeCoins(msg.Amount),
                    },
                },
            }, nil
        }).
        WithQueryHandler("balance", func(ctx Context, req BalanceQuery) (BalanceResponse, error) {
            balance, err := balanceCap.GetBalance(req.Account, req.Denom)
            if err != nil {
                return BalanceResponse{}, err
            }

            return BalanceResponse{
                Account: req.Account,
                Denom:   req.Denom,
                Amount:  balance,
            }, nil
        }).
        Build()
}
```

### 3. Staking Module (Effect-Based)

```go
func NewStakingModule(
    authModule Authorizer,
    bankModule Balancer,
) Module {
    var validatorCap ValidatorCapability
    var delegationCap Capability[Delegation]
    var balanceCap BalanceCapability

    return NewModuleBuilder("staking").
        WithDependency("auth", "bank").
        WithGenesisInit(func(ctx Context, data []byte) error {
            validatorCap = GetCap[ValidatorCapability](ctx, "validator")
            delegationCap = GetCap[Capability[Delegation]](ctx, "delegation")
            balanceCap = GetCap[BalanceCapability](ctx, "balance")

            var genesis StakingGenesis
            if err := cramberry.Unmarshal(data, &genesis); err != nil {
                return err
            }

            var effects []Effect

            for _, val := range genesis.Validators {
                effects = append(effects, WriteEffect[Validator]{
                    Store: "validator",
                    Key:   []byte(val.Name),
                    Value: val,
                })
            }

            for _, del := range genesis.Delegations {
                effects = append(effects, WriteEffect[Delegation]{
                    Store: "delegation",
                    Key:   delegationKey(del.Delegator, del.Validator),
                    Value: del,
                })
            }

            return ctx.ExecuteEffects(effects)
        }).
        WithMsgHandler(func(ctx Context, msg *MsgDelegate) ([]Effect, error) {
            // Verify delegator is tx account
            if msg.Delegator != ctx.Account() {
                return nil, fmt.Errorf("delegator must be tx account")
            }

            // Get validator
            val, err := validatorCap.GetValidator(msg.Validator)
            if err != nil {
                return nil, fmt.Errorf("validator not found")
            }

            // Compute shares
            shares := computeShares(val, msg.Amount)

            return []Effect{
                // Transfer tokens to staking pool
                TransferEffect{
                    From:   msg.Delegator,
                    To:     "staking_pool",
                    Amount: Coins{msg.Amount},
                },
                // Update or create delegation
                WriteEffect[Delegation]{
                    Store: "delegation",
                    Key:   delegationKey(msg.Delegator, msg.Validator),
                    Value: Delegation{
                        Delegator: msg.Delegator,
                        Validator: msg.Validator,
                        Shares:    shares,
                    },
                },
                // Update validator voting power
                WriteEffect[Validator]{
                    Store: "validator",
                    Key:   []byte(msg.Validator),
                    Value: Validator{
                        Name:        val.Name,
                        PublicKey:   val.PublicKey,
                        VotingPower: val.VotingPower + msg.Amount.Amount,
                        Tokens:      val.Tokens + msg.Amount.Amount,
                        Commission:  val.Commission,
                        Status:      val.Status,
                    },
                },
                EventEffect{
                    Type: "delegate",
                    Attributes: map[string][]byte{
                        "delegator": []byte(msg.Delegator),
                        "validator": []byte(msg.Validator),
                        "amount":    encodeCoin(msg.Amount),
                        "shares":    encodeUint64(shares),
                    },
                },
            }, nil
        }).
        WithEndBlocker(func(ctx Context) ([]ValidatorUpdate, error) {
            // Get all validators
            validators, err := validatorCap.GetValidatorSet()
            if err != nil {
                return nil, err
            }

            // Filter to bonded validators only
            var bonded []Validator
            for _, val := range validators {
                if val.Status == ValidatorStatusBonded {
                    bonded = append(bonded, val)
                }
            }

            // Sort by voting power (descending), then by name
            sort.Slice(bonded, func(i, j int) bool {
                if bonded[i].VotingPower == bonded[j].VotingPower {
                    return bonded[i].Name < bonded[j].Name
                }
                return bonded[i].VotingPower > bonded[j].VotingPower
            })

            // Take top N validators (from params)
            maxVals := 100  // From consensus params
            if len(bonded) > maxVals {
                bonded = bonded[:maxVals]
            }

            // Convert to validator updates
            updates := make([]ValidatorUpdate, len(bonded))
            for i, val := range bonded {
                updates[i] = ValidatorUpdate{
                    Name:        val.Name,
                    PublicKey:   val.PublicKey,
                    VotingPower: val.VotingPower,
                }
            }

            return updates, nil
        }).
        Build()
}
```

---

## Performance Optimizations

### 1. Object Pooling

```go
type ObjectPool[T any] struct {
    pool sync.Pool
}

func NewObjectPool[T any]() *ObjectPool[T] {
    return &ObjectPool[T]{
        pool: sync.Pool{
            New: func() any {
                return new(T)
            },
        },
    }
}

func (p *ObjectPool[T]) Get() *T {
    return p.pool.Get().(*T)
}

func (p *ObjectPool[T]) Put(obj *T) {
    // Reset to zero value
    *obj = *new(T)
    p.pool.Put(obj)
}

// Usage
var accountPool = NewObjectPool[Account]()

func getAccount(key []byte) (*Account, error) {
    acc := accountPool.Get()
    defer accountPool.Put(acc)

    // Use account...
}
```

### 2. Batch Operations

```go
type BatchLoader[T any] struct {
    store ObjectStore[T]
    batch map[string]*T
    mu    sync.Mutex
}

func (bl *BatchLoader[T]) Schedule(key []byte) {
    bl.mu.Lock()
    defer bl.mu.Unlock()
    bl.batch[string(key)] = nil
}

func (bl *BatchLoader[T]) Load() error {
    bl.mu.Lock()
    keys := make([][]byte, 0, len(bl.batch))
    for k := range bl.batch {
        keys = append(keys, []byte(k))
    }
    bl.mu.Unlock()

    // Batch load from store
    values, err := bl.store.GetBatch(keys)
    if err != nil {
        return err
    }

    bl.mu.Lock()
    for i, key := range keys {
        bl.batch[string(key)] = values[i]
    }
    bl.mu.Unlock()

    return nil
}

func (bl *BatchLoader[T]) Get(key []byte) (*T, error) {
    bl.mu.Lock()
    defer bl.mu.Unlock()

    val, ok := bl.batch[string(key)]
    if !ok {
        return nil, ErrNotScheduled
    }
    if val == nil {
        return nil, ErrNotFound
    }

    return val, nil
}
```

### 3. Predictive Cache Warming

```go
type CachePredictor struct {
    // Track access patterns
    accessHistory map[uint64][]AccountName  // height -> accounts
    historySize   int
}

func (cp *CachePredictor) RecordAccess(height uint64, account AccountName) {
    if cp.accessHistory[height] == nil {
        cp.accessHistory[height] = []AccountName{}
    }
    cp.accessHistory[height] = append(cp.accessHistory[height], account)

    // Prune old history
    if len(cp.accessHistory) > cp.historySize {
        delete(cp.accessHistory, height-uint64(cp.historySize))
    }
}

func (cp *CachePredictor) PredictAccounts(height uint64) []AccountName {
    // Use recent history to predict likely accounts
    var predicted []AccountName
    seen := make(map[AccountName]bool)

    // Look at last N blocks
    for i := height - 10; i < height; i++ {
        for _, acc := range cp.accessHistory[i] {
            if !seen[acc] {
                predicted = append(predicted, acc)
                seen[acc] = true
            }
        }
    }

    return predicted
}
```

---

## Package Structure

```
punnet-sdk/
├── ARCHITECTURE.md
├── go.mod
├── go.sum
│
├── runtime/                    # SDK core runtime
│   ├── app.go                  # Application orchestration
│   ├── context.go              # Execution context
│   ├── module.go               # Module interface
│   ├── builder.go              # ModuleBuilder
│   ├── router.go               # Message routing
│   ├── effect.go               # Effect system
│   ├── executor.go             # Effect execution
│   ├── capability.go           # Capability management
│   ├── cache.go                # Multi-level caching
│   ├── pool.go                 # Object pooling
│   └── parallel.go             # Parallel execution
│
├── store/                      # Storage layer
│   ├── objectstore.go          # ObjectStore interface
│   ├── cached_store.go         # CachedObjectStore implementation
│   ├── account_store.go        # Typed AccountStore
│   ├── balance_store.go        # Typed BalanceStore
│   ├── validator_store.go      # Typed ValidatorStore
│   └── delegation_store.go     # Typed DelegationStore
│
├── types/                      # Core types
│   ├── account.go              # Account, Authority
│   ├── transaction.go          # Transaction structure
│   ├── message.go              # Message interface
│   ├── authorization.go        # Authorization
│   ├── coin.go                 # Coin, Coins
│   ├── validator.go            # Validator types
│   ├── result.go               # Result types
│   └── effect.go               # Effect types
│
├── traits/                     # Reusable traits
│   ├── trait.go                # Trait interface
│   ├── authorizer.go           # Authorizer trait
│   ├── balancer.go             # Balancer trait
│   ├── staker.go               # Staker trait
│   └── slasher.go              # Slasher trait
│
├── modules/                    # Core modules
│   ├── auth/                   # Auth module
│   │   ├── module.go
│   │   ├── messages.go
│   │   ├── genesis.go
│   │   └── queries.go
│   ├── bank/                   # Bank module
│   │   ├── module.go
│   │   ├── messages.go
│   │   ├── genesis.go
│   │   └── queries.go
│   └── staking/                # Staking module
│       ├── module.go
│       ├── messages.go
│       ├── genesis.go
│       └── queries.go
│
├── schema/                     # Cramberry schemas
│   ├── types.cram              # Core types
│   ├── auth.cram               # Auth module
│   ├── bank.cram               # Bank module
│   └── staking.cram            # Staking module
│
├── examples/                   # Example applications
│   ├── namechain/              # Name registry
│   └── tokenchain/             # Token exchange
│
└── tests/                      # Tests
    ├── unit/
    ├── integration/
    └── benchmark/
```

---

## Example Application

```go
package main

import (
    "github.com/blockberries/punnet-sdk/runtime"
    "github.com/blockberries/punnet-sdk/modules/auth"
    "github.com/blockberries/punnet-sdk/modules/bank"
    "github.com/blockberries/punnet-sdk/modules/staking"
    "github.com/blockberries/raspberry"
)

func NewMyChainApp(stateStore StateStore) *runtime.Application {
    app := runtime.NewApplication(stateStore)

    // Create and register modules
    authModule := auth.NewModule()
    bankModule := bank.NewModule(authModule)
    stakingModule := staking.NewModule(authModule, bankModule)

    app.RegisterModule(authModule)
    app.RegisterModule(bankModule)
    app.RegisterModule(stakingModule)

    // Optional: Add custom module
    myModule := NewModuleBuilder("custom").
        WithDependency("auth", "bank").
        WithMsgHandler(func(ctx Context, msg *MsgCustom) ([]Effect, error) {
            // Custom logic here
            return []Effect{...}, nil
        }).
        Build()

    app.RegisterModule(myModule)

    return app
}

func main() {
    // Load config
    cfg := loadConfig()

    // Create state store
    stateStore, err := blockberry.NewIAVLStore(cfg.StateStorePath)
    if err != nil {
        panic(err)
    }

    // Create application
    app := NewMyChainApp(stateStore)

    // Initialize Raspberry node
    node, err := raspberry.NewValidatorNode(
        cfg,
        raspberry.WithApplication(app),
    )
    if err != nil {
        panic(err)
    }

    // Start
    if err := node.Start(); err != nil {
        panic(err)
    }

    waitForShutdown(node)
}
```

---

## Performance Characteristics

### Throughput Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Transaction throughput | 100,000+ tx/sec | With parallel execution |
| CheckTx latency | < 1ms | Cached account/balance reads |
| ExecuteTx latency | < 5ms | Effect collection + execution |
| Parallel execution | 4-8x speedup | Depends on tx dependencies |
| Cache hit rate | > 95% | With proper warming |

### Memory Footprint

| Component | Memory | Notes |
|-----------|--------|-------|
| L1 Cache | ~1 MB | 10k entries |
| L2 Cache | ~10 MB | 100k entries |
| Object pools | ~5 MB | Pre-allocated objects |
| Per-transaction context | ~1 KB | Pooled and reused |
| Total (excluding IAVL) | ~20 MB | For hot path |

### Parallelization Gains

```
Sequential execution: N × t
Parallel execution (P groups): G × t

Speedup = N / G

Example with 1000 txs, avg 8 independent groups:
  Speedup = 1000 / 8 = 125x theoretical
  Practical: 4-8x (overhead, synchronization)
```

---

## Security Considerations

### Capability Isolation

1. **Modules cannot access stores directly** - Only through granted capabilities
2. **Capabilities scoped per-module** - Each module gets its own capabilities
3. **Effect validation** - All effects validated before execution
4. **Audit trail** - All state access tracked

### Effect Validation

1. **Type safety** - Effects are strongly typed
2. **Conflict detection** - Read-write, write-write conflicts detected
3. **Authorization checks** - All effects verified against account authorities
4. **Gas limits** - Effect execution consumes gas

### Authorization Security

1. **Cycle detection** - Prevents infinite delegation loops
2. **Threshold enforcement** - Weight must meet or exceed threshold
3. **Signature verification** - All signatures verified
4. **Nonce checking** - Replay protection

---

## Merkle Proofs and Light Client Support

### Compatibility with IAVL Proofs

The object store architecture is fully compatible with merkle proofs and light clients. Since object stores are a typed, cached layer on top of IAVL (same as Cosmos SDK), proof generation works identically:

```go
// ObjectStore exposes proof generation
type ObjectStore[T any] interface {
    Get(key []byte) (*T, error)
    Set(key []byte, value T) error
    Delete(key []byte) error
    Has(key []byte) bool
    Iterator(start, end []byte) Iterator[T]

    // Merkle proof generation (delegates to underlying IAVL)
    GetProof(key []byte) (*ics23.CommitmentProof, error)
}

// Implementation
func (s *CachedObjectStore[T]) GetProof(key []byte) (*ics23.CommitmentProof, error) {
    // Construct full key with store prefix
    fullKey := append([]byte(s.prefix+":"), key...)

    // Delegate to underlying IAVL store
    // This is the SAME IAVL that Cosmos SDK uses
    return s.underlying.GetProof(fullKey)
}

// Capability exposes proofs to modules
type AccountCapability interface {
    Capability[Account]

    // Get account with merkle proof
    GetAccountWithProof(name AccountName) (*Account, *ics23.CommitmentProof, error)
}
```

### How Light Clients Work

Light clients work **identically** to Cosmos SDK since both use IAVL:

```
1. Light client has trusted block header with AppHash (IAVL root)
2. Client requests account "alice" with proof from full node
3. Full node:
   - Calls accountStore.GetProof([]byte("alice"))
   - Object store constructs key: "account:alice"
   - IAVL generates merkle proof for "account:alice" → value
   - Returns proof + account data (cramberry-encoded)
4. Light client:
   - Verifies merkle proof against trusted AppHash
   - If valid, deserializes account data with cramberry
   - Trusts the account data
```

**Key insight**: The object store abstraction is **transparent to proof generation**. We're using the **same IAVL** as Cosmos SDK, just with:
- Automatic prefixing for isolation
- Typed interfaces for type safety
- Automatic caching for performance

### Proof Verification

```go
// Light client verification (same as Cosmos SDK)
import "github.com/cosmos/ics23/go"

func (lc *LightClient) VerifyAccount(
    name AccountName,
    accountData []byte,
    proof *ics23.CommitmentProof,
    trustedAppHash []byte,
) error {
    key := []byte("account:" + string(name))

    // Verify merkle proof (same as Cosmos SDK)
    if err := ics23.VerifyMembership(
        proof,
        ics23.IavlSpec,
        trustedAppHash,
        key,
        accountData,
    ); err != nil {
        return fmt.Errorf("proof verification failed: %w", err)
    }

    return nil
}
```

**Conclusion**: Merkle proofs and light clients are **fully supported** with **zero compromises**. The object store layer doesn't interfere with IAVL's proof capabilities.

---

## Future Enhancements

### Planned Features (Priority Order)

1. **Governance Module**: On-chain parameter governance with proposals, deposits, voting
2. **Distribution Module**: Fee and staking reward distribution using F1 algorithm
3. **Slashing Module**: Validator misbehavior penalties (double-sign, downtime)
4. **Evidence Module**: Byzantine evidence handling and slashing triggers
5. **Upgrade Module**: Coordinated chain upgrades with height-based activation
6. **State Snapshots**: Fast sync via state snapshots (similar to Cosmos SDK)
7. **IBC Support**: Inter-blockchain communication (large undertaking, 6-12 months)

### Research Areas

1. **Optimistic Parallel Execution**: Execute all txs speculatively, detect conflicts, rollback (inspired by Aptos/Sui)
2. **Software Transactional Memory**: STM for automatic conflict resolution
3. **Adaptive Scheduling**: ML-based prediction of transaction dependencies for better parallelization
4. **Hardware Acceleration**: GPU-based signature verification for high-throughput blocks

**Caveat on Parallel Execution**: Real-world benefits depend on transaction independence. DeFi workloads with high contention on shared state (liquidity pools) may see **minimal gains**. Profiling with production workloads is essential before claiming speedups.

---

## Design Trade-Offs vs Cosmos SDK

### Architectural Differences

| Aspect | Cosmos SDK | Punnet SDK | Trade-Off |
|--------|-----------|------------|-----------|
| **State Access** | Direct KVStore | Capability-based object stores | More security, more abstraction layers |
| **Module Composition** | Keeper passing | Trait composition | Cleaner interfaces, steeper learning curve |
| **Side Effects** | Implicit | Explicit effects | Easier to reason about, more verbose |
| **Parallelization** | Sequential | Dependency-based parallel | Potential speedup, unproven in practice |
| **Caching** | Manual | Automatic multi-level | Better performance, more complexity |
| **Memory** | Allocate-per-request | Object pooling | Lower GC pressure, more code |
| **Developer Experience** | Proven patterns | Novel patterns | Cleaner API, fewer examples |
| **Type Safety** | Runtime reflection | Compile-time generics | Fewer runtime errors, more type parameters |
| **Accounts** | Address-based | Named with hierarchy | Better UX, different namespace design |
| **Serialization** | Protobuf | Cramberry | Faster decode, smaller ecosystem |
| **Ecosystem** | Massive (50+ modules) | None (3 core modules) | **Critical disadvantage** |
| **Battle-Testing** | 5+ years production | Zero production use | **Critical disadvantage** |

See [COSMOS_COMPARISON.md](COSMOS_COMPARISON.md) for a brutally honest feature-by-feature comparison.

---

## Conclusion

Punnet SDK introduces several novel concepts to blockchain application development:

1. **Named Accounts**: Borrowed from Bitshares, genuinely better UX than cryptographic addresses
2. **Hierarchical Permissions**: More flexible than single-key or basic multisig
3. **Effect System**: Explicit side effects enable dependency analysis and potential parallelization
4. **Capability Security**: Fine-grained access control prevents modules from accessing arbitrary state
5. **Object Stores**: Typed storage with automatic caching and serialization
6. **Trait Composition**: Reusable behaviors that compose cleanly
7. **Type-Safe Builders**: Declarative module creation with compile-time guarantees

**However, Punnet SDK is:**
- **Unproven** in production
- **Incomplete** (missing critical modules)
- **Unaudited** for security
- **Lacking ecosystem** (no tooling, wallets, explorers)
- **Risky** for production use

**Recommended use cases:**
- Chains that specifically need named accounts and hierarchical permissions
- Experimental or research blockchains
- Learning and contributing to novel blockchain architecture
- Proof-of-concept applications

**Not recommended for:**
- Production chains with real economic value (use Cosmos SDK)
- Chains requiring IBC (use Cosmos SDK)
- Teams needing extensive tooling and community support (use Cosmos SDK)
- Risk-averse projects (use Cosmos SDK)

The **named account model** is Punnet SDK's killer feature and may justify adoption for specific use cases. Everything else is **interesting but unproven**. See [COSMOS_COMPARISON.md](COSMOS_COMPARISON.md) for detailed analysis.

---

## Ecosystem Integration

Punnet SDK is part of the Blockberries ecosystem. For complete integration documentation, see:

- **[../ECOSYSTEM.md](../ECOSYSTEM.md)** - Complete ecosystem architecture and integration guide
- **[../raspberry/ARCHITECTURE.md](../raspberry/ARCHITECTURE.md)** - Blockchain node that hosts Punnet SDK applications
- **[../blockberry/ARCHITECTURE.md](../blockberry/ARCHITECTURE.md)** - Node framework with canonical Application interface
- **[../leaderberry/ARCHITECTURE.md](../leaderberry/ARCHITECTURE.md)** - BFT consensus engine
- **[../looseberry/ARCHITECTURE.md](../looseberry/ARCHITECTURE.md)** - DAG mempool for high throughput
- **[../glueberry/ARCHITECTURE.md](../glueberry/ARCHITECTURE.md)** - Encrypted P2P networking
- **[../cramberry/ARCHITECTURE.md](../cramberry/ARCHITECTURE.md)** - Binary serialization
