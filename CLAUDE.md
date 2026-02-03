# Punnet SDK Development Guidelines

## Project Overview

Punnet SDK is a next-generation blockchain application framework built on the Blockberries ecosystem. It introduces a novel **effect-based module system** with **capability security**, **zero-copy operations**, and **parallel transaction execution**. Unlike traditional blockchain frameworks, Punnet SDK uses modern patterns from game engines (ECS), functional programming (effect systems), and high-performance computing (object pooling, cache-first design).

**Key Design Principle**: Modules don't mutate state directly. Instead, they declare **effects** that describe their intent. The runtime collects, validates, and executes effects with automatic dependency detection for parallelization.

## Key Dependencies

| Dependency | Location | Purpose |
|------------|----------|---------|
| Blockberry | `../blockberry` | Node framework, IAVL state storage, Application interface |
| Raspberry | `../raspberry` | Blockchain node that hosts Punnet SDK applications |
| Leaderberry | `../leaderberry` | Tendermint BFT consensus engine |
| Looseberry | `../looseberry` | DAG-based mempool (validators only) |
| Cramberry | `../cramberry` | Binary serialization |
| IAVL | `github.com/cosmos/iavl` | Merkleized key-value state storage |

## Architecture Summary

### Component Stack

```
Custom Modules → Punnet SDK Runtime → Raspberry → Leaderberry → Looseberry → Blockberry
```

### System Layers

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          MODULE LAYER                                    │
│  (User-defined modules using SDK primitives)                            │
│  module := NewModuleBuilder("mymodule").WithMsgHandler(...).Build()     │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────┐
│                         RUNTIME LAYER                                    │
│  Effect Executor │ Capability Manager │ Message Router                  │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────┐
│                         STORAGE LAYER                                    │
│  Object Stores (Typed) │ Cache Layer (L1/L2/L3) │ Object Pool           │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────▼──────────────────────────────────────┐
│                      BLOCKBERRY (via ABI)                                │
│               StateStore (IAVL), BlockStore, Mempool                    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Core Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Runtime | `runtime/` | Application lifecycle, module management |
| Effect System | `effects/` | Effect types, executor, dependency graph |
| Capabilities | `capability/` | Capability manager, typed capabilities |
| Object Stores | `store/` | Typed, cached storage with auto-serialization |
| Module Builder | `module/` | Declarative module construction |
| Traits | `traits/` | Reusable behavior interfaces |
| Core Types | `types/` | Account, Transaction, Message, Coin |
| Core Modules | `modules/` | Auth, Bank, Staking implementations |

### Core Modules

| Module | Purpose | Key Effects |
|--------|---------|-------------|
| Auth | Account management, authorization verification | WriteEffect[Account], nonce tracking |
| Bank | Token balances, transfers | TransferEffect, WriteEffect[Balance] |
| Staking | Validator management, delegations | WriteEffect[Validator], WriteEffect[Delegation] |

## Design Constraints

### Effect-Based Execution
- Handlers return `[]Effect` instead of mutating state directly
- Effects are collected, validated, and executed by the runtime
- Enables dependency analysis and parallel execution

### Capability Security
- Modules receive capabilities, not direct store access
- `AccountCapability`, `BalanceCapability`, `ValidatorCapability`
- All state access is traceable and auditable

### Named Accounts with Hierarchical Permissions
- Accounts are identified by human-readable names, not addresses
- Authority has threshold, key weights, and account delegation weights
- Supports recursive authorization with cycle detection

### Application Interface
```go
type Application interface {
    CheckTx(ctx context.Context, tx []byte) error              // Validation
    BeginBlock(ctx context.Context, header *BlockHeader) error // Block start
    ExecuteTx(ctx context.Context, tx []byte) (*TxResult, error) // Execution
    EndBlock(ctx context.Context) (*EndBlockResult, error)     // Validator updates
    Commit(ctx context.Context) (*CommitResult, error)         // State commit
    Query(ctx context.Context, path string, data []byte, height int64) (*QueryResult, error)
    InitChain(ctx context.Context, validators []Validator, appState []byte) error
}
```

### Transaction Flow
1. Transaction received with account name, authorization, message
2. `CheckTx()` validates authorization and message
3. Handler returns effects (does not mutate state)
4. Runtime builds dependency graph from effects
5. Runtime executes effects (potentially in parallel)
6. State committed to IAVL

## Code Conventions

### Package Structure

```
punnet-sdk/
├── runtime/                   # Application runtime
│   ├── application.go         # Main Application type
│   ├── router.go              # Message routing
│   └── context.go             # Execution context
│
├── effects/                   # Effect system
│   ├── effect.go              # Effect interface and types
│   ├── executor.go            # Effect execution
│   ├── graph.go               # Dependency graph
│   └── scheduler.go           # Parallel scheduler
│
├── capability/                # Capability system
│   ├── manager.go             # CapabilityManager
│   ├── account.go             # AccountCapability
│   ├── balance.go             # BalanceCapability
│   └── validator.go           # ValidatorCapability
│
├── module/                    # Module system
│   ├── module.go              # Module interface
│   ├── builder.go             # ModuleBuilder
│   └── handler.go             # Handler types
│
├── store/                     # Object stores
│   ├── store.go               # ObjectStore interface
│   ├── cached_store.go        # CachedObjectStore implementation
│   ├── account_store.go       # Typed AccountStore
│   ├── balance_store.go       # Typed BalanceStore
│   └── validator_store.go     # Typed ValidatorStore
│
├── types/                     # Core types
│   ├── account.go             # Account, Authority
│   ├── transaction.go         # Transaction structure
│   ├── message.go             # Message interface
│   ├── authorization.go       # Authorization
│   ├── coin.go                # Coin, Coins
│   ├── deprecation.go         # Rate-limited deprecation logging
│   └── effect.go              # Effect types
│
├── traits/                    # Reusable traits
│   ├── trait.go               # Trait interface
│   ├── authorizer.go          # Authorizer trait
│   ├── balancer.go            # Balancer trait
│   └── staker.go              # Staker trait
│
├── modules/                   # Core modules
│   ├── auth/                  # Auth module
│   ├── bank/                  # Bank module
│   └── staking/               # Staking module
│
├── schema/                    # Cramberry schemas
│   ├── types.cram             # Core types
│   ├── auth.cram              # Auth module
│   ├── bank.cram              # Bank module
│   └── staking.cram           # Staking module
│
├── examples/                  # Example applications
└── tests/                     # Tests
```

### Naming Conventions

- Interfaces: `type Effect interface`, `type Capability[T any] interface`
- Implementations: `type WriteEffect[T any] struct`, `type CachedObjectStore[T any] struct`
- Capabilities: `type AccountCapability interface`, `type BalanceCapability interface`
- Effects: `type TransferEffect struct`, `type EventEffect struct`
- Traits: `type Authorizer interface`, `type Balancer interface`
- Builders: `type ModuleBuilder struct`
- Errors: `var ErrNotFound = errors.New("not found")`

### Error Handling

- Use `fmt.Errorf("context: %w", err)` for wrapping
- Define sentinel errors at package level
- Never silently ignore errors
- Return errors from effect handlers, don't panic
- Validate effects before execution

### Effect Pattern

```go
func handleSend(ctx Context, msg *MsgSend) ([]Effect, error) {
    // 1. Validate inputs
    if msg.From != ctx.Account() {
        return nil, fmt.Errorf("sender must be tx account")
    }

    // 2. Read state through capabilities (for validation)
    balance, err := balanceCap.GetBalance(msg.From, msg.Amount.Denom)
    if err != nil {
        return nil, err
    }

    // 3. Return effects (don't mutate state)
    return []Effect{
        TransferEffect{From: msg.From, To: msg.To, Amount: msg.Amount},
        EventEffect{Type: "transfer", Attributes: ...},
    }, nil
}
```

## Build Commands

```bash
# Generate cramberry code from schemas
make generate

# Build all packages
make build

# Run tests with race detection
make test

# Run linter
make lint

# Clean generated files
make clean

# Or manually:
cramberry generate -lang go -out ./types/generated ./schema/*.cram
go build ./...
go test -race -v ./...
golangci-lint run
```

## Testing Guidelines

- Unit tests in same package with `_test.go` suffix
- Integration tests in `tests/integration/`
- Benchmark tests in `tests/benchmark/`
- Use table-driven tests
- Run with `-race` flag
- Test effect generation and execution separately
- Test capability access patterns
- Test parallel execution correctness
- Test authorization with hierarchical permissions
- Test cycle detection in delegation chains

## Mandatory Workflow for Implementation

**ALWAYS ultrathink about implementation decisions before starting each item in a plan**

**ALWAYS re-read CLAUDE.md and all documentation *.md files to ensure strict adherence to instructions and implementation plan before starting each item. If any part of the plan needs to be modified or re-ordered, ultrathink and perform any necessary modifications ensuring all documentation matches before proceeding.**

**ALWAYS follow these steps after completing each item in a plan:**

1. **Write comprehensive tests** - Add unit tests covering the new functionality, edge cases, and error conditions. Tests should be in the same package with `_test.go` suffix.

2. **Run all tests and fix failures** - Run `make test` or `go test -race ./...`. Fix any failures.

3. **Run integration tests** - Fix any failures.

4. **Verify build with no errors or warnings** - Run `go build ./...` and `golangci-lint run`. Fix any compiler errors, warnings, or linter issues before proceeding.

5. **Ensure no mention of AI tools** - Generated code or documentation should make absolutely no mention of AI usage.

6. **Update existing documentation** - Before summarizing, make sure that any relevant documentation (ARCHITECTURE.md, README, etc.) has been updated, especially if new implementation decisions have been made or changed.

7. **Append summary to PROGRESS_REPORT.md** - Before committing, append a comprehensive summary of what was implemented to `PROGRESS_REPORT.md`. Include:
   - Phase/task name and completion status
   - Files created or modified
   - Key functionality implemented
   - Test coverage summary
   - Any notable design decisions or trade-offs

8. **Commit with comprehensive message** - After all tests pass and the build is clean, commit the changes with a detailed commit message describing what was implemented and why. Do not add AI co-authoring credits. Make no mention of AI.

## Critical Invariants

1. **Effect immutability**: Handlers return effects, never mutate state directly
2. **Capability scope**: Modules only access state through granted capabilities
3. **Authorization validation**: All account actions verified via hierarchical authority
4. **Cycle detection**: Delegation chains checked for cycles before verification
5. **Effect validation**: All effects validated before execution
6. **Deterministic execution**: Same effects produce same state on all nodes
7. **IAVL compatibility**: Object stores use IAVL, merkle proofs fully supported

## Performance Targets

| Metric | Target |
|--------|--------|
| Transaction throughput | 100,000+ tx/sec (with parallelization) |
| CheckTx latency | < 1ms (cached reads) |
| ExecuteTx latency | < 5ms (effect collection + execution) |
| Parallel speedup | 4-8x (depends on tx dependencies) |
| Cache hit rate | > 95% (with proper warming) |
| L1 cache | ~1 MB (10k entries) |
| L2 cache | ~10 MB (100k entries) |

## Security Considerations

### Capability Isolation
- Modules cannot access stores directly
- Capabilities scoped per-module
- All state access tracked for auditing

### Effect Security
- Effects are strongly typed
- Conflict detection (read-write, write-write)
- Authorization checks on all effects
- Gas limits enforced on effect execution

### Authorization Security
- Cycle detection in delegation chains
- Threshold enforcement (weight >= threshold)
- Signature verification via Ed25519
- Nonce checking for replay protection

### Cryptographic Primitives
- Ed25519 for identity and signing
- SHA-256 for hashing
- IAVL for state commitments and merkle proofs

### SignDoc Security and Deprecation Logging
Messages should implement `SignDocSerializable` to bind signatures to full message content. Messages that don't implement this interface fall back to signers-only mode, which is a security weakness.

> **Migration Guide**: For detailed documentation on migrating from binary Cramberry signing to JSON-based SignDoc signing, see [`docs/migration/SIGNDOC_MIGRATION.md`](docs/migration/SIGNDOC_MIGRATION.md).

**Deprecation Logging**: The SDK logs rate-limited warnings when messages use the signers-only fallback. This helps identify messages that need migration to `SignDocSerializable`.

```go
// Example log output:
// DEPRECATION WARNING: message does not implement SignDocSerializable, using signers-only fallback.
// msg_type=/punnet.bank.v1.MsgSend security_note="signatures do not bind to full message content"
```

**Configuration**:
- `SetDeprecationLoggingEnabled(bool)` - Enable/disable deprecation warnings
- `SetDeprecationWarningInterval(duration)` - Set rate limit interval (default: 60s per message type)
- `SetDeprecationLogger(*log.Logger)` - Use custom logger

**Deprecation Timeline**:
- v0.x: Warning logged when fallback is used (current)
- v1.0: Consider making `SignDocSerializable` required
- Future: Remove signers-only fallback entirely
