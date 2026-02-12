package effects

import (
	"fmt"
	"math"

	"github.com/blockberries/punnet-sdk/types"
)

// MaxTransferAmount is the maximum amount that can be transferred in a single effect.
// This prevents overflow during balance calculations.
const MaxTransferAmount = math.MaxUint64 / 2

// TransferEffect represents a token transfer effect
type TransferEffect struct {
	// From is the sender account
	From types.AccountName

	// To is the recipient account
	To types.AccountName

	// Amount is the coins to transfer
	Amount types.Coins
}

// Type returns the effect type
func (e TransferEffect) Type() EffectType {
	return EffectTypeTransfer
}

// Validate performs validation including overflow checks
func (e TransferEffect) Validate() error {
	if !e.From.IsValid() {
		return fmt.Errorf("invalid from account: %s", e.From)
	}
	if !e.To.IsValid() {
		return fmt.Errorf("invalid to account: %s", e.To)
	}
	if e.From == e.To {
		return fmt.Errorf("cannot transfer to self")
	}

	// Check for duplicate denominations first (before IsValid which also checks this)
	// to provide a more specific error message
	seen := make(map[string]bool)
	for _, coin := range e.Amount {
		if seen[coin.Denom] {
			return fmt.Errorf("duplicate denomination %s in transfer", coin.Denom)
		}
		seen[coin.Denom] = true
	}

	if !e.Amount.IsValid() {
		return fmt.Errorf("invalid amount")
	}
	if !e.Amount.IsAllPositive() {
		return fmt.Errorf("amount must be positive")
	}

	// Check for amounts that could cause overflow
	for _, coin := range e.Amount {
		if coin.Amount > MaxTransferAmount {
			return fmt.Errorf("amount %d for %s exceeds maximum safe transfer amount %d",
				coin.Amount, coin.Denom, MaxTransferAmount)
		}
	}

	return nil
}

// SafeAdd adds two uint64 values with overflow checking.
// Returns the result and true if successful, or 0 and false if overflow would occur.
func SafeAdd(a, b uint64) (uint64, bool) {
	result := a + b
	if result < a {
		return 0, false
	}
	return result, true
}

// SafeSub subtracts two uint64 values with underflow checking.
// Returns the result and true if successful, or 0 and false if underflow would occur.
func SafeSub(a, b uint64) (uint64, bool) {
	if b > a {
		return 0, false
	}
	return a - b, true
}

// Dependencies returns the dependencies
func (e TransferEffect) Dependencies() []Dependency {
	deps := make([]Dependency, 0, 2+len(e.Amount))

	// From account dependency
	deps = append(deps, Dependency{
		Type:     DependencyTypeAccount,
		Key:      []byte(e.From),
		ReadOnly: false,
	})

	// To account dependency
	deps = append(deps, Dependency{
		Type:     DependencyTypeAccount,
		Key:      []byte(e.To),
		ReadOnly: false,
	})

	// Balance dependencies for each denomination
	for _, coin := range e.Amount {
		// From balance
		deps = append(deps, Dependency{
			Type:     DependencyTypeBalance,
			Key:      balanceKey(e.From, coin.Denom),
			ReadOnly: false,
		})

		// To balance
		deps = append(deps, Dependency{
			Type:     DependencyTypeBalance,
			Key:      balanceKey(e.To, coin.Denom),
			ReadOnly: false,
		})
	}

	return deps
}

// Key returns the primary key (from account for conflict detection)
func (e TransferEffect) Key() []byte {
	// Use from account as primary key
	return []byte(e.From)
}

// balanceKey constructs a balance key from account and denomination
func balanceKey(account types.AccountName, denom string) []byte {
	return []byte(fmt.Sprintf("balance/%s/%s", account, denom))
}
