// Package participation implements the per-validator participation
// counters per tokenomics spec §3.4 / §7. The module observes the
// bapi.MempoolObserver events and tracks:
//
//   - leader_blocks[v]: incremented when validator v proposes a
//     committed block that contains ≥1 certified batch (PLAN
//     decision D11). Empty blocks earn no leader credit.
//   - batches_certified[v]: incremented when a batch produced by
//     any of v's W workers reaches cert-quorum. Credit goes to
//     the validator, not the worker (D16).
//
// State layout (typed-store prefix "participation/"):
//
//	current/v/<address-hex>        →  ValidatorParticipation
//	current/totals                 →  EpochTotals
//	epoch/<N>/v/<address-hex>      →  ValidatorParticipation (frozen)
//	epoch/<N>/totals               →  EpochTotals (frozen)
//
// At every epoch-close EndBlock, the current rows are FROZEN
// under "epoch/<N>/" and the current set is cleared. The
// distribution module reads the frozen records to compute
// share(v) at the same epoch close.
//
// Determinism caveat: counter writes happen in the
// bapi.MempoolObserver callbacks, which fire from raspberry
// outside the ExecuteBlock effect pipeline. Cross-validator
// agreement depends on raspberry delivering the same event
// sequence to every node — PLAN §7 Phase 3.8 is the consensus
// layer's piece.
//
// PLAN §7 Phases 3.3 + 3.4.
package participation
