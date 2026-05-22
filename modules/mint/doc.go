// Package mint implements the per-block VRP drain that funds
// validator rewards per tokenomics spec §4.1:
//
//	B_t = ρ · CS_t · taper(VRP_t)
//	taper(V) = min(1, V / V_threshold), V_threshold = 5% · S
//
// "Mint" is a misnomer inherited from cosmos-sdk: this module
// does NOT mint new supply (the chain is fixed-supply per spec
// §0.1). It moves tokens from the pre-allocated VRP pool (25% of
// S at genesis) into the chain-wide Emission Pool on a rule-bound
// schedule. Total supply is unchanged at all times.
//
// State layout (typed-store prefix "mint/"):
//
//	params/active   →  MintParams { RhoScaled, VThresholdMicro }
//
// The transfer from module.vrp → module.emission is applied
// DIRECTLY in EndBlock (bypassing the effects pipeline) so the
// distribution module's same-block read of EmissionPoolAccount
// sees the post-emission balance — see commit 7337b96.
//
// PLAN §7 Phases 3.1 + 3.2.
package mint
