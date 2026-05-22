package staking

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBootstrapFixture(t *testing.T) (*BAPIStakingModule, *store.BAPIValidatorStore, *store.BAPIBalanceStore, *effects.BAPIExecutor, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	mod, err := NewBAPIStakingModule(provider.GetValidatorStore(), provider.GetBalanceStore())
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	// Seed module.bl with a generous balance so SeedBootstrapValidators
	// can debit the per-validator share. In production this is done by
	// the runtime's protocol-account seeding step (Phase 0.6); tests
	// bypass that and call SeedBootstrapValidators directly.
	ctx := context.Background()
	require.NoError(t, provider.GetBalanceStore().Set(ctx, "module.bl", "stake", 1_000_000_000_000))
	return mod, provider.GetValidatorStore(), provider.GetBalanceStore(), exec, ctx
}

// TestSeedBootstrapValidators_CreatesAllState exercises the runtime
// hook end-to-end against the staking module: every bootstrap
// validator gets a validator row (commission pinned at 500 bps), a
// self-delegation, an active-set entry, and a BootstrapInfo row with
// the expected vest start.
func TestSeedBootstrapValidators_CreatesAllState(t *testing.T) {
	mod, vs, _, _, ctx := newBootstrapFixture(t)

	bvs := []runtime.BootstrapValidator{
		{Name: ptypes.AccountName("genesis1"), PubKey: make([]byte, 32)},
		{Name: ptypes.AccountName("genesis2"), PubKey: make([]byte, 32)},
	}
	// Make pubkeys distinct.
	bvs[0].PubKey[0] = 0xa1
	bvs[1].PubKey[0] = 0xa2

	const (
		perVal     uint64 = 12_500_000_000
		vestStart  uint64 = 1_000_000
	)

	require.NoError(t, mod.SeedBootstrapValidators(ctx, bvs, perVal, vestStart))

	for _, bv := range bvs {
		v, err := vs.GetValidator(ctx, bv.PubKey)
		require.NoError(t, err)
		require.NotNil(t, v, "validator missing for %s", bv.Name)
		assert.Equal(t, perVal, v.Power, "bootstrap power = perValidatorShare")
		assert.Equal(t, perVal, v.TotalDelegation)
		assert.Equal(t, uint32(500), v.Commission, "bootstrap commission pinned at 5%")
		assert.False(t, v.Jailed)

		d, err := vs.GetDelegation(ctx, string(bv.Name), bv.PubKey)
		require.NoError(t, err)
		assert.Equal(t, perVal, d.Amount, "self-delegation = perValidatorShare")
		assert.Equal(t, string(bv.Name), d.Delegator)
		assert.Equal(t, hex.EncodeToString(bv.PubKey), d.ValidatorPubKey)

		bi, err := vs.GetBootstrapInfo(ctx, bv.PubKey)
		require.NoError(t, err)
		require.NotNil(t, bi)
		assert.Equal(t, perVal, bi.LockedAmount)
		assert.Equal(t, vestStart, bi.VestStartHeight)
		assert.Equal(t, uint64(0), bi.VestedAmount, "no vest progress at seed time")
	}

	// Active set must contain both validators (they're admitted as
	// the initial baseline; subsequent epoch closes diff against
	// this).
	set, err := vs.GetActiveSet()
	require.NoError(t, err)
	require.Len(t, set, 2)
}

// TestBootstrap_SelfUndelegatePreVestRejected pins Phase 2.5's
// linear-vest gating: a bootstrap validator's self-undelegate
// cannot bring the self-stake below the unvested portion. At
// VestStartHeight the vested amount is still 0; at
// VestStartHeight + 30d/2 it's half; at VestStartHeight + 30d
// it's full.
func TestBootstrap_SelfUndelegatePreVestRejected(t *testing.T) {
	mod, _, _, _, ctx := newBootstrapFixture(t)

	pubKey := make([]byte, 32)
	pubKey[0] = 0xaa
	bv := runtime.BootstrapValidator{Name: ptypes.AccountName("bootstrap1"), PubKey: pubKey}
	const perVal uint64 = 1_000_000
	const vestStart uint64 = 5_000

	require.NoError(t, mod.SeedBootstrapValidators(ctx, []runtime.BootstrapValidator{bv}, perVal, vestStart))

	tryUndelegate := func(height, amount uint64) error {
		_, err := mod.handleUndelegate(ctx, &runtime.BAPITxContext{
			BAPIBlockContext: &runtime.BAPIBlockContext{Height: height},
			Account:          bv.Name,
		}, &MsgUndelegate{
			Delegator: bv.Name,
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: amount},
		})
		return err
	}

	// Before vest start: vested = 0, no amount is permitted.
	err := tryUndelegate(100, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vested at height")

	// At vest start: vested = 0 still (elapsed = 0).
	err = tryUndelegate(vestStart, 1)
	require.Error(t, err)

	// Halfway through vesting: vested ≈ LockedAmount / 2. We can
	// undelegate up to half.
	halfwayHeight := vestStart + runtime.BootstrapVestBlocks/2
	err = tryUndelegate(halfwayHeight, perVal/2-1) // safely under half
	require.NoError(t, err, "halfway through vest, half of locked is unlocked")

	// Re-seed and try to over-undelegate at halfway.
	mod2, _, _, _, ctx2 := newBootstrapFixture(t)
	require.NoError(t, mod2.SeedBootstrapValidators(ctx2, []runtime.BootstrapValidator{bv}, perVal, vestStart))
	_, err = mod2.handleUndelegate(ctx2, &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: halfwayHeight},
		Account:          bv.Name,
	}, &MsgUndelegate{
		Delegator: bv.Name,
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: perVal/2 + 1},
	})
	require.Error(t, err, "can't undelegate more than the vested portion at halfway")

	// At/after vest end: full amount can be undelegated.
	mod3, _, _, _, ctx3 := newBootstrapFixture(t)
	require.NoError(t, mod3.SeedBootstrapValidators(ctx3, []runtime.BootstrapValidator{bv}, perVal, vestStart))
	require.NoError(t, tryUndelegate2(t, mod3, ctx3, bv, pubKey, vestStart+runtime.BootstrapVestBlocks, perVal))
}

// tryUndelegate2 is a small helper so the mod3 leg of the test
// above stays readable.
func tryUndelegate2(t *testing.T, mod *BAPIStakingModule, ctx context.Context, bv runtime.BootstrapValidator, pubKey []byte, height, amount uint64) error {
	t.Helper()
	_, err := mod.handleUndelegate(ctx, &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: height},
		Account:          bv.Name,
	}, &MsgUndelegate{
		Delegator: bv.Name,
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: amount},
	})
	return err
}

// TestBootstrap_NonSelfDelegatorUnaffected: a third-party delegator
// to a bootstrap validator can undelegate freely (subject to the
// regular 21-day unbonding period, but not the bootstrap lockup).
func TestBootstrap_NonSelfDelegatorUnaffected(t *testing.T) {
	mod, vs, bs, exec, ctx := newBootstrapFixture(t)

	pubKey := make([]byte, 32)
	pubKey[0] = 0xbb
	bv := runtime.BootstrapValidator{Name: ptypes.AccountName("bootstrap2"), PubKey: pubKey}
	const perVal uint64 = 1_000_000

	require.NoError(t, mod.SeedBootstrapValidators(ctx, []runtime.BootstrapValidator{bv}, perVal, 5_000))
	require.NoError(t, bs.Set(ctx, "staking.pool", "stake", perVal*2))

	// A third party delegates 500 to the bootstrap validator.
	require.NoError(t, vs.Delegate(ctx, "alice", pubKey, 500))

	// Alice undelegates 200 well before the bootstrap vest start.
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: 100},
		Account:          ptypes.AccountName("alice"),
	}
	effs, err := mod.handleUndelegate(ctx, txCtx, &MsgUndelegate{
		Delegator: ptypes.AccountName("alice"),
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: 200},
	})
	require.NoError(t, err, "third-party delegators to a bootstrap validator must be able to undelegate freely")
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	// The delegation row should now reflect alice's reduced amount.
	d, err := vs.GetDelegation(ctx, "alice", pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(300), d.Amount)
}

// TestSeedBootstrapValidators_RejectsBadInput covers the input
// validation: invalid AccountName, wrong-length pubkey.
func TestSeedBootstrapValidators_RejectsBadInput(t *testing.T) {
	mod, _, _, _, ctx := newBootstrapFixture(t)

	t.Run("invalid name", func(t *testing.T) {
		err := mod.SeedBootstrapValidators(ctx, []runtime.BootstrapValidator{
			{Name: ptypes.AccountName("BAD-name!"), PubKey: make([]byte, 32)},
		}, 100, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bootstrap validator name")
	})

	t.Run("short pubkey", func(t *testing.T) {
		err := mod.SeedBootstrapValidators(ctx, []runtime.BootstrapValidator{
			{Name: ptypes.AccountName("good"), PubKey: make([]byte, 16)},
		}, 100, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pubkey length")
	})
}
