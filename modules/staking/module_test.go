package staking

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func setupTestStakingModule(t *testing.T) (*StakingModule, capability.ValidatorCapability, capability.BalanceCapability) {
	t.Helper()

	// Create memory store
	memStore := store.NewMemoryStore()

	// Create capability manager
	capMgr := capability.NewCapabilityManager(memStore)

	// Register module
	if err := capMgr.RegisterModule("staking"); err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	// Grant capabilities
	validatorCap, err := capMgr.GrantValidatorCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant validator capability: %v", err)
	}

	balanceCap, err := capMgr.GrantBalanceCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant balance capability: %v", err)
	}

	// Create staking module
	stakingMod, err := NewStakingModule(validatorCap, balanceCap)
	if err != nil {
		t.Fatalf("failed to create staking module: %v", err)
	}

	return stakingMod, validatorCap, balanceCap
}

func setupTestContext(t *testing.T, account types.AccountName) *runtime.Context {
	t.Helper()

	header := runtime.NewBlockHeader(1, time.Now(), "test-chain", []byte("proposer"))
	ctx, err := runtime.NewContext(context.Background(), header, account)
	if err != nil {
		t.Fatalf("failed to create context: %v", err)
	}

	return ctx
}

func TestNewStakingModule(t *testing.T) {
	_, validatorCap, balanceCap := setupTestStakingModule(t)

	t.Run("valid capabilities", func(t *testing.T) {
		mod, err := NewStakingModule(validatorCap, balanceCap)
		if err != nil {
			t.Errorf("NewStakingModule() error = %v, want nil", err)
		}
		if mod == nil {
			t.Error("NewStakingModule() returned nil module")
		}
	})

	t.Run("nil validator capability", func(t *testing.T) {
		mod, err := NewStakingModule(nil, balanceCap)
		if err == nil {
			t.Error("NewStakingModule(nil, ...) error = nil, want error")
		}
		if mod != nil {
			t.Error("NewStakingModule(nil, ...) returned non-nil module")
		}
	})

	t.Run("nil balance capability", func(t *testing.T) {
		mod, err := NewStakingModule(validatorCap, nil)
		if err == nil {
			t.Error("NewStakingModule(..., nil) error = nil, want error")
		}
		if mod != nil {
			t.Error("NewStakingModule(..., nil) returned non-nil module")
		}
	})
}

func TestCreateModule(t *testing.T) {
	_, validatorCap, balanceCap := setupTestStakingModule(t)

	t.Run("valid module", func(t *testing.T) {
		mod, err := CreateModule(validatorCap, balanceCap)
		if err != nil {
			t.Errorf("CreateModule() error = %v, want nil", err)
		}
		if mod == nil {
			t.Error("CreateModule() returned nil")
		}
		if mod.Name() != ModuleName {
			t.Errorf("Module.Name() = %v, want %v", mod.Name(), ModuleName)
		}
	})

	t.Run("nil capabilities", func(t *testing.T) {
		mod, err := CreateModule(nil, nil)
		if err == nil {
			t.Error("CreateModule(nil, nil) error = nil, want error")
		}
		if mod != nil {
			t.Error("CreateModule(nil, nil) returned non-nil module")
		}
	})
}

func TestStakingModule_HandleCreateValidator(t *testing.T) {
	stakingMod, _, _ := setupTestStakingModule(t)

	tests := []struct {
		name    string
		msg     *MsgCreateValidator
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid create",
			msg: &MsgCreateValidator{
				Delegator:    "alice",
				PubKey:       []byte("validator-key-1"),
				InitialPower: 100,
				Commission:   500,
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "delegator mismatch",
			msg: &MsgCreateValidator{
				Delegator:    "alice",
				PubKey:       []byte("validator-key-2"),
				InitialPower: 100,
				Commission:   500,
			},
			account: "bob",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := stakingMod.handleCreateValidator(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleCreateValidator() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleCreateValidator() returned no effects")
				}
			}
		})
	}
}

func TestStakingModule_HandleCreateValidator_Duplicate(t *testing.T) {
	stakingMod, validatorCap, _ := setupTestStakingModule(t)

	// Create a validator first
	pubKey := []byte("validator-key")
	validator := store.NewValidator(pubKey, 100, "alice")
	if err := validatorCap.SetValidator(context.Background(), validator); err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	ctx := setupTestContext(t, "alice")
	msg := &MsgCreateValidator{
		Delegator:    "alice",
		PubKey:       pubKey,
		InitialPower: 100,
		Commission:   500,
	}

	effects, err := stakingMod.handleCreateValidator(ctx, msg)
	if err == nil {
		t.Error("handleCreateValidator() with duplicate key should error")
	}
	if effects != nil {
		t.Error("handleCreateValidator() with duplicate key should return nil effects")
	}
}

func TestStakingModule_HandleDelegate(t *testing.T) {
	stakingMod, validatorCap, balanceCap := setupTestStakingModule(t)

	// Setup: create a validator and give delegator balance
	pubKey := []byte("validator-key")
	validator := store.NewValidator(pubKey, 100, "bob")
	if err := validatorCap.SetValidator(context.Background(), validator); err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	if err := balanceCap.SetBalance(context.Background(), "alice", "stake", 1000); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	tests := []struct {
		name    string
		msg     *MsgDelegate
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid delegate",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: pubKey,
				Amount:    types.NewCoin("stake", 100),
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "delegator mismatch",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: pubKey,
				Amount:    types.NewCoin("stake", 100),
			},
			account: "bob",
			wantErr: true,
		},
		{
			name: "insufficient balance",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: pubKey,
				Amount:    types.NewCoin("stake", 10000),
			},
			account: "alice",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := stakingMod.handleDelegate(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleDelegate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleDelegate() returned no effects")
				}
			}
		})
	}
}

func TestStakingModule_HandleDelegate_ValidatorNotFound(t *testing.T) {
	stakingMod, _, balanceCap := setupTestStakingModule(t)

	// Give delegator balance but don't create validator
	if err := balanceCap.SetBalance(context.Background(), "alice", "stake", 1000); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	ctx := setupTestContext(t, "alice")
	msg := &MsgDelegate{
		Delegator: "alice",
		Validator: []byte("non-existent-validator"),
		Amount:    types.NewCoin("stake", 100),
	}

	effects, err := stakingMod.handleDelegate(ctx, msg)
	if err == nil {
		t.Error("handleDelegate() with non-existent validator should error")
	}
	if effects != nil {
		t.Error("handleDelegate() with non-existent validator should return nil effects")
	}
}

func TestStakingModule_HandleUndelegate(t *testing.T) {
	stakingMod, validatorCap, _ := setupTestStakingModule(t)

	// Setup: create validator and delegation
	pubKey := []byte("validator-key")
	validator := store.NewValidator(pubKey, 100, "bob")
	if err := validatorCap.SetValidator(context.Background(), validator); err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	delegation := store.NewDelegation("alice", pubKey, 500)
	if err := validatorCap.SetDelegation(context.Background(), delegation); err != nil {
		t.Fatalf("failed to set delegation: %v", err)
	}

	tests := []struct {
		name    string
		msg     *MsgUndelegate
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid undelegate partial",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: pubKey,
				Amount:    types.NewCoin("stake", 100),
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "delegator mismatch",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: pubKey,
				Amount:    types.NewCoin("stake", 100),
			},
			account: "bob",
			wantErr: true,
		},
		{
			name: "insufficient shares",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: pubKey,
				Amount:    types.NewCoin("stake", 10000),
			},
			account: "alice",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := stakingMod.handleUndelegate(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleUndelegate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleUndelegate() returned no effects")
				}
			}
		})
	}
}

func TestStakingModule_HandleUndelegate_DelegationNotFound(t *testing.T) {
	stakingMod, validatorCap, _ := setupTestStakingModule(t)

	// Create validator but no delegation
	pubKey := []byte("validator-key")
	validator := store.NewValidator(pubKey, 100, "bob")
	if err := validatorCap.SetValidator(context.Background(), validator); err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	ctx := setupTestContext(t, "alice")
	msg := &MsgUndelegate{
		Delegator: "alice",
		Validator: pubKey,
		Amount:    types.NewCoin("stake", 100),
	}

	effects, err := stakingMod.handleUndelegate(ctx, msg)
	if err == nil {
		t.Error("handleUndelegate() without delegation should error")
	}
	if effects != nil {
		t.Error("handleUndelegate() without delegation should return nil effects")
	}
}

func TestStakingModule_HandleQueryValidator(t *testing.T) {
	stakingMod, validatorCap, _ := setupTestStakingModule(t)

	// Create a validator
	pubKey := []byte("validator-key")
	validator := store.NewValidator(pubKey, 100, "alice")
	if err := validatorCap.SetValidator(context.Background(), validator); err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	ctx := setupTestContext(t, "alice")

	t.Run("valid query with hex", func(t *testing.T) {
		data := []byte(hex.EncodeToString(pubKey))
		result, err := stakingMod.handleQueryValidator(ctx.Context(), "/validator", data)
		if err != nil {
			t.Errorf("handleQueryValidator() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryValidator() returned nil result")
		}
	})

	t.Run("valid query with raw bytes", func(t *testing.T) {
		result, err := stakingMod.handleQueryValidator(ctx.Context(), "/validator", pubKey)
		if err != nil {
			t.Errorf("handleQueryValidator() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryValidator() returned nil result")
		}
	})

	t.Run("empty public key", func(t *testing.T) {
		data := []byte("")
		result, err := stakingMod.handleQueryValidator(ctx.Context(), "/validator", data)
		if err == nil {
			t.Error("handleQueryValidator() with empty key should error")
		}
		if result != nil {
			t.Error("handleQueryValidator() with empty key should return nil result")
		}
	})
}

func TestStakingModule_HandleQueryValidators(t *testing.T) {
	stakingMod, validatorCap, _ := setupTestStakingModule(t)

	// Create multiple validators
	for i := 0; i < 3; i++ {
		pubKey := []byte{byte(i)}
		validator := store.NewValidator(pubKey, 100, "alice")
		if err := validatorCap.SetValidator(context.Background(), validator); err != nil {
			t.Fatalf("failed to set validator: %v", err)
		}
	}

	ctx := setupTestContext(t, "alice")

	result, err := stakingMod.handleQueryValidators(ctx.Context(), "/validators", nil)
	if err != nil {
		t.Errorf("handleQueryValidators() error = %v, want nil", err)
	}
	if result == nil {
		t.Error("handleQueryValidators() returned nil result")
	}
}

func TestStakingModule_HandleQueryDelegation(t *testing.T) {
	stakingMod, validatorCap, _ := setupTestStakingModule(t)

	// Create delegation
	pubKey := []byte("validator-key")
	delegation := store.NewDelegation("alice", pubKey, 500)
	if err := validatorCap.SetDelegation(context.Background(), delegation); err != nil {
		t.Fatalf("failed to set delegation: %v", err)
	}

	ctx := setupTestContext(t, "alice")

	t.Run("valid query", func(t *testing.T) {
		data := []byte("alice/" + hex.EncodeToString(pubKey))
		result, err := stakingMod.handleQueryDelegation(ctx.Context(), "/delegation", data)
		if err != nil {
			t.Errorf("handleQueryDelegation() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryDelegation() returned nil result")
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		data := []byte("alice")
		result, err := stakingMod.handleQueryDelegation(ctx.Context(), "/delegation", data)
		if err == nil {
			t.Error("handleQueryDelegation() with invalid format should error")
		}
		if result != nil {
			t.Error("handleQueryDelegation() with invalid format should return nil result")
		}
	})

	t.Run("invalid account", func(t *testing.T) {
		data := []byte("/" + hex.EncodeToString(pubKey))
		result, err := stakingMod.handleQueryDelegation(ctx.Context(), "/delegation", data)
		if err == nil {
			t.Error("handleQueryDelegation() with invalid account should error")
		}
		if result != nil {
			t.Error("handleQueryDelegation() with invalid account should return nil result")
		}
	})
}

func TestSplitOnce(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sep  rune
		want []string
	}{
		{
			name: "normal split",
			s:    "alice/validator",
			sep:  '/',
			want: []string{"alice", "validator"},
		},
		{
			name: "no separator",
			s:    "alice",
			sep:  '/',
			want: []string{"alice"},
		},
		{
			name: "multiple separators",
			s:    "alice/validator/extra",
			sep:  '/',
			want: []string{"alice", "validator/extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitOnce(tt.s, tt.sep)
			if len(got) != len(tt.want) {
				t.Errorf("splitOnce() returned %d parts, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitOnce()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
