package governance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubParamModule is a minimal BAPIModuleParams implementation
// used by the enactment tests. It records every applied change
// in `applied` so the test can assert the dispatch fired.
//
// `failOn` lets a test inject a failure for a specific parameter
// name (simulating cross-parameter validation rejection after the
// band check passed).
type stubParamModule struct {
	name    string
	applied map[string]int64
	failOn  map[string]error
}

func newStubParamModule(name string) *stubParamModule {
	return &stubParamModule{
		name:    name,
		applied: make(map[string]int64),
		failOn:  make(map[string]error),
	}
}

func (s *stubParamModule) Name() string { return s.name }
func (s *stubParamModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return nil
}
func (s *stubParamModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return nil
}
func (s *stubParamModule) ApplyParameterChange(_ context.Context, name string, newValue int64) error {
	if err := s.failOn[name]; err != nil {
		return err
	}
	s.applied[name] = newValue
	return nil
}

func newEnactmentFixture(t *testing.T) (*BAPIGovernanceModule, *store.BAPIProposalStore, *stubParamModule, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	ps := store.NewBAPIProposalStore(ss)
	gov, err := NewBAPIGovernanceModule(ps, provider.GetBalanceStore())
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, gov.InitGenesis(ctx, nil))

	target := newStubParamModule("fees")
	require.NoError(t, gov.RegisterParameterTarget("fees", target))
	require.NoError(t, gov.Parameters.Register(ParameterBand{
		Name: "byte_fee",
		SoftMin: 0, SoftMax: 100,
		HardMin: 0, HardMax: 1000,
	}))
	return gov, ps, target, ctx
}

func passedProposalForChange(class string, effectiveHeight uint64, changes []store.ProposalChange) *store.BAPIProposal {
	return &store.BAPIProposal{
		ID:              1,
		Status:          string(StatusPassed),
		Class:           class,
		EffectiveHeight: effectiveHeight,
		Changes:         changes,
	}
}

// TestEnactment_AppliesAtEffectiveHeight: a passed proposal whose
// EffectiveHeight matches the current block applies the change.
func TestEnactment_AppliesAtEffectiveHeight(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 100,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)

	assert.Equal(t, int64(50), target.applied["byte_fee"], "target module saw the change")
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnacted), p.Status)
}

// TestEnactment_DoesNotApplyBeforeEffectiveHeight: at heights
// before EffectiveHeight, the proposal stays Passed and the
// target sees nothing.
func TestEnactment_DoesNotApplyBeforeEffectiveHeight(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 1000,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 999,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	assert.Empty(t, target.applied)
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusPassed), p.Status, "still in passed state pre-EffectiveHeight")
}

// TestEnactment_BandValidationGate: a Simple7d proposal whose
// value sits outside the soft band fails enactment without
// touching the target module.
func TestEnactment_BandValidationGate(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	// byte_fee soft band is 0–100; value 500 exceeds it.
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 100,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 500},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	assert.Empty(t, target.applied, "out-of-band value never dispatched")
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnactmentFailed), p.Status)
}

// TestEnactment_SuperClassAllowsExtendedRange: a Super30d
// proposal can push a value beyond soft (up to hard band).
func TestEnactment_SuperClassAllowsExtendedRange(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	// Out of soft (0–100) but within hard (0–1000).
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSuper30d, 100,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 500},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(500), target.applied["byte_fee"])
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnacted), p.Status)
}

// TestEnactment_BundledProposalAllValid: a bundle of two valid
// changes applies both atomically (band-validation passes for
// each before any dispatch).
func TestEnactment_BundledProposalAllValid(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	require.NoError(t, gov.Parameters.Register(ParameterBand{
		Name: "op_fee_transfer", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 100,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
			{TargetModule: "fees", ParameterName: "op_fee_transfer", NewValueInt: 80},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(50), target.applied["byte_fee"])
	assert.Equal(t, int64(80), target.applied["op_fee_transfer"])
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnacted), p.Status)
}

// TestEnactment_BundledOneOutOfBandAtomic: a bundle with one
// valid change and one out-of-band change pre-validates ALL
// changes before dispatching any. Both get aborted, neither
// applies. Validates "half-applied enactment cannot happen" for
// the band-rejection path (Phase 4.7 acceptance).
func TestEnactment_BundledOneOutOfBandAtomic(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	require.NoError(t, gov.Parameters.Register(ParameterBand{
		Name: "op_fee_transfer", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))
	// First change valid (byte_fee=50, soft OK); second out of soft (500).
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 100,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
			{TargetModule: "fees", ParameterName: "op_fee_transfer", NewValueInt: 500},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	assert.Empty(t, target.applied,
		"pre-validation must abort the whole bundle when any change is out of band")
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnactmentFailed), p.Status)
}

// TestEnactment_UnknownTargetFails: a proposal referencing an
// un-registered target module fails enactment with a clear error.
func TestEnactment_UnknownTargetFails(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 100,
		[]store.ProposalChange{
			{TargetModule: "nonexistent", ParameterName: "byte_fee", NewValueInt: 50},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	assert.Empty(t, target.applied)
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnactmentFailed), p.Status)
}

// TestEnactment_MidBundleDispatchFailHalfApplies documents the
// v1 limitation noted in the enactProposal doc-comment: if a
// target module rejects a change at dispatch time (after band
// validation passed), earlier changes in the same bundle are
// already applied. The post-status is StatusEnactmentFailed.
func TestEnactment_MidBundleDispatchFailHalfApplies(t *testing.T) {
	gov, ps, target, ctx := newEnactmentFixture(t)
	require.NoError(t, gov.Parameters.Register(ParameterBand{
		Name: "op_fee_transfer", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))
	// Configure the target to reject op_fee_transfer at dispatch.
	target.failOn["op_fee_transfer"] = fmt.Errorf("cross-param constraint violated")

	require.NoError(t, ps.SetProposal(ctx, passedProposalForChange(
		ProposalClassSimple7d, 100,
		[]store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
			{TargetModule: "fees", ParameterName: "op_fee_transfer", NewValueInt: 80},
		},
	)))

	_, _, err := gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 100,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)

	// Documented v1 limitation: first change applied; second rejected.
	assert.Equal(t, int64(50), target.applied["byte_fee"],
		"first change applied before the second's dispatch failed")
	_, has := target.applied["op_fee_transfer"]
	assert.False(t, has, "second change not applied")

	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusEnactmentFailed), p.Status)
}

// Compile-time satisfaction: the stub must implement the interface.
var _ runtime.BAPIModuleParams = (*stubParamModule)(nil)

// Avoid unused-import warnings.
var _ = ptypes.AccountName("")
var _ effects.Effect
