package integration

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/modules/fees"
	"github.com/blockberries/punnet-sdk/modules/governance"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGovernanceLifecycle_FeesThroughGovernance walks an end-to-end
// proposal through every Phase 4 surface, with the fees module as
// the target:
//
//  1. Wire fees + governance, register the byte_fee parameter
//     band, register fees as the BAPIModuleParams target for the
//     "fees" module name.
//  2. Skip the submission/voting machinery (that's covered by
//     handleSubmitProposal + handleVote unit tests) and write a
//     StatusPassed proposal directly with EffectiveHeight = N.
//  3. EndBlock at heights below N: no enactment.
//  4. EndBlock at height N: enactment dispatches; fees' byte_fee
//     reflects the change.
//
// Phase 4.7 acceptance: timelock not enacted before height; band
// rejection; bundled atomicity; half-applied cannot happen for
// the band-rejection path.
func TestGovernanceLifecycle_FeesThroughGovernance(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	ps := store.NewBAPIProposalStore(ss)

	feesMod, err := fees.NewBAPIFeesModule(ss)
	require.NoError(t, err)
	gov, err := governance.NewBAPIGovernanceModule(ps, provider.GetBalanceStore())
	require.NoError(t, err)

	require.NoError(t, feesMod.InitGenesis(ctx, nil))
	require.NoError(t, gov.InitGenesis(ctx, nil))

	// Register fees as a parameter target + register the byte_fee
	// band in the gov registry. Bands: soft 0–100, hard 0–1000.
	require.NoError(t, gov.RegisterParameterTarget("fees", feesMod))
	require.NoError(t, gov.Parameters.Register(governance.ParameterBand{
		Name: "byte_fee", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))

	// Helper: persist a fully-formed passed proposal.
	mkProposal := func(id uint64, class string, eff uint64, changes []store.ProposalChange) {
		require.NoError(t, ps.SetProposal(ctx, &store.BAPIProposal{
			ID:              id,
			Status:          string(governance.StatusPassed),
			Class:           class,
			EffectiveHeight: eff,
			Changes:         changes,
		}))
	}

	// --- Step 1: proposal not yet at EffectiveHeight ---
	mkProposal(1, governance.ProposalClassSimple7d, 1000, []store.ProposalChange{
		{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
	})
	_, _, err = gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 999,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)

	sched, _ := feesMod.Schedule(ctx)
	assert.Equal(t, uint64(0), sched.ByteFee,
		"timelock honoured: byte_fee unchanged before EffectiveHeight")

	// --- Step 2: at EffectiveHeight, enactment dispatches ---
	_, _, err = gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 1000,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	sched, _ = feesMod.Schedule(ctx)
	assert.Equal(t, uint64(50), sched.ByteFee, "byte_fee applied at EffectiveHeight")

	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(governance.StatusEnacted), p.Status)
}

// TestGovernanceLifecycle_BandRejection: a Simple7d proposal
// proposing a byte_fee outside the soft band (0–100) fails
// enactment without touching the schedule.
func TestGovernanceLifecycle_BandRejection(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	ps := store.NewBAPIProposalStore(ss)
	feesMod, err := fees.NewBAPIFeesModule(ss)
	require.NoError(t, err)
	gov, err := governance.NewBAPIGovernanceModule(ps, provider.GetBalanceStore())
	require.NoError(t, err)
	require.NoError(t, feesMod.InitGenesis(ctx, nil))
	require.NoError(t, gov.InitGenesis(ctx, nil))
	require.NoError(t, gov.RegisterParameterTarget("fees", feesMod))
	require.NoError(t, gov.Parameters.Register(governance.ParameterBand{
		Name: "byte_fee", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))

	// Out-of-soft Simple proposal.
	require.NoError(t, ps.SetProposal(ctx, &store.BAPIProposal{
		ID:              1,
		Status:          string(governance.StatusPassed),
		Class:           governance.ProposalClassSimple7d,
		EffectiveHeight: 1,
		Changes: []store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 500},
		},
	}))
	_, _, err = gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)

	sched, _ := feesMod.Schedule(ctx)
	assert.Equal(t, uint64(0), sched.ByteFee, "out-of-soft value rejected; schedule untouched")
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(governance.StatusEnactmentFailed), p.Status)
}

// TestGovernanceLifecycle_BundledAtomicity: a Simple7d bundle of
// two changes — one in band, one out — aborts pre-dispatch.
// Neither applies. Acceptance for "half-applied enactment cannot
// happen" via the band-validation path.
func TestGovernanceLifecycle_BundledAtomicity(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	ps := store.NewBAPIProposalStore(ss)
	feesMod, err := fees.NewBAPIFeesModule(ss)
	require.NoError(t, err)
	gov, err := governance.NewBAPIGovernanceModule(ps, provider.GetBalanceStore())
	require.NoError(t, err)
	require.NoError(t, feesMod.InitGenesis(ctx, nil))
	require.NoError(t, gov.InitGenesis(ctx, nil))
	require.NoError(t, gov.RegisterParameterTarget("fees", feesMod))
	require.NoError(t, gov.Parameters.Register(governance.ParameterBand{
		Name: "byte_fee", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))
	require.NoError(t, gov.Parameters.Register(governance.ParameterBand{
		Name: "op_fee:/bank.MsgSend", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))

	require.NoError(t, ps.SetProposal(ctx, &store.BAPIProposal{
		ID:              1,
		Status:          string(governance.StatusPassed),
		Class:           governance.ProposalClassSimple7d,
		EffectiveHeight: 1,
		Changes: []store.ProposalChange{
			// In band.
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 50},
			// OUT of soft (max 100).
			{TargetModule: "fees", ParameterName: "op_fee:/bank.MsgSend", NewValueInt: 500},
		},
	}))
	_, _, err = gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)

	sched, _ := feesMod.Schedule(ctx)
	assert.Equal(t, uint64(0), sched.ByteFee, "first change NOT applied — bundle pre-validation aborted")
	assert.Empty(t, sched.OpFees, "second change NOT applied")
	p, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(governance.StatusEnactmentFailed), p.Status)
}

// TestGovernanceLifecycle_ConstitutionalCanExitHardBand: a
// constitutional proposal can push byte_fee beyond the hard band.
// (Spec §11: constitutional changes are the only path that can
// exit the safety bands.)
func TestGovernanceLifecycle_ConstitutionalCanExitHardBand(t *testing.T) {
	ctx := context.Background()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	ps := store.NewBAPIProposalStore(ss)
	feesMod, err := fees.NewBAPIFeesModule(ss)
	require.NoError(t, err)
	gov, err := governance.NewBAPIGovernanceModule(ps, provider.GetBalanceStore())
	require.NoError(t, err)
	require.NoError(t, feesMod.InitGenesis(ctx, nil))
	require.NoError(t, gov.InitGenesis(ctx, nil))
	require.NoError(t, gov.RegisterParameterTarget("fees", feesMod))
	require.NoError(t, gov.Parameters.Register(governance.ParameterBand{
		Name: "byte_fee", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 1000,
	}))

	require.NoError(t, ps.SetProposal(ctx, &store.BAPIProposal{
		ID:              1,
		Status:          string(governance.StatusPassed),
		Class:           governance.ProposalClassConstitutional,
		EffectiveHeight: 1,
		Changes: []store.ProposalChange{
			{TargetModule: "fees", ParameterName: "byte_fee", NewValueInt: 5000}, // way above hard 1000
		},
	}))
	_, _, err = gov.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 1,
		Time:   types.TimeToTimestamp(time.Now()),
	})
	require.NoError(t, err)
	sched, _ := feesMod.Schedule(ctx)
	assert.Equal(t, uint64(5000), sched.ByteFee, "constitutional may exit hard band")
}
