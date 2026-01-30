package types

import (
	"fmt"
	"sort"
	"strings"
)

// Coin represents a single token with denomination and amount
type Coin struct {
	Denom  string `json:"denom"`
	Amount uint64 `json:"amount"`
}

// NewCoin creates a new coin with validation
func NewCoin(denom string, amount uint64) Coin {
	return Coin{Denom: denom, Amount: amount}
}

// IsValid checks if the coin is valid
func (c Coin) IsValid() bool {
	return c.Denom != "" && len(c.Denom) <= 64
}

// IsZero returns true if the coin amount is zero
func (c Coin) IsZero() bool {
	return c.Amount == 0
}

// IsPositive returns true if the coin amount is positive
func (c Coin) IsPositive() bool {
	return c.Amount > 0
}

// String returns a string representation of the coin
func (c Coin) String() string {
	return fmt.Sprintf("%d%s", c.Amount, c.Denom)
}

// Coins represents a collection of coins
type Coins []Coin

// NewCoins creates a new Coins collection from a list of coins
func NewCoins(coins ...Coin) Coins {
	return Coins(coins)
}

// IsValid checks if all coins are valid and properly sorted
func (coins Coins) IsValid() bool {
	// Check each coin is valid
	for _, coin := range coins {
		if !coin.IsValid() {
			return false
		}
	}

	// Check for duplicates and ensure sorted
	seenDenoms := make(map[string]bool)
	var prevDenom string
	for i, coin := range coins {
		// Check for duplicates
		if seenDenoms[coin.Denom] {
			return false
		}
		seenDenoms[coin.Denom] = true

		// Check sorted (lexicographic)
		if i > 0 && strings.Compare(coin.Denom, prevDenom) <= 0 {
			return false
		}
		prevDenom = coin.Denom
	}

	return true
}

// IsZero returns true if there are no coins or all amounts are zero
func (coins Coins) IsZero() bool {
	for _, coin := range coins {
		if !coin.IsZero() {
			return false
		}
	}
	return true
}

// IsAllPositive returns true if all coins have positive amounts
func (coins Coins) IsAllPositive() bool {
	if len(coins) == 0 {
		return false
	}
	for _, coin := range coins {
		if !coin.IsPositive() {
			return false
		}
	}
	return true
}

// AmountOf returns the amount of a specific denomination
func (coins Coins) AmountOf(denom string) uint64 {
	for _, coin := range coins {
		if coin.Denom == denom {
			return coin.Amount
		}
	}
	return 0
}

// Add adds two Coins collections
func (coins Coins) Add(other Coins) Coins {
	result := make(map[string]uint64)

	// Add all coins from first collection
	for _, coin := range coins {
		result[coin.Denom] += coin.Amount
	}

	// Add all coins from second collection
	for _, coin := range other {
		result[coin.Denom] += coin.Amount
	}

	// Convert back to Coins
	merged := make(Coins, 0, len(result))
	for denom, amount := range result {
		if amount > 0 {
			merged = append(merged, Coin{Denom: denom, Amount: amount})
		}
	}

	// Sort by denomination
	sort.Slice(merged, func(i, j int) bool {
		return strings.Compare(merged[i].Denom, merged[j].Denom) < 0
	})

	return merged
}

// Sub subtracts other from coins
// Returns error if result would be negative
func (coins Coins) Sub(other Coins) (Coins, error) {
	result := make(map[string]uint64)

	// Add all coins from first collection
	for _, coin := range coins {
		result[coin.Denom] = coin.Amount
	}

	// Subtract coins from second collection
	for _, coin := range other {
		if result[coin.Denom] < coin.Amount {
			return nil, ErrInsufficientFunds
		}
		result[coin.Denom] -= coin.Amount
	}

	// Convert back to Coins
	subtracted := make(Coins, 0, len(result))
	for denom, amount := range result {
		if amount > 0 {
			subtracted = append(subtracted, Coin{Denom: denom, Amount: amount})
		}
	}

	// Sort by denomination
	sort.Slice(subtracted, func(i, j int) bool {
		return strings.Compare(subtracted[i].Denom, subtracted[j].Denom) < 0
	})

	return subtracted, nil
}

// IsAllGTE returns true if coins >= other for all denominations
func (coins Coins) IsAllGTE(other Coins) bool {
	for _, coin := range other {
		if coins.AmountOf(coin.Denom) < coin.Amount {
			return false
		}
	}
	return true
}

// String returns a string representation of the coins
func (coins Coins) String() string {
	if len(coins) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, coin := range coins {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(coin.String())
	}
	return sb.String()
}

// Sort sorts coins by denomination
func (coins Coins) Sort() Coins {
	sorted := make(Coins, len(coins))
	copy(sorted, coins)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.Compare(sorted[i].Denom, sorted[j].Denom) < 0
	})
	return sorted
}
