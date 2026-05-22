package fees

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// State-store keys under StorePrefix.
const (
	keyCurrentSchedule = "current"
	keyPendingPrefix   = "pending/" // pending/<8-byte-big-endian-height>
)

// BAPIFeesModule owns the chain's fee schedule and the queued
// schedule updates. PLAN §7 Phase 1.1.
//
// The module does NOT register an AnteHandler itself — that's the
// runtime's responsibility (Phase 1.4 will register a
// ProvideAnteHandler-style closure). The module just owns the state
// + exposes a Schedule() accessor.
type BAPIFeesModule struct {
	scheduleStore *store.TypedStore[FeeSchedule]
	pendingStore  *store.TypedStore[PendingFeeUpdate]
}

// NewBAPIFeesModule constructs the module backed by the given state
// store. The two typed stores share the StorePrefix namespace
// (scheduleStore writes "fees/current"; pendingStore writes
// "fees/pending/<height>").
func NewBAPIFeesModule(stateStore statestore.StateStore) (*BAPIFeesModule, error) {
	if stateStore == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}
	return &BAPIFeesModule{
		scheduleStore: store.NewTypedStore[FeeSchedule](stateStore, StorePrefix),
		pendingStore:  store.NewTypedStore[PendingFeeUpdate](stateStore, StorePrefix),
	}, nil
}

// Name returns the module name.
func (m *BAPIFeesModule) Name() string { return ModuleName }

// RegisterMsgHandlers returns the message handler table.
//
// MsgProposeFeeUpdate is registered but its handler returns an
// "unimplemented" error in v1 — Phase 4 (governance) wires the
// timelock-and-enactment path that uses this message.
func (m *BAPIFeesModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgProposeFeeUpdate: m.handleProposeFeeUpdate,
	}
}

// RegisterQueryHandlers returns query handlers for reading the
// schedule and queued pending updates.
func (m *BAPIFeesModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/fees/schedule":     m.handleQuerySchedule,
		"/fees/pending":      m.handleQueryPending,
		"/fees/op_fee":       m.handleQueryOpFee,
		"/fees/byte_fee":     m.handleQueryByteFee,
	}
}

// InitGenesis initializes the schedule + any pending updates from
// the per-module genesis blob.
func (m *BAPIFeesModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		// No genesis data → install a zero schedule. The AnteHandler
		// will reject every tx that carries non-zero Fee until the
		// chain installs a real schedule via governance.
		return m.scheduleStore.Set(ctx, keyCurrentSchedule, FeeSchedule{})
	}

	var gs FeesGenesisState
	if err := json.Unmarshal(data, &gs); err != nil {
		return fmt.Errorf("unmarshal fees genesis: %w", err)
	}

	// Canonicalize and validate the initial schedule.
	gs.Schedule.OpFees = SortedOpFees(gs.Schedule.OpFees)
	if err := gs.Schedule.Validate(); err != nil {
		return fmt.Errorf("invalid initial fee schedule: %w", err)
	}
	if err := m.scheduleStore.Set(ctx, keyCurrentSchedule, gs.Schedule); err != nil {
		return fmt.Errorf("set initial schedule: %w", err)
	}

	// Pending updates, if any.
	for i, p := range gs.Pending {
		p.Schedule.OpFees = SortedOpFees(p.Schedule.OpFees)
		if err := p.Schedule.Validate(); err != nil {
			return fmt.Errorf("invalid pending update %d: %w", i, err)
		}
		if err := m.pendingStore.Set(ctx, pendingKey(p.EffectiveHeight), p); err != nil {
			return fmt.Errorf("seed pending update at height %d: %w", p.EffectiveHeight, err)
		}
	}
	return nil
}

// ExportGenesis writes the current schedule + remaining pending
// updates back into a JSON blob suitable for genesis bootstrap.
func (m *BAPIFeesModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	current, err := m.scheduleStore.Get(ctx, keyCurrentSchedule)
	if err != nil {
		return nil, fmt.Errorf("get current schedule: %w", err)
	}

	// Walk the pending store via IterateRelative and collect entries.
	// The Iterable contract: callback returns true to STOP early; false
	// to continue. Filter to the "pending/" sub-namespace; the
	// keyCurrentSchedule row decodes as a zero-valued PendingFeeUpdate.
	var pending []PendingFeeUpdate
	err = m.pendingStore.IterateRelative(func(relKey string, value PendingFeeUpdate) bool {
		if len(relKey) < len(keyPendingPrefix) || relKey[:len(keyPendingPrefix)] != keyPendingPrefix {
			return false
		}
		pending = append(pending, value)
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("iterate pending: %w", err)
	}
	// Sort defensively by EffectiveHeight for deterministic export.
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].EffectiveHeight < pending[j].EffectiveHeight
	})

	return json.Marshal(FeesGenesisState{
		Schedule: current,
		Pending:  pending,
	})
}

// Schedule returns the current fee schedule. Used by the AnteHandler
// (Phase 1.4) and by ad-hoc consumers.
func (m *BAPIFeesModule) Schedule(ctx context.Context) (FeeSchedule, error) {
	return m.scheduleStore.Get(ctx, keyCurrentSchedule)
}

// BeginBlock activates any pending updates whose EffectiveHeight is
// ≤ the current block height. Walking via IterateRelative keeps the
// O(N) pending-queue scan bounded by the small number of queued
// updates (governance produces at most one per timelock window).
//
// The current schedule is replaced wholesale by the newest activated
// update; entries are deleted on activation.
func (m *BAPIFeesModule) BeginBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	if blockCtx == nil {
		return nil, fmt.Errorf("block context is nil")
	}

	var due []PendingFeeUpdate
	err := m.pendingStore.IterateRelative(func(relKey string, value PendingFeeUpdate) bool {
		if len(relKey) < len(keyPendingPrefix) || relKey[:len(keyPendingPrefix)] != keyPendingPrefix {
			return false
		}
		if value.EffectiveHeight <= uint64(blockCtx.Height) {
			due = append(due, value)
		}
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("iterate pending: %w", err)
	}
	if len(due) == 0 {
		return nil, nil
	}
	// Apply in height order so the highest-EffectiveHeight schedule wins.
	sort.Slice(due, func(i, j int) bool {
		return due[i].EffectiveHeight < due[j].EffectiveHeight
	})

	// BeginBlock returns effects, but the runtime doesn't yet expose
	// a "delete + write under a foreign prefix" effect shape. The
	// staking module uses the same direct-store-write pattern for its
	// BeginBlock activations (see staking/bapi_module.go); the BAPI
	// runtime executes BeginBlock under the block-store transaction so
	// these writes commit atomically with the block.
	for _, p := range due {
		if err := m.pendingStore.Delete(ctx, pendingKey(p.EffectiveHeight)); err != nil {
			return nil, fmt.Errorf("delete activated update at height %d: %w", p.EffectiveHeight, err)
		}
	}
	winner := due[len(due)-1].Schedule
	if err := m.scheduleStore.Set(ctx, keyCurrentSchedule, winner); err != nil {
		return nil, fmt.Errorf("install schedule at height %d: %w", blockCtx.Height, err)
	}
	return nil, nil
}

// EndBlock is a no-op for the fees module; required to satisfy
// BAPIBlockProcessor alongside BeginBlock.
func (m *BAPIFeesModule) EndBlock(_ context.Context, _ *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	return nil, nil, nil
}

// --- Message handlers ---

// handleProposeFeeUpdate is the Phase 4 governance entrypoint. Until
// the governance module learns to enact proposals at their effective
// height, direct submission returns an error so apps don't
// accidentally treat unsigned proposals as authoritative.
func (m *BAPIFeesModule) handleProposeFeeUpdate(_ context.Context, _ *runtime.BAPITxContext, _ ptypes.Message) ([]effects.Effect, error) {
	return nil, fmt.Errorf("MsgProposeFeeUpdate not yet enabled — Phase 4 of the tokenomics SDK plan wires governance enactment")
}

// --- Query handlers ---

func (m *BAPIFeesModule) handleQuerySchedule(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	sched, err := m.scheduleStore.Get(ctx, keyCurrentSchedule)
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return json.Marshal(sched)
}

func (m *BAPIFeesModule) handleQueryPending(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	var pending []PendingFeeUpdate
	err := m.pendingStore.IterateRelative(func(relKey string, value PendingFeeUpdate) bool {
		if len(relKey) < len(keyPendingPrefix) || relKey[:len(keyPendingPrefix)] != keyPendingPrefix {
			return false
		}
		pending = append(pending, value)
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("iterate pending: %w", err)
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].EffectiveHeight < pending[j].EffectiveHeight
	})
	return json.Marshal(pending)
}

func (m *BAPIFeesModule) handleQueryOpFee(ctx context.Context, data []byte, _ int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("op_fee query requires message_type in data")
	}
	sched, err := m.scheduleStore.Get(ctx, keyCurrentSchedule)
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	amount, _ := sched.OpFee(string(data))
	return json.Marshal(amount)
}

func (m *BAPIFeesModule) handleQueryByteFee(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	sched, err := m.scheduleStore.Get(ctx, keyCurrentSchedule)
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return json.Marshal(sched.ByteFee)
}

// pendingKey is the typed-store key for a pending update at the
// given height: "pending/" + zero-padded decimal height. Decimal
// pad (20 digits) keeps lexical order == numeric order so iteration
// is height-sorted without extra sorting cost.
func pendingKey(height uint64) string {
	return fmt.Sprintf("%s%020d", keyPendingPrefix, height)
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule             = (*BAPIFeesModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIFeesModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIFeesModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIFeesModule)(nil)
	_ runtime.BAPIModuleParams       = (*BAPIFeesModule)(nil)
)

// Parameter names governance proposals can reference. The "op_fee:"
// prefix carries the message-type identifier as a suffix:
//
//	op_fee:/bank.MsgSend
//
// PLAN §7 Phase 4.5.
const (
	ParamByteFee   = "byte_fee"
	ParamOpFeePref = "op_fee:"
)

// ApplyParameterChange implements runtime.BAPIModuleParams. The
// governance enactment hook (Phase 4.4) calls this on each Change
// targeting the "fees" module. Two parameter shapes:
//
//   - "byte_fee"             : update FeeSchedule.ByteFee
//   - "op_fee:<message_type>": upsert FeeSchedule.OpFees[message_type]
//
// Both validate value bounds (must fit uint64). MsgProposeFeeUpdate
// (Phase 1.2 stub) remains rejected — the canonical path is
// MsgSubmitProposal with one or more Changes targeting "fees".
func (m *BAPIFeesModule) ApplyParameterChange(ctx context.Context, name string, newValue int64) error {
	if m == nil || m.scheduleStore == nil {
		return fmt.Errorf("module not initialized")
	}
	if newValue < 0 {
		return fmt.Errorf("fee parameter %q: value must be non-negative", name)
	}

	current, err := m.scheduleStore.Get(ctx, keyCurrentSchedule)
	if err != nil {
		current = FeeSchedule{}
	}

	switch {
	case name == ParamByteFee:
		current.ByteFee = uint64(newValue)
	case len(name) > len(ParamOpFeePref) && name[:len(ParamOpFeePref)] == ParamOpFeePref:
		messageType := name[len(ParamOpFeePref):]
		current.OpFees = upsertOpFee(current.OpFees, messageType, uint64(newValue))
	default:
		return fmt.Errorf("fees module does not own parameter %q (want %s or %s<msg-type>)",
			name, ParamByteFee, ParamOpFeePref)
	}

	if err := current.Validate(); err != nil {
		return fmt.Errorf("post-change schedule invalid: %w", err)
	}
	return m.scheduleStore.Set(ctx, keyCurrentSchedule, current)
}

// upsertOpFee returns a sorted OpFees slice with the given
// (messageType, amount) entry inserted or replaced. The slice
// invariant — sorted by MessageType — is preserved so the binary
// search in FeeSchedule.OpFee continues to work.
func upsertOpFee(entries []OpFeeEntry, messageType string, amount uint64) []OpFeeEntry {
	for i := range entries {
		if entries[i].MessageType == messageType {
			entries[i].Amount = amount
			return entries
		}
	}
	entries = append(entries, OpFeeEntry{MessageType: messageType, Amount: amount})
	return SortedOpFees(entries)
}
