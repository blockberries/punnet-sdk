package bank

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func setupTestBankModule(t *testing.T) (*BankModule, capability.BalanceCapability) {
	t.Helper()

	// Create memory store
	memStore := store.NewMemoryStore()

	// Create capability manager
	capMgr := capability.NewCapabilityManager(memStore)

	// Register module
	if err := capMgr.RegisterModule("bank"); err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	// Grant balance capability
	balanceCap, err := capMgr.GrantBalanceCapability("bank")
	if err != nil {
		t.Fatalf("failed to grant balance capability: %v", err)
	}

	// Create bank module
	bankMod, err := NewBankModule(balanceCap)
	if err != nil {
		t.Fatalf("failed to create bank module: %v", err)
	}

	return bankMod, balanceCap
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

func TestNewBankModule(t *testing.T) {
	_, balanceCap := setupTestBankModule(t)

	t.Run("valid capability", func(t *testing.T) {
		mod, err := NewBankModule(balanceCap)
		if err != nil {
			t.Errorf("NewBankModule() error = %v, want nil", err)
		}
		if mod == nil {
			t.Error("NewBankModule() returned nil module")
		}
	})

	t.Run("nil capability", func(t *testing.T) {
		mod, err := NewBankModule(nil)
		if err == nil {
			t.Error("NewBankModule(nil) error = nil, want error")
		}
		if mod != nil {
			t.Error("NewBankModule(nil) returned non-nil module")
		}
	})
}

func TestCreateModule(t *testing.T) {
	_, balanceCap := setupTestBankModule(t)

	t.Run("valid module", func(t *testing.T) {
		mod, err := CreateModule(balanceCap)
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

	t.Run("nil capability", func(t *testing.T) {
		mod, err := CreateModule(nil)
		if err == nil {
			t.Error("CreateModule(nil) error = nil, want error")
		}
		if mod != nil {
			t.Error("CreateModule(nil) returned non-nil module")
		}
	})
}

func TestBankModule_HandleSend(t *testing.T) {
	bankMod, balanceCap := setupTestBankModule(t)

	// Setup initial balance for alice
	if err := balanceCap.SetBalance(context.Background(), "alice", "token", 1000); err != nil {
		t.Fatalf("failed to set initial balance: %v", err)
	}

	tests := []struct {
		name    string
		msg     *MsgSend
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid send",
			msg: &MsgSend{
				From:   "alice",
				To:     "bob",
				Amount: types.NewCoin("token", 100),
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "sender mismatch",
			msg: &MsgSend{
				From:   "alice",
				To:     "bob",
				Amount: types.NewCoin("token", 100),
			},
			account: "bob",
			wantErr: true,
		},
		{
			name: "insufficient balance",
			msg: &MsgSend{
				From:   "alice",
				To:     "bob",
				Amount: types.NewCoin("token", 10000),
			},
			account: "alice",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := bankMod.handleSend(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleSend() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleSend() returned no effects")
				}
			}
		})
	}
}

func TestBankModule_HandleSend_InvalidMessage(t *testing.T) {
	bankMod, _ := setupTestBankModule(t)
	ctx := setupTestContext(t, "alice")

	// Pass wrong message type
	msg := &MsgMultiSend{}

	effects, err := bankMod.handleSend(ctx, msg)
	if err == nil {
		t.Error("handleSend() with wrong message type should error")
	}
	if effects != nil {
		t.Error("handleSend() with wrong message type should return nil effects")
	}
}

func TestBankModule_HandleSend_NilModule(t *testing.T) {
	var bankMod *BankModule
	ctx := setupTestContext(t, "alice")

	msg := &MsgSend{
		From:   "alice",
		To:     "bob",
		Amount: types.NewCoin("token", 100),
	}

	effects, err := bankMod.handleSend(ctx, msg)
	if err == nil {
		t.Error("handleSend() on nil module should error")
	}
	if effects != nil {
		t.Error("handleSend() on nil module should return nil effects")
	}
}

func TestBankModule_HandleMultiSend(t *testing.T) {
	bankMod, balanceCap := setupTestBankModule(t)

	// Setup initial balances
	if err := balanceCap.SetBalance(context.Background(), "alice", "token", 1000); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}
	if err := balanceCap.SetBalance(context.Background(), "bob", "token", 500); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	tests := []struct {
		name    string
		msg     *MsgMultiSend
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid multi-send",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
				Outputs: []Output{
					{
						Address: "charlie",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "signer not in inputs",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
				Outputs: []Output{
					{
						Address: "charlie",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
			},
			account: "bob",
			wantErr: true,
		},
		{
			name: "insufficient balance",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 10000)),
					},
				},
				Outputs: []Output{
					{
						Address: "charlie",
						Coins:   types.NewCoins(types.NewCoin("token", 10000)),
					},
				},
			},
			account: "alice",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := bankMod.handleMultiSend(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleMultiSend() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleMultiSend() returned no effects")
				}
			}
		})
	}
}

func TestBankModule_HandleQueryBalance(t *testing.T) {
	bankMod, balanceCap := setupTestBankModule(t)

	// Set up balance
	if err := balanceCap.SetBalance(context.Background(), "alice", "token", 1000); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	ctx := setupTestContext(t, "alice")

	t.Run("valid query", func(t *testing.T) {
		data := []byte("alice/token")
		result, err := bankMod.handleQueryBalance(ctx.Context(), "/balance", data)
		if err != nil {
			t.Errorf("handleQueryBalance() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryBalance() returned nil result")
		}
		if string(result) != "1000" {
			t.Errorf("handleQueryBalance() = %s, want 1000", string(result))
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		data := []byte("alice")
		result, err := bankMod.handleQueryBalance(ctx.Context(), "/balance", data)
		if err == nil {
			t.Error("handleQueryBalance() with invalid format should error")
		}
		if result != nil {
			t.Error("handleQueryBalance() with invalid format should return nil result")
		}
	})

	t.Run("invalid account", func(t *testing.T) {
		data := []byte("ALICE/token")
		result, err := bankMod.handleQueryBalance(ctx.Context(), "/balance", data)
		if err == nil {
			t.Error("handleQueryBalance() with invalid account should error")
		}
		if result != nil {
			t.Error("handleQueryBalance() with invalid account should return nil result")
		}
	})
}

func TestBankModule_HandleQueryAllBalances(t *testing.T) {
	bankMod, balanceCap := setupTestBankModule(t)

	// Set up multiple balances
	if err := balanceCap.SetBalance(context.Background(), "alice", "token1", 1000); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}
	if err := balanceCap.SetBalance(context.Background(), "alice", "token2", 500); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	ctx := setupTestContext(t, "alice")

	t.Run("valid query", func(t *testing.T) {
		data := []byte("alice")
		result, err := bankMod.handleQueryAllBalances(ctx.Context(), "/all_balances", data)
		if err != nil {
			t.Errorf("handleQueryAllBalances() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryAllBalances() returned nil result")
		}
	})

	t.Run("invalid account", func(t *testing.T) {
		data := []byte("")
		result, err := bankMod.handleQueryAllBalances(ctx.Context(), "/all_balances", data)
		if err == nil {
			t.Error("handleQueryAllBalances() with invalid account should error")
		}
		if result != nil {
			t.Error("handleQueryAllBalances() with invalid account should return nil result")
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
			s:    "alice/token",
			sep:  '/',
			want: []string{"alice", "token"},
		},
		{
			name: "no separator",
			s:    "alice",
			sep:  '/',
			want: []string{"alice"},
		},
		{
			name: "multiple separators",
			s:    "alice/token/extra",
			sep:  '/',
			want: []string{"alice", "token/extra"},
		},
		{
			name: "empty string",
			s:    "",
			sep:  '/',
			want: []string{""},
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
