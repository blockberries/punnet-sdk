package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestBlockHeader_NewBlockHeader(t *testing.T) {
	now := time.Now()
	proposer := []byte("proposer")

	header := NewBlockHeader(100, now, "test-chain", proposer)

	require.NotNil(t, header)
	require.Equal(t, uint64(100), header.Height)
	require.Equal(t, now, header.Time)
	require.Equal(t, "test-chain", header.ChainID)
	require.Equal(t, proposer, header.ProposerAddress)

	// Verify defensive copy
	proposer[0] = 'x'
	require.NotEqual(t, proposer, header.ProposerAddress)
}

func TestBlockHeader_ValidateBasic(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		header  *BlockHeader
		wantErr bool
	}{
		{
			name: "valid header",
			header: &BlockHeader{
				Height:          100,
				Time:            now,
				ChainID:         "test-chain",
				ProposerAddress: []byte("proposer"),
			},
			wantErr: false,
		},
		{
			name:    "nil header",
			header:  nil,
			wantErr: true,
		},
		{
			name: "zero height",
			header: &BlockHeader{
				Height:          0,
				Time:            now,
				ChainID:         "test-chain",
				ProposerAddress: []byte("proposer"),
			},
			wantErr: true,
		},
		{
			name: "empty chain ID",
			header: &BlockHeader{
				Height:          100,
				Time:            now,
				ChainID:         "",
				ProposerAddress: []byte("proposer"),
			},
			wantErr: true,
		},
		{
			name: "zero time",
			header: &BlockHeader{
				Height:          100,
				Time:            time.Time{},
				ChainID:         "test-chain",
				ProposerAddress: []byte("proposer"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.header.ValidateBasic()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestContext_NewContext(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)
	require.NotNil(t, rctx)
	require.Equal(t, ctx, rctx.Context())
	require.Equal(t, account, rctx.Account())
	require.False(t, rctx.IsReadOnly())
	require.Equal(t, uint64(0), rctx.GasUsed())
}

func TestContext_NewContext_Errors(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	tests := []struct {
		name    string
		ctx     context.Context
		header  *BlockHeader
		account types.AccountName
		wantErr bool
	}{
		{
			name:    "nil context",
			ctx:     nil,
			header:  header,
			account: account,
			wantErr: true,
		},
		{
			name:    "nil header",
			ctx:     ctx,
			header:  nil,
			account: account,
			wantErr: true,
		},
		{
			name: "invalid header",
			ctx:  ctx,
			header: &BlockHeader{
				Height:  0, // invalid
				Time:    time.Now(),
				ChainID: "test-chain",
			},
			account: account,
			wantErr: true,
		},
		{
			name:    "invalid account",
			ctx:     ctx,
			header:  header,
			account: types.AccountName("INVALID"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewContext(tt.ctx, tt.header, tt.account)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestContext_NewReadOnlyContext(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewReadOnlyContext(ctx, header, account)
	require.NoError(t, err)
	require.NotNil(t, rctx)
	require.True(t, rctx.IsReadOnly())
}

func TestContext_BlockInfo(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	proposer := []byte("proposer")
	header := NewBlockHeader(100, now, "test-chain", proposer)
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	require.Equal(t, uint64(100), rctx.BlockHeight())
	require.Equal(t, now, rctx.BlockTime())
	require.Equal(t, "test-chain", rctx.ChainID())

	// Verify defensive copy of proposer address
	returnedProposer := rctx.ProposerAddress()
	require.Equal(t, proposer, returnedProposer)

	// Modify returned proposer - should not affect internal state
	returnedProposer[0] = 'x'
	require.Equal(t, proposer, rctx.ProposerAddress())
}

func TestContext_NilSafety(t *testing.T) {
	var rctx *Context

	// All methods should handle nil safely
	require.NotNil(t, rctx.Context())
	require.Equal(t, uint64(0), rctx.BlockHeight())
	require.True(t, rctx.BlockTime().IsZero())
	require.Equal(t, "", rctx.ChainID())
	require.Nil(t, rctx.ProposerAddress())
	require.Equal(t, types.AccountName(""), rctx.Account())
	require.True(t, rctx.IsReadOnly())
	require.Equal(t, 0, rctx.EffectCount())
	require.Nil(t, rctx.CollectEffects())
	require.Equal(t, uint64(0), rctx.GasUsed())

	// These should not panic
	rctx.ClearEffects()
	rctx.ConsumeGas(100)
}

func TestContext_EmitEffect(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	effect := effects.NewEventEffect("test", map[string][]byte{
		"key": []byte("value"),
	})

	err = rctx.EmitEffect(effect)
	require.NoError(t, err)
	require.Equal(t, 1, rctx.EffectCount())
}

func TestContext_EmitEffect_ReadOnly(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewReadOnlyContext(ctx, header, account)
	require.NoError(t, err)

	effect := effects.NewEventEffect("test", map[string][]byte{
		"key": []byte("value"),
	})

	err = rctx.EmitEffect(effect)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read-only")
}

func TestContext_EmitEffects(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	effs := []effects.Effect{
		effects.NewEventEffect("test1", map[string][]byte{"key": []byte("value1")}),
		effects.NewEventEffect("test2", map[string][]byte{"key": []byte("value2")}),
	}

	err = rctx.EmitEffects(effs)
	require.NoError(t, err)
	require.Equal(t, 2, rctx.EffectCount())
}

func TestContext_CollectEffects(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	effect1 := effects.NewEventEffect("test1", map[string][]byte{"key": []byte("value1")})
	effect2 := effects.NewEventEffect("test2", map[string][]byte{"key": []byte("value2")})

	err = rctx.EmitEffect(effect1)
	require.NoError(t, err)
	err = rctx.EmitEffect(effect2)
	require.NoError(t, err)

	collected := rctx.CollectEffects()
	require.Len(t, collected, 2)

	// Collector should be cleared after collection
	require.Equal(t, 0, rctx.EffectCount())
}

func TestContext_ClearEffects(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	effect := effects.NewEventEffect("test", map[string][]byte{"key": []byte("value")})
	err = rctx.EmitEffect(effect)
	require.NoError(t, err)

	rctx.ClearEffects()
	require.Equal(t, 0, rctx.EffectCount())
}

func TestContext_GasMetering(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	require.Equal(t, uint64(0), rctx.GasUsed())

	rctx.ConsumeGas(100)
	require.Equal(t, uint64(100), rctx.GasUsed())

	rctx.ConsumeGas(50)
	require.Equal(t, uint64(150), rctx.GasUsed())
}

func TestContext_WithContext(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	newCtx := context.WithValue(ctx, "key", "value")
	rctx2 := rctx.WithContext(newCtx)

	require.NotNil(t, rctx2)
	require.Equal(t, newCtx, rctx2.Context())
	require.Equal(t, account, rctx2.Account())

	// Original context should be unchanged
	require.Equal(t, ctx, rctx.Context())
}

func TestContext_WithContext_Nil(t *testing.T) {
	var rctx *Context
	rctx2 := rctx.WithContext(context.Background())
	require.Nil(t, rctx2)
}

func TestContext_WithAccount(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	// Emit an effect in the original context
	effect := effects.NewEventEffect("test", map[string][]byte{"key": []byte("value")})
	err = rctx.EmitEffect(effect)
	require.NoError(t, err)

	// Create new context with different account
	newAccount := types.AccountName("bob")
	rctx2, err := rctx.WithAccount(newAccount)
	require.NoError(t, err)
	require.NotNil(t, rctx2)
	require.Equal(t, newAccount, rctx2.Account())

	// New context should have fresh collector and gas counter
	require.Equal(t, 0, rctx2.EffectCount())
	require.Equal(t, uint64(0), rctx2.GasUsed())

	// Original context should be unchanged
	require.Equal(t, account, rctx.Account())
	require.Equal(t, 1, rctx.EffectCount())
}

func TestContext_WithAccount_Invalid(t *testing.T) {
	ctx := context.Background()
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	account := types.AccountName("alice")

	rctx, err := NewContext(ctx, header, account)
	require.NoError(t, err)

	// Try with invalid account
	_, err = rctx.WithAccount(types.AccountName("INVALID"))
	require.Error(t, err)
}

func TestContext_WithAccount_Nil(t *testing.T) {
	var rctx *Context
	_, err := rctx.WithAccount(types.AccountName("alice"))
	require.Error(t, err)
}
