# Punnet SDK

An application framework for the [Stealth blockchain stack](../README.md).
Modules return effects, not state mutations. Accounts are named, not
hash-derived. Authorization is hierarchical multi-sig with delegation.

## Features

- **Effect-based execution** — handlers return `Read` / `Write` /
  `Delete` / `Transfer` / `Event` effects which are validated and applied.
- **Named accounts** with regex `^[a-z0-9.]+$`, max 64 chars.
- **Hierarchical authority** — `Authority{Threshold, KeyWeights, AccountWeights}`
  with delegation cycle detection (DFS, max depth 10) and constant-time
  pubkey comparison.
- **Module system** with dependency-ordered initialization, BeginBlock /
  EndBlock hooks, and genesis init/export.
- **Three-tier cache** (L1 + L2 + IAVL backing) with dirty-flag tracking.
- **bapi.Lifecycle implementation** — punnet apps drop into raspberry
  via the `BAPIApplication` runtime.
- **Built-in modules**: `auth`, `bank`, `staking`, `governance`.
  Tokenomics modules (`fees`, `mint`, `participation`, `distribution`,
  `wsync`) are planned — see Phase T in
  [`/Volumes/Tendermint/stealth/PLAN.md`](../PLAN.md) §7.

## Quick start

```go
import (
    "github.com/blockberries/punnet-sdk/runtime"
    "github.com/blockberries/punnet-sdk/modules/auth"
    "github.com/blockberries/punnet-sdk/modules/bank"
    "github.com/blockberries/punnet-sdk/modules/staking"
)

stateStore := /* construct via blockberry/pkg/statestore */
accStore   := /* construct typed BAPI store from stateStore */
balStore   := /* construct typed BAPI store from stateStore */
valStore   := /* construct typed BAPI store from stateStore */

app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
    ChainID:    "mainnet-1",
    StateStore: stateStore,
    Modules: []runtime.BAPIModule{
        auth.NewBAPIModule(accStore),
        bank.NewBAPIModule(accStore, balStore),
        staking.NewBAPIModule(accStore, balStore, valStore),
    },
})

// app implements bapi.Lifecycle — pass to raspberry or any BAPI consumer.
```

## Module skeleton

```go
type MyModule struct { /* ... */ }

func (m *MyModule) Name() string { return "my" }

func (m *MyModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
    return map[string]runtime.BAPIMsgHandler{
        "MsgFoo": m.handleFoo,
    }
}

func (m *MyModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
    return map[string]runtime.BAPIQueryHandler{
        "/my/foo": m.queryFoo,
    }
}

func (m *MyModule) handleFoo(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
    return []effects.Effect{
        effects.NewWriteEffect(myStore, "key", value),
        effects.NewEventEffect("foo", attrs),
    }, nil
}
```

## Built-in modules

Today's runtime ships four modules. Six more are planned as part of the
Phase T tokenomics build-out (see
[`/Volumes/Tendermint/stealth/PLAN.md`](../PLAN.md) §7); existing modules
also pick up extensions for spec coverage.

### Available today

| Module        | Messages                                      | State                       |
|---------------|-----------------------------------------------|------------------------------|
| `auth`        | MsgCreateAccount, MsgUpdateAuthority, MsgDeleteAccount | Accounts (name, authority, nonce) |
| `bank`        | MsgSend, MsgMultiSend                         | Balances per (account, denom) |
| `staking`     | MsgCreateValidator, MsgDelegate, MsgUndelegate | Validators (commission, total delegation), evidence skeleton |
| `governance`  | MsgSubmitProposal, MsgVote, MsgDeposit       | Proposals, votes, deposits  |

### Planned — Phase T (tokenomics)

| Spec § | Module          | Status   | Scope (summary)                                      |
|--------|-----------------|----------|------------------------------------------------------|
| §1     | `bank`          | extends  | Module-account addresses (VRP/CT/E/BL), supply invariant |
| §2     | `fees`          | new      | Fee schedule registry + pending-update queue          |
| §3     | runtime AnteHandler | new  | Pre-execution fee deduction + routing                 |
| §4     | `staking`       | extends  | Unbonding queue (21d), 5% commission floor            |
| §5     | `staking`       | extends  | Top-N active set, per-epoch refresh, bootstrap reg.  |
| §6     | `mint`          | new      | Block emission (ρ, V_threshold), credits Emission Pool |
| §7     | `participation` | new      | Implements `bapi.MempoolObserver`; leader/cert tallies |
| §8     | `distribution`  | new      | Cosmos F1 model with claim-rewards                    |
| §9     | `staking`       | extends  | Slash severity 5%/0.1%/5%, CT credit, jail/tombstone  |
| §10    | `governance`    | extends  | Timelock classes (7/30/60d), super/simple thresholds  |
| §11    | `wsync`         | new      | Weak-subjectivity checkpoints (hourly, ≥2/3 stake)   |
| §12    | `staking`       | extends  | BL accounts, 12mo lock, 30d linear vest, 5% cap      |
| §13    | `bapi.MempoolObserver` | new | Opt-in capability; raspberry fires, consumers tally   |

Phase T is phased and dependency-ordered; see PLAN.md §7 for the
phase-by-phase task list and §7.1 for the full decisions log (D1–D25).

## Layout

```
punnet-sdk/
├── types/                Account, Authority, Authorization, Coin, Tx, Message
├── effects/              Effect interface + Read/Write/Delete/Transfer/Event impls
│                         + dependency graph + scheduler + executor
├── store/                Cache (3-tier), typed BAPI stores, IAVL/memory backends
├── capability/           Account/Balance/Validator capabilities (legacy path)
├── module/               Legacy module interface + builder + registry
├── modules/auth/         auth module (legacy + bapi variants)
├── modules/bank/         bank module
├── modules/staking/      staking module
├── modules/governance/   governance module
├── runtime/              Application (legacy) + BAPIApplication (current)
├── di/                   Dependency injection container
├── client/               RPC client SDK (per-module wrappers)
├── config/               Config types
├── examples/minimal/     Minimal working app
├── schema/               .cram schemas
└── tests/                Integration tests
```

## Status

Pre-alpha. The framework compiles, 725 unit tests pass, and the example
app boots. Several real bugs prevent shipping in production — see
[`/Volumes/Tendermint/stealth/PLAN.md`](../PLAN.md) §2.2:

Phase B (T1-6 SignDoc cramberry, T1-7 bank multi-send sort, T1-8
staking effects/undelegate) and Phase C (evidence pipeline,
ConsensusParams slash-fraction, BAPI store iteration) are all green.
725 tests pass.

The earlier parallel-scheduler stub (`effects/scheduler.go`, `graph.go`,
`Executor.ExecuteParallel`) was removed (2026-05-15, PLAN Q5). Any
future parallel-execution effort should be a fresh design
(Block-STM-style with abort/retry) rather than wiring up that stub.

## Development

See [`CLAUDE.md`](./CLAUDE.md) for development guidelines.
[`ARCHITECTURE.md`](./ARCHITECTURE.md) for design details.

## License

Apache-2.0.
