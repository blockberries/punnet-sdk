package effects

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

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

// Validate performs validation
func (e TransferEffect) Validate() error {
	if !e.From.IsValid() {
		return fmt.Errorf("invalid from account: %s", e.From)
	}
	if !e.To.IsValid() {
		return fmt.Errorf("invalid to account: %s", e.To)
	}
	if !e.Amount.IsValid() {
		return fmt.Errorf("invalid amount")
	}
	if !e.Amount.IsAllPositive() {
		return fmt.Errorf("amount must be positive")
	}
	return nil
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
