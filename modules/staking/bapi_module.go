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

	// mu guards the in-block change tracking and the previous-block power
	// snapshot. These are not consensus state — they're a private record of
	// which validator pubkeys were touched in the current block so EndBlock
	// can emit only the resulting power deltas (PLAN B2-2). Real state lives
	// in validatorStore.
	mu sync.Mutex

	// dirtyValidators holds the hex-encoded pubkey of every validator whose
	// power may have changed in the current block. Populated by tx handlers,
	// drained by EndBlock. The keys are hex strings so the set has stable
	// iteration order when sorted.
	dirtyValidators map[string]struct{}

	// pubkeyByHex memoizes the raw pubkey bytes per hex key so EndBlock can
	// rebuild a types.PublicKey without round-tripping through the store.
	pubkeyByHex map[string][]byte

	// lastEmittedPowers is the power EndBlock last reported to consensus for
	// each validator (keyed by hex pubkey). EndBlock only emits a
	// ValidatorUpdate when the current power differs from this snapshot,
	// avoiding spurious "Power unchanged" updates on every block.
	lastEmittedPowers map[string]uint64

	// unbondingSeq is a monotonic counter used to disambiguate
	// unbonding entries that share a maturity height (same block,
	// same delegator+validator). The value is consensus-irrelevant
	// — the typed-store key just needs to be unique within a
	// height — but the counter is bumped under mu so the
	// assignment order is deterministic for tests. PLAN §7 Phase
	// 2.1.
	unbondingSeq uint64
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
		validatorStore:    validatorStore,
		balanceStore:      balanceStore,
		dirtyValidators:   make(map[string]struct{}),
		pubkeyByHex:       make(map[string][]byte),
		lastEmittedPowers: make(map[string]uint64),
	}, nil
}

// markValidatorDirty records that a validator was touched in the current
// block, so EndBlock will reconsider its power. Called by tx handlers
// (handleCreateValidator) and InitGenesis. The dirty set is private
// bookkeeping for the block-end aggregation step — it is NOT persisted state
// and does not violate the effects-not-mutations invariant any more than the
// runtime collecting events does.
func (m *BAPIStakingModule) markValidatorDirty(pubKey []byte) {
	if len(pubKey) == 0 {
		return
	}
	key := hex.EncodeToString(pubKey)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirtyValidators[key] = struct{}{}
	if _, ok := m.pubkeyByHex[key]; !ok {
		buf := make([]byte, len(pubKey))
		copy(buf, pubKey)
		m.pubkeyByHex[key] = buf
	}
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
// Resets the in-block dirty-validator tracking. Any validator whose state is
// modified between this call and the matching EndBlock will be considered for
// inclusion in the EndBlock's ValidatorUpdates.
func (m *BAPIStakingModule) BeginBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	m.mu.Lock()
	m.dirtyValidators = make(map[string]struct{})
	m.mu.Unlock()
	return nil, nil
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

	// Mark the validator dirty so EndBlock emits a ValidatorUpdate
	// reflecting the post-slash power. Without this, EndBlock would not
	// know the validator changed and consensus would never learn about
	// the slash.
	m.markValidatorDirty(evidence.PubKey.Data)

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
// Walks the set of validators dirtied during the block, reads each one's
// current power from validatorStore (after all tx effects have been applied
// by the runtime), and emits a ValidatorUpdate for every validator whose
// power differs from the value EndBlock last reported. A validator that is
// no longer in the store is reported with Power=0, which BAPI defines as
// removal. The updates are sorted by hex pubkey so the result is
// deterministic across nodes (PLAN B2-2).
func (m *BAPIStakingModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	// Phase 2.1: dequeue matured unbonding entries first. The refund
	// transfers and delete effects are independent of the validator-set
	// aggregation below, so they ride alongside in the same effect batch.
	var blockEffects []effects.Effect
	if blockCtx != nil {
		matured, err := m.processMaturedUnbondings(uint64(blockCtx.Height))
		if err != nil {
			return nil, nil, fmt.Errorf("process matured unbondings: %w", err)
		}
		blockEffects = append(blockEffects, matured...)
	}

	m.mu.Lock()
	// Snapshot the dirty set so we can release the lock before doing store
	// I/O. We hold the lock again briefly later to update lastEmittedPowers.
	dirtyKeys := make([]string, 0, len(m.dirtyValidators))
	for k := range m.dirtyValidators {
		dirtyKeys = append(dirtyKeys, k)
	}
	// Capture the pubkey bytes too.
	pubkeysByHex := make(map[string][]byte, len(dirtyKeys))
	for _, k := range dirtyKeys {
		if pk, ok := m.pubkeyByHex[k]; ok {
			pubkeysByHex[k] = pk
		}
	}
	m.mu.Unlock()

	if len(dirtyKeys) == 0 {
		return blockEffects, nil, nil
	}

	// Deterministic emission order: sort hex pubkeys lexicographically.
	sort.Strings(dirtyKeys)

	updates := make([]types.ValidatorUpdate, 0, len(dirtyKeys))
	newPowers := make(map[string]uint64, len(dirtyKeys))

	for _, hexKey := range dirtyKeys {
		pkBytes := pubkeysByHex[hexKey]
		if len(pkBytes) == 0 {
			// We can still try to decode from hex if pubkeyByHex didn't
			// have the entry (defensive — shouldn't happen).
			decoded, err := hex.DecodeString(hexKey)
			if err != nil {
				continue
			}
			pkBytes = decoded
		}

		var currentPower uint64
		validator, err := m.validatorStore.GetValidator(ctx, pkBytes)
		switch {
		case err == nil && validator != nil:
			currentPower = validator.Power
		default:
			// Validator no longer present (or read error treated as
			// removal). Power=0 signals removal to BAPI consumers.
			currentPower = 0
		}

		newPowers[hexKey] = currentPower

		m.mu.Lock()
		prev, hadPrev := m.lastEmittedPowers[hexKey]
		m.mu.Unlock()
		// Only emit when the power actually changed since the previous
		// EndBlock. The first time we see a validator (no `prev` recorded)
		// we always emit so consensus learns the initial power.
		if hadPrev && prev == currentPower {
			continue
		}

		updates = append(updates, types.ValidatorUpdate{
			PubKey: types.PublicKey{
				Type: types.KeyTypeEd25519,
				Data: pkBytes,
			},
			Power: currentPower,
		})
	}

	// Persist the new powers as the baseline for the next block's diffs.
	m.mu.Lock()
	for k, v := range newPowers {
		m.lastEmittedPowers[k] = v
	}
	m.mu.Unlock()

	return blockEffects, updates, nil
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
func (m *BAPIStakingModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil // No genesis data for staking module is acceptable
	}

	var genesisState StakingGenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		return fmt.Errorf("unmarshal staking genesis: %w", err)
	}

	// Create all genesis validators
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

		// Pre-seed lastEmittedPowers with the genesis power so the first
		// EndBlock doesn't re-emit every genesis validator as a "change".
		// Consensus already knows the genesis ValidatorSet — only future
		// power changes need to be reported. Tests / hosts that want the
		// initial set re-emitted can call markValidatorDirty manually.
		m.mu.Lock()
		hexKey := hex.EncodeToString(pubKey)
		m.lastEmittedPowers[hexKey] = bapiValidator.Power
		pk := make([]byte, len(pubKey))
		copy(pk, pubKey)
		m.pubkeyByHex[hexKey] = pk
		m.mu.Unlock()
	}

	return nil
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
type StakingGenesisState struct {
	Validators []GenesisValidator `json:"validators"`
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

	// Mark this validator as dirty so EndBlock will emit a ValidatorUpdate
	// reflecting the new power. Does not write any state.
	m.markValidatorDirty(createMsg.PubKey)

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

	updatedDelegation := &store.BAPIDelegation{
		Delegator:       delegation.Delegator,
		ValidatorPubKey: delegation.ValidatorPubKey,
		Amount:          delegation.Amount - undelegateMsg.Amount.Amount,
	}
	delegationStoreKey := string(undelegateMsg.Delegator) + "/" + hex.EncodeToString(undelegateMsg.Validator)

	// Validator's TotalDelegation drops immediately even though the
	// tokens themselves wait 21 days. Power-derivation reads this
	// field, so the validator's voting weight reflects the unbonding
	// from the next BeginBlock onward.
	updatedValidator := *validator
	updatedValidator.TotalDelegation -= undelegateMsg.Amount.Amount
	updatedValidator.Power = updatedValidator.TotalDelegation
	m.markValidatorDirty(undelegateMsg.Validator)

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
