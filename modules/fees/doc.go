// Package fees implements the fee schedule registry and the
// fee-routing AnteHandler per tokenomics spec §3.
//
// State layout (typed-store prefix "fees/"):
//
//	current        →  FeeSchedule         (active schedule)
//	pending/<H>    →  PendingFeeUpdate    (governance-queued updates;
//	                                       activated at EndBlock for
//	                                       height == H)
//
// The AnteHandler reads the schedule on every tx, validates the
// submitted Fee against it, deducts the total from the payer, and
// routes the proceeds:
//
//   - per-op + per-byte components → CT (module.ct)
//   - priority component           → priority_pool_pending (module.pp)
//
// PLAN §7 Phases 1.1–1.4 and 4.5 (governance-tunable byte_fee +
// op_fee:<type> via BAPIModuleParams).
package fees
