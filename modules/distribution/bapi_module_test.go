package distribution

import (
	"context"
	"encoding/hex"
	"math/big"
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

type distFixture struct {
	mod  *BAPIDistributionModule
	bs   *store.BAPIBalanceStore
	vs   *store.BAPIValidatorStore
	part *participation.BAPIParticipationModule
	exec *effects.BAPIExecutor
	ctx  context.Context
}

func newDistFixture(t *testing.T) *distFixture {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	part, err := participation.NewBAPIParticipationModule(ss)
	require.NoError(t, err)
	mod, err := NewBAPIDistributionModule(ss, provider.GetBalanceStore(), provider.GetValidatorStore(), part)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, mod.InitGenesis(ctx, nil))
	return &distFixture{
		mod:  mod,
		bs:   provider.GetBalanceStore(),
		vs:   provider.GetValidatorStore(),
		part: part,
		exec: effects.NewBAPIExecutor(provider),
		ctx:  ctx,
	}
}

func seedValidator(t *testing.T, vs *store.BAPIValidatorStore, ctx context.Context, pubKey []byte, totalStake uint64, commissionBps uint32) {
	t.Helper()
	require.NoError(t, vs.SetValidator(ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           totalStake,
		TotalDelegation: totalStake,
		Commission:      commissionBps,
	}))
}

// TestEndBlock_NoOpMidEpoch: distribution does nothing between
// epoch boundaries.
func TestEndBlock_NoOpMidEpoch(t *testing.T) {
	f := newDistFixture(t)
	effs, _, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: 100})
	require.NoError(t, err)
	assert.Empty(t, effs)
}

// TestEndBlock_CreditsValidatorOnEpochClose: at epoch close, a
// validator with participation gets R_v advanced and outstanding
// commission accrued. The pool drain transfers tokens from
// EmissionPool to module.distribution.
func TestEndBlock_CreditsValidatorOnEpochClose(t *testing.T) {
	f := newDistFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xa1
	const (
		stake         uint64 = 1_000_000
		emissionPool  uint64 = 100_000
		priorityPool  uint64 = 50_000
		commissionBps uint32 = 1000 // 10%
	)
	seedValidator(t, f.vs, f.ctx, pubKey, stake, commissionBps)
	require.NoError(t, f.bs.Set(f.ctx, EmissionPoolAccount, "stake", emissionPool))
	require.NoError(t, f.bs.Set(f.ctx, PriorityPoolAccount, "stake", priorityPool))

	// Seed participation: this single validator gets all the share.
	require.NoError(t, f.part.OnBatchCertified(f.ctx, types.BatchCertifiedEvent{
		Validator: pubKeyToAddr(pubKey),
	}))
	require.NoError(t, f.part.OnBlockConstructed(f.ctx, types.BlockConstructedEvent{
		Leader:              pubKeyToAddr(pubKey),
		IncludedBatchHashes: []types.Hash{{}},
	}))
	_, _, err := f.part.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)

	// Now drive distribution's EndBlock at the same height.
	effs, _, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)
	require.NotEmpty(t, effs, "epoch close must emit pool drains")
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	// Pool tokens moved to module.distribution.
	distBal, _ := f.bs.GetAmount(f.ctx, distributionAccount, "stake")
	assert.Equal(t, emissionPool+priorityPool, distBal,
		"all pool tokens moved to module.distribution at epoch close")

	// Validator distribution record has commission + non-zero R_v.
	vd, err := f.mod.validatorDist.Get(f.ctx, keyValidatorPrefix+hex.EncodeToString(pubKey))
	require.NoError(t, err)
	require.NotNil(t, vd)
	expectedCommission := (emissionPool + priorityPool) * uint64(commissionBps) / 10000
	assert.Equal(t, expectedCommission, vd.OutstandingCommissionMicro)
	require.NotEmpty(t, vd.RewardPerShareScaled, "R_v should be advanced")
}

// TestClaimFlow_BasicHappyPath: an end-to-end "delegate → epoch
// passes → claim" scenario. The delegator receives stake × R_v.
func TestClaimFlow_BasicHappyPath(t *testing.T) {
	f := newDistFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xb1
	const (
		stake        uint64 = 1_000_000
		emissionPool uint64 = 100_000
		priorityPool uint64 = 0
		commissionBps uint32 = 0 // no commission for simpler math
	)
	seedValidator(t, f.vs, f.ctx, pubKey, stake, commissionBps)
	require.NoError(t, f.vs.Delegate(f.ctx, "alice", pubKey, stake))
	// Reset TotalDelegation after the Delegate doubled it.
	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	v.TotalDelegation = stake
	require.NoError(t, f.vs.SetValidator(f.ctx, v))

	require.NoError(t, f.bs.Set(f.ctx, EmissionPoolAccount, "stake", emissionPool))

	// All participation goes to this validator.
	require.NoError(t, f.part.OnBatchCertified(f.ctx, types.BatchCertifiedEvent{Validator: pubKeyToAddr(pubKey)}))
	require.NoError(t, f.part.OnBlockConstructed(f.ctx, types.BlockConstructedEvent{
		Leader: pubKeyToAddr(pubKey), IncludedBatchHashes: []types.Hash{{}},
	}))
	_, _, err := f.part.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)

	effs, _, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	// Alice claims.
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: staking.EpochBlocks + 1},
		Account:          "alice",
	}
	claimEffs, err := f.mod.handleWithdrawDelegatorReward(f.ctx, txCtx, &MsgWithdrawDelegatorReward{
		Delegator: "alice",
		Validator: pubKey,
	})
	require.NoError(t, err)
	require.NotEmpty(t, claimEffs)
	_, err = f.exec.Execute(claimEffs)
	require.NoError(t, err)

	aliceBal, _ := f.bs.GetAmount(f.ctx, "alice", "stake")
	// With zero commission, alice's full stake (= total stake = sole
	// delegator) earns the entire pool. Expected: emissionPool.
	assert.Equal(t, emissionPool, aliceBal, "single delegator with no commission gets the full pool")
}

// TestClaimFlow_TwoEpochsCompound verifies multi-epoch accumulation:
// two consecutive epochs both credit the validator; the delegator's
// claim picks up both.
func TestClaimFlow_TwoEpochsCompound(t *testing.T) {
	f := newDistFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xc1
	const (
		stake             uint64 = 1_000_000
		emissionPerEpoch  uint64 = 100_000
	)
	seedValidator(t, f.vs, f.ctx, pubKey, stake, 0)
	require.NoError(t, f.vs.Delegate(f.ctx, "alice", pubKey, stake))
	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	v.TotalDelegation = stake
	require.NoError(t, f.vs.SetValidator(f.ctx, v))

	runEpoch := func(epoch uint64) {
		require.NoError(t, f.bs.Set(f.ctx, EmissionPoolAccount, "stake", emissionPerEpoch))
		require.NoError(t, f.part.OnBatchCertified(f.ctx, types.BatchCertifiedEvent{Validator: pubKeyToAddr(pubKey)}))
		require.NoError(t, f.part.OnBlockConstructed(f.ctx, types.BlockConstructedEvent{
			Leader: pubKeyToAddr(pubKey), IncludedBatchHashes: []types.Hash{{}},
		}))
		_, _, err := f.part.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks * epoch})
		require.NoError(t, err)
		effs, _, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks * epoch})
		require.NoError(t, err)
		_, err = f.exec.Execute(effs)
		require.NoError(t, err)
	}
	runEpoch(1)
	runEpoch(2)

	// Claim should pick up two epochs' worth.
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: staking.EpochBlocks*2 + 1},
		Account:          "alice",
	}
	effs, err := f.mod.handleWithdrawDelegatorReward(f.ctx, txCtx, &MsgWithdrawDelegatorReward{
		Delegator: "alice", Validator: pubKey,
	})
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	aliceBal, _ := f.bs.GetAmount(f.ctx, "alice", "stake")
	assert.Equal(t, emissionPerEpoch*2, aliceBal,
		"two epochs accumulate; full pool over both")
}

// TestClaim_LazyLeader_NoReward: a validator with zero participation
// (no leader blocks, no batches) gets no R_v advance; their
// delegator's claim returns nothing.
func TestClaim_LazyLeader_NoReward(t *testing.T) {
	f := newDistFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xd1
	seedValidator(t, f.vs, f.ctx, pubKey, 1_000_000, 0)
	require.NoError(t, f.vs.Delegate(f.ctx, "alice", pubKey, 1_000_000))
	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	v.TotalDelegation = 1_000_000
	require.NoError(t, f.vs.SetValidator(f.ctx, v))

	// Some OTHER validator does the work — totals are non-zero
	// (so epoch credit runs) but our validator has no
	// participation entry.
	otherPub := make([]byte, 32)
	otherPub[0] = 0xd2
	require.NoError(t, f.part.OnBatchCertified(f.ctx, types.BatchCertifiedEvent{Validator: pubKeyToAddr(otherPub)}))
	_, _, err := f.part.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)
	require.NoError(t, f.bs.Set(f.ctx, EmissionPoolAccount, "stake", 100_000))
	_, _, err = f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)

	// Alice's claim returns nothing (no R_v advance for her validator).
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: staking.EpochBlocks + 1},
		Account:          "alice",
	}
	_, err = f.mod.handleWithdrawDelegatorReward(f.ctx, txCtx, &MsgWithdrawDelegatorReward{
		Delegator: "alice", Validator: pubKey,
	})
	// Validator distribution record doesn't exist for the lazy
	// validator; the handler returns an error in that case.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestWithdrawValidatorCommission: operator claims accrued commission.
func TestWithdrawValidatorCommission(t *testing.T) {
	f := newDistFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xe1
	const commissionBps uint32 = 1000 // 10%
	seedValidator(t, f.vs, f.ctx, pubKey, 1_000_000, commissionBps)
	require.NoError(t, f.bs.Set(f.ctx, EmissionPoolAccount, "stake", 100_000))

	require.NoError(t, f.part.OnBlockConstructed(f.ctx, types.BlockConstructedEvent{
		Leader: pubKeyToAddr(pubKey), IncludedBatchHashes: []types.Hash{{}},
	}))
	_, _, err := f.part.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)
	effs, _, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	// Operator claims.
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: staking.EpochBlocks + 1},
		Account:          "operator",
	}
	cEffs, err := f.mod.handleWithdrawValidatorCommission(f.ctx, txCtx, &MsgWithdrawValidatorCommission{
		Operator: "operator", Validator: pubKey,
	})
	require.NoError(t, err)
	require.NotEmpty(t, cEffs)
	_, err = f.exec.Execute(cEffs)
	require.NoError(t, err)

	opBal, _ := f.bs.GetAmount(f.ctx, "operator", "stake")
	// share(v) = α × 1.0 + (1−α) × 0 = 0.3 (since no batches at all).
	// pool × share = 30_000; commission = 30_000 × 10% = 3_000.
	assert.Equal(t, uint64(3_000), opBal,
		"operator gets share × commission of pool (α=0.3 default, leader-only)")

	// Second claim is zero.
	cEffs, err = f.mod.handleWithdrawValidatorCommission(f.ctx, txCtx, &MsgWithdrawValidatorCommission{
		Operator: "operator", Validator: pubKey,
	})
	require.NoError(t, err)
	assert.Empty(t, cEffs)
}

func pubKeyToAddr(pubKey []byte) types.ValidatorAddress {
	var a types.ValidatorAddress
	copy(a[:], pubKey[:20])
	return a
}

// Compile guard so big.Int import isn't optimised away in tests.
var _ = new(big.Int)
