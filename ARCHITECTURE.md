# Punnet SDK — Architecture

## Goals

1. **Effect-based execution** — handlers return effects, not state
   mutations. Effects are validated, then applied. Enables (in principle)
   parallel scheduling, replay, and state diff inspection.
2. **Capability-based or DI-based isolation** — modules access state
   only through scoped handles, not the global store.
3. **Named accounts** with hierarchical multi-sig authorities.
4. **bapi.Lifecycle native** — applications drop into any BAPI-compatible
   consensus engine.

## Type system

```go
type AccountName string                      // [a-z0-9.]+ ≤ 64

type Account struct {
    Name      AccountName
    Authority Authority
    Nonce     uint64
    CreatedAt int64
    UpdatedAt int64
}

type Authority struct {
    Threshold      uint64
    KeyWeights     map[string]uint64        // hex pubkey → weight
    AccountWeights map[AccountName]uint64   // delegated account → weight
}

type Authorization struct {
    Signatures            []Signature
    AccountAuthorizations map[AccountName]*Authorization   // recursive
}

type Transaction struct {
    Account       AccountName
    Messages      []Message
    Authorization *Authorization
    Nonce         uint64
}
```

`Authority.ValidateBasic` checks for `uint64` overflow when summing weights
and rejects unachievable thresholds. `Authorization.VerifyAuthorization`
walks the delegation graph DFS with a `visited` set, max depth 10, with
constant-time pubkey comparison in the signature-matching path.

## Effect system

`Effect` interface in `effects/effect.go`:

```go
type Effect interface {
    Type() EffectType
    Validate() error
    Dependencies() []Dependency
    Key() []byte
}
```

Concrete effects:

- `WriteEffect[T]` — `(store, key, value)`. Cramberry-serialized.
- `ReadEffect[T]` — `(store, key)`. Records dependency.
- `DeleteEffect[T]` — `(store, key)`.
- `TransferEffect` — `(from, to, denom, amount)`.
- `EventEffect` — `(eventType, attrs)`.

### Execution

`Executor.Execute(effects)` iterates effects in declaration order and
applies each against the typed stores. Sequential, deterministic.

An earlier parallel-scheduler stub
(`effects/scheduler.go`, `graph.go`, `Executor.ExecuteParallel`) was
removed (2026-05-15, PLAN Q5). It had no production caller and its
event-collection path was non-deterministic (goroutine scheduling
determined `result.events` ordering, which feeds into block hashing).
A real parallel execution layer should be Block-STM-style with
speculative execution + abort/retry, not a static dependency-graph
batching.

### Conflict detection caveat

`TransferEffect.Key()` returns `[]byte(e.From)` — the from account name.
`WriteEffect[Balance].Key()` returns `<store>/<key>` — a path. These do
**not** share a namespace, so a transfer from `alice` and a write at key
`balance/alice/stake` will not collide in `DetectConflicts`. Treat
conflict detection as best-effort until this is unified.

## Three-tier cache

`store/cache.go`:

- L1: ~10K entry LRU.
- L2: ~100K entry LRU.
- L3 ("backing"): IAVL or memory. Conceptually a third tier, mechanically
  just the store the cache wraps.

`MultiLevelCache.Get` checks L1 → L2 → backing. On hit, promotes to L1.

`CachedObjectStore.Set` writes to the cache only with `Dirty=true`.
`Flush` sorts dirty entries lex, writes through to IAVL,
`ClearDirtyFlags`. **No rollback semantics**: a tx that errors mid-execute
leaves dirty entries in cache; they get committed on next `Flush`.

## Stores

Two parallel store layers:

- **Legacy `IAVLStore`** (`store/iavl_store.go`) — uses `cosmos/iavl`
  directly. Fed by `JSONSerializer[T]`.
- **BAPI typed stores** (`store/bapi_*.go`) — `BAPIAccountStore`,
  `BAPIBalanceStore`, `BAPIValidatorStore`, `BAPIProposalStore`. Each
  takes a generic `statestore.StateStore` (which raspberry constructs
  via blockberry → avlberry) and uses `CramberryCodec[T]` for value
  serialization.

The production path uses BAPI stores. Don't mix them.

## Capability layer (legacy)

`capability/` exposes:

- `CapabilityManager.RegisterModule(name)`.
- `GrantBalanceCapability(moduleName) → BalanceCapability`.

Each capability prefixes its `BalanceStore` / `AccountStore` /
`ValidatorStore` under `module/<name>/`. This isolates modules but
prevents any cross-module read by design.

The BAPI runtime does not use this — modules receive direct typed-store
references via DI markers.

## Module system

Two coexisting interfaces:

### Legacy `module.Module`

```go
type Module interface {
    Name() string
    Dependencies() []string
    RegisterMsgHandlers() map[string]MsgHandler
    RegisterQueryHandlers() map[string]QueryHandler
    BeginBlock() BeginBlocker
    EndBlock() EndBlocker
    InitGenesis() InitGenesis
    ExportGenesis() ExportGenesis
}
```

`Registry` does Kahn's algorithm topological sort with deterministic
queue ordering. Cycles detected.

### BAPI `runtime.BAPIModule`

```go
type BAPIModule interface {
    Name() string
    RegisterMsgHandlers() map[string]BAPIMsgHandler
    RegisterQueryHandlers() map[string]BAPIQueryHandler
}

// Optional sub-interfaces:
type BAPIBlockProcessor    interface { BeginBlock(...); EndBlock(...) }
type BAPIGenesisInitializer interface { InitGenesis(...) }
type BAPIGenesisExporter    interface { ExportGenesis(...) }
```

No `Dependencies()` method. `BAPIRouter.RegisterModule` doesn't do
dep-resolution; module hooks fire in alphabetical order. (Dep-resolution
fix is a future cleanup.)

## Built-in modules

| Module        | File                                              | Status              |
|---------------|---------------------------------------------------|---------------------|
| `auth`        | `modules/auth/bapi_module.go`                     | Real; ExportGenesis stub |
| `bank`        | `modules/bank/bapi_module.go`                     | Real; multi-send map iteration is non-deterministic |
| `staking`     | `modules/staking/bapi_module.go`                  | Real; uses direct mutation; undelegate broken |
| `governance`  | `modules/governance/bapi_module.go`               | Real; not in old docs |

## Runtime

`runtime.BAPIApplication` (`runtime/bapi_application.go`) implements
`bapi.Lifecycle`:

- `Handshake` — cold start vs warm start, fallback genesis on inconsistent.
- `CheckTx` — pending-nonce tracking allows multiple in-flight txs from
  the same account.
- `ExecuteBlock` — `orderTxsByNonce` re-sorts txs by `(account, nonce)`
  (DAG mempool can deliver out of order). Sequentially executes each tx.
- `Commit` — `stateStore.Commit()`.
- `Query` — routes to `BAPIRouter.GetQueryHandler`.

Two TODOs:
- `bapi_application.go:797` — params updates dropped.
- `bapi_application.go:804` — evidence processing (slashing) is no-op.

## Tokenomics module roadmap (Phase T)

Phase T extends punnet-sdk into a full tokenomics SDK. The
phase-by-phase task list and the full decisions log (D1–D25) live in
[`/Volumes/Tendermint/stealth/PLAN.md`](../PLAN.md) §7 — that is the
authoritative spec. This section describes *what* the architecture
will look like once Phase T lands; it does not restate the rationale
behind each decision.

The build maps spec §1–§13 onto the modules below. "new" = greenfield;
"extends" = significant additions to an existing module.

| Spec § | Module(s)                              | Status   |
|--------|----------------------------------------|----------|
| §1 Supply & Account Manager       | `bank` extends                  | partial  |
| §2 Fee Schedule Registry          | `fees` (new)                    | new      |
| §3 Fee Computation & Routing      | new AnteHandler in `runtime/`   | new      |
| §4 Staking & Delegation           | `staking` extends               | partial  |
| §5 Validator Registry & Set       | `staking` extends               | partial  |
| §6 Block Emission                 | `mint` (new)                    | new      |
| §7 Participation Tracker          | `participation` (new)           | new      |
| §8 Reward Pool & Distribution     | `distribution` (new)            | new      |
| §9 Slashing                       | `staking.ProcessEvidence` extends | partial |
| §10 Governance                    | `governance` extends            | partial  |
| §11 Weak Subjectivity             | `wsync` (new)                   | new      |
| §12 Bootstrap Lockup              | `staking` extends               | partial  |
| §13 Mempool Interface             | `bapi.MempoolObserver` (new)    | new      |

### New module responsibilities

- **`fees`** — fee schedule registry. State is `FeeSchedule{OpFees,
  ByteFee}` plus a pending-update queue keyed by effective height.
  Schedule changes route through governance (Phase 4) with a 7-day
  timelock.
- **`mint`** — block emission. Each EndBlock computes
  `B_t = ρ·CS_t·min(1, VRP/V_threshold)`, debits the validator-reward
  pool (VRP) module account, and credits the per-epoch Emission Pool.
  Internal accumulators run at 18-decimal precision and round down on
  credit; the remainder stays in VRP.
- **`participation`** — implements `bapi.MempoolObserver`. Holds
  in-memory `{leader_blocks[v], batches_certified[v]}` per epoch and
  persists at epoch close. `OnBlockConstructed` bumps `leader_blocks`
  only when the block included ≥1 certified batch (D11). Mid-epoch
  counters are part of the state-sync export so a joiner converges to
  the same numbers.
- **`distribution`** — Cosmos F1-style reward distribution. Per-
  validator `reward_per_share_index`, `last_period_index`,
  `slash_events[]`; per-delegator `joined_at_index`. Epoch-close hook
  computes `share(v) = α·(leader_blocks_v/total) +
  (1−α)·(batches_certified_v/total)` with `α = 0.3`, credits each
  validator's accumulator, and `MsgWithdrawDelegatorReward` walks the
  delegator's joined index forward applying slash-period adjustments.
  Reward distribution lives in its own module (not folded into
  `staking`) to keep the F1 model self-contained.
- **`wsync`** — weak-subjectivity checkpoints. Rolling window of ≥21d
  of `(height, app_hash, validator_set_hash)` checkpoints, emitted at
  every `Height % blocks_per_hour == 0`, signed by the active set, and
  committed when ≥2/3 stake have attested. Bounds the trust horizon
  at the unbonding period.

### Extensions to existing modules

- **`bank`** — adds module-account addressing (`module:<name>` →
  address) and a permission table for mint/burn/transfer. Adds a
  supply-conservation invariant: `Σ(balances) ≡ total_supply` at every
  block height. The four protocol accounts (VRP, CT, E, BL) are
  ordinary module-controlled bank accounts (D5).
- **`staking`** — gains an unbonding queue (21-day maturity, per-
  validator FIFO drained in EndBlock), a 5% commission floor, top-N
  active-set computation refreshed at epoch boundaries only, the
  bootstrap-validator state machine (12-month lock, 30-day linear
  vest starting at `genesis + 12mo`, commission pinned at exactly 5%
  — D17, D19), and the slashing severity table (equivocation 5%,
  liveness 0.1%, leader-equivocation 5%) with jail/tombstone state.
  All slashed funds route to CT (D20). Per-validator register bond
  (`0.01%·S`) is refunded on clean deregister and forfeited to CT on
  jail/tombstone (D23).
- **`governance`** — gains timelock classes (`Simple7d`, `Super30d`,
  `Super60d`, `Constitutional`), super/simple vote thresholds (simple
  majority vs ≥2/3), per-parameter `(soft, hard)` band registry, and a
  `PendingEnactment` queue applied at `effective_height` via target
  modules' parameter-update hooks. Bundled proposals are supported so
  related parameters can move atomically.

### Cross-cutting plumbing

- **`bapi.MempoolObserver`** — a new opt-in bapi capability with
  `OnBatchCertified(validator, batchHash, txCount, byteCount)` and
  `OnBlockConstructed(leader, includedBatchHashes)`. Capability
  discovery at handshake (same pattern as `BAPIEvidenceHandler`).
  Raspberry fires `OnBatchCertified` on cert-quorum (D10) and
  `OnBlockConstructed` on commit; `participation` consumes both.
  Mempool observation does not change `bapi.Lifecycle`.
- **`Transaction` wire format** — extended with an explicit `Fee`
  field carrying `{OpFees []OpFeeEntry, ByteFee uint64, Priority
  int64, Payer AccountName}`. The fee is part of the signed payload
  so submitters cannot undercut a schedule change after signing
  (D8). The priority unit is a fixed token amount independent of
  OpFee/ByteFee schedule (D14).

## AnteHandler

The AnteHandler is a new pre-execution stage in
`runtime/bapi_application.go`. It is an ordered chain of handlers that
run *before* module handlers see the transaction. Each handler is
either pure-validation (rejects the tx) or effect-emitting (its effects
join the tx's effect set and flush atomically with the handler effects).

```
ExecuteTx(tx)
  ├── AnteHandler.Run(tx)         // pre-execution stage
  │     ├── nonce check
  │     ├── fee deduction         // Phase 1
  │     └── ... (extensible)
  └── ModuleRouter.Handle(tx)     // module handlers
```

Key properties:

- **Failed-execution txs still pay.** Because the fee handler runs in
  the pre-execution stage, fees deduct before the module handler runs
  and stay deducted even if the handler errors. The AnteHandler stage
  itself is the unit that decides "tx is admissible"; once admissible,
  the tx's fee effects and any handler effects flush together as one
  block-atomic write.
- **Same-sender sequencing.** The AnteHandler enforces nonce at
  execution time (D24). DAG sharding makes mempool-level sequencing
  infeasible, so a tx that arrives with a stale or future nonce still
  pays its fee on rejection. Replace-by-fee is deferred to v2.
- **Atomicity.** AnteHandler effects and handler effects belong to
  the same tx-scoped effect set. Either both apply or neither does;
  the tx is the unit of atomicity, not the AnteHandler stage in
  isolation.

The Phase 0 milestone wires the empty chain; the fee-deduction handler
lands in Phase 1 (see PLAN.md §7).

## Parameter tiers

Phase T introduces three parameter tiers (D25). Every tunable in the
tokenomics spec falls into exactly one tier; the tier determines who
can change the parameter and through what mechanism.

- **Constitutional** — hardcoded Go constants. Changing the value
  requires a code change, a coordinated upgrade, and the
  `Constitutional` governance class (Phase 4). Examples:
    - Allocation splits (`25/30/30/10/5` for VRP/CT/E/BL/initial
      supply).
    - Commission floor (`5%`).
    - Slashing severities (`5%` equivocation / `0.1%` liveness /
      `5%` leader-equivocation).
    - `α = 0.3` in the participation share formula.
    - Bootstrap commission (`= 5%` exact).
- **Genesis-tunable** — set in the genesis doc, immutable post-
  launch. Per-chain configuration; the *space* of valid values is
  constitutional, but the *specific* value is the operator's choice.
  Examples:
    - `total_supply` (`int64` in genesis).
    - Bootstrap validator list and per-validator BL share.
    - `blocks_per_epoch`, `blocks_per_hour`.
- **Governance-tunable** — changeable at runtime via §10 governance
  proposals, subject to the per-parameter `(soft, hard)` band and the
  appropriate timelock class. Examples:
    - Fee schedule (`OpFees`, `ByteFee`) — `Simple7d`.
    - `ρ` (emission rate) — `Super30d`.
    - `V_threshold` — `Super30d`.
    - Unbonding period (`21d` default) — `Super30d`.

Tiers are enforced at the boundary they apply to: constitutional
constants are unreachable from `MsgSubmitProposal`; genesis-tunable
parameters have no governance message; governance-tunable parameters
are gated by the class and band registry in `governance`.

## Layout

```
punnet-sdk/
├── types/                Account, Authority, Authorization, Coin, Tx, Message, codec, errors
├── effects/              Effect + impls + graph + scheduler + executor + bapi_executor + validator
├── store/                Cache + cached_store + iavl_store + memory_store + prefix + serializer
│                         + bapi_account_store + bapi_balance_store + bapi_validator_store
│                         + bapi_proposal_store + bapi_provider
├── capability/           CapabilityManager (legacy)
├── module/               Module interface (legacy) + builder + registry
├── modules/auth/         auth (legacy + bapi)
├── modules/bank/         bank (legacy + bapi)
├── modules/staking/      staking (legacy + bapi)
├── modules/governance/   governance (bapi only)
├── runtime/              Application (legacy) + BAPIApplication (current) + router + context + lifecycle + genesis
├── di/                   Dependency injection container + markers
├── client/               RPC client SDK
├── config/               Config types
├── examples/minimal/     Working example
├── schema/               .cram schemas
├── traits/               EMPTY (PLAN D2 — fill or delete)
└── tests/                Integration tests
```
