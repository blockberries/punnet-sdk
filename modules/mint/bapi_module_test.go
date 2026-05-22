package mint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMintFixture(t *testing.T, totalSupply uint64) (*BAPIMintModule, *store.BAPIBalanceStore, *effects.BAPIExecutor, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	mod, err := NewBAPIMintModule(provider.GetBalanceStore())
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()
	require.NoError(t, mod.ConsumeTokenomics(ctx, runtime.TokenomicsParams{TotalSupply: totalSupply}))
	require.NoError(t, mod.InitGenesis(ctx, nil))
	return mod, provider.GetBalanceStore(), exec, ctx
}

// TestComputeEmission_SpecExample pins the spec §4.1 example to
// concrete numbers. With CS = 50% × S (the steady-state value cited
// in the spec) and VRP > V_threshold (so taper = 1), the per-block
// emission B_t should be approximately ρ × CS_micro.
func TestComputeEmission_SpecExample(t *testing.T) {
	const (
		totalSupply uint64 = 1_000_000_000_000_000 // 10^15 micro-tokens = 1B tokens
		csMicro            = totalSupply / 2       // 50% of S
		vrpMicro           = totalSupply / 4       // 25% — above V_threshold (5%)
		vTh                = totalSupply / 20      // 5%
	)
	b := computeEmission(DefaultRhoScaled, csMicro, vrpMicro, vTh)

	// Expected: 1.057e-9 × 5e14 ≈ 5.285e5 micro-tokens = 0.5285 tokens/block
	// (≈ 1.66M tokens/year at 31.5M blocks)
	require.InDelta(t, 528_500, b, 1000, "B_t ≈ 0.5285 tokens at 50%% CS")
}

// TestComputeEmission_TaperKicksIn verifies that as VRP drops below
// V_threshold, B_t scales down linearly. At VRP = V_threshold/2,
// B_t should be half what it was at VRP ≥ V_threshold.
func TestComputeEmission_TaperKicksIn(t *testing.T) {
	const (
		totalSupply uint64 = 1_000_000_000_000_000
		csMicro            = totalSupply / 2
		vTh                = totalSupply / 20
	)
	full := computeEmission(DefaultRhoScaled, csMicro, vTh, vTh)
	half := computeEmission(DefaultRhoScaled, csMicro, vTh/2, vTh)
	assert.InDelta(t, float64(full/2), float64(half), float64(full)/100,
		"VRP at half V_threshold should yield ~half emission; got full=%d half=%d", full, half)
}

// TestComputeEmission_EmptyVRPZeroes: when VRP is empty (the chain
// has emitted everything), B_t must be zero — no negative emission,
// no underflow.
func TestComputeEmission_EmptyVRPZeroes(t *testing.T) {
	got := computeEmission(DefaultRhoScaled, 1_000_000_000_000_000/2, 0, 50_000_000_000_000)
	assert.Equal(t, uint64(0), got)
}

// TestComputeEmission_ZeroSupplyZeroes: with CS = 0 the formula's
// product collapses to zero (no circulating supply means nothing
// to drive emission against). Edge case.
func TestComputeEmission_ZeroSupplyZeroes(t *testing.T) {
	assert.Equal(t, uint64(0),
		computeEmission(DefaultRhoScaled, 0, 1_000_000, 1_000_000))
}

// TestEndBlock_TransfersFromVRPDirectly exercises the EndBlock
// path: seed VRP, run EndBlock, verify the balance moved from
// VRP to the Emission Pool. The transfer is applied directly
// (not returned as an effect) so distribution's same-block read
// sees post-emission state. PLAN §7 Phase 3.7.
func TestEndBlock_TransfersFromVRPDirectly(t *testing.T) {
	const totalSupply uint64 = 1_000_000_000_000_000
	mod, bs, _, ctx := newMintFixture(t, totalSupply)

	// Seed protocol accounts as the runtime would post-Phase 0.6.
	// VRP > V_threshold so taper = 1.
	require.NoError(t, bs.Set(ctx, VRPAccount, "stake", totalSupply/4))
	require.NoError(t, bs.Set(ctx, CTAccount, "stake", totalSupply*3/10))
	require.NoError(t, bs.Set(ctx, EcosystemAccount, "stake", totalSupply/10))
	require.NoError(t, bs.Set(ctx, BootstrapAccount, "stake", totalSupply/20))

	effs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: 1})
	require.NoError(t, err)
	assert.Empty(t, effs, "mint applies the transfer directly; no effects returned")

	vrpPost, _ := bs.GetAmount(ctx, VRPAccount, "stake")
	emPost, _ := bs.GetAmount(ctx, EmissionPoolAccount, "stake")
	assert.Less(t, vrpPost, totalSupply/4, "VRP should have been debited")
	assert.Greater(t, emPost, uint64(0), "Emission Pool should have been credited")
	assert.Equal(t, totalSupply/4-emPost, vrpPost,
		"VRP debit equals Emission Pool credit (supply conservation)")
}

// TestEndBlock_NoEmissionWhenVRPEmpty: the post-VRP-exhaustion
// endgame. EndBlock returns cleanly without transferring when VRP
// is zero (or below the integer-truncation floor).
func TestEndBlock_NoEmissionWhenVRPEmpty(t *testing.T) {
	mod, bs, _, ctx := newMintFixture(t, 1_000_000_000_000_000)

	effs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: 1})
	require.NoError(t, err)
	assert.Empty(t, effs)
	emPost, _ := bs.GetAmount(ctx, EmissionPoolAccount, "stake")
	assert.Equal(t, uint64(0), emPost, "empty VRP → no credit")
}

// TestEndBlock_SaturatesToVRP: when the computed B_t would exceed
// the remaining VRP balance (a numerical edge near exhaustion),
// the transfer saturates to VRP itself.
func TestEndBlock_SaturatesToVRP(t *testing.T) {
	const totalSupply uint64 = 1_000_000_000_000_000
	mod, bs, _, ctx := newMintFixture(t, totalSupply)

	// VRP very small, well below where the taper goes to zero —
	// at this scale B_t computed naively might be larger than VRP.
	require.NoError(t, bs.Set(ctx, VRPAccount, "stake", 100))
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: 1})
	require.NoError(t, err)
	emPost, _ := bs.GetAmount(ctx, EmissionPoolAccount, "stake")
	assert.LessOrEqual(t, emPost, uint64(100), "emission must not exceed VRP balance")
}

// TestGenesisRoundTrip pins the InitGenesis/ExportGenesis pair:
// initial state matches defaults, ExportGenesis round-trips.
func TestGenesisRoundTrip(t *testing.T) {
	const totalSupply uint64 = 1_000_000_000_000_000
	mod, _, _, ctx := newMintFixture(t, totalSupply)

	exported, err := mod.ExportGenesis(ctx)
	require.NoError(t, err)
	var p MintParams
	require.NoError(t, json.Unmarshal(exported, &p))
	assert.Equal(t, DefaultRhoScaled, p.RhoScaled)
	assert.Equal(t, totalSupply*VThresholdFractionBps/10000, p.VThresholdMicro,
		"V_threshold = 5%% of TotalSupply")
}

// TestConsumeTokenomicsCapturesTotalSupply pins the runtime hook:
// after ConsumeTokenomics, V_threshold equals 5% × TotalSupply.
func TestConsumeTokenomicsCapturesTotalSupply(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	mod, err := NewBAPIMintModule(provider.GetBalanceStore())
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, mod.ConsumeTokenomics(ctx, runtime.TokenomicsParams{TotalSupply: 2_000_000_000}))
	require.NoError(t, mod.InitGenesis(ctx, nil))

	params, err := mod.paramsStore.Get(ctx, keyParams)
	require.NoError(t, err)
	assert.Equal(t, uint64(2_000_000_000*500/10000), params.VThresholdMicro)
}
