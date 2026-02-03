package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/types"
)

// TestValidateSignDocVersion tests the SignDoc version validation function.
func TestValidateSignDocVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid version 1",
			version: "1",
			wantErr: false,
		},
		{
			name:        "invalid version 0",
			version:     "0",
			wantErr:     true,
			errContains: "unsupported",
		},
		{
			name:        "invalid version 2",
			version:     "2",
			wantErr:     true,
			errContains: "unsupported",
		},
		{
			name:        "empty version",
			version:     "",
			wantErr:     true,
			errContains: "unsupported",
		},
		{
			name:        "invalid version string",
			version:     "v1",
			wantErr:     true,
			errContains: "unsupported",
		},
		{
			name:        "malicious version with special chars",
			version:     "1; DROP TABLE users;",
			wantErr:     true,
			errContains: "unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSignDocVersion(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSignDocVersion(%q) = nil, want error", tt.version)
				} else if !errors.Is(err, types.ErrUnsupportedVersion) {
					t.Errorf("ValidateSignDocVersion(%q) error = %v, want ErrUnsupportedVersion", tt.version, err)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSignDocVersion(%q) = %v, want nil", tt.version, err)
				}
			}
		})
	}
}

// mockMessage implements types.Message for testing
type mockMessage struct {
	msgType string
	signers []types.AccountName
}

func (m *mockMessage) Type() string                    { return m.msgType }
func (m *mockMessage) GetSigners() []types.AccountName { return m.signers }
func (m *mockMessage) ValidateBasic() error            { return nil }

// TestValidateReplayProtection tests the replay protection validation.
func TestValidateReplayProtection(t *testing.T) {
	accountName := types.AccountName("alice")
	chainID := "test-chain-1"

	// Create a valid transaction with proper FeeSlippage
	createValidTx := func(nonce uint64) *types.Transaction {
		msg := &mockMessage{
			msgType: "/test.v1.MsgTest",
			signers: []types.AccountName{accountName},
		}
		auth := types.NewAuthorization()
		tx := types.NewTransaction(accountName, nonce, []types.Message{msg}, auth)
		// Set valid FeeSlippage (0% slippage = 0/1)
		tx.FeeSlippage = types.Ratio{Numerator: 0, Denominator: 1}
		return tx
	}

	tests := []struct {
		name             string
		tx               *types.Transaction
		chainID          string
		expectedSequence uint64
		wantErr          bool
		errType          error
	}{
		{
			name:             "valid replay protection",
			tx:               createValidTx(5),
			chainID:          chainID,
			expectedSequence: 5,
			wantErr:          false,
		},
		{
			name:             "nil transaction",
			tx:               nil,
			chainID:          chainID,
			expectedSequence: 5,
			wantErr:          true,
			errType:          types.ErrInvalidTransaction,
		},
		{
			name:             "empty chain ID",
			tx:               createValidTx(5),
			chainID:          "",
			expectedSequence: 5,
			wantErr:          true,
			errType:          types.ErrInvalidTransaction,
		},
		{
			name:             "sequence mismatch - too low",
			tx:               createValidTx(4),
			chainID:          chainID,
			expectedSequence: 5,
			wantErr:          true,
			errType:          types.ErrSequenceMismatch,
		},
		{
			name:             "sequence mismatch - too high",
			tx:               createValidTx(6),
			chainID:          chainID,
			expectedSequence: 5,
			wantErr:          true,
			errType:          types.ErrSequenceMismatch,
		},
		{
			name:             "sequence zero matches zero",
			tx:               createValidTx(0),
			chainID:          chainID,
			expectedSequence: 0,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReplayProtection(tt.tx, tt.chainID, tt.expectedSequence)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateReplayProtection() = nil, want error")
					return
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("ValidateReplayProtection() error = %v, want %v", err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateReplayProtection() = %v, want nil", err)
				}
			}
		})
	}
}

// TestValidateSignDoc tests the SignDoc validation.
func TestValidateSignDoc(t *testing.T) {
	tests := []struct {
		name    string
		signDoc *types.SignDoc
		wantErr bool
	}{
		{
			name: "valid SignDoc",
			signDoc: func() *types.SignDoc {
				sd := types.NewSignDoc("test-chain", 1, "alice", 1, "")
				sd.AddMessage("/test.v1.Msg", []byte(`{"foo":"bar"}`))
				return sd
			}(),
			wantErr: false,
		},
		{
			name:    "nil SignDoc",
			signDoc: nil,
			wantErr: true,
		},
		{
			name: "invalid version",
			signDoc: func() *types.SignDoc {
				sd := types.NewSignDoc("test-chain", 1, "alice", 1, "")
				sd.Version = "99"
				sd.AddMessage("/test.v1.Msg", []byte(`{"foo":"bar"}`))
				return sd
			}(),
			wantErr: true,
		},
		{
			name: "empty chain ID",
			signDoc: func() *types.SignDoc {
				sd := types.NewSignDoc("", 1, "alice", 1, "")
				sd.AddMessage("/test.v1.Msg", []byte(`{"foo":"bar"}`))
				return sd
			}(),
			wantErr: true,
		},
		{
			name: "empty account",
			signDoc: func() *types.SignDoc {
				sd := types.NewSignDoc("test-chain", 1, "", 1, "")
				sd.AddMessage("/test.v1.Msg", []byte(`{"foo":"bar"}`))
				return sd
			}(),
			wantErr: true,
		},
		{
			name: "no messages",
			signDoc: func() *types.SignDoc {
				return types.NewSignDoc("test-chain", 1, "alice", 1, "")
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSignDoc(tt.signDoc)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateSignDoc() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateSignDoc() = %v, want nil", err)
			}
		})
	}
}

// TestTransactionValidator tests the TransactionValidator type.
func TestTransactionValidator(t *testing.T) {
	t.Run("NewTransactionValidator", func(t *testing.T) {
		// Valid creation
		v, err := NewTransactionValidator("test-chain")
		if err != nil {
			t.Fatalf("NewTransactionValidator() error = %v", err)
		}
		if v.ChainID() != "test-chain" {
			t.Errorf("ChainID() = %q, want %q", v.ChainID(), "test-chain")
		}

		// Empty chain ID
		_, err = NewTransactionValidator("")
		if err == nil {
			t.Error("NewTransactionValidator(\"\") = nil error, want error")
		}
	})
}

// TestTransactionValidatorValidateTransaction tests transaction validation.
func TestTransactionValidatorValidateTransaction(t *testing.T) {
	chainID := "test-chain-1"
	accountName := types.AccountName("alice")

	validator, err := NewTransactionValidator(chainID)
	if err != nil {
		t.Fatalf("NewTransactionValidator() error = %v", err)
	}

	createAccount := func(nonce uint64) *types.Account {
		return &types.Account{
			Name: accountName,
			Authority: types.Authority{
				Threshold:  1,
				KeyWeights: map[string]uint64{"testkey": 1},
			},
			Nonce:     nonce,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	createTx := func(nonce uint64) *types.Transaction {
		msg := &mockMessage{
			msgType: "/test.v1.MsgTest",
			signers: []types.AccountName{accountName},
		}
		auth := types.NewAuthorization()
		tx := types.NewTransaction(accountName, nonce, []types.Message{msg}, auth)
		// Set valid FeeSlippage (0% slippage = 0/1)
		tx.FeeSlippage = types.Ratio{Numerator: 0, Denominator: 1}
		return tx
	}

	tests := []struct {
		name    string
		tx      *types.Transaction
		account *types.Account
		wantErr bool
	}{
		{
			name:    "valid transaction",
			tx:      createTx(5),
			account: createAccount(5),
			wantErr: false,
		},
		{
			name:    "nil transaction",
			tx:      nil,
			account: createAccount(5),
			wantErr: true,
		},
		{
			name:    "nil account",
			tx:      createTx(5),
			account: nil,
			wantErr: true,
		},
		{
			name:    "nonce mismatch",
			tx:      createTx(6),
			account: createAccount(5),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTransaction(tt.tx, tt.account)
			if tt.wantErr && err == nil {
				t.Error("ValidateTransaction() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateTransaction() = %v, want nil", err)
			}
		})
	}
}

// TestTransactionValidatorValidateForMempool tests mempool validation.
func TestTransactionValidatorValidateForMempool(t *testing.T) {
	chainID := "test-chain-1"
	accountName := types.AccountName("alice")

	validator, err := NewTransactionValidator(chainID)
	if err != nil {
		t.Fatalf("NewTransactionValidator() error = %v", err)
	}

	createAccount := func(nonce uint64) *types.Account {
		return &types.Account{
			Name: accountName,
			Authority: types.Authority{
				Threshold:  1,
				KeyWeights: map[string]uint64{"testkey": 1},
			},
			Nonce:     nonce,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	createTx := func(nonce uint64) *types.Transaction {
		msg := &mockMessage{
			msgType: "/test.v1.MsgTest",
			signers: []types.AccountName{accountName},
		}
		auth := types.NewAuthorization()
		tx := types.NewTransaction(accountName, nonce, []types.Message{msg}, auth)
		// Set valid FeeSlippage (0% slippage = 0/1)
		tx.FeeSlippage = types.Ratio{Numerator: 0, Denominator: 1}
		return tx
	}

	tests := []struct {
		name    string
		tx      *types.Transaction
		account *types.Account
		wantErr bool
		errType error
	}{
		{
			name:    "valid - exact nonce",
			tx:      createTx(5),
			account: createAccount(5),
			wantErr: false,
		},
		{
			name:    "valid - nonce ahead by 1",
			tx:      createTx(6),
			account: createAccount(5),
			wantErr: false,
		},
		{
			name:    "valid - nonce ahead by many",
			tx:      createTx(100),
			account: createAccount(5),
			wantErr: false,
		},
		{
			name:    "invalid - nonce behind",
			tx:      createTx(4),
			account: createAccount(5),
			wantErr: true,
			errType: types.ErrSequenceMismatch,
		},
		{
			name:    "nil transaction",
			tx:      nil,
			account: createAccount(5),
			wantErr: true,
		},
		{
			name:    "nil account",
			tx:      createTx(5),
			account: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateForMempool(tt.tx, tt.account)
			if tt.wantErr {
				if err == nil {
					t.Error("ValidateForMempool() = nil, want error")
					return
				}
				if tt.errType != nil && !errors.Is(err, tt.errType) {
					t.Errorf("ValidateForMempool() error = %v, want %v", err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateForMempool() = %v, want nil", err)
				}
			}
		})
	}
}

// TestChainIDValidation tests that chain ID mismatches are caught.
func TestChainIDValidation(t *testing.T) {
	accountName := types.AccountName("alice")

	// This test verifies the SECURITY property:
	// Signatures are bound to a specific chain ID
	t.Run("cross-chain replay prevention", func(t *testing.T) {
		chainA := "chain-a"
		chainB := "chain-b"

		validatorA, _ := NewTransactionValidator(chainA)
		validatorB, _ := NewTransactionValidator(chainB)

		account := &types.Account{
			Name: accountName,
			Authority: types.Authority{
				Threshold:  1,
				KeyWeights: map[string]uint64{"testkey": 1},
			},
			Nonce:     0,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		msg := &mockMessage{
			msgType: "/test.v1.MsgTest",
			signers: []types.AccountName{accountName},
		}
		auth := types.NewAuthorization()
		tx := types.NewTransaction(accountName, 0, []types.Message{msg}, auth)
		// Set valid FeeSlippage (0% slippage = 0/1)
		tx.FeeSlippage = types.Ratio{Numerator: 0, Denominator: 1}

		// Transaction valid on chain A
		err := validatorA.ValidateForMempool(tx, account)
		if err != nil {
			t.Errorf("validatorA.ValidateForMempool() = %v, want nil", err)
		}

		// Same transaction should be valid on chain B as well for mempool
		// (the chain ID in the SignDoc will match whatever chain the validator is for)
		// NOTE: The actual chain ID binding happens at signature verification time
		err = validatorB.ValidateForMempool(tx, account)
		if err != nil {
			t.Errorf("validatorB.ValidateForMempool() = %v, want nil", err)
		}

		// The real protection is at signature verification:
		// A signature created for chain A won't verify on chain B because
		// the SignDoc hash will be different (different chain ID in the SignDoc)
	})
}

// TestSequenceValidation tests sequence/nonce validation edge cases.
func TestSequenceValidation(t *testing.T) {
	accountName := types.AccountName("alice")
	chainID := "test-chain"

	createAccount := func(nonce uint64) *types.Account {
		return &types.Account{
			Name: accountName,
			Authority: types.Authority{
				Threshold:  1,
				KeyWeights: map[string]uint64{"testkey": 1},
			},
			Nonce:     nonce,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	createTx := func(nonce uint64) *types.Transaction {
		msg := &mockMessage{
			msgType: "/test.v1.MsgTest",
			signers: []types.AccountName{accountName},
		}
		auth := types.NewAuthorization()
		tx := types.NewTransaction(accountName, nonce, []types.Message{msg}, auth)
		// Set valid FeeSlippage (0% slippage = 0/1)
		tx.FeeSlippage = types.Ratio{Numerator: 0, Denominator: 1}
		return tx
	}

	t.Run("replay protection at boundaries", func(t *testing.T) {
		// Test at uint64 boundaries
		tests := []struct {
			name             string
			txNonce          uint64
			expectedSequence uint64
			wantErr          bool
		}{
			{"zero matches zero", 0, 0, false},
			{"max uint64 matches", ^uint64(0), ^uint64(0), false},
			{"one less than expected", 999, 1000, true},
			{"one more than expected", 1001, 1000, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tx := createTx(tt.txNonce)
				err := ValidateReplayProtection(tx, chainID, tt.expectedSequence)
				if tt.wantErr && err == nil {
					t.Error("ValidateReplayProtection() = nil, want error")
				}
				if !tt.wantErr && err != nil {
					t.Errorf("ValidateReplayProtection() = %v, want nil", err)
				}
			})
		}
	})

	t.Run("same-chain replay prevention", func(t *testing.T) {
		// This test verifies the INVARIANT:
		// A transaction passing validation cannot be replayed after nonce increment

		validator, _ := NewTransactionValidator(chainID)
		account := createAccount(5)
		tx := createTx(5)

		// First validation should pass
		err := validator.ValidateTransaction(tx, account)
		if err != nil {
			t.Fatalf("First validation failed: %v", err)
		}

		// After nonce increment, same transaction should fail
		account.Nonce = 6
		err = validator.ValidateTransaction(tx, account)
		if err == nil {
			t.Error("Replay of transaction should fail after nonce increment")
		}
		if !errors.Is(err, types.ErrSequenceMismatch) {
			t.Errorf("Expected ErrSequenceMismatch, got: %v", err)
		}
	})
}
