package staking

import (
	"context"
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type slashFixture struct {
	mod    *BAPIStakingModule
	vs     *store.BAPIValidatorStore
	bs     *store.BAPIBalanceStore
	exec   *effects.BAPIExecutor
	ctx    context.Context
}

func newSlashFixture(t *testing.T) *slashFixture {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	mod, err := NewBAPIStakingModule(provider.GetValidatorStore(), provider.GetBalanceStore())
	require.NoError(t, err)
	return &slashFixture{
		mod:  mod,
		vs:   provider.GetValidatorStore(),
		bs:   provider.GetBalanceStore(),
		exec: effects.NewBAPIExecutor(provider),
		ctx:  context.Background(),
	}
}

// TestSlash_DelegatorsProportionallySlashed verifies the key Phase
// 2.6 invariant: a slash hits every delegator's stake by the same
// basis-point fraction. With a validator carrying TotalDelegation
// = 1000 spread across two delegators (alice 600, bob 400) and a
// 10% slash, alice's row should drop to 540 and bob's to 360.
func TestSlash_DelegatorsProportionallySlashed(t *testing.T) {
	f := newSlashFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xd1

	// Seed validator + two delegations.
	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           1000,
		TotalDelegation: 1000,
	}))
	require.NoError(t, f.vs.Delegate(f.ctx, "alice", pubKey, 600))
	require.NoError(t, f.vs.Delegate(f.ctx, "bob", pubKey, 400))
	// Validator's Delegate calls bumped TotalDelegation to 2000.
	// Reset to 1000 so the slash math expects 10% off 1000.
	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	v.TotalDelegation = 1000
	v.Power = 1000
	require.NoError(t, f.vs.SetValidator(f.ctx, v))
	require.NoError(t, f.bs.Set(f.ctx, "staking.pool", "stake", 1000))

	// Override slash to 10% (1000 bps) for a clean number.
	params := store.DefaultConsensusParams()
	params.SlashFractionDoubleSignBps = 1000
	blockCtx := &runtime.BAPIBlockContext{Height: 1, Params: params}

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, err := f.mod.ProcessEvidence(f.ctx, blockCtx, ev)
	require.NoError(t, err)
	require.NotEmpty(t, effs)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	// Check delegations were each slashed by 10%.
	dAlice, _ := f.vs.GetDelegation(f.ctx, "alice", pubKey)
	dBob, _ := f.vs.GetDelegation(f.ctx, "bob", pubKey)
	assert.Equal(t, uint64(540), dAlice.Amount, "alice 600 - 10%% = 540")
	assert.Equal(t, uint64(360), dBob.Amount, "bob 400 - 10%% = 360")

	// Validator's TotalDelegation matches the new sum.
	vPost, _ := f.vs.GetValidator(f.ctx, pubKey)
	assert.Equal(t, uint64(900), vPost.TotalDelegation, "1000 - 10%% = 900")
	assert.Equal(t, uint64(900), vPost.Power)
	assert.True(t, vPost.Jailed)
}

// TestSlash_CreditsToCommonTreasury verifies the slash transfer:
// the slashed amount lands in module.ct, and supply across
// (staking.pool + module.ct) is conserved.
func TestSlash_CreditsToCommonTreasury(t *testing.T) {
	f := newSlashFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xd2

	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           1000,
		TotalDelegation: 1000,
	}))
	require.NoError(t, f.bs.Set(f.ctx, "staking.pool", "stake", 1000))
	// module.ct starts empty.

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, err := f.mod.ProcessEvidence(f.ctx, &runtime.BAPIBlockContext{Height: 1}, ev)
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	poolPost, _ := f.bs.GetAmount(f.ctx, "staking.pool", "stake")
	ctPost, _ := f.bs.GetAmount(f.ctx, string(runtime.ModuleAccountCT), "stake")

	expectedSlash := uint64(1000) * uint64(DefaultSlashFractionDoubleSignBps) / 10000
	assert.Equal(t, uint64(1000)-expectedSlash, poolPost, "staking.pool drained by slash amount")
	assert.Equal(t, expectedSlash, ctPost, "module.ct credited by slash amount")
}

// TestSlash_SeveritySpec covers the three severity values listed in
// spec §9 (PLAN 2.6):
//
//   - DuplicateVote (equivocation)   -> 500 bps (5%)
//   - Liveness                       -> 10 bps (0.1%)
//   - Leader equivocation            -> 500 bps (5%)
//
// Liveness and leader-equivocation constants are exported even
// though their wire support is pending in bapi's EvidenceType
// enum — pinning them keeps the future plumbing change honest to
// the spec values.
func TestSlash_SeveritySpec(t *testing.T) {
	assert.Equal(t, uint32(500), DefaultSlashFractionDoubleSignBps,
		"§9 equivocation slash = 5%")
	assert.Equal(t, uint32(10), DefaultSlashFractionLivenessBps,
		"§9 liveness slash = 0.1%")
	assert.Equal(t, uint32(500), DefaultSlashFractionLeaderEquivocationBps,
		"§9 leader-equivocation slash = 5%")
}

// TestSlash_NoBackingPoolFailsAfterEffects: when staking.pool lacks
// the slash amount the effect executor refuses the transfer. This
// is by design — Phase 1.5's atomicity gives the runtime a clean
// error rather than partial state.
func TestSlash_NoBackingPoolFails(t *testing.T) {
	f := newSlashFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xd3

	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           1000,
		TotalDelegation: 1000,
	}))
	// Do NOT seed staking.pool.

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, err := f.mod.ProcessEvidence(f.ctx, &runtime.BAPIBlockContext{Height: 1}, ev)
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.Error(t, err, "executor must surface the unbacked transfer attempt")
}

var _ = ptypes.AccountName("")
