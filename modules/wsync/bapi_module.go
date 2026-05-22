package wsync

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
)

// BAPIWsyncModule maintains the weak-subjectivity checkpoint
// rolling window. Implements runtime.BAPIModule,
// runtime.BAPIBlockProcessor, and runtime.BAPIGenesisInitializer.
//
// The checkpoint source is set via WithCheckpointSource at module
// construction. Production chains wire the runtime as the source;
// tests pass a stub that returns canned values.
type BAPIWsyncModule struct {
	checkpoints *store.TypedStore[*Checkpoint]
	source      CheckpointSource
}

// NewBAPIWsyncModule constructs the module backed by the given
// state store. The CheckpointSource is initially nil; call
// WithCheckpointSource before any EndBlock fires at an hourly
// boundary, otherwise the boundary EndBlock is a no-op.
func NewBAPIWsyncModule(ss statestore.StateStore) (*BAPIWsyncModule, error) {
	if ss == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}
	return &BAPIWsyncModule{
		checkpoints: store.NewTypedStore[*Checkpoint](ss, StorePrefix),
	}, nil
}

// WithCheckpointSource sets the source used at hourly EndBlock
// boundaries to populate Checkpoint.AppHash + ValidatorSetHash.
func (m *BAPIWsyncModule) WithCheckpointSource(src CheckpointSource) *BAPIWsyncModule {
	m.source = src
	return m
}

func (m *BAPIWsyncModule) Name() string { return ModuleName }

func (m *BAPIWsyncModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{}
}

func (m *BAPIWsyncModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/wsync/checkpoint": m.handleQueryCheckpoint,
		"/wsync/latest":     m.handleQueryLatest,
	}
}

func (m *BAPIWsyncModule) InitGenesis(_ context.Context, _ []byte) error {
	return nil
}

func (m *BAPIWsyncModule) ExportGenesis(_ context.Context) ([]byte, error) {
	return json.Marshal(struct{}{})
}

func (m *BAPIWsyncModule) BeginBlock(_ context.Context, _ *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	return nil, nil
}

// EndBlock has two responsibilities:
//
//  1. At hourly boundaries (Height % HourlyBlocks == 0), if a
//     CheckpointSource is configured, build the unsigned
//     checkpoint and store it. Attestations arrive later via
//     AddAttestation.
//  2. Prune checkpoints older than UnbondingPeriodBlocks so the
//     rolling window never exceeds the unbonding horizon.
//
// Returns no effects — the module owns its own state store, not
// the bank or validator stores, so all writes happen directly.
func (m *BAPIWsyncModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	if blockCtx == nil {
		return nil, nil, nil
	}
	height := uint64(blockCtx.Height)

	if height > 0 && height%HourlyBlocks == 0 && m.source != nil {
		ck := &Checkpoint{
			Height:           height,
			AppHash:          m.source.LatestAppHash(),
			ValidatorSetHash: m.source.ActiveValidatorSetHash(),
		}
		if err := m.checkpoints.Set(ctx, fmt.Sprintf(keyCheckpointFmt, height), ck); err != nil {
			return nil, nil, fmt.Errorf("persist checkpoint at height %d: %w", height, err)
		}
	}

	// Pruning. The horizon is currentHeight - UnbondingPeriodBlocks;
	// everything strictly older drops out.
	if height > UnbondingPeriodBlocks {
		horizon := height - UnbondingPeriodBlocks
		if err := m.pruneOlderThan(ctx, horizon); err != nil {
			return nil, nil, fmt.Errorf("prune checkpoints: %w", err)
		}
	}
	return nil, nil, nil
}

// pruneOlderThan deletes every checkpoint with Height < horizon.
func (m *BAPIWsyncModule) pruneOlderThan(ctx context.Context, horizon uint64) error {
	var toDelete []uint64
	err := m.checkpoints.IterateRelative(func(_ string, ck *Checkpoint) bool {
		if ck == nil {
			return false
		}
		if ck.Height < horizon {
			toDelete = append(toDelete, ck.Height)
		}
		return false
	})
	if err != nil {
		// Iteration unsupported — no prune, but not fatal.
		return nil
	}
	for _, h := range toDelete {
		if err := m.checkpoints.Delete(ctx, fmt.Sprintf(keyCheckpointFmt, h)); err != nil {
			return fmt.Errorf("delete checkpoint %d: %w", h, err)
		}
	}
	return nil
}

// GetCheckpoint returns the checkpoint stored at `height`, or
// nil + ErrNotFound when no such checkpoint exists.
func (m *BAPIWsyncModule) GetCheckpoint(ctx context.Context, height uint64) (*Checkpoint, error) {
	return m.checkpoints.Get(ctx, fmt.Sprintf(keyCheckpointFmt, height))
}

// LatestCheckpoint returns the highest-height checkpoint
// currently in the store. Returns nil when no checkpoints exist.
func (m *BAPIWsyncModule) LatestCheckpoint(_ context.Context) (*Checkpoint, error) {
	var latest *Checkpoint
	err := m.checkpoints.IterateRelative(func(_ string, ck *Checkpoint) bool {
		if ck == nil {
			return false
		}
		if latest == nil || ck.Height > latest.Height {
			latest = ck
		}
		return false
	})
	return latest, err
}

// AddAttestation appends one validator's signature to the
// checkpoint at `height`. The runtime/gossip layer calls this
// when an attestation arrives. Returns an error if no checkpoint
// exists at the height, or if the signature does not verify
// against the checkpoint's SignedContent.
//
// Idempotent: adding the same validator's attestation twice keeps
// only the first; later duplicates are silently dropped (matches
// cosmos-sdk's prevote-aggregation behaviour).
func (m *BAPIWsyncModule) AddAttestation(ctx context.Context, height uint64, att Attestation) error {
	ck, err := m.GetCheckpoint(ctx, height)
	if err != nil || ck == nil {
		return fmt.Errorf("no checkpoint at height %d", height)
	}
	if len(att.ValidatorPubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("validator pubkey length %d != %d", len(att.ValidatorPubKey), ed25519.PublicKeySize)
	}
	if !ed25519.Verify(ed25519.PublicKey(att.ValidatorPubKey), ck.SignedContent(), att.Signature) {
		return fmt.Errorf("invalid signature for validator %x at height %d", att.ValidatorPubKey, height)
	}

	for _, existing := range ck.Attestations {
		if bytesEqual(existing.ValidatorPubKey, att.ValidatorPubKey) {
			// Duplicate; silently drop.
			return nil
		}
	}
	ck.Attestations = append(ck.Attestations, att)
	return m.checkpoints.Set(ctx, fmt.Sprintf(keyCheckpointFmt, height), ck)
}

// VerifyCheckpoint is the fresh-node entrypoint per PLAN §7
// Phase 5.4. Returns nil when the checkpoint is valid given the
// supplied validator set; a descriptive error otherwise.
//
// Validates:
//   - height freshness: height ≥ currentHeight - UnbondingPeriodBlocks
//     (so a peer can't trick a fresh client into accepting a
//     stale checkpoint signed by a long-unbonded set)
//   - validator-set hash matches: ck.ValidatorSetHash =
//     ComputeValidatorSetHash(validatorSet)
//   - each attestation's signature verifies against the
//     declared pubkey + SignedContent
//   - each attestation's pubkey is in the supplied set
//   - aggregate VotingPower across valid attestations ≥
//     SignatureThresholdBps × totalPower / 10000
//
// The currentHeight argument is the chain tip the verifier knows
// from a trusted peer or recent state. Passing zero disables the
// freshness check (useful for unit tests).
func VerifyCheckpoint(ck *Checkpoint, validatorSet map[string]uint64, currentHeight uint64) error {
	if ck == nil {
		return fmt.Errorf("nil checkpoint")
	}
	if currentHeight > 0 && currentHeight > UnbondingPeriodBlocks && ck.Height < currentHeight-UnbondingPeriodBlocks {
		return fmt.Errorf("checkpoint expired: height %d below trust horizon %d",
			ck.Height, currentHeight-UnbondingPeriodBlocks)
	}
	if got := ComputeValidatorSetHash(validatorSet); got != ck.ValidatorSetHash {
		return fmt.Errorf("validator-set hash mismatch: ck=%x got=%x", ck.ValidatorSetHash, got)
	}

	var totalPower uint64
	for _, p := range validatorSet {
		totalPower += p
	}
	if totalPower == 0 {
		return fmt.Errorf("empty validator set")
	}

	var aggregatedPower uint64
	seen := make(map[string]struct{}, len(ck.Attestations))
	signed := ck.SignedContent()
	for i, att := range ck.Attestations {
		if len(att.ValidatorPubKey) != ed25519.PublicKeySize {
			return fmt.Errorf("attestation %d: pubkey length %d", i, len(att.ValidatorPubKey))
		}
		key := string(att.ValidatorPubKey)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("attestation %d: duplicate signer", i)
		}
		seen[key] = struct{}{}
		stake, inSet := validatorSet[key]
		if !inSet {
			return fmt.Errorf("attestation %d: signer not in active set", i)
		}
		if !ed25519.Verify(ed25519.PublicKey(att.ValidatorPubKey), signed, att.Signature) {
			return fmt.Errorf("attestation %d: signature invalid", i)
		}
		aggregatedPower += stake
	}

	required := totalPower * SignatureThresholdBps / 10000
	if aggregatedPower < required {
		return fmt.Errorf("insufficient stake signed: have %d, need %d (%d bps of %d)",
			aggregatedPower, required, SignatureThresholdBps, totalPower)
	}
	return nil
}

// ComputeValidatorSetHash returns the spec-pinned hash of the
// active validator set: SHA-256 of the pubkeys concatenated in
// ascending-hex order with each pubkey followed by its
// big-endian-encoded VotingPower.
//
// This format is the cross-implementation interop point — every
// language port must produce the same bytes for the same set so
// VerifyCheckpoint can run on any of them.
func ComputeValidatorSetHash(validatorSet map[string]uint64) Hash32 {
	keys := make([]string, 0, len(validatorSet))
	for k := range validatorSet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	powerBytes := make([]byte, 8)
	for _, k := range keys {
		h.Write([]byte(k))
		binary.BigEndian.PutUint64(powerBytes, validatorSet[k])
		h.Write(powerBytes)
	}
	var out Hash32
	copy(out[:], h.Sum(nil))
	return out
}

// handleQueryCheckpoint serves /wsync/checkpoint queries. The
// query payload is the height as a decimal string.
func (m *BAPIWsyncModule) handleQueryCheckpoint(ctx context.Context, data []byte, _ int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("height required")
	}
	var height uint64
	if _, err := fmt.Sscanf(string(data), "%d", &height); err != nil {
		return nil, fmt.Errorf("invalid height: %w", err)
	}
	ck, err := m.GetCheckpoint(ctx, height)
	if err != nil || ck == nil {
		return nil, fmt.Errorf("no checkpoint at height %d", height)
	}
	return json.Marshal(ck)
}

func (m *BAPIWsyncModule) handleQueryLatest(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	ck, err := m.LatestCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	if ck == nil {
		return nil, fmt.Errorf("no checkpoints exist")
	}
	return json.Marshal(ck)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var (
	_ runtime.BAPIModule             = (*BAPIWsyncModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIWsyncModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIWsyncModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIWsyncModule)(nil)
)
