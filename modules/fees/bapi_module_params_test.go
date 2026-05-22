package fees

import (
	"context"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newParamsFixture(t *testing.T) (*BAPIFeesModule, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	mod, err := NewBAPIFeesModule(ss)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, mod.InitGenesis(ctx, nil))
	return mod, ctx
}

// TestApplyParameterChange_ByteFee covers the byte_fee path of the
// governance enactment hook. PLAN §7 Phase 4.5.
func TestApplyParameterChange_ByteFee(t *testing.T) {
	mod, ctx := newParamsFixture(t)

	require.NoError(t, mod.ApplyParameterChange(ctx, ParamByteFee, 42))

	sched, err := mod.Schedule(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), sched.ByteFee)
}

// TestApplyParameterChange_OpFeeInsert: a new message type
// inserts an OpFee entry in sorted order.
func TestApplyParameterChange_OpFeeInsert(t *testing.T) {
	mod, ctx := newParamsFixture(t)

	require.NoError(t, mod.ApplyParameterChange(ctx, ParamOpFeePref+"/bank.MsgSend", 100))
	require.NoError(t, mod.ApplyParameterChange(ctx, ParamOpFeePref+"/staking.MsgDelegate", 50))

	sched, _ := mod.Schedule(ctx)
	require.Len(t, sched.OpFees, 2)
	// Sorted by message type ascending.
	assert.Equal(t, "/bank.MsgSend", sched.OpFees[0].MessageType)
	assert.Equal(t, uint64(100), sched.OpFees[0].Amount)
	assert.Equal(t, "/staking.MsgDelegate", sched.OpFees[1].MessageType)
	assert.Equal(t, uint64(50), sched.OpFees[1].Amount)
}

// TestApplyParameterChange_OpFeeReplace: applying op_fee for an
// existing message type updates the amount in place (no
// duplicate entry).
func TestApplyParameterChange_OpFeeReplace(t *testing.T) {
	mod, ctx := newParamsFixture(t)
	require.NoError(t, mod.ApplyParameterChange(ctx, ParamOpFeePref+"/bank.MsgSend", 100))
	require.NoError(t, mod.ApplyParameterChange(ctx, ParamOpFeePref+"/bank.MsgSend", 250))

	sched, _ := mod.Schedule(ctx)
	require.Len(t, sched.OpFees, 1)
	assert.Equal(t, uint64(250), sched.OpFees[0].Amount, "second apply replaced, not appended")
}

// TestApplyParameterChange_NegativeValueRejected: fees are
// non-negative; a negative governance proposal value is rejected.
func TestApplyParameterChange_NegativeValueRejected(t *testing.T) {
	mod, ctx := newParamsFixture(t)
	err := mod.ApplyParameterChange(ctx, ParamByteFee, -1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-negative")
}

// TestApplyParameterChange_UnknownParamRejected.
func TestApplyParameterChange_UnknownParamRejected(t *testing.T) {
	mod, ctx := newParamsFixture(t)
	err := mod.ApplyParameterChange(ctx, "max_validators", 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not own parameter")
}
