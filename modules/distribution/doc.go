// Package distribution implements the validator/delegator reward
// split per the cosmos-sdk-style F1 algorithm described in
// tokenomics spec §4.4, including per-period historical
// snapshots for correct slash-period behaviour.
//
// Each validator maintains a cumulative reward-per-share index
// R_v (18-decimal internal precision). At every epoch close the
// Emission Pool + Priority Pool are partitioned by participation
// share — share(v) = α·leader_share + (1-α)·batch_share per spec
// §3.4 — and the validator's CurrentRewardsScaled accumulator is
// folded into R_v via IncrementPeriod, which snapshots history.
//
// State layout (typed-store prefix "distribution/"):
//
//	validator/<hex>                       →  ValidatorDistribution
//	validator/<hex>/history/<period>      →  ValidatorHistoricalRewards
//	validator/<hex>/slash/<period>        →  ValidatorSlashEvent
//	delegator/<addr>/<validator-hex>      →  DelegatorReward
//	params                                →  DistributionParams { AlphaScaled }
//
// On MsgWithdrawDelegatorReward, the claim walks slash events
// between the delegator's PreviousPeriod and the
// just-incremented current period; each slash boundary reduces
// the carried stake by the slash fraction before the next segment
// is computed. The pre-slash stake × pre-slash rewards
// correctness invariant is preserved.
//
// External hooks (called by app wiring):
//
//	RecordSlash         called by staking's ProcessEvidence
//	                    BEFORE a slash applies
//	SnapshotDelegation  called by staking's handleDelegate /
//	                    handleUndelegate after the new stake is
//	                    computed
//
// PLAN §7 Phases 3.5 + 3.6.
package distribution
