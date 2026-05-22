// Package wsync implements weak-subjectivity checkpoints per
// tokenomics spec §7 / §11 and PLAN §7 Phase 5.
//
// A weak-subjectivity (WS) checkpoint pins the chain state at a
// specific height with a tuple
//
//	(height, app_hash, validator_set_hash)
//
// signed by ≥2/3 of the active set's stake at that height. A
// fresh client joining the network is given any checkpoint
// no older than the unbonding period (21 days per spec §7);
// they verify the signatures against the active set known to
// them (e.g. from a trusted snapshot or peer attestation), and
// once verified they sync forward from that point with normal
// fork-choice rules.
//
// This module maintains the rolling-window store of checkpoints
// and exposes the verification entrypoint used by light clients +
// state-syncing nodes. Per-validator attestation gossip is a
// cross-repo concern (looseberry-adjacent stream + raspberry
// wiring; PLAN §7 Phase 5.3) and is not in this package. The
// module accepts attestations via AddAttestation, which the
// network layer calls when a validator's signature arrives.
//
// Pruning: at every EndBlock, checkpoints older than
// UnbondingPeriodBlocks are removed from the store. The
// rolling-window invariant gives clients a bounded staleness
// horizon: a peer cannot trick a fresh client into accepting a
// checkpoint signed by a validator set that has since been fully
// unbonded.
package wsync

// ModuleName is the module's unique name.
const ModuleName = "wsync"

// StorePrefix is the typed-store namespace.
const StorePrefix = "wsync/"

// HourlyBlocks is the checkpoint cadence. At a 1-second block
// cadence this is one hour. Spec §7: "Emitted hourly; older
// checkpoints remain valid for clients within the trust horizon."
const HourlyBlocks uint64 = 3600

// UnbondingPeriodBlocks is the chain-wide unbonding-period length
// in blocks (21 days at 1s cadence). Checkpoints older than this
// are pruned and rejected by VerifyCheckpoint. Mirrors
// staking.UnbondingPeriodBlocks but inlined here to avoid an
// import cycle when wsync grows downstream consumers.
const UnbondingPeriodBlocks uint64 = 21 * 24 * 60 * 60

// SignatureThresholdBps is the supermajority stake threshold
// required for a checkpoint to be considered valid. Spec §7:
// "valid when signatures aggregating ≥ 2/3 of active stake are
// present." 6667 bps = 2/3 + epsilon.
const SignatureThresholdBps uint64 = 6667

// Hash32 is a 32-byte content hash used for app_hash and
// validator_set_hash. Aliased here so the spec-level vocabulary
// matches the field names; the underlying type is just a byte
// array.
type Hash32 = [32]byte

// Checkpoint pins a single height to its post-commit chain state.
// Once attestations totalling ≥SignatureThresholdBps of the
// active stake are attached, the checkpoint is "valid" — fresh
// nodes can sync from it.
type Checkpoint struct {
	// Height is the block height the checkpoint references.
	// Always a multiple of HourlyBlocks.
	Height uint64 `cramberry:"1" json:"height"`

	// AppHash is the application state root after committing
	// block Height. Identical to what `Commit` returns at that
	// height; ties the checkpoint to a specific deterministic
	// state.
	AppHash Hash32 `cramberry:"2" json:"app_hash"`

	// ValidatorSetHash is a deterministic hash of the active
	// validator set at Height, used by the fresh client to
	// reconstruct the set the signatures must verify against.
	// The hash function is spec-pinned (sha256 of the
	// concatenated, hex-sorted, big-endian-power-prefixed
	// pubkeys) so independent implementations agree.
	ValidatorSetHash Hash32 `cramberry:"3" json:"validator_set_hash"`

	// Attestations are the per-validator signatures aggregated
	// for this checkpoint. Each entry's VotingPower is the
	// validator's stake at Height; the sum across attestations
	// must reach SignatureThresholdBps of TotalActiveStake for
	// the checkpoint to be considered valid.
	Attestations []Attestation `cramberry:"4" json:"attestations"`
}

// SignedContent returns the bytes the validator signed: the
// height + app_hash + validator_set_hash concatenated. Public so
// both the gossip layer and the verifier compute the same digest.
func (c *Checkpoint) SignedContent() []byte {
	out := make([]byte, 0, 8+32+32)
	for i := uint(0); i < 8; i++ {
		out = append(out, byte(c.Height>>(8*i)))
	}
	out = append(out, c.AppHash[:]...)
	out = append(out, c.ValidatorSetHash[:]...)
	return out
}

// Attestation is one validator's signature over a Checkpoint's
// SignedContent.
type Attestation struct {
	// ValidatorPubKey is the validator's Ed25519 pubkey (32 bytes).
	ValidatorPubKey []byte `cramberry:"1" json:"validator_pub_key"`

	// Signature is the Ed25519 signature over SignedContent.
	Signature []byte `cramberry:"2" json:"signature"`

	// VotingPower is the validator's stake at the checkpoint's
	// height. Stored on the attestation rather than re-derived
	// to keep verification self-contained.
	VotingPower uint64 `cramberry:"3" json:"voting_power"`
}

// CheckpointSource is the interface the runtime implements so
// the wsync module can pull the current app_hash and active
// validator-set hash without importing the runtime directly.
// Phase 5.2 wiring sets a source at construction; wsync's
// EndBlock at hourly boundaries calls these methods to build the
// fresh checkpoint.
//
// An app that doesn't wire a CheckpointSource gets no automatic
// checkpoint emission — useful for unit tests, but production
// chains must provide one.
type CheckpointSource interface {
	// LatestAppHash returns the post-commit app hash for the
	// most recently committed block. wsync calls this at the
	// END of an EndBlock for an hourly height to capture the
	// hash of the about-to-commit block.
	LatestAppHash() Hash32

	// ActiveValidatorSetHash returns the hash of the current
	// active validator set. Format is spec-pinned (see
	// Checkpoint.ValidatorSetHash); implementers must produce
	// the same digest as the verifier.
	ActiveValidatorSetHash() Hash32
}

// Typed-store key helper. Checkpoints are keyed by zero-padded
// height so lexical iteration matches numeric order.
const keyCheckpointFmt = "checkpoint/%020d"
