package staking

import (
	"context"
	"testing"

	"github.com/blockberries/punnet-sdk/runtime"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommission_FloorRejectsBelow500Bps pins spec §5 / Phase 2.2:
// MsgCreateValidator submissions with Commission < CommissionFloorBps
// (500 = 5%) must be rejected by the handler with a clear error.
func TestCommission_FloorRejectsBelow500Bps(t *testing.T) {
	mod, _, _ := createTestStakingModule(t)
	ctx := context.Background()
	blockCtx := &runtime.BAPIBlockContext{Height: 1}
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: blockCtx,
		Account:          ptypes.AccountName("alice"),
	}

	cases := []struct {
		name       string
		commission uint64
		wantErr    bool
	}{
		{"zero rejected", 0, true},
		{"100 bps rejected", 100, true},
		{"499 bps rejected (one below floor)", 499, true},
		{"500 bps accepted (at floor)", 500, false},
		{"1000 bps accepted (above floor)", 1000, false},
		{"5000 bps accepted (50% — sane)", 5000, false},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pubKey := []byte("commission-test-pk-")
			pubKey = append(pubKey, byte('a'+i))
			msg := &MsgCreateValidator{
				Delegator:    ptypes.AccountName("alice"),
				PubKey:       pubKey,
				InitialPower: 100,
				Commission:   tc.commission,
			}
			_, err := mod.handleCreateValidator(ctx, txCtx, msg)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "below floor")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCommission_FloorConstantIsFiveHundredBps documents the constant
// — if anyone changes CommissionFloorBps this test fails and forces
// them to confirm the spec change is intentional.
func TestCommission_FloorConstantIsFiveHundredBps(t *testing.T) {
	assert.Equal(t, uint32(500), CommissionFloorBps,
		"spec §5 fixes c_min at 5%% = 500 basis points; changing this "+
			"is a constitutional amendment")
}
