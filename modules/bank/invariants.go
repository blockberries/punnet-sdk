package bank

import (
	"context"
	"fmt"
)

// SupplyTotal returns the sum of every balance the bank module
// currently tracks for the given denom. Used as the LHS of the
// supply-conservation invariant: after every block in a tokenomics
// chain, SupplyTotal(baseDenom) == TokenomicsGenesis.TotalSupply.
//
// The summation iterates BAPIBalanceStore.TrackedKeys, so test
// callers must ensure the in-memory key index has been populated
// (typically by either touching every balance once during setup or
// by running BAPIBalanceStore.RebuildIndex). The bank module's
// own InitGenesis touches every seeded balance, so chains using
// the standard genesis path get this for free.
//
// Returns an error only on transient store failures; an empty
// store legitimately returns (0, nil).
func (m *BAPIBankModule) SupplyTotal(ctx context.Context, denom string) (uint64, error) {
	if m == nil || m.balanceStore == nil {
		return 0, fmt.Errorf("bank module not initialized")
	}
	if denom == "" {
		return 0, fmt.Errorf("denom must be non-empty")
	}

	var sum uint64
	for _, k := range m.balanceStore.TrackedKeys() {
		account, d, ok := splitBalanceKey(k)
		if !ok || d != denom {
			continue
		}
		amount, err := m.balanceStore.GetAmount(ctx, account, denom)
		if err != nil {
			return 0, fmt.Errorf("get balance %s/%s: %w", account, denom, err)
		}
		next := sum + amount
		if next < sum {
			return 0, fmt.Errorf("supply total overflow at %s/%s", account, denom)
		}
		sum = next
	}
	return sum, nil
}

// AssertSupplyConserved verifies that SupplyTotal(denom) equals
// expected. Used by chain integration tests that run a sequence of
// transactions and want a single line to confirm the per-tx
// invariant held through every block. Returns nil when the
// invariant holds; otherwise returns a descriptive error.
//
// Phase 1.6 of the tokenomics SDK plan. Designed as a test fixture
// — emphatically NOT a runtime hot-path check: iterating every
// tracked balance is O(N_accounts) per call.
func (m *BAPIBankModule) AssertSupplyConserved(ctx context.Context, denom string, expected uint64) error {
	actual, err := m.SupplyTotal(ctx, denom)
	if err != nil {
		return fmt.Errorf("compute supply total: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("supply conservation violated: expected %d but accounts sum to %d (delta %+d)",
			expected, actual, int64(actual)-int64(expected))
	}
	return nil
}
