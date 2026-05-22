package distribution

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/modules/participation"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type slashPeriodFixture struct {
	dist *BAPIDistributionModule
	bs   *store.BAPIBalanceStore
	vs   *store.BAPIValidatorStore
	part *participation.BAPIParticipationModule
	exec *effects.BAPIExecutor
	ctx  context.Context
}

func newSlashPeriodFixture(t *testing.T) *slashPeriodFixture {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	part, err := participation.NewBAPIParticipationModule(ss)
	require.NoError(t, err)
	dist, err := NewBAPIDistributionModule(ss, provider.GetBalanceStore(), provider.GetValidatorStore(), part)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, dist.InitGenesis(ctx, nil))
	return &slashPeriodFixture{
		dist: dist,
		bs:   provider.GetBalanceStore(),
		vs:   provider.GetValidatorStore(),
		part: part,
		exec: effects.NewBAPIExecutor(provider),
		ctx:  ctx,
	}
}

// seedValidatorAndDelegator sets up a validator with `totalStake`,
// delegates alice's `aliceStake` to it, snapshots the delegation
// in distribution (mirroring what a proper delegate flow would
// do), and stakes the pool balance. Returns the validator pubkey.
func (f *slashPeriodFixture) seedValidatorAndDelegator(
	t *testing.T,
	totalStake, aliceStake uint64,
	commissionBps uint32,
) []byte {
	t.Helper()
	pubKey := make([]byte, 32)
	pubKey[0] = 0xaa
	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           totalStake,
		TotalDelegation: totalStake,
		Commission:      commissionBps,
	}))
	require.NoError(t, f.vs.Delegate(f.ctx, "alice", pubKey, aliceStake))
	// Reset TotalDelegation since Delegate doubled it.
	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	v.TotalDelegation = totalStake
	require.NoError(t, f.vs.SetValidator(f.ctx, v))
	// Snapshot the delegation in distribution (mirrors what a
	// proper staking→distribution hook at delegate time would do).
	require.NoError(t, f.dist.SnapshotDelegation(f.ctx, "alice", pubKey, aliceStake))
	return pubKey
}

// runEpoch credits the validator with `pool` micro-tokens at epoch
// close (single-validator scenario where alice gets it all).
func (f *slashPeriodFixture) runEpoch(t *testing.T, pubKey []byte, pool uint64, epochNum uint64) {
	t.Helper()
	require.NoError(t, f.bs.Set(f.ctx, EmissionPoolAccount, "stake", pool))
	require.NoError(t, f.part.OnBatchCertified(f.ctx, types.BatchCertifiedEvent{Validator: pubKeyToAddr20(pubKey)}))
	require.NoError(t, f.part.OnBlockConstructed(f.ctx, types.BlockConstructedEvent{
		Leader: pubKeyToAddr20(pubKey), IncludedBatchHashes: []types.Hash{{}},
	}))
	_, _, err := f.part.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks * epochNum})
	require.NoError(t, err)
	effs, _, err := f.dist.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks * epochNum})
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)
}

// applySlash runs the distribution-side period bump + records the
// slash event, then reduces alice's delegation in the validator
// store to mirror what staking's ProcessEvidence would do.
func (f *slashPeriodFixture) applySlash(t *testing.T, pubKey []byte, height, fractionBps uint64) {
	t.Helper()
	require.NoError(t, f.dist.RecordSlash(f.ctx, pubKey, height, fractionBps))
	// Mirror staking's stake reduction.
	d, _ := f.vs.GetDelegation(f.ctx, "alice", pubKey)
	d.Amount = d.Amount * (10000 - fractionBps) / 10000
	require.NoError(t, f.vs.SetDelegation(f.ctx, d))
	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	v.TotalDelegation = v.TotalDelegation * (10000 - fractionBps) / 10000
	v.Power = v.TotalDelegation
	require.NoError(t, f.vs.SetValidator(f.ctx, v))
}

// claim drives MsgWithdrawDelegatorReward and returns the
// rewards alice received.
func (f *slashPeriodFixture) claim(t *testing.T, pubKey []byte) uint64 {
	t.Helper()
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: staking.EpochBlocks * 10},
		Account:          "alice",
	}
	preBal, _ := f.bs.GetAmount(f.ctx, "alice", "stake")
	require.NoError(t, f.bs.Set(f.ctx, "module.distribution", "stake", 1_000_000_000))
	effs, err := f.dist.handleWithdrawDelegatorReward(f.ctx, txCtx, &MsgWithdrawDelegatorReward{
		Delegator: "alice", Validator: pubKey,
	})
	require.NoError(t, err)
	if len(effs) > 0 {
		_, err = f.exec.Execute(effs)
		require.NoError(t, err)
	}
	postBal, _ := f.bs.GetAmount(f.ctx, "alice", "stake")
	return postBal - preBal
}

// TestSlashPeriod_NoSlash sanity: with no slash, alice claims the
// full reward from her stake × CRR. Validates the SnapshotDelegation
// → epoch credit → claim happy path before adding slashes.
func TestSlashPeriod_NoSlash(t *testing.T) {
	f := newSlashPeriodFixture(t)
	pubKey := f.seedValidatorAndDelegator(t, 1000, 1000, 0)

	f.runEpoch(t, pubKey, 100, 1) // 100 reward at full stake = 100 to alice

	rewards := f.claim(t, pubKey)
	assert.Equal(t, uint64(100), rewards, "no-slash: full reward × full stake")
}

// TestSlashPeriod_PreSlashRewardsUndiminished is the F1
// correctness invariant: a delegator who delegated BEFORE a slash
// should receive their full pre-slash rewards. Without the
// snapshot machinery, the naive formula would apply post-slash
// stake to pre-slash rewards (underpay).
//
// Scenario:
//   - alice delegates 1000 at period 0
//   - Epoch 1: pool = 100. Alice's "earnings" at stake 1000 are 100.
//   - Slash 50% — alice's stake drops to 500. But she earned 100
//     BEFORE this. The walk should credit her with 100, not 50.
//   - Alice claims. Receives 100.
func TestSlashPeriod_PreSlashRewardsUndiminished(t *testing.T) {
	f := newSlashPeriodFixture(t)
	pubKey := f.seedValidatorAndDelegator(t, 1000, 1000, 0)

	f.runEpoch(t, pubKey, 100, 1)
	f.applySlash(t, pubKey, 100, 5000) // 50% slash

	rewards := f.claim(t, pubKey)
	assert.Equal(t, uint64(100), rewards,
		"pre-slash rewards must be credited at pre-slash stake (F1 invariant)")
}

// TestSlashPeriod_PostSlashRewardsAtReducedStake: an epoch
// AFTER the slash awards rewards at the post-slash stake.
//
// Scenario:
//   - alice delegates 1000
//   - Epoch 1: pool = 100, alice earns 100 at stake 1000
//   - Slash 50%, alice's stake = 500
//   - Epoch 2: pool = 50 (since validator power dropped). Alice
//     earns 50 at stake 500.
//   - Claim: 100 + 50 = 150
//
// The CRITICAL difference from a non-period-aware F1: a naive
// claim using current stake (500) × total CRR change would give
// (100/1000 + 50/500) × 500 = (0.1 + 0.1) × 500 = 100, underpaying
// alice by 50. The period-aware walk correctly attributes
// (100/1000) × 1000 + (50/500) × 500 = 100 + 50 = 150.
func TestSlashPeriod_PostSlashRewardsAtReducedStake(t *testing.T) {
	f := newSlashPeriodFixture(t)
	pubKey := f.seedValidatorAndDelegator(t, 1000, 1000, 0)

	f.runEpoch(t, pubKey, 100, 1)
	f.applySlash(t, pubKey, 100, 5000)
	f.runEpoch(t, pubKey, 50, 2)

	rewards := f.claim(t, pubKey)
	assert.Equal(t, uint64(150), rewards,
		"full pre-slash reward + reduced-stake post-slash reward (F1 algebra)")
}

// TestSlashPeriod_TwoConsecutiveSlashes: the walk handles
// multiple slash boundaries in sequence.
//
// Scenario:
//   - alice delegates 1000
//   - Epoch 1: pool = 100, alice earns 100 at stake 1000
//   - Slash 50%, stake = 500
//   - Epoch 2: pool = 50, alice earns 50 at stake 500
//   - Slash 50% again, stake = 250
//   - Epoch 3: pool = 25, alice earns 25 at stake 250
//   - Claim: 100 + 50 + 25 = 175
func TestSlashPeriod_TwoConsecutiveSlashes(t *testing.T) {
	f := newSlashPeriodFixture(t)
	pubKey := f.seedValidatorAndDelegator(t, 1000, 1000, 0)

	f.runEpoch(t, pubKey, 100, 1)
	f.applySlash(t, pubKey, 100, 5000)
	f.runEpoch(t, pubKey, 50, 2)
	f.applySlash(t, pubKey, 200, 5000)
	f.runEpoch(t, pubKey, 25, 3)

	rewards := f.claim(t, pubKey)
	assert.Equal(t, uint64(175), rewards,
		"three segments with two slash boundaries: 100 + 50 + 25")
}

// TestSlashPeriod_RecordSlashCreatesEvent: the slash event is
// persisted with the correct period number (the period that just
// ENDED) and fraction.
func TestSlashPeriod_RecordSlashCreatesEvent(t *testing.T) {
	f := newSlashPeriodFixture(t)
	pubKey := f.seedValidatorAndDelegator(t, 1000, 1000, 0)
	f.runEpoch(t, pubKey, 100, 1)

	require.NoError(t, f.dist.RecordSlash(f.ctx, pubKey, 100, 5000))

	// After RecordSlash, the dist record's CurrentPeriod should
	// have advanced past the snapshot.
	pubKeyHex := hex.EncodeToString(pubKey)
	d, err := f.dist.validatorDist.Get(f.ctx, keyValidatorPrefix+pubKeyHex)
	require.NoError(t, err)
	assert.Greater(t, d.CurrentPeriod, uint64(2), "RecordSlash bumped the period")

	// And a slash event exists at the just-ended period.
	var found bool
	require.NoError(t, f.dist.slashEvents.IterateRelative(func(relKey string, ev *ValidatorSlashEvent) bool {
		if ev != nil && ev.FractionBps == 5000 {
			found = true
			return true
		}
		return false
	}))
	assert.True(t, found, "slash event with 5000 bps recorded")
}

func pubKeyToAddr20(pubKey []byte) types.ValidatorAddress {
	var a types.ValidatorAddress
	copy(a[:], pubKey[:20])
	return a
}
