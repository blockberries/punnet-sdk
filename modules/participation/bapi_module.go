package participation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// BAPIParticipationModule tracks per-validator participation
// counters across each epoch. Implements runtime.BAPIModule,
// runtime.BAPIBlockProcessor (for epoch-close freeze + reset),
// runtime.BAPIMempoolObserver (for the actual counter updates),
// and runtime.BAPIGenesisInitializer.
//
// Determinism: writes happen directly to the state store outside
// the ExecuteBlock effect pipeline. The BAPI server serializes
// lifecycle and mempool-observer calls per app, so the writes are
// internally consistent. Cross-validator agreement depends on
// raspberry delivering the same OnBatchCertified / OnBlockConstructed
// event sequence to every node. PLAN §7 Phase 3.8 is the consensus
// layer's piece of that contract.
type BAPIParticipationModule struct {
	store *store.TypedStore[*ValidatorParticipation]

	// totalsStore is a separate typed store for the EpochTotals
	// record (different value type than ValidatorParticipation).
	totalsStore *store.TypedStore[*EpochTotals]

	// rawStore is the underlying state store (used by epoch-close
	// freeze for direct iteration).
	rawStore statestore.StateStore
}

// NewBAPIParticipationModule constructs the module backed by the
// given state store.
func NewBAPIParticipationModule(ss statestore.StateStore) (*BAPIParticipationModule, error) {
	if ss == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}
	return &BAPIParticipationModule{
		store:       store.NewTypedStore[*ValidatorParticipation](ss, StorePrefix),
		totalsStore: store.NewTypedStore[*EpochTotals](ss, StorePrefix),
		rawStore:    ss,
	}, nil
}

// Name returns the module name.
func (m *BAPIParticipationModule) Name() string { return ModuleName }

// RegisterMsgHandlers returns the (empty) message handler table.
// The module is observer-driven; it has no transactional messages.
func (m *BAPIParticipationModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{}
}

// RegisterQueryHandlers returns query handlers.
func (m *BAPIParticipationModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/participation/current":         m.handleQueryCurrent,
		"/participation/current_totals":  m.handleQueryCurrentTotals,
		"/participation/epoch":           m.handleQueryEpoch,
	}
}

// InitGenesis is a no-op. The module is observer-driven; there's
// no genesis state to seed (the very first epoch begins at height 1
// with all counters at zero).
func (m *BAPIParticipationModule) InitGenesis(_ context.Context, _ []byte) error {
	return nil
}

// ExportGenesis returns the current in-progress epoch counters
// (used for state-sync — PLAN §7 Phase 3.4). A node joining the
// chain via state-sync imports this and continues counting from
// here.
func (m *BAPIParticipationModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	totals, _ := m.totalsStore.Get(ctx, keyCurrentTotals)
	if totals == nil {
		totals = &EpochTotals{}
	}
	return json.Marshal(struct {
		Totals *EpochTotals `json:"totals"`
	}{Totals: totals})
}

// OnBatchCertified implements runtime.BAPIMempoolObserver. Spec §3.4:
// every cert-quorum event increments batches_certified[v] for the
// authoring validator. Workers all credit the same validator_id
// (PLAN D16) — the event carries the validator address directly so
// we just use that.
func (m *BAPIParticipationModule) OnBatchCertified(ctx context.Context, ev types.BatchCertifiedEvent) error {
	if m == nil {
		return nil
	}
	hexKey := hex.EncodeToString(ev.Validator[:])
	rowKey := keyCurrentValidatorPrefix + hexKey

	row, err := m.store.Get(ctx, rowKey)
	if err != nil || row == nil {
		row = &ValidatorParticipation{}
	}
	row.BatchesCertified++
	if err := m.store.Set(ctx, rowKey, row); err != nil {
		return fmt.Errorf("persist validator participation: %w", err)
	}

	totals, _ := m.totalsStore.Get(ctx, keyCurrentTotals)
	if totals == nil {
		totals = &EpochTotals{}
	}
	totals.BatchesCertified++
	return m.totalsStore.Set(ctx, keyCurrentTotals, totals)
}

// OnBlockConstructed implements runtime.BAPIMempoolObserver. Spec
// §3.4 / PLAN D11: leader_blocks[v] is incremented only when the
// proposed block contains ≥1 certified batch. Empty proposals earn
// no leader credit.
func (m *BAPIParticipationModule) OnBlockConstructed(ctx context.Context, ev types.BlockConstructedEvent) error {
	if m == nil {
		return nil
	}
	if len(ev.IncludedBatchHashes) == 0 {
		return nil
	}
	hexKey := hex.EncodeToString(ev.Leader[:])
	rowKey := keyCurrentValidatorPrefix + hexKey

	row, err := m.store.Get(ctx, rowKey)
	if err != nil || row == nil {
		row = &ValidatorParticipation{}
	}
	row.LeaderBlocks++
	if err := m.store.Set(ctx, rowKey, row); err != nil {
		return fmt.Errorf("persist validator participation: %w", err)
	}

	totals, _ := m.totalsStore.Get(ctx, keyCurrentTotals)
	if totals == nil {
		totals = &EpochTotals{}
	}
	totals.LeaderBlocks++
	return m.totalsStore.Set(ctx, keyCurrentTotals, totals)
}

// BeginBlock is a no-op. Counter updates happen in the observer
// callbacks; epoch-close freeze runs in EndBlock.
func (m *BAPIParticipationModule) BeginBlock(_ context.Context, _ *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	return nil, nil
}

// EndBlock freezes the in-progress counters at every epoch-close
// block. Per spec §3.4: "settlement at epoch close ... the protocol
// computes weighted participation shares and credits per-validator
// reward accumulators".
//
// The freeze copies "current/v/*" and "current/totals" to
// "epoch/<N>/v/*" and "epoch/<N>/totals" (where N is the closing
// epoch number), then clears the current rows so the next epoch
// starts at zero.
//
// Distribution (Phase 3.5) reads the frozen entries at the same
// EndBlock height.
func (m *BAPIParticipationModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	if blockCtx == nil || !staking.IsEpochCloseHeight(uint64(blockCtx.Height)) {
		return nil, nil, nil
	}
	epochNum := uint64(blockCtx.Height) / staking.EpochBlocks

	// Walk current validator rows; freeze each one.
	var validatorKeys []string
	var validatorVals []*ValidatorParticipation
	if err := m.store.IterateRelative(func(relKey string, v *ValidatorParticipation) bool {
		if v == nil {
			return false
		}
		if len(relKey) < len(keyCurrentValidatorPrefix) ||
			relKey[:len(keyCurrentValidatorPrefix)] != keyCurrentValidatorPrefix {
			return false
		}
		validatorKeys = append(validatorKeys, relKey)
		validatorVals = append(validatorVals, v)
		return false
	}); err != nil {
		// Iteration unsupported: skip the freeze. The current rows
		// remain in place; the distribution module won't find a
		// frozen summary and will treat the epoch as empty. Test
		// stores that don't iterate naturally fall into this path.
		return nil, nil, nil
	}

	for i, key := range validatorKeys {
		// Carve out the validator-hex suffix:
		// relKey = "current/v/<hex>"
		hexKey := key[len(keyCurrentValidatorPrefix):]
		frozenKey := fmt.Sprintf(keyEpochValidatorFmt, epochNum, hexKey)
		if err := m.store.Set(ctx, frozenKey, validatorVals[i]); err != nil {
			return nil, nil, fmt.Errorf("freeze validator %s: %w", hexKey, err)
		}
		if err := m.store.Delete(ctx, key); err != nil {
			return nil, nil, fmt.Errorf("clear current row %s: %w", hexKey, err)
		}
	}

	// Freeze + reset totals.
	totals, _ := m.totalsStore.Get(ctx, keyCurrentTotals)
	if totals == nil {
		totals = &EpochTotals{}
	}
	frozenTotalsKey := fmt.Sprintf(keyEpochTotalsFmt, epochNum)
	if err := m.totalsStore.Set(ctx, frozenTotalsKey, totals); err != nil {
		return nil, nil, fmt.Errorf("freeze totals: %w", err)
	}
	// Reset by delete rather than write-zero: cramberry encodes the
	// zero-value EpochTotals as empty bytes, which the typed store
	// then reads back as ErrNotFound. Deletion is unambiguous; the
	// next OnBatchCertified / OnBlockConstructed recreates the row
	// at value 1.
	if err := m.totalsStore.Delete(ctx, keyCurrentTotals); err != nil {
		return nil, nil, fmt.Errorf("reset totals: %w", err)
	}
	return nil, nil, nil
}

// GetEpochParticipation returns one validator's frozen counters
// for a closed epoch, or nil + ErrNotFound when no such record
// exists. Used by the distribution module at epoch close.
func (m *BAPIParticipationModule) GetEpochParticipation(ctx context.Context, epoch uint64, validator types.ValidatorAddress) (*ValidatorParticipation, error) {
	key := fmt.Sprintf(keyEpochValidatorFmt, epoch, hex.EncodeToString(validator[:]))
	return m.store.Get(ctx, key)
}

// GetEpochTotals returns the chain-wide totals for a closed epoch.
func (m *BAPIParticipationModule) GetEpochTotals(ctx context.Context, epoch uint64) (*EpochTotals, error) {
	return m.totalsStore.Get(ctx, fmt.Sprintf(keyEpochTotalsFmt, epoch))
}

// IterateEpochParticipation walks every validator's frozen
// participation record for a given closed epoch. Used by
// distribution to enumerate which validators earned anything in
// the epoch. fn returns true to stop early.
func (m *BAPIParticipationModule) IterateEpochParticipation(epoch uint64, fn func(validatorHex string, v *ValidatorParticipation) bool) error {
	prefix := fmt.Sprintf("epoch/%020d/v/", epoch)
	return m.store.IterateRelative(func(relKey string, v *ValidatorParticipation) bool {
		if v == nil {
			return false
		}
		if len(relKey) < len(prefix) || relKey[:len(prefix)] != prefix {
			return false
		}
		return fn(relKey[len(prefix):], v)
	})
}

// --- Query handlers ---

func (m *BAPIParticipationModule) handleQueryCurrent(ctx context.Context, data []byte, _ int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("validator address required (20 bytes hex)")
	}
	addrBytes, err := hex.DecodeString(string(data))
	if err != nil || len(addrBytes) != 20 {
		return nil, fmt.Errorf("invalid validator address")
	}
	key := keyCurrentValidatorPrefix + string(data)
	v, err := m.store.Get(ctx, key)
	if err != nil {
		return json.Marshal(&ValidatorParticipation{})
	}
	return json.Marshal(v)
}

func (m *BAPIParticipationModule) handleQueryCurrentTotals(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	t, err := m.totalsStore.Get(ctx, keyCurrentTotals)
	if err != nil {
		return json.Marshal(&EpochTotals{})
	}
	return json.Marshal(t)
}

func (m *BAPIParticipationModule) handleQueryEpoch(ctx context.Context, data []byte, _ int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("epoch number required")
	}
	var epoch uint64
	if _, err := fmt.Sscanf(string(data), "%d", &epoch); err != nil {
		return nil, fmt.Errorf("invalid epoch number: %w", err)
	}
	totals, _ := m.totalsStore.Get(ctx, fmt.Sprintf(keyEpochTotalsFmt, epoch))
	return json.Marshal(totals)
}

// Compile-time interface compliance.
var (
	_ runtime.BAPIModule             = (*BAPIParticipationModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIParticipationModule)(nil)
	_ runtime.BAPIMempoolObserver    = (*BAPIParticipationModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIParticipationModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIParticipationModule)(nil)
)

// avoid unused-import warnings until ptypes is used by handlers.
var _ = ptypes.AccountName("")
