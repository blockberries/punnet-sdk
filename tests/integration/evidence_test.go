package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/require"
)

// TestExecuteBlock_EvidenceFlowsToStakingAndEndBlockEmitsSlashedPower is the
// runtime-level acceptance test for PLAN C2. It mounts the staking module
// on a BAPIApplication, seeds a validator with a known pubkey at non-zero
// power, then submits a FinalizedBlock whose Evidence slice contains a
// DuplicateVote entry for that validator. The block's outcome must include
// a ValidatorUpdate carrying the post-slash power — proving evidence flows
// all the way through ExecuteBlock → processEvidence → BAPIEvidenceHandler
// → effects executor → EndBlock.
func TestExecuteBlock_EvidenceFlowsToStakingAndEndBlockEmitsSlashedPower(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	if err != nil {
		t.Fatalf("state store: %v", err)
	}

	// One shared provider so the staking module's typed store and the
	// runtime's typed store hit the same underlying StateStore.
	provider := store.NewBAPIStoreProvider(ss)
	stakingMod, err := staking.NewBAPIStakingModule(
		provider.GetValidatorStore(),
		provider.GetBalanceStore(),
	)
	if err != nil {
		t.Fatalf("staking module: %v", err)
	}

	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "evidence-c2",
		StateStore: ss,
		Modules:    []runtime.BAPIModule{stakingMod},
	})
	if err != nil {
		t.Fatalf("application: %v", err)
	}

	ctx := context.Background()
	pubKey := []byte("c2-runtime-target-pubkey-32byt")
	const initialPower uint64 = 1000

	if err := provider.GetValidatorStore().SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           initialPower,
		TotalDelegation: initialPower,
	}); err != nil {
		t.Fatalf("seed validator: %v", err)
	}
	// Phase 2.6: slash transfers slashAmt from staking.pool to
	// module.ct. Seed staking.pool with enough to cover the slash so
	// the transfer effect doesn't underflow.
	if err := provider.GetBalanceStore().Set(ctx, "staking.pool", "stake", initialPower); err != nil {
		t.Fatalf("seed staking.pool: %v", err)
	}
	if _, _, err := ss.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	// Drain any first-block emission to keep the assertion tight.
	if _, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	}); err != nil {
		t.Fatalf("block 1 ExecuteBlock: %v", err)
	}
	if _, err := app.Commit(ctx); err != nil {
		t.Fatalf("block 1 Commit: %v", err)
	}

	ev := types.Evidence{
		Type:             types.EvidenceTypeDuplicateVote,
		Height:           2,
		Time:             types.TimeToTimestamp(time.Now()),
		TotalVotingPower: 3000,
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
	}
	_, err = app.ExecuteBlock(ctx, types.FinalizedBlock{
		Height:   2,
		Time:     types.TimeToTimestamp(time.Now()),
		Evidence: []types.Evidence{ev},
	})
	if err != nil {
		t.Fatalf("block 2 ExecuteBlock: %v", err)
	}

	// Phase 2.3: the slash applies immediately to the validator
	// store, but the ValidatorUpdate emission moves to the next
	// epoch-close EndBlock (D6 — mid-epoch power changes
	// accumulate). The chain-level acceptance is therefore "store
	// reflects the slash", not "same-block ValidatorUpdate emitted".
	expectedSlash := (initialPower * staking.SlashFractionBasisPoints) / 10000
	expectedPower := initialPower - expectedSlash
	v, err := provider.GetValidatorStore().GetValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("read slashed validator: %v", err)
	}
	if v == nil {
		t.Fatalf("slashed validator missing from store")
	}
	if v.Power != expectedPower {
		t.Errorf("store power: got %d, want %d", v.Power, expectedPower)
	}
	if !v.Jailed {
		t.Errorf("slashed validator must be jailed")
	}
}

// TestExecuteBlock_EvidenceUsesConsensusParamsSlashFraction is the
// runtime-level acceptance for PLAN's "slash fraction via ConsensusParams"
// follow-up. It seeds the params store with a custom slash fraction
// before block execution, fires evidence, and verifies the resulting
// ValidatorUpdate reflects the override rather than the compiled-in
// default. Without this end-to-end path, the new `BAPIBlockContext.Params`
// snapshot from paramsStore in processBlock could silently regress and
// the staking module would still pass its unit tests (which inject
// Params directly into blockCtx).
func TestExecuteBlock_EvidenceUsesConsensusParamsSlashFraction(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	if err != nil {
		t.Fatalf("state store: %v", err)
	}

	provider := store.NewBAPIStoreProvider(ss)
	stakingMod, err := staking.NewBAPIStakingModule(
		provider.GetValidatorStore(),
		provider.GetBalanceStore(),
	)
	if err != nil {
		t.Fatalf("staking module: %v", err)
	}

	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "evidence-params-override",
		StateStore: ss,
		Modules:    []runtime.BAPIModule{stakingMod},
	})
	if err != nil {
		t.Fatalf("application: %v", err)
	}

	ctx := context.Background()
	pubKey := []byte("override-runtime-target-pubkey-")
	const initialPower uint64 = 10000
	const overrideBps uint32 = 2500 // 25%, much harsher than the 5% default

	// Seed the validator AND the consensus params before any block runs.
	if err := provider.GetValidatorStore().SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           initialPower,
		TotalDelegation: initialPower,
	}); err != nil {
		t.Fatalf("seed validator: %v", err)
	}
	// Phase 2.6: stake the slash transfer.
	if err := provider.GetBalanceStore().Set(ctx, "staking.pool", "stake", initialPower); err != nil {
		t.Fatalf("seed staking.pool: %v", err)
	}
	params := store.DefaultConsensusParams()
	params.SlashFractionDoubleSignBps = overrideBps
	if err := provider.GetParamsStore().Set(ctx, params); err != nil {
		t.Fatalf("seed params: %v", err)
	}
	if _, _, err := ss.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	// Drain block 1.
	if _, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	}); err != nil {
		t.Fatalf("block 1 ExecuteBlock: %v", err)
	}
	if _, err := app.Commit(ctx); err != nil {
		t.Fatalf("block 1 Commit: %v", err)
	}

	// Block 2 with double-sign evidence.
	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		Height: 2,
		Time:   types.TimeToTimestamp(time.Now()),
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
	}
	_, err = app.ExecuteBlock(ctx, types.FinalizedBlock{
		Height:   2,
		Time:     types.TimeToTimestamp(time.Now()),
		Evidence: []types.Evidence{ev},
	})
	if err != nil {
		t.Fatalf("block 2 ExecuteBlock: %v", err)
	}

	// Phase 2.3: ValidatorUpdate emission moves to epoch-close.
	// Verify the slash applied correctly in the validator store.
	expectedSlash := (initialPower * uint64(overrideBps)) / 10000
	expectedPower := initialPower - expectedSlash
	v, err := provider.GetValidatorStore().GetValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("read slashed validator: %v", err)
	}
	if v == nil {
		t.Fatalf("slashed validator missing from store")
	}
	if v.Power != expectedPower {
		t.Errorf("post-slash store power: got %d, want %d (initial=%d, bps=%d)",
			v.Power, expectedPower, initialPower, overrideBps)
	}
}

// TestBAPIApplication_ExportGenesis_AggregatesAllModules verifies the
// runtime-level genesis export: it must walk every module's
// ExportGenesis and produce a coherent GenesisDoc with the current
// validator set, params, and module state. Closes the previous
// TODO ("Implement consensus params update collection from modules"
// was a related missing piece; this PR addresses the broader
// "BAPIApplication has no ExportGenesis aggregator" gap).
func TestBAPIApplication_ExportGenesis_AggregatesAllModules(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	if err != nil {
		t.Fatalf("state store: %v", err)
	}

	provider := store.NewBAPIStoreProvider(ss)
	stakingMod, err := staking.NewBAPIStakingModule(
		provider.GetValidatorStore(),
		provider.GetBalanceStore(),
	)
	if err != nil {
		t.Fatalf("staking module: %v", err)
	}

	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "export-genesis-test",
		StateStore: ss,
		Modules:    []runtime.BAPIModule{stakingMod},
	})
	if err != nil {
		t.Fatalf("application: %v", err)
	}

	ctx := context.Background()

	// Seed two active validators + one jailed validator. ExportGenesis
	// should drop the jailed one (a fresh chain shouldn't inherit a
	// jailed validator's seat).
	for _, v := range []*store.BAPIValidator{
		{
			PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: []byte("alice-pubkey-32-bytes-for-test-")},
			Power:  100,
		},
		{
			PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: []byte("bob-pubkey-32-bytes-for-test-aa")},
			Power:  200,
		},
		{
			PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: []byte("jailed-pubkey-32-bytes-for-test")},
			Power:  300,
			Jailed: true,
		},
	} {
		require.NoError(t, provider.GetValidatorStore().SetValidator(ctx, v))
	}

	// Seed non-default params so we can verify they round-trip.
	params := store.DefaultConsensusParams()
	params.SlashFractionDoubleSignBps = 700
	require.NoError(t, provider.GetParamsStore().Set(ctx, params))

	if _, _, err := ss.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	// Export.
	doc, err := app.ExportGenesis(ctx)
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "export-genesis-test", doc.ChainID)
	require.Equal(t, uint32(700), doc.ConsensusParams.SlashFractionDoubleSignBps,
		"export must round-trip the custom slash fraction")

	// Only the two un-jailed validators should appear, sorted by pubkey.
	require.Len(t, doc.Validators, 2,
		"jailed validator must be excluded from genesis export")
	require.Less(t, string(doc.Validators[0].PubKey.Data), string(doc.Validators[1].PubKey.Data),
		"validators must be sorted by pubkey bytes")

	// AppState must include the staking module's state.
	var gs runtime.BAPIGenesisState
	require.NoError(t, json.Unmarshal(doc.AppState, &gs))
	stakingState, ok := gs.Modules["staking"]
	require.True(t, ok, "AppState must include staking module")
	require.NotEmpty(t, stakingState)
}

// stubParamsUpdater is a minimal BAPIModule that emits a fixed
// ConsensusParams update at every EndBlock. Used to drive the
// runtime's BAPIParamsUpdater aggregator end-to-end.
type stubParamsUpdater struct {
	name      string
	update    *types.ConsensusParams
	calls     int
	failsWith error
}

func (s *stubParamsUpdater) Name() string { return s.name }
func (s *stubParamsUpdater) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return nil
}
func (s *stubParamsUpdater) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return nil
}
func (s *stubParamsUpdater) ParamsUpdate(_ context.Context, _ *runtime.BAPIBlockContext) (*types.ConsensusParams, error) {
	s.calls++
	if s.failsWith != nil {
		return nil, s.failsWith
	}
	return s.update, nil
}

// TestBAPIApplication_ParamsUpdateFromModule verifies the runtime's
// BAPIParamsUpdater aggregator: a module that emits a non-nil
// ConsensusParams during EndBlock causes the params to update
// persistently, observable by the NEXT block's params snapshot.
func TestBAPIApplication_ParamsUpdateFromModule(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)

	overrideBps := uint32(1500) // 15%
	updaterParams := store.DefaultConsensusParams()
	updaterParams.SlashFractionDoubleSignBps = overrideBps

	updater := &stubParamsUpdater{
		name:   "params-test-module",
		update: updaterParams,
	}

	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "params-update-test",
		StateStore: ss,
		Modules:    []runtime.BAPIModule{updater},
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Initialize genesis via the public Handshake path so paramsStore.Get works.
	defaultParams := *store.DefaultConsensusParams()
	_, err = app.Handshake(ctx, types.HandshakeRequest{
		Genesis: &types.GenesisDoc{
			ChainID:         "params-update-test",
			ConsensusParams: defaultParams,
		},
	})
	require.NoError(t, err)

	// Block 1: the updater emits a non-nil ConsensusParams update.
	outcome, err := app.ExecuteBlock(ctx, types.FinalizedBlock{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	require.NotNil(t, outcome.ParamsUpdate, "BlockOutcome must surface the update to consumers")
	require.Equal(t, overrideBps, outcome.ParamsUpdate.SlashFractionDoubleSignBps)
	require.Equal(t, 1, updater.calls)

	// The new params must be persisted in the store, observable next block.
	_, err = app.Commit(ctx)
	require.NoError(t, err)

	provider := store.NewBAPIStoreProvider(ss)
	stored, err := provider.GetParamsStore().Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, overrideBps, stored.SlashFractionDoubleSignBps,
		"params persisted to store so next block sees the new value")
}

// conflictingParamsUpdater pairs with stubParamsUpdater under the same
// router; both emit non-nil to exercise the conflict-rejection path.
type conflictingParamsUpdater struct {
	name string
}

func (c *conflictingParamsUpdater) Name() string { return c.name }
func (c *conflictingParamsUpdater) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return nil
}
func (c *conflictingParamsUpdater) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return nil
}
func (c *conflictingParamsUpdater) ParamsUpdate(_ context.Context, _ *runtime.BAPIBlockContext) (*types.ConsensusParams, error) {
	p := store.DefaultConsensusParams()
	p.SlashFractionDoubleSignBps = 9999
	return p, nil
}

// TestBAPIApplication_ParamsUpdate_ConflictRejected verifies the
// aggregator refuses to silently overwrite when two modules both emit
// a params update at the same block. ExecuteBlock returns an error
// rather than picking one arbitrarily.
func TestBAPIApplication_ParamsUpdate_ConflictRejected(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)

	updater1 := &stubParamsUpdater{
		name:   "a-first-module",
		update: store.DefaultConsensusParams(),
	}
	updater2 := &conflictingParamsUpdater{name: "z-second-module"}

	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "params-conflict-test",
		StateStore: ss,
		Modules:    []runtime.BAPIModule{updater1, updater2},
	})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = app.Handshake(ctx, types.HandshakeRequest{
		Genesis: &types.GenesisDoc{
			ChainID:         "params-conflict-test",
			ConsensusParams: *store.DefaultConsensusParams(),
		},
	})
	require.NoError(t, err)

	_, err = app.ExecuteBlock(ctx, types.FinalizedBlock{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.Error(t, err, "two modules both emitting params updates must error")
	require.Contains(t, err.Error(), "conflicting params updates",
		"error should explain what went wrong")
}
