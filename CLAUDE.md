# Punnet SDK — Development Guidelines

## What this is

Punnet SDK is the application framework for Stealth chains. The
production runtime is `runtime.BAPIApplication`, which implements
`bapi.Lifecycle`. Raspberry's `--app=punnet` mode wires this in.

## Two coexisting runtimes

There are two parallel runtimes in this codebase, and only the second is
actually used:

- **Legacy** (`runtime.Application`) — the older path with
  `runtime.Context` handlers, `CapabilityManager` for module isolation,
  and seven-method ABCI-style methods. Still tested. Not used by raspberry.
- **BAPI** (`runtime.BAPIApplication`) — the current path. Implements
  `bapi.Lifecycle` (5 methods). Modules implement `BAPIModule`. Stores
  are typed-store-injected via `di/` markers.

When in doubt, **work in the BAPI path**. Touch the legacy path only if
you understand it will not affect production.

## Conventions

- Module handlers MUST return effects, never mutate state directly.
  Today's staking module violates this (PLAN T1-8) and that is being
  fixed; do not add new violations.
- Do not iterate Go maps when computing consensus-critical bytes. Sort
  keys first. Bank multi-send currently violates this (T1-7); the fix
  is straightforward.
- Use `cramberry`-encoded Tx/SignDoc, never `json.Marshal`. Today's
  `Transaction.GetSignBytes` uses JSON (T1-6); will move to cramberry.
- All exported symbols have doc comments beginning with the symbol name.

## Effects

```go
type Effect interface {
    Type() EffectType
    Validate() error
    Dependencies() []Dependency
    Key() []byte
}
```

Concrete effects: `WriteEffect[T]`, `ReadEffect[T]`, `DeleteEffect[T]`,
`TransferEffect`, `EventEffect`.

The runtime calls `effectExecutor.Execute(allEffects)` sequentially. The
earlier parallel-scheduler stub (`effects/scheduler.go`, `graph.go`,
`Executor.ExecuteParallel`) was removed (PLAN Q5, 2026-05-15) because
its event-collection path was non-deterministic in a way that's not
appropriate for a deterministic blockchain runtime, and it had no
production caller. Any future parallel-execution effort should be a
fresh design (e.g. Block-STM-style speculative execution with abort/retry)
rather than wiring up that stub.

## Capabilities (legacy)

The capability layer is in `capability/`. It works by prefixing each
module's stores under `module/<name>/`, isolating namespaces. **The
current BAPI runtime does not use this.** Modules in the BAPI path
receive typed stores directly via DI markers.

## Testing

```bash
go test -race ./...     # All
```

725 tests pass.

## Building

```bash
go build ./...
go vet ./...
```

The `examples/minimal` example builds a working app:

```bash
go build -o minimal ./examples/minimal
./minimal
```

## Workflow

After every change:

1. Add tests in the same package.
2. `go test -race ./...` — must pass.
3. `go vet ./...` and `golangci-lint run` (if configured).
4. Update README / ARCHITECTURE if interfaces or behavior changed.

## Known issues

Tracked in [`/Volumes/Tendermint/stealth/PLAN.md`](../PLAN.md):

- `IAVLStore` legacy path uses `cosmos/iavl` directly; BAPI path uses
  avlberry via `blockberry/pkg/statestore`. Don't mix them.
