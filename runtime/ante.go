package runtime

import (
	"context"

	"github.com/blockberries/punnet-sdk/effects"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// AnteHandler is a pre-execution stage that runs after authorization
// validation and before module message handlers in executeTx. The
// chain is intended for cross-cutting concerns that every transaction
// must clear regardless of which messages it carries — fee deduction,
// gas metering (if ever added), spam guards, etc.
//
// Semantics:
//
//   - The chain runs in registration order. The first handler returning
//     a non-nil error aborts the transaction; subsequent handlers in
//     the chain (and the module handlers) do not run.
//   - Effects returned by handlers are folded into the same effect
//     batch as the message handlers' effects. Today everything commits
//     atomically as one batch. Phase 1.5 of the tokenomics plan (PLAN
//     §7) will split the commit so AnteHandler effects (fee deduction)
//     persist even when message handler execution fails — the
//     "failed-execution txs still pay" invariant from spec §3.
//   - Handlers MUST be deterministic and MUST NOT observe wall-clock
//     time, randomness, or other nondeterministic sources. They run
//     on every validator and the resulting state mutations must be
//     byte-identical across the network.
//
// Empty chain is the default; behaviour is identical to the
// pre-tokenomics path. PLAN §7 Phase 0.4 / Phase 1.4 wires the fee
// handler.
type AnteHandler func(ctx context.Context, txCtx *BAPITxContext, tx *ptypes.Transaction) ([]effects.Effect, error)
