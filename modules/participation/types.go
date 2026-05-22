// Package participation implements the per-validator participation
// counters per tokenomics spec §3.4 / §7. The module observes the
// bapi.MempoolObserver events and tracks:
//
//   - leader_blocks[v]: incremented when validator v proposes a
//     committed block that contains ≥1 certified batch (spec §3.4 /
//     PLAN decision D11). Empty blocks earn no leader credit.
//   - batches_certified[v]: incremented when a batch produced by
//     any of v's W workers reaches cert-quorum. Credit goes to the
//     validator (not the worker — D16).
//
// At epoch close (Height % EpochBlocks == 0), the distribution
// module reads the frozen counters to compute share(v); the
// counters then reset for the next epoch.
//
// PLAN §7 Phase 3.3 / 3.4.
package participation

// ModuleName is the module's unique name used for genesis blob
// keying and the BAPI router registry.
const ModuleName = "participation"

// StorePrefix is the typed-store namespace for participation state.
// All keys live under "participation/" — current epoch counters and
// frozen per-epoch summaries share the prefix; sub-keys disambiguate.
const StorePrefix = "participation/"

// ValidatorParticipation is the per-validator counter pair for one
// epoch. Stored under
// "current/<validator-hex>" while the epoch is in progress, then
// frozen at epoch close under "epoch/<epoch-num>/<validator-hex>"
// for distribution to read.
type ValidatorParticipation struct {
	// LeaderBlocks is the count of blocks where this validator was
	// the proposer AND the block contained ≥1 certified batch.
	// PLAN decision D11.
	LeaderBlocks uint64 `cramberry:"1" json:"leader_blocks"`

	// BatchesCertified is the count of cert-quorum events for
	// batches authored by any of this validator's workers in this
	// epoch.
	BatchesCertified uint64 `cramberry:"2" json:"batches_certified"`
}

// EpochTotals is the chain-wide sum across all validators for one
// epoch. Stored as "current/totals" (mutable) and
// "epoch/<epoch-num>/totals" (frozen). The distribution module
// uses these as the denominators in share(v).
type EpochTotals struct {
	LeaderBlocks     uint64 `cramberry:"1" json:"leader_blocks"`
	BatchesCertified uint64 `cramberry:"2" json:"batches_certified"`
}

// Key prefixes within StorePrefix.
const (
	// keyCurrentValidator: "current/v/<address-hex>"
	// — mutable per-validator counters for the in-progress epoch.
	keyCurrentValidatorPrefix = "current/v/"
	// keyCurrentTotals: "current/totals"
	keyCurrentTotals = "current/totals"
	// keyEpochValidator: "epoch/<num>/v/<address-hex>" — frozen at
	// epoch close.
	keyEpochValidatorFmt = "epoch/%020d/v/%s"
	// keyEpochTotals: "epoch/<num>/totals" — frozen at epoch close.
	keyEpochTotalsFmt = "epoch/%020d/totals"
)
