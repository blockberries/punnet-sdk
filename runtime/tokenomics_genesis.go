package runtime

import (
	"fmt"

	ptypes "github.com/blockberries/punnet-sdk/types"
)

// Tokenomics constitutional constants. The five allocation percentages
// sum to 100% — that's a hard invariant; if you change one, change at
// least one other to compensate. PLAN §7 decision D7 (Constitutional
// tier): these are NOT genesis-tunable and NOT governance-tunable; they
// can only change via a binary upgrade plus a supermajority vote with
// the 60-day timelock per spec §10's amend table.
//
// Units: basis points of 10000 (10000 = 100%).
const (
	// AllocPctVRP is the validator-rewards pool share — drained per-
	// block by the mint module's emission curve.
	AllocPctVRP uint64 = 2500 // 25%

	// AllocPctCT is the common-treasury share — destination of all
	// fee revenue (op + byte components) and slashed funds.
	AllocPctCT uint64 = 3000 // 30%

	// AllocPctAirdrop is the airdrop share — pre-distributed at
	// genesis to specific recipient accounts (carried in genesis
	// itself; the percentage just budgets the pool).
	AllocPctAirdrop uint64 = 3000 // 30%

	// AllocPctEcosystem is the ecosystem-fund share — vests linearly
	// over 4 years from genesis.
	AllocPctEcosystem uint64 = 1000 // 10%

	// AllocPctBootstrap is the bootstrap-lockup share — equal-split
	// across the bootstrap validators, locked for 12 months then
	// vested linearly over 30 days.
	AllocPctBootstrap uint64 = 500 // 5%

	// allocPctTotal is the sum of all allocation percentages. Must
	// equal 10000 (100% in basis points). Compile-time enforced via
	// the static assertion below.
	allocPctTotal = AllocPctVRP + AllocPctCT + AllocPctAirdrop +
		AllocPctEcosystem + AllocPctBootstrap

	// BootstrapLockBlocks is the chain-wide bootstrap-validator
	// lock-up duration in blocks. At 1-second block cadence this is
	// 12 months ≈ 365.25 days. PLAN §7 decision D17/D19.
	BootstrapLockBlocks uint64 = 365 * 24 * 60 * 60

	// BootstrapVestBlocks is the duration over which BL vests
	// linearly after the lockup expires. 30 days at 1-second
	// blocks.
	BootstrapVestBlocks uint64 = 30 * 24 * 60 * 60

	// BootstrapCommission is the pinned commission rate for
	// bootstrap validators (basis points). 5% is both the spec's
	// floor (c_min) and the bootstrap cap — they intersect at a
	// single point. PLAN §7 decision D17.
	BootstrapCommission uint64 = 500 // 5%
)

// Static assertion that allocation percentages sum to 100%. Triggers
// a compile-time error if a future edit breaks the invariant.
var _ [1]struct{} = [allocPctTotal - 9999]struct{}{} // pads to length 1 when sum == 10000

// BootstrapValidator describes one bootstrap validator at genesis.
// Each bootstrap validator gets an equal share of AllocPctBootstrap
// of the total supply, locked for BootstrapLockBlocks and then
// vested linearly over BootstrapVestBlocks. PLAN §7 decision D22.
//
// Bootstrap commission is pinned at BootstrapCommission for the
// lockup duration; after the lockup expires the validator may set
// commission freely but never below c_min.
type BootstrapValidator struct {
	// Name is the bootstrap validator's account name. Must be a
	// valid AccountName (12 chars max, alphanumeric).
	Name ptypes.AccountName `json:"name"`

	// PubKey is the validator's hex-encoded Ed25519 public key.
	// Same format as the bapi ValidatorUpdate.PubKey.Data field.
	PubKey []byte `json:"pub_key"`
}

// TokenomicsGenesis carries the chain-wide tokenomics parameters
// that must be known at chain construction time. Set in
// BAPIGenesisState.Tokenomics; nil means the runtime does not enforce
// the tokenomics supply model (acceptable for non-tokenomics test
// apps; mandatory for chains running the full tokenomics module set).
//
// PLAN §7 Phase 0.5 / decision D7. The allocation percentages
// themselves are constitutional (above), not represented here;
// downstream genesis initialisation computes the four protocol-account
// initial balances from TotalSupply × Alloc... constants.
type TokenomicsGenesis struct {
	// TotalSupply is the chain's fixed total supply in the base
	// unit (6-decimal micro-tokens, per PLAN §7 decision D18). Set
	// at genesis and immutable thereafter (Genesis-tunable tier).
	TotalSupply uint64 `json:"total_supply"`

	// BootstrapValidators is the explicit list of bootstrap validators.
	// Each gets an equal share of AllocPctBootstrap of TotalSupply,
	// locked for BootstrapLockBlocks then vested over BootstrapVestBlocks.
	// May be empty when running a non-bootstrap chain (in which case
	// AllocPctBootstrap is added to AllocPctCT — TBD; flagged for
	// Phase 2.4 when bootstrap state actually gets implemented).
	BootstrapValidators []BootstrapValidator `json:"bootstrap_validators"`
}

// ValidateBasic enforces stateless invariants on the TokenomicsGenesis
// payload. Called from BAPIApplication.initGenesis before any module
// genesis blob is processed.
func (tg *TokenomicsGenesis) ValidateBasic() error {
	if tg == nil {
		return nil // optional; nil means tokenomics not enforced
	}
	if tg.TotalSupply == 0 {
		return fmt.Errorf("total_supply must be > 0")
	}
	seen := make(map[ptypes.AccountName]struct{}, len(tg.BootstrapValidators))
	for i, bv := range tg.BootstrapValidators {
		if !bv.Name.IsValid() {
			return fmt.Errorf("bootstrap_validators[%d].name %q invalid", i, bv.Name)
		}
		if _, dup := seen[bv.Name]; dup {
			return fmt.Errorf("bootstrap_validators[%d].name %q duplicated", i, bv.Name)
		}
		seen[bv.Name] = struct{}{}
		// Ed25519 pubkey length check; KeyTypeEd25519 is the only
		// supported algorithm at v1.
		if len(bv.PubKey) != 32 {
			return fmt.Errorf("bootstrap_validators[%d].pub_key length %d != 32", i, len(bv.PubKey))
		}
	}
	return nil
}

// InitialAllocations computes each protocol account's genesis balance
// in micro-tokens, given the total supply. Order: VRP, CT, Airdrop,
// Ecosystem, Bootstrap. Sum equals TotalSupply by construction (the
// allocation percentages sum to 10000 basis points, enforced at
// compile time via allocPctTotal).
//
// Used by Phase 0.6 (bank module-account seeding) and Phase 1.x
// (mint VRP debits). Returning a fixed-order slice keeps the call
// site deterministic without iterating a map.
func (tg *TokenomicsGenesis) InitialAllocations() (vrp, ct, airdrop, ecosystem, bootstrap uint64) {
	if tg == nil || tg.TotalSupply == 0 {
		return 0, 0, 0, 0, 0
	}
	// Compute in order; the last allocation gets any rounding
	// remainder so the four parts sum exactly to TotalSupply.
	bp := func(p uint64) uint64 { return tg.TotalSupply * p / 10000 }
	vrp = bp(AllocPctVRP)
	ct = bp(AllocPctCT)
	airdrop = bp(AllocPctAirdrop)
	ecosystem = bp(AllocPctEcosystem)
	bootstrap = tg.TotalSupply - (vrp + ct + airdrop + ecosystem)
	return
}
