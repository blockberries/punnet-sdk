// Package wsync implements weak-subjectivity checkpoints per
// tokenomics spec §7 / §11.
//
// A WS checkpoint pins the chain state at a height with the tuple
//
//	(height, app_hash, validator_set_hash)
//
// signed by ≥2/3 of the active set's stake at that height. A
// fresh client joining the network is given any checkpoint no
// older than the unbonding period (21 days); they verify the
// signatures against the active set known to them (e.g. from a
// trusted snapshot or peer attestation), and once verified they
// sync forward from that point with normal fork-choice rules.
//
// State layout (typed-store prefix "wsync/"):
//
//	checkpoint/<padded-height>  →  Checkpoint
//
// Pruning at every EndBlock removes entries below
// currentHeight - UnbondingPeriodBlocks so the rolling window
// stays bounded.
//
// Emission cadence: at every Height % HourlyBlocks == 0, EndBlock
// reads the chain's app_hash + active-validator-set hash from a
// CheckpointSource (interface implemented by the runtime; nil
// disables emission) and stores the unsigned checkpoint. The
// gossip layer (looseberry-adjacent stream — cross-repo) collects
// validator attestations and calls AddAttestation as signatures
// arrive.
//
// VerifyCheckpoint is the fresh-node entrypoint: checks height
// freshness, validator-set-hash match, per-attestation signature
// validity, and 2/3 aggregated stake. ComputeValidatorSetHash is
// the spec-pinned cross-implementation digest.
//
// PLAN §7 Phase 5.
package wsync
