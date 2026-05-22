package governance

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newClassFixture(t *testing.T) (*BAPIGovernanceModule, *store.BAPIProposalStore, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	ps := store.NewBAPIProposalStore(ss)
	mod, err := NewBAPIGovernanceModule(ps, provider.GetBalanceStore())
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, mod.InitGenesis(ctx, nil))
	// Seed alice with enough balance to cover any test deposit.
	require.NoError(t, provider.GetBalanceStore().Set(ctx, "alice", "stake", 1_000_000_000))
	return mod, ps, ctx
}

// TestTimelockForClass pins the spec §11 timelock table.
func TestTimelockForClass(t *testing.T) {
	const day = uint64(86400)
	assert.Equal(t, 7*day, TimelockForClass(ProposalClassSimple7d))
	assert.Equal(t, 30*day, TimelockForClass(ProposalClassSuper30d))
	assert.Equal(t, 60*day, TimelockForClass(ProposalClassSuper60d))
	assert.Equal(t, 60*day, TimelockForClass(ProposalClassConstitutional))
	assert.Equal(t, 7*day, TimelockForClass(""), "empty defaults to Simple7d")
	assert.Equal(t, 7*day, TimelockForClass("bogus"), "unknown defaults to Simple7d (conservative)")
}

// TestThresholdForClass pins spec §11 / Phase 4.2:
//
//	Simple7d        : 50% (default; tunable via DefaultThreshold)
//	Super30d/60d    : 6667 bps = 2/3 supermajority
//	Constitutional  : 8000 bps = 80% supermajority
func TestThresholdForClass(t *testing.T) {
	const simpleDef uint32 = 5000
	assert.Equal(t, simpleDef, ThresholdForClass(ProposalClassSimple7d, simpleDef))
	assert.Equal(t, simpleDef, ThresholdForClass("", simpleDef))
	assert.Equal(t, uint32(6667), ThresholdForClass(ProposalClassSuper30d, simpleDef))
	assert.Equal(t, uint32(6667), ThresholdForClass(ProposalClassSuper60d, simpleDef))
	assert.Equal(t, uint32(8000), ThresholdForClass(ProposalClassConstitutional, simpleDef))
}

// TestSubmitProposal_StoresClass round-trips the Class field on
// MsgSubmitProposal into BAPIProposal.Class. Empty defaults to
// ProposalClassSimple7d.
func TestSubmitProposal_StoresClass(t *testing.T) {
	cases := []struct {
		given string
		want  string
	}{
		{"", ProposalClassSimple7d},
		{ProposalClassSimple7d, ProposalClassSimple7d},
		{ProposalClassSuper30d, ProposalClassSuper30d},
		{ProposalClassSuper60d, ProposalClassSuper60d},
		{ProposalClassConstitutional, ProposalClassConstitutional},
	}

	for _, tc := range cases {
		t.Run("class="+tc.given, func(t *testing.T) {
			mod, ps, ctx := newClassFixture(t)
			txCtx := &runtime.BAPITxContext{
				BAPIBlockContext: &runtime.BAPIBlockContext{
					Height: 1,
					Time:   types.TimeToTimestamp(time.Now()),
				},
				Account: ptypes.AccountName("alice"),
			}
			_, err := mod.handleSubmitProposal(ctx, txCtx, &MsgSubmitProposal{
				Proposer:       "alice",
				Title:          "T",
				Description:    "D",
				ProposalType:   ProposalTypeText,
				InitialDeposit: ptypes.Coin{Denom: "stake", Amount: DefaultMinDeposit},
				Class:          tc.given,
			})
			require.NoError(t, err)
			// Each sub-test gets a fresh store, so the first
			// proposal ID is always 1.
			p, err := ps.GetProposal(ctx, 1)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.Class)
			assert.Equal(t, uint64(0), p.EffectiveHeight,
				"EffectiveHeight is zero until the proposal passes")
		})
	}
}

// TestSubmitProposal_UnknownClassRejected: garbage in the class
// field is rejected at handler time.
func TestSubmitProposal_UnknownClassRejected(t *testing.T) {
	mod, _, ctx := newClassFixture(t)
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height: 1,
			Time:   types.TimeToTimestamp(time.Now()),
		},
		Account: ptypes.AccountName("alice"),
	}
	_, err := mod.handleSubmitProposal(ctx, txCtx, &MsgSubmitProposal{
		Proposer:       "alice",
		Title:          "T",
		Description:    "D",
		ProposalType:   ProposalTypeText,
		InitialDeposit: ptypes.Coin{Denom: "stake", Amount: DefaultMinDeposit},
		Class:          "freeform-poll",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown proposal class")
}

// TestEndBlock_TallyTransitionsByClass: a Super30d proposal with
// 60% Yes / 40% No fails (below 2/3); the same vote on a Simple7d
// proposal passes.
func TestEndBlock_TallyTransitionsByClass(t *testing.T) {
	cases := []struct {
		class      string
		yes, no    uint64
		wantStatus ProposalStatus
	}{
		{ProposalClassSimple7d, 60, 40, StatusPassed},        // 60% > 50%
		{ProposalClassSuper30d, 60, 40, StatusRejected},      // 60% < 66.67%
		{ProposalClassSuper30d, 70, 30, StatusPassed},        // 70% > 66.67%
		{ProposalClassConstitutional, 70, 30, StatusRejected}, // 70% < 80%
		{ProposalClassConstitutional, 80, 20, StatusPassed},  // 80% ≥ 80%
	}
	for _, tc := range cases {
		t.Run(string(tc.wantStatus)+"_"+tc.class, func(t *testing.T) {
			mod, ps, ctx := newClassFixture(t)
			now := time.Now()
			p := &store.BAPIProposal{
				ID:             1,
				Status:         string(StatusVoting),
				Class:          tc.class,
				YesVotes:       tc.yes,
				NoVotes:        tc.no,
				VotingEndTime:  now.Add(-time.Second).Unix(), // already ended
			}
			require.NoError(t, ps.SetProposal(ctx, p))

			currentHeight := uint64(100)
			_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{
				Height: currentHeight,
				Time:   types.TimeToTimestamp(now),
			})
			require.NoError(t, err)

			pPost, err := ps.GetProposal(ctx, 1)
			require.NoError(t, err)
			assert.Equal(t, string(tc.wantStatus), pPost.Status)

			if tc.wantStatus == StatusPassed {
				expected := currentHeight + TimelockForClass(tc.class)
				assert.Equal(t, expected, pPost.EffectiveHeight,
					"passed proposal must have EffectiveHeight = currentHeight + timelock")
			} else {
				assert.Equal(t, uint64(0), pPost.EffectiveHeight,
					"rejected proposal must NOT have EffectiveHeight set")
			}
		})
	}
}

// TestEndBlock_ZeroParticipationRejected: a voting proposal that
// nobody voted on (totals all zero) rejects at tally time —
// matches cosmos-sdk x/gov's "no quorum" behavior.
func TestEndBlock_ZeroParticipationRejected(t *testing.T) {
	mod, ps, ctx := newClassFixture(t)
	now := time.Now()
	p := &store.BAPIProposal{
		ID:            1,
		Status:        string(StatusVoting),
		Class:         ProposalClassSimple7d,
		VotingEndTime: now.Add(-time.Second).Unix(),
	}
	require.NoError(t, ps.SetProposal(ctx, p))

	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 1,
		Time:   types.TimeToTimestamp(now),
	})
	require.NoError(t, err)

	pPost, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusRejected), pPost.Status)
}

// TestEndBlock_VotingNotEndedYet: a proposal whose VotingEndTime
// is in the future is NOT tallied yet.
func TestEndBlock_VotingNotEndedYet(t *testing.T) {
	mod, ps, ctx := newClassFixture(t)
	now := time.Now()
	p := &store.BAPIProposal{
		ID:            1,
		Status:        string(StatusVoting),
		Class:         ProposalClassSimple7d,
		YesVotes:      100,
		VotingEndTime: now.Add(time.Hour).Unix(), // future
	}
	require.NoError(t, ps.SetProposal(ctx, p))

	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{
		Height: 1,
		Time:   types.TimeToTimestamp(now),
	})
	require.NoError(t, err)
	pPost, _ := ps.GetProposal(ctx, 1)
	assert.Equal(t, string(StatusVoting), pPost.Status, "tally must wait for voting end")
}
