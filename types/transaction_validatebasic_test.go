package types

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// FEE VALIDATION TESTS
// =============================================================================

func TestFee_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		fee     Fee
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid fee with single coin",
			fee: Fee{
				Amount:   Coins{{Denom: "uatom", Amount: 1000}},
				GasLimit: 200000,
			},
			wantErr: false,
		},
		{
			name: "valid fee with multiple coins",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 1000},
					{Denom: "uosmo", Amount: 2000},
				},
				GasLimit: 300000,
			},
			wantErr: false,
		},
		{
			name: "valid empty fee (zero gas, no coins)",
			fee: Fee{
				Amount:   Coins{},
				GasLimit: 0,
			},
			wantErr: false,
		},
		{
			name: "valid fee with zero amount coin",
			fee: Fee{
				Amount:   Coins{{Denom: "uatom", Amount: 0}},
				GasLimit: 100000,
			},
			wantErr: false,
		},
		{
			name: "invalid - empty denom",
			fee: Fee{
				Amount:   Coins{{Denom: "", Amount: 1000}},
				GasLimit: 100000,
			},
			wantErr: true,
			errMsg:  "fee coin 0",
		},
		{
			name: "invalid - denom too long",
			fee: Fee{
				Amount:   Coins{{Denom: strings.Repeat("a", 65), Amount: 1000}},
				GasLimit: 100000,
			},
			wantErr: true,
			errMsg:  "fee coin 0",
		},
		{
			name: "invalid - too many coins",
			fee: Fee{
				Amount:   createManyCoins(MaxFeeCoins + 1),
				GasLimit: 100000,
			},
			wantErr: true,
			errMsg:  "too many fee coins",
		},
		{
			name: "valid - exactly MaxFeeCoins",
			fee: Fee{
				Amount:   createManyCoins(MaxFeeCoins),
				GasLimit: 100000,
			},
			wantErr: false,
		},
		{
			name: "invalid - duplicate denomination",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 1000},
					{Denom: "uatom", Amount: 2000},
				},
				GasLimit: 100000,
			},
			wantErr: true,
			errMsg:  "duplicate denomination",
		},
		{
			name: "invalid - duplicate denom with different amount",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 500},
					{Denom: "uosmo", Amount: 1000},
					{Denom: "uatom", Amount: 500}, // same denom as first
				},
				GasLimit: 200000,
			},
			wantErr: true,
			errMsg:  "duplicate denomination \"uatom\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fee.ValidateBasic()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// RATIO VALIDATION TESTS
// =============================================================================

func TestRatio_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		ratio   Ratio
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid - 1% slippage",
			ratio:   Ratio{Numerator: 1, Denominator: 100},
			wantErr: false,
		},
		{
			name:    "valid - zero slippage (0/1)",
			ratio:   Ratio{Numerator: 0, Denominator: 1},
			wantErr: false,
		},
		{
			name:    "valid - max values",
			ratio:   Ratio{Numerator: ^uint64(0), Denominator: ^uint64(0)},
			wantErr: false,
		},
		{
			name:    "valid - 50% slippage",
			ratio:   Ratio{Numerator: 1, Denominator: 2},
			wantErr: false,
		},
		{
			name:    "invalid - zero denominator",
			ratio:   Ratio{Numerator: 1, Denominator: 0},
			wantErr: true,
			errMsg:  "denominator cannot be zero",
		},
		{
			name:    "invalid - zero numerator with zero denominator",
			ratio:   Ratio{Numerator: 0, Denominator: 0},
			wantErr: true,
			errMsg:  "denominator cannot be zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ratio.ValidateBasic()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// TRANSACTION VALIDATION - FEE AND FEE SLIPPAGE TESTS
// =============================================================================

func TestTransaction_ValidateBasic_Fee(t *testing.T) {
	// Setup valid auth for all tests
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	auth := NewAuthorization(Signature{
		Algorithm: AlgorithmEd25519,
		PubKey:    pub,
		Signature: make([]byte, ed25519.SignatureSize),
	})

	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tests := []struct {
		name    string
		tx      *Transaction
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid transaction with proper fee and slippage",
			tx: &Transaction{
				Account:       "alice",
				Messages:      []Message{msg},
				Authorization: auth,
				Nonce:         1,
				Fee: Fee{
					Amount:   Coins{{Denom: "uatom", Amount: 1000}},
					GasLimit: 200000,
				},
				FeeSlippage: Ratio{
					Numerator:   1,
					Denominator: 100,
				},
			},
			wantErr: false,
		},
		{
			name: "valid transaction with zero fee",
			tx: &Transaction{
				Account:       "alice",
				Messages:      []Message{msg},
				Authorization: auth,
				Nonce:         1,
				Fee: Fee{
					Amount:   Coins{},
					GasLimit: 0,
				},
				FeeSlippage: Ratio{
					Numerator:   0,
					Denominator: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - fee with empty denom",
			tx: &Transaction{
				Account:       "alice",
				Messages:      []Message{msg},
				Authorization: auth,
				Nonce:         1,
				Fee: Fee{
					Amount:   Coins{{Denom: "", Amount: 1000}},
					GasLimit: 200000,
				},
				FeeSlippage: Ratio{
					Numerator:   1,
					Denominator: 100,
				},
			},
			wantErr: true,
			errMsg:  "invalid fee",
		},
		{
			name: "invalid - fee slippage with zero denominator",
			tx: &Transaction{
				Account:       "alice",
				Messages:      []Message{msg},
				Authorization: auth,
				Nonce:         1,
				Fee: Fee{
					Amount:   Coins{{Denom: "uatom", Amount: 1000}},
					GasLimit: 200000,
				},
				FeeSlippage: Ratio{
					Numerator:   1,
					Denominator: 0, // INVALID: zero denominator
				},
			},
			wantErr: true,
			errMsg:  "invalid fee_slippage",
		},
		{
			name: "invalid - too many fee coins",
			tx: &Transaction{
				Account:       "alice",
				Messages:      []Message{msg},
				Authorization: auth,
				Nonce:         1,
				Fee: Fee{
					Amount:   createManyCoins(MaxFeeCoins + 1),
					GasLimit: 200000,
				},
				FeeSlippage: Ratio{
					Numerator:   1,
					Denominator: 100,
				},
			},
			wantErr: true,
			errMsg:  "invalid fee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tx.ValidateBasic()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// EDGE CASE: ZERO DENOMINATOR BEFORE GOSSIP LAYER
// =============================================================================

// TestTransaction_ValidateBasic_ZeroDenominator_CaughtBeforeGossip verifies that
// the zero denominator issue identified in PR #50 is now caught at ValidateBasic,
// before transactions enter the gossip layer.
//
// BACKGROUND: The Tinkerer identified in PR #50 that zero denominators were not
// being validated on Transaction creation - only on SignDocRatio.ValidateBasic().
// This means malformed transactions could propagate through gossip before failing.
//
// INVARIANT: Transaction.ValidateBasic() MUST reject zero denominators in FeeSlippage.
func TestTransaction_ValidateBasic_ZeroDenominator_CaughtBeforeGossip(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	auth := NewAuthorization(Signature{
		Algorithm: AlgorithmEd25519,
		PubKey:    pub,
		Signature: make([]byte, ed25519.SignatureSize),
	})

	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	// Create transaction with zero denominator - this is the exact case from PR #50
	tx := &Transaction{
		Account:       "alice",
		Messages:      []Message{msg},
		Authorization: auth,
		Nonce:         1,
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 1000}},
			GasLimit: 200000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 0, // Zero denominator - would cause issues downstream
		},
	}

	// CRITICAL: ValidateBasic must catch this BEFORE the transaction enters gossip
	err = tx.ValidateBasic()

	assert.Error(t, err, "ValidateBasic MUST reject zero denominator in FeeSlippage")
	assert.Contains(t, err.Error(), "invalid fee_slippage")
	assert.Contains(t, err.Error(), "denominator cannot be zero")

	t.Log("Zero denominator is caught at ValidateBasic, before gossip layer")
}

// =============================================================================
// DUPLICATE DENOMINATION TESTS (Issue #91)
// =============================================================================

// TestFee_ValidateBasic_DuplicateDenom_Rejected documents and tests the invariant
// that fee coins must not contain duplicate denominations.
//
// RATIONALE: Duplicate denominations create ambiguity in fee calculations.
// Consider: Fee{Amount: [{uatom: 1000}, {uatom: 2000}]}
//
// Without this validation, downstream code faces undefined behavior:
// - Is the total 1000 or 2000 or 3000?
// - Which value takes precedence?
// - How should fee deduction handle this?
//
// INVARIANT: Fee.Amount defines a unique mapping from denomination to amount.
// PROOF: By rejecting duplicates at validation time, we guarantee that for any
// denom d, at most one Coin in Amount has Denom == d.
//
// This was identified in PR #84 review by The Tinkerer.
func TestFee_ValidateBasic_DuplicateDenom_Rejected(t *testing.T) {
	tests := []struct {
		name   string
		fee    Fee
		errMsg string
	}{
		{
			name: "exact duplicate - same denom same amount",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 1000},
					{Denom: "uatom", Amount: 1000},
				},
				GasLimit: 100000,
			},
			errMsg: "duplicate denomination \"uatom\"",
		},
		{
			name: "duplicate with different amounts",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 1000},
					{Denom: "uatom", Amount: 2000},
				},
				GasLimit: 100000,
			},
			errMsg: "duplicate denomination \"uatom\"",
		},
		{
			name: "duplicate in middle of list",
			fee: Fee{
				Amount: Coins{
					{Denom: "aaa", Amount: 100},
					{Denom: "bbb", Amount: 200},
					{Denom: "bbb", Amount: 300}, // duplicate
					{Denom: "ccc", Amount: 400},
				},
				GasLimit: 100000,
			},
			errMsg: "duplicate denomination \"bbb\"",
		},
		{
			name: "duplicate at end of list",
			fee: Fee{
				Amount: Coins{
					{Denom: "aaa", Amount: 100},
					{Denom: "bbb", Amount: 200},
					{Denom: "ccc", Amount: 300},
					{Denom: "aaa", Amount: 400}, // duplicate of first
				},
				GasLimit: 100000,
			},
			errMsg: "duplicate denomination \"aaa\"",
		},
		{
			name: "multiple duplicates - reports first found",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 100},
					{Denom: "uatom", Amount: 200}, // first duplicate
					{Denom: "uosmo", Amount: 300},
					{Denom: "uosmo", Amount: 400}, // second duplicate
				},
				GasLimit: 100000,
			},
			errMsg: "duplicate denomination \"uatom\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fee.ValidateBasic()

			assert.Error(t, err, "duplicate denominations MUST be rejected")
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestFee_ValidateBasic_UniqueDenoms_Accepted verifies that fees with unique
// denominations pass validation (counterexamples to the duplicate rejection).
func TestFee_ValidateBasic_UniqueDenoms_Accepted(t *testing.T) {
	tests := []struct {
		name string
		fee  Fee
	}{
		{
			name: "single coin",
			fee: Fee{
				Amount:   Coins{{Denom: "uatom", Amount: 1000}},
				GasLimit: 100000,
			},
		},
		{
			name: "multiple distinct denoms",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 1000},
					{Denom: "uosmo", Amount: 2000},
					{Denom: "ujuno", Amount: 3000},
				},
				GasLimit: 100000,
			},
		},
		{
			name: "empty coins (zero fee)",
			fee: Fee{
				Amount:   Coins{},
				GasLimit: 0,
			},
		},
		{
			name: "similar but distinct denoms",
			fee: Fee{
				Amount: Coins{
					{Denom: "uatom", Amount: 1000},
					{Denom: "atom", Amount: 1000},   // different (no 'u' prefix)
					{Denom: "uatom2", Amount: 1000}, // different (has suffix)
				},
				GasLimit: 100000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fee.ValidateBasic()
			assert.NoError(t, err, "unique denominations MUST be accepted")
		})
	}
}

// =============================================================================
// BOUNDARY TESTS
// =============================================================================

func TestFee_ValidateBasic_BoundaryConditions(t *testing.T) {
	// Test exactly at MaxFeeCoins boundary
	t.Run("exactly MaxFeeCoins coins", func(t *testing.T) {
		fee := Fee{
			Amount:   createManyCoins(MaxFeeCoins),
			GasLimit: 100000,
		}
		err := fee.ValidateBasic()
		assert.NoError(t, err, "exactly MaxFeeCoins should be valid")
	})

	// Test one over MaxFeeCoins boundary
	t.Run("MaxFeeCoins + 1 coins", func(t *testing.T) {
		fee := Fee{
			Amount:   createManyCoins(MaxFeeCoins + 1),
			GasLimit: 100000,
		}
		err := fee.ValidateBasic()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many fee coins")
	})

	// Test denom exactly at 64 chars (valid)
	t.Run("denom exactly 64 chars", func(t *testing.T) {
		fee := Fee{
			Amount:   Coins{{Denom: strings.Repeat("a", 64), Amount: 1000}},
			GasLimit: 100000,
		}
		err := fee.ValidateBasic()
		assert.NoError(t, err, "64-char denom should be valid")
	})

	// Test denom at 65 chars (invalid)
	t.Run("denom 65 chars", func(t *testing.T) {
		fee := Fee{
			Amount:   Coins{{Denom: strings.Repeat("a", 65), Amount: 1000}},
			GasLimit: 100000,
		}
		err := fee.ValidateBasic()
		assert.Error(t, err)
	})
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// createManyCoins creates n valid coins with unique denoms
func createManyCoins(n int) Coins {
	coins := make(Coins, n)
	for i := 0; i < n; i++ {
		coins[i] = Coin{
			Denom:  strings.Repeat("a", i%64+1) + string(rune('a'+i%26)), // unique denoms
			Amount: uint64(i + 1),
		}
	}
	return coins
}
