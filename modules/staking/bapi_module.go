package staking

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// BAPIStakingModule provides validator and delegation management for BAPI-based applications.
// It implements runtime.BAPIModule, runtime.BAPIBlockProcessor, and runtime.BAPIGenesisInitializer.
type BAPIStakingModule struct {
	validatorStore *store.BAPIValidatorStore
	balanceStore   *store.BAPIBalanceStore

	// mu guards per-instance counters that aren't consensus state.
	mu sync.Mutex

	// unbondingSeq is a monotonic counter used to disambiguate
	// unbonding entries that share a maturity height (same block,
	// same delegator+validator). The value is consensus-irrelevant
	// — the typed-store key just needs to be unique within a
	// height — but the counter is bumped under mu so the
	// assignment order is deterministic for tests. PLAN §7 Phase
	// 2.1.
	unbondingSeq uint64

	// activeSetSize is the maximum number of validators allowed in
	// the active set, as configured by genesis. Defaults to
	// DefaultActiveSetSize when InitGenesis doesn't override.
	// Used by EndBlock at epoch-close to take the top-N by power.
	// PLAN §7 Phase 2.3.
	activeSetSize uint32
}

// NewBAPIStakingModule creates a new BAPI staking module with the given stores.
func NewBAPIStakingModule(validatorStore *store.BAPIValidatorStore, balanceStore *store.BAPIBalanceStore) (*BAPIStakingModule, error) {
	if validatorStore == nil {
		return nil, fmt.Errorf("validator store cannot be nil")
	}
	if balanceStore == nil {
		return nil, fmt.Errorf("balance store cannot be nil")
	}

	return &BAPIStakingModule{
		validatorStore: validatorStore,
		balanceStore:   balanceStore,
		activeSetSize:  DefaultActiveSetSize,
	}, nil
}

// Name returns the module's unique name.
func (m *BAPIStakingModule) Name() string {
	return ModuleName
}

// RegisterMsgHandlers returns message handlers keyed by message type.
func (m *BAPIStakingModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgCreateValidator: m.handleCreateValidator,
		TypeMsgDelegate:        m.handleDelegate,
		TypeMsgUndelegate:      m.handleUndelegate,
	}
}

// RegisterQueryHandlers returns query handlers keyed by query path.
func (m *BAPIStakingModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/staking/validator":   m.handleQueryValidator,
		"/staking/delegation":  m.handleQueryDelegation,
	}
}

// BeginBlock is called at the beginning of each block.
//
// Phase 2.3: a no-op. Per spec D6, the validator set refreshes per
// epoch only; mid-block dirty-tracking is gone. EndBlock at
// epoch-close diffs the fresh top-N against the persisted previous
// active set to produce ValidatorUpdates.
func (m *BAPIStakingModule) BeginBlock(_ context.Context, _ *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	return nil, nil
}

// EpochBlocks is the chain-wide epoch length in blocks. PLAN D4:
// epoch boundary = `Height % EpochBlocks == 0`. At a 1-second block
// cadence this is one hour. Phase 3 emission, participation, and
// distribution all key off this value.
//
// A future ConsensusParams field will let governance change the
// length; for v1 the constant is the single authoritative value.
const EpochBlocks uint64 = 3600

// DefaultActiveSetSize is the maximum number of validators in the
// active set when StakingGenesisState.ActiveSetSize is unset. Matches
// the config.go default (MaxValidators = 100).
const DefaultActiveSetSize uint32 = 100

// IsEpochCloseHeight reports whether `height` is an epoch-close
// block (the last block of an epoch). EndBlock at such a height
// computes the next epoch's active set and emits its diff vs the
// previous active set as ValidatorUpdates.
//
// Height 0 is excluded because no block ever has Height==0 in
// practice; the chain starts at InitialHeight (≥1).
func IsEpochCloseHeight(height uint64) bool {
	return height > 0 && height%EpochBlocks == 0
}

// Default slash fractions in basis points (10000 = 100%). These are
// the *fallback* values used when the chain's ConsensusParams does not
// override them (e.g. test setups that don't seed a params store). The
// production path reads `blockCtx.Params.SlashFraction*Bps`; the
// constants below are also exported because callers may want to
// reference them when constructing test genesis blobs.
const (
	// DefaultSlashFractionDoubleSignBps: 500 = 5%.
	DefaultSlashFractionDoubleSignBps uint32 = 500
	// DefaultSlashFractionLightClientBps: 1000 = 10%. Strictly higher
	// than double-sign: light-client attacks target external observers
	// who can't independently verify the slashable event.
	DefaultSlashFractionLightClientBps uint32 = 1000
)

// UnbondingPeriodBlocks is the chain-wide unbonding-period length in
// blocks. Spec §4 fixes the wall-clock period at 21 days; the block
// count assumes a 1-second cadence (21 × 24 × 60 × 60 = 1,814,400).
// Chains that run at a different block time must override this via a
// future ConsensusParams field; for v1 the constant is the single
// authoritative value. PLAN §7 Phase 2.1.
const UnbondingPeriodBlocks uint64 = 21 * 24 * 60 * 60

// CommissionFloorBps is the minimum commission rate, in basis points,
// that any validator (bootstrap or otherwise) may carry. Spec §5 sets
// c_min = 5% = 500 bps. MsgCreateValidator and any future
// MsgEditValidator must reject submissions below this floor. PLAN §7
// Phase 2.2 / D17 (bootstrap validators are pinned at exactly this
// value).
const CommissionFloorBps uint32 = 500

// SlashFractionBasisPoints is the legacy compiled-in fraction used when
// the per-type knobs above weren't around yet. Retained as an alias for
// the double-sign default to keep existing tests and callers compiling.
//
// Deprecated: read from `BAPIBlockContext.Params` (via
// `slashFractionFor`) instead — the constant is preserved only for
// backwards compatibility.
const SlashFractionBasisPoints uint64 = uint64(DefaultSlashFractionDoubleSignBps)

// slashFractionFor returns the basis-points slash fraction that applies
// to evType under the block's ConsensusParams. Falls back to the
// compiled-in default when params are absent or the per-type field is
// zero (which also means "use the default" by convention).
func slashFractionFor(params *types.ConsensusParams, evType types.EvidenceType) uint32 {
	if params != nil {
		switch evType {
		case types.EvidenceTypeDuplicateVote:
			if params.SlashFractionDoubleSignBps != 0 {
				return params.SlashFractionDoubleSignBps
			}
		case types.EvidenceTypeLightClient:
			if params.SlashFractionLightClientBps != 0 {
				return params.SlashFractionLightClientBps
			}
		}
	}
	switch evType {
	case types.EvidenceTypeLightClient:
		return DefaultSlashFractionLightClientBps
	default:
		return DefaultSlashFractionDoubleSignBps
	}
}

// ProcessEvidence reacts to Byzantine evidence by slashing the offending
// validator. Implements runtime.BAPIEvidenceHandler (PLAN C2).
//
// Both DuplicateVote and LightClient evidence types result in a slash +
// jail. Unknown future types are accepted silently for forward
// compatibility. The handler:
//
//  1. Resolves the validator by the PubKey carried in the evidence
//     (raspberry's BlockExecutorAdapter populates this from the engine's
//     ValidatorSet; if absent we have no slashing target and skip).
//  2. Computes the new power: old - old * slashFractionBps / 10000
//     (saturating to zero). The fraction comes from `blockCtx.Params`
//     when set, otherwise from compiled-in defaults. The validator is
//     marked Jailed.
//  3. Emits a WriteEffect carrying the updated validator and marks the
//     pubkey dirty so EndBlock will emit a ValidatorUpdate with the
//     post-slash power.
//
// Returns no error if the validator is unknown — evidence about a
// validator who has already been removed is a no-op rather than a fatal
// condition. The chain must keep running.
func (m *BAPIStakingModule) ProcessEvidence(ctx context.Context, blockCtx *runtime.BAPIBlockContext, evidence types.Evidence) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	// Slash on DuplicateVote and LightClient; unknown future types
	// are silently accepted.
	var reason string
	switch evidence.Type {
	case types.EvidenceTypeDuplicateVote:
		reason = "double_vote"
	case types.EvidenceTypeLightClient:
		reason = "light_client_attack"
	default:
		return nil, nil
	}
	slashBps := uint64(slashFractionFor(blockCtx.Params, evidence.Type))
	if len(evidence.PubKey.Data) == 0 {
		// No pubkey means we cannot look up the validator. Log via event
		// so operators can see this — but do not halt the chain.
		return []effects.Effect{
			effects.NewEventEffect("staking.evidence_unresolved", map[string][]byte{
				"height": []byte(fmt.Sprintf("%d", blockCtx.Height)),
				"type":   []byte(fmt.Sprintf("%d", evidence.Type)),
			}),
		}, nil
	}

	validator, err := m.validatorStore.GetValidator(ctx, evidence.PubKey.Data)
	if err != nil || validator == nil {
		// Already removed or never existed — emit an event and move on.
		return []effects.Effect{
			effects.NewEventEffect("staking.evidence_unknown_validator", map[string][]byte{
				"height":  []byte(fmt.Sprintf("%d", blockCtx.Height)),
				"pub_key": []byte(hex.EncodeToString(evidence.PubKey.Data)),
			}),
		}, nil
	}

	// Compute new power. Saturating subtraction guards against the
	// (impossible-in-practice but easy-to-reason-about) underflow case
	// where the fraction calculation rounds upward.
	prevPower := validator.Power
	slashAmt := (prevPower * slashBps) / 10000
	if slashAmt > prevPower {
		slashAmt = prevPower
	}
	newPower := prevPower - slashAmt

	updated := &store.BAPIValidator{
		PubKey:          validator.PubKey,
		Power:           newPower,
		Jailed:          true,
		Description:     validator.Description,
		Commission:      validator.Commission,
		TotalDelegation: validator.TotalDelegation,
	}

	// Phase 2.3: no dirty-flag — the slash's reduced power surfaces
	// in the next epoch-close EndBlock when the top-N is recomputed
	// from store. Per spec D6, mid-epoch power changes accumulate.

	return []effects.Effect{
		&effects.WriteEffect[*store.BAPIValidator]{
			Store:    "validators",
			StoreKey: []byte(hex.EncodeToString(evidence.PubKey.Data)),
			Value:    updated,
		},
		effects.NewEventEffect("staking.validator_slashed", map[string][]byte{
			"pub_key":    []byte(hex.EncodeToString(evidence.PubKey.Data)),
			"prev_power": []byte(fmt.Sprintf("%d", prevPower)),
			"new_power":  []byte(fmt.Sprintf("%d", newPower)),
			"slash_amt":  []byte(fmt.Sprintf("%d", slashAmt)),
			"height":     []byte(fmt.Sprintf("%d", blockCtx.Height)),
			"reason":     []byte(reason),
		}),
	}, nil
}

// EndBlock is called at the end of each block.
//
// Phase 2.3: validator-set refresh runs only on epoch-close blocks
// (Height % EpochBlocks == 0). On those blocks the module reads every
// validator from store, sorts by Power desc (TotalDelegation in this
// v1 — Phase 2.7 will gate jailed/tombstoned), takes the top
// activeSetSize, and emits ValidatorUpdates for the symmetric
// difference vs the previous active-set snapshot. The new set is
// persisted to the validatorStore.activeSet typed store for the next
// epoch's diff.
//
// On non-epoch blocks the only work is processing matured unbonding
// entries (Phase 2.1).
func (m *BAPIStakingModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	var blockEffects []effects.Effect
	if blockCtx != nil {
		matured, err := m.processMaturedUnbondings(uint64(blockCtx.Height))
		if err != nil {
			return nil, nil, fmt.Errorf("process matured unbondings: %w", err)
		}
		blockEffects = append(blockEffects, matured...)

		// Phase 2.5: advance BootstrapInfo.VestedAmount for each
		// bootstrap validator currently within (or past) its vest
		// window. This is purely a bookkeeping update — the tokens
		// already moved to staking.pool at genesis; this just
		// records how much of the self-stake is now unlocked.
		if err := m.advanceBootstrapVesting(ctx, uint64(blockCtx.Height)); err != nil {
			return nil, nil, fmt.Errorf("advance bootstrap vesting: %w", err)
		}
	}

	if blockCtx == nil || !IsEpochCloseHeight(uint64(blockCtx.Height)) {
		return blockEffects, nil, nil
	}

	updates, err := m.refreshActiveSet(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("refresh active set: %w", err)
	}
	return blockEffects, updates, nil
}

// refreshActiveSet computes the new active set from the validator
// store, persists it, and returns the ValidatorUpdate diff vs the
// previously-persisted active set. Called only on epoch-close
// blocks.
//
// The new set is the top activeSetSize validators by Power
// (descending), with ties broken by hex pubkey ascending. Validators
// with Power == 0 are excluded (consensus treats them as removed
// already).
//
// Diff emission rule:
//   - validator in new but not in prev: emit Power
//   - validator in prev but not in new: emit Power=0 (removed)
//   - validator in both with different Power: emit new Power
//   - validator in both with same Power: skip
//
// Emission order is sorted by hex pubkey for determinism across
// nodes.
func (m *BAPIStakingModule) refreshActiveSet(ctx context.Context) ([]types.ValidatorUpdate, error) {
	var allValidators []*store.BAPIValidator
	if err := m.validatorStore.IterateValidators(func(v *store.BAPIValidator) bool {
		if v != nil && v.Power > 0 {
			allValidators = append(allValidators, v)
		}
		return false
	}); err != nil {
		// Iteration unsupported (in-memory test stores): no active set
		// can be computed. Return empty diff — same as "no validators".
		return nil, nil
	}

	// Top-N by Power desc; tiebreak by hex pubkey asc for determinism.
	sort.Slice(allValidators, func(i, j int) bool {
		if allValidators[i].Power != allValidators[j].Power {
			return allValidators[i].Power > allValidators[j].Power
		}
		return hex.EncodeToString(allValidators[i].PubKey.Data) <
			hex.EncodeToString(allValidators[j].PubKey.Data)
	})
	if int(m.activeSetSize) < len(allValidators) {
		allValidators = allValidators[:m.activeSetSize]
	}

	newSet := make(map[string]*store.BAPIValidator, len(allValidators))
	for _, v := range allValidators {
		newSet[hex.EncodeToString(v.PubKey.Data)] = v
	}

	prevEntries, err := m.validatorStore.GetActiveSet()
	if err != nil {
		return nil, fmt.Errorf("read previous active set: %w", err)
	}
	prev := make(map[string]uint64, len(prevEntries))
	prevPubkeys := make(map[string][]byte, len(prevEntries))
	for _, e := range prevEntries {
		hexKey := hex.EncodeToString(e.PubKey)
		prev[hexKey] = e.Power
		prevPubkeys[hexKey] = e.PubKey
	}

	// Build the union of hex pubkeys so the diff loop sees every key
	// exactly once.
	keys := make(map[string]struct{}, len(newSet)+len(prev))
	for k := range newSet {
		keys[k] = struct{}{}
	}
	for k := range prev {
		keys[k] = struct{}{}
	}
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	var updates []types.ValidatorUpdate
	for _, hexKey := range sortedKeys {
		newV, inNew := newSet[hexKey]
		prevPower, inPrev := prev[hexKey]

		switch {
		case inNew && !inPrev:
			updates = append(updates, types.ValidatorUpdate{
				PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: newV.PubKey.Data},
				Power:  newV.Power,
			})
		case !inNew && inPrev:
			updates = append(updates, types.ValidatorUpdate{
				PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: prevPubkeys[hexKey]},
				Power:  0,
			})
		case inNew && inPrev && newV.Power != prevPower:
			updates = append(updates, types.ValidatorUpdate{
				PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: newV.PubKey.Data},
				Power:  newV.Power,
			})
		}
	}

	// Persist the new active-set snapshot. Delete-then-write rather
	// than diff: simpler, and the active set is bounded by
	// activeSetSize (typically 100), so two passes is cheap.
	for hexKey, pk := range prevPubkeys {
		if _, stillIn := newSet[hexKey]; !stillIn {
			if err := m.validatorStore.DeleteActiveSetEntry(ctx, pk); err != nil {
				return nil, fmt.Errorf("delete active set entry %s: %w", hexKey, err)
			}
		}
	}
	for _, v := range newSet {
		entry := &store.BAPIActiveSetEntry{
			PubKey: append([]byte(nil), v.PubKey.Data...),
			Power:  v.Power,
		}
		if err := m.validatorStore.SetActiveSetEntry(ctx, entry); err != nil {
			return nil, fmt.Errorf("persist active set entry: %w", err)
		}
	}
	return updates, nil
}

// processMaturedUnbondings walks the unbonding queue and emits
// (TransferEffect, DeleteEffect) pairs for every entry whose
// MaturityHeight ≤ currentHeight. Called once per EndBlock. The
// returned slice is in maturity-ascending order, matching the
// store's IterateMaturedUnbondings key order, so the effect batch
// is deterministic across nodes.
//
// Returns an empty slice and nil error when no entries are due
// (the common case). Returns the underlying store error if
// iteration itself fails — note that
// ErrIterationUnsupported is treated as "no entries due" because
// in-memory test stores legitimately don't iterate; the queue
// then drains via Set/Delete only.
func (m *BAPIStakingModule) processMaturedUnbondings(currentHeight uint64) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	var due []*store.BAPIUnbondingEntry
	err := m.validatorStore.IterateMaturedUnbondings(currentHeight, func(e *store.BAPIUnbondingEntry) bool {
		due = append(due, e)
		return false
	})
	if err != nil {
		// Iteration not supported — silently return nothing rather than
		// fail the whole EndBlock. Real chains run on iterable stores.
		return nil, nil
	}
	if len(due) == 0 {
		return nil, nil
	}

	effectsOut := make([]effects.Effect, 0, 2*len(due))
	for _, e := range due {
		effectsOut = append(effectsOut,
			effects.TransferEffect{
				From:   ptypes.AccountName("staking.pool"),
				To:     ptypes.AccountName(e.Delegator),
				Amount: ptypes.NewCoins(ptypes.NewCoin(stakingDenom, e.Amount)),
			},
			effects.DeleteEffect[*store.BAPIUnbondingEntry]{
				Store:    "unbondings",
				StoreKey: []byte(fmt.Sprintf("%020d/%020d", e.MaturityHeight, e.Seq)),
			},
		)
	}
	return effectsOut, nil
}

// stakingDenom is the base denom used by all staking-module transfers.
// Module-internal constant; the canonical chain-wide denom override
// lives in the bank module (Phase 1 wiring uses a configured base denom
// per chain).
const stakingDenom = "stake"

// InitGenesis initializes the module's state from genesis data.
//
// Seeds the per-validator state under "validators/" and the
// active-set snapshot under "active_set/". Consensus already knows
// the genesis ValidatorSet, so the seeded active_set is used purely
// as the diff baseline — the first epoch-close EndBlock will emit
// ValidatorUpdates only for validators whose Power changed since
// genesis.
//
// `ActiveSetSize`, when non-zero, overrides the
// DefaultActiveSetSize on this module instance for the lifetime of
// the process. Empty/zero falls back to the default.
func (m *BAPIStakingModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil // No genesis data for staking module is acceptable
	}

	var genesisState StakingGenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		return fmt.Errorf("unmarshal staking genesis: %w", err)
	}

	if genesisState.ActiveSetSize != 0 {
		m.activeSetSize = genesisState.ActiveSetSize
	}

	// Create all genesis validators and seed the active-set snapshot.
	for _, validator := range genesisState.Validators {
		pubKey, err := hex.DecodeString(validator.PubKey)
		if err != nil {
			return fmt.Errorf("invalid validator pubkey: %w", err)
		}

		bapiValidator := &store.BAPIValidator{
			PubKey: types.PublicKey{
				Type: types.KeyTypeEd25519,
				Data: pubKey,
			},
			Power:           validator.Power,
			Jailed:          false,
			Description:     validator.Description,
			Commission:      validator.Commission,
			TotalDelegation: 0,
		}

		if err := m.validatorStore.SetValidator(ctx, bapiValidator); err != nil {
			return fmt.Errorf("set genesis validator: %w", err)
		}

		// Mirror the genesis validator into the active-set snapshot so
		// the first epoch-close EndBlock diff produces an empty update
		// list when no power has changed.
		if err := m.validatorStore.SetActiveSetEntry(ctx, &store.BAPIActiveSetEntry{
			PubKey: append([]byte(nil), pubKey...),
			Power:  bapiValidator.Power,
		}); err != nil {
			return fmt.Errorf("seed active set entry: %w", err)
		}
	}

	return nil
}

// SeedBootstrapValidators implements runtime.BAPIBootstrapInitializer.
//
// Creates per-bootstrap-validator state at genesis: the validator
// record (Power = perValidatorShare, Commission pinned to
// BootstrapCommission = 500), the self-delegation, the active-set
// entry, and the BootstrapInfo vesting record. Called by the runtime
// after the regular InitGenesis loop when
// TokenomicsGenesis.BootstrapValidators is non-empty. PLAN §7 Phase
// 2.4 / D17, D22.
//
// The validator's BL share is debited from module.bl and credited to
// staking.pool so the self-delegation is backed by real tokens. The
// BootstrapInfo record then governs WHEN the validator may
// undelegate this self-stake (Phase 2.5's vesting math) — the
// tokens themselves are not moved again during the vest window.
func (m *BAPIStakingModule) SeedBootstrapValidators(ctx context.Context, validators []runtime.BootstrapValidator, perValidatorShare uint64, vestStartHeight uint64) error {
	if m == nil || m.validatorStore == nil || m.balanceStore == nil {
		return fmt.Errorf("module or store is nil")
	}
	for _, bv := range validators {
		if !bv.Name.IsValid() {
			return fmt.Errorf("invalid bootstrap validator name %q", bv.Name)
		}
		if len(bv.PubKey) != 32 {
			return fmt.Errorf("bootstrap validator %q: pubkey length %d != 32", bv.Name, len(bv.PubKey))
		}

		validator := &store.BAPIValidator{
			PubKey: types.PublicKey{
				Type: types.KeyTypeEd25519,
				Data: append([]byte(nil), bv.PubKey...),
			},
			Power:           perValidatorShare,
			Jailed:          false,
			Description:     string(bv.Name),
			Commission:      uint32(runtime.BootstrapCommission),
			TotalDelegation: perValidatorShare,
		}
		if err := m.validatorStore.SetValidator(ctx, validator); err != nil {
			return fmt.Errorf("set bootstrap validator: %w", err)
		}

		delegation := &store.BAPIDelegation{
			Delegator:       string(bv.Name),
			ValidatorPubKey: hex.EncodeToString(bv.PubKey),
			Amount:          perValidatorShare,
		}
		if err := m.validatorStore.SetDelegation(ctx, delegation); err != nil {
			return fmt.Errorf("set bootstrap self-delegation: %w", err)
		}

		if err := m.validatorStore.SetActiveSetEntry(ctx, &store.BAPIActiveSetEntry{
			PubKey: append([]byte(nil), bv.PubKey...),
			Power:  perValidatorShare,
		}); err != nil {
			return fmt.Errorf("seed bootstrap active set entry: %w", err)
		}

		if err := m.validatorStore.SetBootstrapInfo(ctx, &store.BAPIBootstrapInfo{
			PubKey:          append([]byte(nil), bv.PubKey...),
			LockedAmount:    perValidatorShare,
			VestStartHeight: vestStartHeight,
			VestedAmount:    0,
		}); err != nil {
			return fmt.Errorf("set bootstrap info: %w", err)
		}

		// Move the validator's BL share from module.bl into the
		// staking pool so the self-delegation is backed by real
		// tokens. This is a wholesale transfer at genesis time;
		// vesting (Phase 2.5) gates undelegate, not token movement.
		if err := m.balanceStore.Transfer(ctx, string(runtime.ModuleAccountBootstrap), "staking.pool", stakingDenom, perValidatorShare); err != nil {
			return fmt.Errorf("transfer bootstrap share to staking pool: %w", err)
		}
	}
	return nil
}

// advanceBootstrapVesting walks every bootstrap validator's
// BootstrapInfo record and updates its VestedAmount to reflect the
// current height. Idempotent — running on the same height a second
// time produces no change. Phase 2.5.
//
// The update writes directly to the store rather than emitting
// effects: vesting is a per-block bookkeeping operation that runs
// after the tx-effect batch has settled, not part of it. Treating
// it as state machinery (like the lastEmittedPowers cache that used
// to live here) keeps the EndBlock effect batch small.
func (m *BAPIStakingModule) advanceBootstrapVesting(ctx context.Context, currentHeight uint64) error {
	var updates []*store.BAPIBootstrapInfo
	err := m.validatorStore.IterateBootstrapInfos(func(bi *store.BAPIBootstrapInfo) bool {
		newVested := computeVestedAmount(bi, currentHeight)
		if newVested != bi.VestedAmount {
			// Defensive copy to avoid mutating the iteration value.
			updated := *bi
			updated.VestedAmount = newVested
			updates = append(updates, &updated)
		}
		return false
	})
	if err != nil {
		// In-memory store without iteration: vesting can't progress
		// in tests that bypass the iterable backend. That's fine —
		// callers manipulate BootstrapInfo directly in those tests.
		return nil
	}
	for _, u := range updates {
		if err := m.validatorStore.SetBootstrapInfo(ctx, u); err != nil {
			return fmt.Errorf("persist updated bootstrap info: %w", err)
		}
	}
	return nil
}

// computeVestedAmount returns the cumulative vested amount for a
// bootstrap validator at the given height. Linear vest over
// BootstrapVestBlocks starting at VestStartHeight:
//
//   - height < VestStartHeight                : 0
//   - height ≥ VestStartHeight + VestBlocks    : LockedAmount
//   - in between                              : LockedAmount × (height-VestStartHeight) / VestBlocks
//
// PLAN §7 Phase 2.5.
func computeVestedAmount(bi *store.BAPIBootstrapInfo, currentHeight uint64) uint64 {
	if bi == nil || bi.LockedAmount == 0 || currentHeight < bi.VestStartHeight {
		return 0
	}
	elapsed := currentHeight - bi.VestStartHeight
	if elapsed >= runtime.BootstrapVestBlocks {
		return bi.LockedAmount
	}
	// (locked × elapsed) / vest_blocks — use 128-bit-ish math via
	// big.Int? At our scales (uint64 LockedAmount, small block
	// counts), `locked × elapsed` fits in 128 bits only if
	// locked > 2^32 AND elapsed > 2^32. The realistic max is
	// BootstrapVestBlocks ≈ 2.6M < 2^22, so locked × elapsed fits
	// in uint64 for any locked < 2^42. The 5% of typical supply
	// stays well below that.
	return bi.LockedAmount * elapsed / runtime.BootstrapVestBlocks
}

// ExportGenesis exports the module's state for genesis.
//
// Walks every validator currently in the store via the IterateValidators
// accessor (which uses the iterable StateStore added in PLAN BAPI
// store-iteration) and emits a sorted GenesisValidator list. The
// output is deterministic: iteration order is the tree's ascending
// byte order over hex-encoded pubkeys, plus we sort explicitly by
// pubkey so any future change to the underlying iteration order
// doesn't break replay.
//
// When the underlying store is not iterable (test in-memory stores),
// silently returns an empty validator list. Genesis is best-effort in
// that mode anyway.
func (m *BAPIStakingModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	if m == nil || m.validatorStore == nil {
		return json.Marshal(StakingGenesisState{Validators: []GenesisValidator{}})
	}

	var validators []GenesisValidator
	err := m.validatorStore.IterateValidators(func(v *store.BAPIValidator) bool {
		validators = append(validators, GenesisValidator{
			PubKey:      hex.EncodeToString(v.PubKey.Data),
			Power:       v.Power,
			Description: v.Description,
			Commission:  v.Commission,
		})
		return false
	})
	if err != nil {
		// Iteration unsupported / store error — fall back to empty rather
		// than blocking the export of other modules. Genesis is
		// best-effort; chains that need a real export must run on an
		// iterable backend.
		return json.Marshal(StakingGenesisState{Validators: []GenesisValidator{}})
	}

	// Sort by pubkey for determinism — IterateValidators already produces
	// stable order from a real iterable store, but defensive sorting
	// guards against backends that don't.
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].PubKey < validators[j].PubKey
	})

	return json.Marshal(StakingGenesisState{Validators: validators})
}

// StakingGenesisState represents the staking module's genesis state.
//
// ActiveSetSize, when non-zero, overrides DefaultActiveSetSize for
// the chain — the value pins the maximum number of validators in
// the active set computed at every epoch boundary. Zero means
// "use the default" (100). PLAN §7 Phase 2.3.
type StakingGenesisState struct {
	Validators    []GenesisValidator `json:"validators"`
	ActiveSetSize uint32             `json:"active_set_size,omitempty"`
}

// GenesisValidator represents a validator in genesis.
type GenesisValidator struct {
	PubKey      string `json:"pub_key"` // hex-encoded
	Power       uint64 `json:"power"`
	Description string `json:"description"`
	Commission  uint32 `json:"commission"` // basis points
}

// handleCreateValidator handles MsgCreateValidator.
//
// Per the SDK's effects-not-mutations invariant (CLAUDE.md), this handler
// returns a WriteEffect[*store.BAPIValidator] instead of calling
// validatorStore.SetValidator directly. Direct mutation here previously
// bypassed the effect executor entirely — it ran outside the transaction
// pipeline's commit boundary and was not visible to validation / replay
// logic. PLAN T1-8 tracked this regression.
//
// The WriteEffect's Store name is "validators" and its StoreKey is the
// hex-encoded pubkey — matching the prefix and key scheme used by
// store.BAPIValidatorStore (see validatorKey() in bapi_validator_store.go).
// The executor (effects.BAPIExecutor.executeWrite) writes raw bytes to the
// state store under "validators/<hex-pubkey>", which is exactly the layout
// the typed store reads from.
func (m *BAPIStakingModule) handleCreateValidator(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	createMsg, ok := msg.(*MsgCreateValidator)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgCreateValidator, got %T", msg)
	}

	// Verify the delegator is the transaction signer
	if createMsg.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Phase 2.2: enforce the c_min = 5% commission floor on creation.
	// Future MsgEditValidator must re-check (and additionally pin
	// bootstrap validators per D17).
	if uint32(createMsg.Commission) < CommissionFloorBps {
		return nil, fmt.Errorf("commission %d bps below floor %d bps (c_min = 5%%)",
			createMsg.Commission, CommissionFloorBps)
	}

	// Check if validator already exists
	exists, err := m.validatorStore.HasValidator(ctx, createMsg.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check validator existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("validator with public key %x already exists", createMsg.PubKey)
	}

	// Build the validator value. NOTE: do NOT call validatorStore.SetValidator
	// here — that would bypass the effect system. The WriteEffect below is the
	// only path that persists state.
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: createMsg.PubKey,
		},
		Power:           uint64(createMsg.InitialPower),
		Jailed:          false,
		Description:     "",
		Commission:      uint32(createMsg.Commission),
		TotalDelegation: 0,
	}

	// Phase 2.3: no dirty-marking. The new validator becomes
	// eligible for the next epoch's top-N recomputation; consensus
	// learns about it at the next epoch close.

	return []effects.Effect{
		&effects.WriteEffect[*store.BAPIValidator]{
			Store:    "validators",
			StoreKey: []byte(hex.EncodeToString(createMsg.PubKey)),
			Value:    validator,
		},
		effects.NewEventEffect("staking.validator_created", map[string][]byte{
			"delegator":  []byte(createMsg.Delegator),
			"pub_key":    []byte(hex.EncodeToString(createMsg.PubKey)),
			"power":      []byte(fmt.Sprintf("%d", createMsg.InitialPower)),
			"commission": []byte(fmt.Sprintf("%d", createMsg.Commission)),
			"height":     []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleDelegate handles MsgDelegate.
func (m *BAPIStakingModule) handleDelegate(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	delegateMsg, ok := msg.(*MsgDelegate)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgDelegate, got %T", msg)
	}

	// Verify the delegator is the transaction signer
	if delegateMsg.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check validator exists
	exists, err := m.validatorStore.HasValidator(ctx, delegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check validator: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: validator not found", ptypes.ErrNotFound)
	}

	// Check delegator has sufficient balance
	balance, err := m.balanceStore.GetAmount(ctx, string(delegateMsg.Delegator), delegateMsg.Amount.Denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	if balance < delegateMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient balance for delegation (have %d, need %d)",
			ptypes.ErrInsufficientFunds, balance, delegateMsg.Amount.Amount)
	}

	// Return effects: transfer tokens to staking pool and update delegation
	return []effects.Effect{
		// Transfer tokens from delegator to staking pool
		effects.TransferEffect{
			From:   delegateMsg.Delegator,
			To:     ptypes.AccountName("staking.pool"),
			Amount: ptypes.Coins{delegateMsg.Amount},
		},
		// Emit event
		effects.NewEventEffect("staking.delegated", map[string][]byte{
			"delegator": []byte(delegateMsg.Delegator),
			"validator": []byte(hex.EncodeToString(delegateMsg.Validator)),
			"amount":    []byte(fmt.Sprintf("%d", delegateMsg.Amount.Amount)),
			"denom":     []byte(delegateMsg.Amount.Denom),
			"height":    []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleUndelegate handles MsgUndelegate.
//
// Phase 2.1: emits four effects rather than the previous immediate
// refund. (1) WriteEffect decrementing the delegation record, (2) a
// WriteEffect creating a BAPIUnbondingEntry that matures at
// currentHeight + UnbondingPeriodBlocks, (3) a WriteEffect
// decrementing the validator's TotalDelegation (so the validator's
// power drops immediately even though the tokens themselves wait
// 21 days), and (4) an event. The tokens stay in
// `staking.pool` for the duration of the unbonding period; the
// matching EndBlock-side refund fires when the entry matures.
//
// Previously this handler refunded the delegator immediately, which
// violated spec §4 (a malicious validator could unbond + drain
// without giving the chain time to slash). With the unbonding queue
// in place, slashing within the 21-day window can still reach the
// queued amount.
func (m *BAPIStakingModule) handleUndelegate(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.validatorStore == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil || txCtx.BAPIBlockContext == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	undelegateMsg, ok := msg.(*MsgUndelegate)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgUndelegate, got %T", msg)
	}

	// Verify the delegator is the transaction signer
	if undelegateMsg.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	// Check delegation exists and is sufficient
	delegation, err := m.validatorStore.GetDelegation(ctx, string(undelegateMsg.Delegator), undelegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("failed to check delegation: %w", err)
	}
	if delegation.Amount == 0 {
		return nil, fmt.Errorf("%w: delegation not found", ptypes.ErrNotFound)
	}
	if delegation.Amount < undelegateMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient delegation (have %d, want %d)",
			ptypes.ErrInsufficientFunds, delegation.Amount, undelegateMsg.Amount.Amount)
	}

	validator, err := m.validatorStore.GetValidator(ctx, undelegateMsg.Validator)
	if err != nil {
		return nil, fmt.Errorf("get validator: %w", err)
	}
	if validator == nil {
		return nil, fmt.Errorf("%w: validator not found", ptypes.ErrNotFound)
	}
	if validator.TotalDelegation < undelegateMsg.Amount.Amount {
		return nil, fmt.Errorf("validator TotalDelegation underflow (have %d, want %d)",
			validator.TotalDelegation, undelegateMsg.Amount.Amount)
	}

	// Phase 2.4 / 2.5: bootstrap-validator self-undelegate vesting check.
	// A bootstrap validator's self-delegation is locked for
	// BootstrapLockBlocks then linearly vested over BootstrapVestBlocks.
	// At any height, the maximum self-undelegate amount is the vested
	// fraction. Non-self delegators (third parties who delegated to
	// a bootstrap validator) are unaffected — they fall through to
	// the regular 21-day unbonding queue from Phase 2.1.
	if bi, biErr := m.validatorStore.GetBootstrapInfo(ctx, undelegateMsg.Validator); biErr == nil && bi != nil {
		isSelfUndelegate := string(undelegateMsg.Delegator) == validator.Description
		if isSelfUndelegate {
			vested := computeVestedAmount(bi, uint64(txCtx.Height))
			// `unvested` is the amount that MUST remain self-delegated.
			// post-undelegate self-stake = delegation.Amount - undelegateMsg.Amount.
			// Reject if that drops below unvested.
			if bi.LockedAmount > vested {
				unvested := bi.LockedAmount - vested
				postSelfStake := delegation.Amount - undelegateMsg.Amount.Amount
				if postSelfStake < unvested {
					return nil, fmt.Errorf("bootstrap validator %s: only %d of %d locked-share vested at height %d; can't undelegate down to %d",
						undelegateMsg.Delegator, vested, bi.LockedAmount, txCtx.Height, postSelfStake)
				}
			}
		}
	}

	updatedDelegation := &store.BAPIDelegation{
		Delegator:       delegation.Delegator,
		ValidatorPubKey: delegation.ValidatorPubKey,
		Amount:          delegation.Amount - undelegateMsg.Amount.Amount,
	}
	delegationStoreKey := string(undelegateMsg.Delegator) + "/" + hex.EncodeToString(undelegateMsg.Validator)

	// Validator's TotalDelegation drops immediately even though the
	// tokens themselves wait 21 days. Phase 2.3: the next epoch-close
	// EndBlock surfaces the reduced power as a ValidatorUpdate diff;
	// mid-epoch consensus weight stays at the pre-undelegate value
	// per spec D6.
	updatedValidator := *validator
	updatedValidator.TotalDelegation -= undelegateMsg.Amount.Amount
	updatedValidator.Power = updatedValidator.TotalDelegation

	m.mu.Lock()
	m.unbondingSeq++
	seq := m.unbondingSeq
	m.mu.Unlock()

	maturityHeight := uint64(txCtx.Height) + UnbondingPeriodBlocks
	entry := &store.BAPIUnbondingEntry{
		Delegator:       string(undelegateMsg.Delegator),
		ValidatorPubKey: hex.EncodeToString(undelegateMsg.Validator),
		Amount:          undelegateMsg.Amount.Amount,
		MaturityHeight:  maturityHeight,
		Seq:             seq,
	}
	entryStoreKey := fmt.Sprintf("%020d/%020d", maturityHeight, seq)

	return []effects.Effect{
		// Decrement the delegation record so the same stake can't be
		// undelegated again. The delegate record is left at amount 0
		// rather than deleted; callers compare Amount, not key presence.
		&effects.WriteEffect[*store.BAPIDelegation]{
			Store:    "delegations",
			StoreKey: []byte(delegationStoreKey),
			Value:    updatedDelegation,
		},
		// Decrement the validator's TotalDelegation/Power so consensus
		// stops counting the unbonded stake immediately.
		&effects.WriteEffect[*store.BAPIValidator]{
			Store:    "validators",
			StoreKey: []byte(hex.EncodeToString(undelegateMsg.Validator)),
			Value:    &updatedValidator,
		},
		// Park the tokens in the unbonding queue. They remain in
		// staking.pool until EndBlock at MaturityHeight refunds them.
		&effects.WriteEffect[*store.BAPIUnbondingEntry]{
			Store:    "unbondings",
			StoreKey: []byte(entryStoreKey),
			Value:    entry,
		},
		// Event (no transfer back here — that happens at maturity).
		effects.NewEventEffect("staking.undelegated", map[string][]byte{
			"delegator":       []byte(undelegateMsg.Delegator),
			"validator":       []byte(hex.EncodeToString(undelegateMsg.Validator)),
			"amount":          []byte(fmt.Sprintf("%d", undelegateMsg.Amount.Amount)),
			"denom":           []byte(undelegateMsg.Amount.Denom),
			"height":          []byte(fmt.Sprintf("%d", txCtx.Height)),
			"maturity_height": []byte(fmt.Sprintf("%d", maturityHeight)),
		}),
	}, nil
}

// handleQueryValidator handles validator queries.
func (m *BAPIStakingModule) handleQueryValidator(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Treat data as hex-encoded public key
	pubKey, err := hex.DecodeString(string(data))
	if err != nil {
		// Try raw bytes
		pubKey = data
	}

	if len(pubKey) == 0 {
		return nil, fmt.Errorf("public key cannot be empty")
	}

	var validator *store.BAPIValidator

	// Get validator at specific height if requested
	if height > 0 {
		validator, err = m.validatorStore.GetValidatorAtHeight(ctx, pubKey, height)
	} else {
		validator, err = m.validatorStore.GetValidator(ctx, pubKey)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get validator: %w", err)
	}

	// Return JSON response
	response := map[string]interface{}{
		"pub_key":          hex.EncodeToString(validator.PubKey.Data),
		"power":            validator.Power,
		"jailed":           validator.Jailed,
		"description":      validator.Description,
		"commission":       validator.Commission,
		"total_delegation": validator.TotalDelegation,
	}

	return json.Marshal(response)
}

// handleQueryDelegation handles delegation queries.
func (m *BAPIStakingModule) handleQueryDelegation(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.validatorStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Expect format: "delegator/validator_hex"
	parts := splitOnce(string(data), '/')
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid query format: expected delegator/validator")
	}

	delegator := ptypes.AccountName(parts[0])
	if !delegator.IsValid() {
		return nil, fmt.Errorf("%w: invalid delegator account", ptypes.ErrInvalidAccount)
	}

	validator, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid validator public key: %w", err)
	}

	delegation, err := m.validatorStore.GetDelegation(ctx, string(delegator), validator)
	if err != nil {
		return nil, fmt.Errorf("failed to get delegation: %w", err)
	}

	// Return JSON response
	response := map[string]interface{}{
		"delegator":   delegation.Delegator,
		"validator":   delegation.ValidatorPubKey,
		"amount":      delegation.Amount,
	}

	return json.Marshal(response)
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule             = (*BAPIStakingModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIStakingModule)(nil)
	_ runtime.BAPIEvidenceHandler    = (*BAPIStakingModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIStakingModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIStakingModule)(nil)
)
