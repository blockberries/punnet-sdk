package runtime

import (
	"sort"

	ptypes "github.com/blockberries/punnet-sdk/types"
)

// ModuleAccountPrefix is the reserved prefix that identifies a
// module-controlled account in the bank's namespace. Combined with
// a short slug it produces a deterministic AccountName like
// "module.vrp". User account creation should reject any name
// starting with this prefix (Phase 1 will add the gate).
const ModuleAccountPrefix = "module."

// Protocol module account slugs. Each names one of the four
// tokenomics protocol accounts seeded at genesis from
// TokenomicsGenesis. The full AccountName is
// ModuleAccountPrefix + slug.
const (
	ModuleAccountVRPSlug       = "vrp"
	ModuleAccountCTSlug        = "ct"
	ModuleAccountEcosystemSlug = "eco"
	ModuleAccountBootstrapSlug = "bl"
)

// ModuleAccountName derives the AccountName for a module-controlled
// account. The slug must be a valid AccountName tail (lowercase
// alphanumeric + dot, total length ≤ 64 after the prefix). Returns
// an empty AccountName if the derived name would violate the
// AccountName format.
func ModuleAccountName(slug string) ptypes.AccountName {
	name := ptypes.AccountName(ModuleAccountPrefix + slug)
	if !name.IsValid() {
		return ""
	}
	return name
}

// Pre-computed protocol account names. Reserved for the tokenomics
// supply model and seeded at genesis when TokenomicsGenesis is set.
var (
	ModuleAccountVRP       = ModuleAccountName(ModuleAccountVRPSlug)
	ModuleAccountCT        = ModuleAccountName(ModuleAccountCTSlug)
	ModuleAccountEcosystem = ModuleAccountName(ModuleAccountEcosystemSlug)
	ModuleAccountBootstrap = ModuleAccountName(ModuleAccountBootstrapSlug)
)

// ModuleAccountPermission describes the permitted operations on a
// module-controlled account. Each flag gates a specific mutation
// path: callers without the corresponding permission cannot issue
// the operation.
//
// Phase 0.6 defines the table; enforcement lands in Phase 1 (fee
// routing reads TransferIn for CT) and later phases (mint reads
// Mint for VRP, distribution reads TransferOut for CT/VRP, etc.).
// The bank module's MsgSend handler stays generic; the gate runs
// in the AnteHandler / specific module handlers that move funds
// in or out of the protocol accounts.
type ModuleAccountPermission struct {
	// Mint grants the right to credit new tokens into this
	// account from outside the bank's bookkeeping. Reserved for
	// chains that change the supply model from "fixed" to
	// "minted"; under the Stealth tokenomics no module account
	// has Mint=true (supply is fixed).
	Mint bool
	// Burn grants the symmetric right to debit tokens out of
	// existence. Like Mint, unused under fixed-supply.
	Burn bool
	// TransferOut grants the right to spend tokens from this
	// account. CT and BL have it (governance withdrawals,
	// bootstrap vesting); VRP has it (mint module's per-block
	// emission); Ecosystem has it (linear vest).
	TransferOut bool
	// TransferIn grants the right to receive tokens into this
	// account from any external source. CT receives fees and
	// slashed funds; the others are seeded at genesis and don't
	// receive top-ups.
	TransferIn bool
}

// ProtocolAccounts returns the canonical permission table for the
// four protocol module accounts. The returned map's keys are stable
// AccountName values; iteration is intentionally NOT ordered — call
// sites that need determinism must sort the keys themselves (see
// SortedProtocolAccountNames).
//
// Permissions reflect the spec §3 fee-routing destination (CT
// receives op + byte; VRP supplies emission; BL pays bootstrap
// vests; Ecosystem pays linear vest). PLAN §7 Phase 0.6 / decision
// D5 (protocol accounts as bank-controlled addresses).
func ProtocolAccounts() map[ptypes.AccountName]ModuleAccountPermission {
	return map[ptypes.AccountName]ModuleAccountPermission{
		ModuleAccountVRP: {
			TransferOut: true, // mint module drains per block
		},
		ModuleAccountCT: {
			TransferOut: true, // governance withdrawals
			TransferIn:  true, // fee revenue + slashing credits
		},
		ModuleAccountEcosystem: {
			TransferOut: true, // 4-year linear vest
		},
		ModuleAccountBootstrap: {
			TransferOut: true, // bootstrap-validator vesting after 12mo lockup
		},
	}
}

// SortedProtocolAccountNames returns the four protocol account
// names in deterministic order. Use this for any consensus-critical
// iteration (genesis seeding, audits, etc.) — never iterate
// ProtocolAccounts() directly without sorting first.
func SortedProtocolAccountNames() []ptypes.AccountName {
	accounts := ProtocolAccounts()
	names := make([]ptypes.AccountName, 0, len(accounts))
	for n := range accounts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		return names[i] < names[j]
	})
	return names
}

// ProtocolAccountForSlug returns the AccountName for a given slug,
// or the empty AccountName when no protocol account uses the slug.
// Convenience for downstream modules that prefer slugs over the
// full module.<slug> form.
func ProtocolAccountForSlug(slug string) ptypes.AccountName {
	switch slug {
	case ModuleAccountVRPSlug:
		return ModuleAccountVRP
	case ModuleAccountCTSlug:
		return ModuleAccountCT
	case ModuleAccountEcosystemSlug:
		return ModuleAccountEcosystem
	case ModuleAccountBootstrapSlug:
		return ModuleAccountBootstrap
	}
	return ""
}
