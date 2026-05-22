package staking

import (
	"testing"

	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeVestedAmount covers the per-height vesting math:
// flat 0 before vest start; linear over BootstrapVestBlocks; clamps
// at LockedAmount past the end. PLAN §7 Phase 2.5.
func TestComputeVestedAmount(t *testing.T) {
	bi := &store.BAPIBootstrapInfo{
		LockedAmount:    1_000_000,
		VestStartHeight: 10_000,
	}

	t.Run("pre-vest", func(t *testing.T) {
		assert.Equal(t, uint64(0), computeVestedAmount(bi, 1))
		assert.Equal(t, uint64(0), computeVestedAmount(bi, 9_999))
	})

	t.Run("at vest start", func(t *testing.T) {
		assert.Equal(t, uint64(0), computeVestedAmount(bi, 10_000),
			"elapsed=0 means vested=0 still")
	})

	t.Run("quarter through", func(t *testing.T) {
		h := bi.VestStartHeight + runtime.BootstrapVestBlocks/4
		got := computeVestedAmount(bi, h)
		assert.InDelta(t, uint64(250_000), got, 2,
			"at quarter vest, expect 25%% of LockedAmount; got %d", got)
	})

	t.Run("halfway", func(t *testing.T) {
		h := bi.VestStartHeight + runtime.BootstrapVestBlocks/2
		got := computeVestedAmount(bi, h)
		assert.InDelta(t, uint64(500_000), got, 2)
	})

	t.Run("at vest end", func(t *testing.T) {
		h := bi.VestStartHeight + runtime.BootstrapVestBlocks
		assert.Equal(t, bi.LockedAmount, computeVestedAmount(bi, h))
	})

	t.Run("past vest end clamps at LockedAmount", func(t *testing.T) {
		h := bi.VestStartHeight + runtime.BootstrapVestBlocks + 100_000
		assert.Equal(t, bi.LockedAmount, computeVestedAmount(bi, h))
	})

	t.Run("zero locked amount is always vested-zero", func(t *testing.T) {
		empty := &store.BAPIBootstrapInfo{LockedAmount: 0, VestStartHeight: 100}
		assert.Equal(t, uint64(0), computeVestedAmount(empty, 1_000_000))
	})

	t.Run("nil info", func(t *testing.T) {
		assert.Equal(t, uint64(0), computeVestedAmount(nil, 1_000_000))
	})
}

// TestAdvanceBootstrapVesting_PersistsAndIsIdempotent verifies the
// EndBlock-side bookkeeping: VestedAmount catches up to the
// height-derived vested amount, and a second pass at the same
// height is a no-op.
func TestAdvanceBootstrapVesting_PersistsAndIsIdempotent(t *testing.T) {
	mod, vs, _, _, ctx := newBootstrapFixture(t)

	pubKey := make([]byte, 32)
	pubKey[0] = 0xc1
	bv := runtime.BootstrapValidator{Name: "vester", PubKey: pubKey}
	const (
		perVal   uint64 = 2_000_000
		vestStart uint64 = 1_000
	)
	require.NoError(t, mod.SeedBootstrapValidators(ctx, []runtime.BootstrapValidator{bv}, perVal, vestStart))

	halfway := vestStart + runtime.BootstrapVestBlocks/2
	require.NoError(t, mod.advanceBootstrapVesting(ctx, halfway))

	bi, err := vs.GetBootstrapInfo(ctx, pubKey)
	require.NoError(t, err)
	require.NotNil(t, bi)
	assert.InDelta(t, uint64(perVal/2), bi.VestedAmount, 2,
		"halfway: VestedAmount ≈ LockedAmount / 2")

	// Idempotent: running again at the same height doesn't change
	// anything (no-op set).
	before := bi.VestedAmount
	require.NoError(t, mod.advanceBootstrapVesting(ctx, halfway))
	bi2, _ := vs.GetBootstrapInfo(ctx, pubKey)
	assert.Equal(t, before, bi2.VestedAmount)

	// Past vest end → fully vested.
	require.NoError(t, mod.advanceBootstrapVesting(ctx, vestStart+runtime.BootstrapVestBlocks))
	bi3, _ := vs.GetBootstrapInfo(ctx, pubKey)
	assert.Equal(t, perVal, bi3.VestedAmount, "post-vest VestedAmount == LockedAmount")
}

// TestBootstrap_SeedTransfersFromModuleBL verifies the
// genesis-time bookkeeping move: SeedBootstrapValidators debits
// module.bl by perValidatorShare and credits staking.pool by the
// same amount, so the self-delegation is actually backed.
func TestBootstrap_SeedTransfersFromModuleBL(t *testing.T) {
	mod, _, bs, _, ctx := newBootstrapFixture(t)

	pubKey := make([]byte, 32)
	pubKey[0] = 0xc2
	bv := runtime.BootstrapValidator{Name: "transferer", PubKey: pubKey}
	const perVal uint64 = 750_000

	bBefore, _ := bs.GetAmount(ctx, "module.bl", "stake")
	require.NoError(t, mod.SeedBootstrapValidators(ctx, []runtime.BootstrapValidator{bv}, perVal, 1))
	bAfter, _ := bs.GetAmount(ctx, "module.bl", "stake")
	poolAfter, _ := bs.GetAmount(ctx, "staking.pool", "stake")

	assert.Equal(t, bBefore-perVal, bAfter, "module.bl debited by perValidatorShare")
	assert.Equal(t, perVal, poolAfter, "staking.pool credited by perValidatorShare")
}
