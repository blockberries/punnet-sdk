package mint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
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

// TestEndBlock_EmitsTransferFromVRP exercises the full EndBlock
// path: seed VRP, run EndBlock, verify the returned effect is a
// VRP → Emission Pool transfer of the expected amount.
func TestEndBlock_EmitsTransferFromVRP(t *testing.T) {
	const totalSupply uint64 = 1_000_000_000_000_000
	mod, bs, exec, ctx := newMintFixture(t, totalSupply)

	// Seed protocol accounts as the runtime would post-Phase 0.6.
	// VRP > V_threshold so taper = 1.
	require.NoError(t, bs.Set(ctx, VRPAccount, "stake", totalSupply/4))
	require.NoError(t, bs.Set(ctx, CTAccount, "stake", totalSupply*3/10))
	require.NoError(t, bs.Set(ctx, EcosystemAccount, "stake", totalSupply/10))
	require.NoError(t, bs.Set(ctx, BootstrapAccount, "stake", totalSupply/20))
	// Remaining (25% airdrop) sits in airdrop accounts; CS includes it.

	effs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: 1})
	require.NoError(t, err)
	require.Len(t, effs, 1, "EndBlock should emit exactly one transfer")

	tr, ok := effs[0].(effects.TransferEffect)
	require.True(t, ok, "effect must be TransferEffect")
	assert.Equal(t, ptypes.AccountName(VRPAccount), tr.From)
	assert.Equal(t, ptypes.AccountName(EmissionPoolAccount), tr.To)
	require.Len(t, tr.Amount, 1)
	assert.Greater(t, tr.Amount[0].Amount, uint64(0))

	// Execute the effect and confirm balances moved.
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	vrpPost, _ := bs.GetAmount(ctx, VRPAccount, "stake")
	emPost, _ := bs.GetAmount(ctx, EmissionPoolAccount, "stake")
	assert.Equal(t, totalSupply/4-tr.Amount[0].Amount, vrpPost)
	assert.Equal(t, tr.Amount[0].Amount, emPost)
}

// TestEndBlock_NoEmissionWhenVRPEmpty: the post-VRP-exhaustion
// endgame. EndBlock returns nil effects when VRP is zero (or below
// the integer-truncation floor).
func TestEndBlock_NoEmissionWhenVRPEmpty(t *testing.T) {
	mod, _, _, ctx := newMintFixture(t, 1_000_000_000_000_000)

	effs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: 1})
	require.NoError(t, err)
	assert.Empty(t, effs, "empty VRP → no emission effect")
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
	effs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: 1})
	require.NoError(t, err)
	if len(effs) == 0 {
		// At extreme tail, B_t may round to zero. Either zero or
		// saturated-to-100 is acceptable; both preserve invariants.
		return
	}
	require.Len(t, effs, 1)
	tr := effs[0].(effects.TransferEffect)
	assert.LessOrEqual(t, tr.Amount[0].Amount, uint64(100),
		"transfer must not exceed VRP balance")
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
