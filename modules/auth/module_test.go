package auth

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func setupTestAuthModule(t *testing.T) (*AuthModule, capability.AccountCapability) {
	t.Helper()

	// Create memory store
	memStore := store.NewMemoryStore()

	// Create capability manager
	capMgr := capability.NewCapabilityManager(memStore)

	// Register module
	if err := capMgr.RegisterModule("auth"); err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	// Grant account capability
	accountCap, err := capMgr.GrantAccountCapability("auth")
	if err != nil {
		t.Fatalf("failed to grant account capability: %v", err)
	}

	// Create auth module
	authMod, err := NewAuthModule(accountCap)
	if err != nil {
		t.Fatalf("failed to create auth module: %v", err)
	}

	return authMod, accountCap
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

func TestNewAuthModule(t *testing.T) {
	_, accountCap := setupTestAuthModule(t)

	t.Run("valid capability", func(t *testing.T) {
		mod, err := NewAuthModule(accountCap)
		if err != nil {
			t.Errorf("NewAuthModule() error = %v, want nil", err)
		}
		if mod == nil {
			t.Error("NewAuthModule() returned nil module")
		}
	})

	t.Run("nil capability", func(t *testing.T) {
		mod, err := NewAuthModule(nil)
		if err == nil {
			t.Error("NewAuthModule(nil) error = nil, want error")
		}
		if mod != nil {
			t.Error("NewAuthModule(nil) returned non-nil module")
		}
	})
}

func TestCreateModule(t *testing.T) {
	_, accountCap := setupTestAuthModule(t)

	t.Run("valid module", func(t *testing.T) {
		mod, err := CreateModule(accountCap)
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

func TestAuthModule_HandleCreateAccount(t *testing.T) {
	authMod, _ := setupTestAuthModule(t)

	tests := []struct {
		name    string
		msg     *MsgCreateAccount
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid create",
			msg: &MsgCreateAccount{
				Name:   "alice",
				PubKey: []byte("test-pubkey"),
				Authority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "account name mismatch",
			msg: &MsgCreateAccount{
				Name:   "alice",
				PubKey: []byte("test-pubkey"),
				Authority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			account: "bob",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := authMod.handleCreateAccount(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleCreateAccount() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleCreateAccount() returned no effects")
				}
			}
		})
	}
}

func TestAuthModule_HandleCreateAccount_InvalidMessage(t *testing.T) {
	authMod, _ := setupTestAuthModule(t)
	ctx := setupTestContext(t, "alice")

	// Pass wrong message type
	msg := &MsgUpdateAuthority{
		Name: "alice",
	}

	effects, err := authMod.handleCreateAccount(ctx, msg)
	if err == nil {
		t.Error("handleCreateAccount() with wrong message type should error")
	}
	if effects != nil {
		t.Error("handleCreateAccount() with wrong message type should return nil effects")
	}
}

func TestAuthModule_HandleCreateAccount_NilModule(t *testing.T) {
	var authMod *AuthModule
	ctx := setupTestContext(t, "alice")

	msg := &MsgCreateAccount{
		Name:   "alice",
		PubKey: []byte("test-pubkey"),
		Authority: types.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"test-pubkey": 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
	}

	effects, err := authMod.handleCreateAccount(ctx, msg)
	if err == nil {
		t.Error("handleCreateAccount() on nil module should error")
	}
	if effects != nil {
		t.Error("handleCreateAccount() on nil module should return nil effects")
	}
}

func TestAuthModule_HandleUpdateAuthority(t *testing.T) {
	authMod, accountCap := setupTestAuthModule(t)

	// Create an account first
	_, err := accountCap.CreateAccount(context.Background(), "alice", []byte("test-pubkey"))
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	tests := []struct {
		name    string
		msg     *MsgUpdateAuthority
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid update",
			msg: &MsgUpdateAuthority{
				Name: "alice",
				NewAuthority: types.Authority{
					Threshold:      2,
					KeyWeights:     map[string]uint64{"key1": 1, "key2": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "account name mismatch",
			msg: &MsgUpdateAuthority{
				Name: "alice",
				NewAuthority: types.Authority{
					Threshold:      2,
					KeyWeights:     map[string]uint64{"key1": 1, "key2": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			account: "bob",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := authMod.handleUpdateAuthority(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleUpdateAuthority() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleUpdateAuthority() returned no effects")
				}
			}
		})
	}
}

func TestAuthModule_HandleUpdateAuthority_NonExistent(t *testing.T) {
	authMod, _ := setupTestAuthModule(t)
	ctx := setupTestContext(t, "alice")

	msg := &MsgUpdateAuthority{
		Name: "alice",
		NewAuthority: types.Authority{
			Threshold:      2,
			KeyWeights:     map[string]uint64{"key1": 1, "key2": 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
	}

	effects, err := authMod.handleUpdateAuthority(ctx, msg)
	if err == nil {
		t.Error("handleUpdateAuthority() on non-existent account should error")
	}
	if effects != nil {
		t.Error("handleUpdateAuthority() on non-existent account should return nil effects")
	}
}

func TestAuthModule_HandleDeleteAccount(t *testing.T) {
	authMod, accountCap := setupTestAuthModule(t)

	// Create an account first
	_, err := accountCap.CreateAccount(context.Background(), "alice", []byte("test-pubkey"))
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	tests := []struct {
		name    string
		msg     *MsgDeleteAccount
		account types.AccountName
		wantErr bool
	}{
		{
			name: "valid delete",
			msg: &MsgDeleteAccount{
				Name: "alice",
			},
			account: "alice",
			wantErr: false,
		},
		{
			name: "account name mismatch",
			msg: &MsgDeleteAccount{
				Name: "alice",
			},
			account: "bob",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.account)
			effects, err := authMod.handleDeleteAccount(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("handleDeleteAccount() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(effects) == 0 {
					t.Error("handleDeleteAccount() returned no effects")
				}
			}
		})
	}
}

func TestAuthModule_HandleDeleteAccount_NonExistent(t *testing.T) {
	authMod, _ := setupTestAuthModule(t)
	ctx := setupTestContext(t, "alice")

	msg := &MsgDeleteAccount{
		Name: "alice",
	}

	effects, err := authMod.handleDeleteAccount(ctx, msg)
	if err == nil {
		t.Error("handleDeleteAccount() on non-existent account should error")
	}
	if effects != nil {
		t.Error("handleDeleteAccount() on non-existent account should return nil effects")
	}
}

func TestAuthModule_HandleQueryAccount(t *testing.T) {
	authMod, accountCap := setupTestAuthModule(t)

	// Create an account first
	_, err := accountCap.CreateAccount(context.Background(), "alice", []byte("test-pubkey"))
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	ctx := setupTestContext(t, "alice")

	t.Run("valid query", func(t *testing.T) {
		data := []byte("alice")
		result, err := authMod.handleQueryAccount(ctx.Context(), "/account", data)
		if err != nil {
			t.Errorf("handleQueryAccount() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryAccount() returned nil result")
		}
	})

	t.Run("invalid account name", func(t *testing.T) {
		data := []byte("ALICE")
		result, err := authMod.handleQueryAccount(ctx.Context(), "/account", data)
		if err == nil {
			t.Error("handleQueryAccount() with invalid name should error")
		}
		if result != nil {
			t.Error("handleQueryAccount() with invalid name should return nil result")
		}
	})
}

func TestAuthModule_HandleQueryNonce(t *testing.T) {
	authMod, accountCap := setupTestAuthModule(t)

	// Create an account first
	_, err := accountCap.CreateAccount(context.Background(), "alice", []byte("test-pubkey"))
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	ctx := setupTestContext(t, "alice")

	t.Run("valid query", func(t *testing.T) {
		data := []byte("alice")
		result, err := authMod.handleQueryNonce(ctx.Context(), "/nonce", data)
		if err != nil {
			t.Errorf("handleQueryNonce() error = %v, want nil", err)
		}
		if result == nil {
			t.Error("handleQueryNonce() returned nil result")
		}
	})

	t.Run("invalid account name", func(t *testing.T) {
		data := []byte("")
		result, err := authMod.handleQueryNonce(ctx.Context(), "/nonce", data)
		if err == nil {
			t.Error("handleQueryNonce() with invalid name should error")
		}
		if result != nil {
			t.Error("handleQueryNonce() with invalid name should return nil result")
		}
	})
}
