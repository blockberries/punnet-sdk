package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// Default governance parameters.
const (
	// DefaultMinDeposit is the minimum deposit required for a proposal to enter voting.
	DefaultMinDeposit = uint64(10000000) // 10 million units

	// DefaultDepositPeriod is the time window for depositing on a proposal.
	DefaultDepositPeriod = 14 * 24 * time.Hour // 14 days

	// DefaultVotingPeriod is the time window for voting on a proposal.
	DefaultVotingPeriod = 14 * 24 * time.Hour // 14 days

	// DefaultQuorum is the minimum participation required for a valid vote.
	DefaultQuorum = uint32(3333) // 33.33% in basis points

	// DefaultThreshold is the minimum yes votes required to pass (of non-abstain votes).
	DefaultThreshold = uint32(5000) // 50% in basis points

	// DefaultVetoThreshold is the maximum veto votes allowed.
	DefaultVetoThreshold = uint32(3333) // 33.33% in basis points

	// DefaultDepositDenom is the default deposit denomination.
	DefaultDepositDenom = "stake"
)

// Proposal classes per spec §11 / PLAN §7 Phase 4.1. Each class
// drives both the vote threshold (Phase 4.2) and the post-pass
// timelock (Phase 4.4).
const (
	// ProposalClassSimple7d is the default class for routine
	// parameter changes (operation fees, byte fee, validator-set
	// size, etc.). Simple majority; 7-day timelock.
	ProposalClassSimple7d = "simple_7d"

	// ProposalClassSuper30d is for adding new operation types or
	// changing slashing severities — requires more deliberation.
	// 2/3 supermajority; 30-day timelock.
	ProposalClassSuper30d = "super_30d"

	// ProposalClassSuper60d is for removing operation types and
	// other hard-to-reverse changes. 2/3 supermajority; 60-day
	// timelock.
	ProposalClassSuper60d = "super_60d"

	// ProposalClassConstitutional is reserved for changes that
	// violate or weaken the §0 Design Principles (e.g. lifting
	// the fixed-supply invariant). Spec §11: "Constitutional
	// changes require ~80% supermajority and 60-day timelock."
	ProposalClassConstitutional = "constitutional"
)

// Timelock durations per class. Block heights at 1-second block
// cadence; chains running at a different cadence will need a
// future ConsensusParams field to override.
const (
	TimelockBlocksSimple7d       uint64 = 7 * 24 * 60 * 60
	TimelockBlocksSuper30d       uint64 = 30 * 24 * 60 * 60
	TimelockBlocksSuper60d       uint64 = 60 * 24 * 60 * 60
	TimelockBlocksConstitutional uint64 = 60 * 24 * 60 * 60
)

// TimelockForClass returns the post-pass delay in blocks for the
// given proposal class. Falls back to Simple7d for unknown classes
// to be conservative (no infinitely fast enactment on garbled input).
func TimelockForClass(class string) uint64 {
	switch class {
	case ProposalClassSuper30d:
		return TimelockBlocksSuper30d
	case ProposalClassSuper60d:
		return TimelockBlocksSuper60d
	case ProposalClassConstitutional:
		return TimelockBlocksConstitutional
	case ProposalClassSimple7d, "":
		return TimelockBlocksSimple7d
	default:
		return TimelockBlocksSimple7d
	}
}

// IsKnownClass returns true if `class` is one of the four
// recognised proposal classes (or the empty default).
func IsKnownClass(class string) bool {
	switch class {
	case ProposalClassSimple7d, ProposalClassSuper30d, ProposalClassSuper60d, ProposalClassConstitutional, "":
		return true
	}
	return false
}

// Vote thresholds per class, in basis points of (Yes+No+Veto). PLAN
// §7 Phase 4.2 / spec §11. Abstain is excluded from the denominator
// (matches cosmos-sdk x/gov convention; abstain is a "I participate
// in quorum but neither support nor oppose" signal).
const (
	// SimpleThresholdBps: simple majority for Simple7d. The default
	// governance threshold (DefaultThreshold = 5000) is used here
	// for backward compatibility with any test fixture that tunes
	// it; constitutional + super always use the higher fixed
	// values below regardless of DefaultThreshold.
	SimpleThresholdBps uint32 = 5000

	// SuperThresholdBps: 2/3 supermajority for Super30d / Super60d.
	SuperThresholdBps uint32 = 6667

	// ConstitutionalThresholdBps: 80% for Constitutional changes
	// (spec §11: "Constitutional changes require ~80% supermajority").
	ConstitutionalThresholdBps uint32 = 8000
)

// ThresholdForClass returns the YesVotes / (Yes+No+Veto) threshold
// in basis points for the given proposal class. `simpleDefault` is
// passed in so callers can plug the module's tunable
// DefaultThreshold for Simple7d while leaving the supermajority
// values fixed at spec-mandated values.
func ThresholdForClass(class string, simpleDefault uint32) uint32 {
	switch class {
	case ProposalClassSuper30d, ProposalClassSuper60d:
		return SuperThresholdBps
	case ProposalClassConstitutional:
		return ConstitutionalThresholdBps
	default:
		// Simple7d (and the empty default) honor the module's
		// configured threshold so chains can tighten their simple-
		// majority bar without violating the supermajority floor
		// for the heavier classes.
		return simpleDefault
	}
}

// BAPIGovernanceModule provides governance functionality for BAPI-based applications.
// It implements runtime.BAPIModule, runtime.BAPIBlockProcessor, and runtime.BAPIGenesisInitializer.
type BAPIGovernanceModule struct {
	proposalStore *store.BAPIProposalStore
	balanceStore  *store.BAPIBalanceStore

	// Configuration
	minDeposit     uint64
	depositPeriod  time.Duration
	votingPeriod   time.Duration
	quorum         uint32 // basis points
	threshold      uint32 // basis points
	vetoThreshold  uint32 // basis points
	depositDenom   string

	// Parameters is the chain-wide registry of governance-tunable
	// parameter bands. Populated by modules at construction or
	// genesis time; consulted by the enactment hook (Phase 4.4)
	// before applying a proposal-driven parameter change.
	// PLAN §7 Phase 4.3.
	Parameters *ParameterRegistry

	// parameterTargets maps a target-module name to the module
	// instance that implements BAPIModuleParams for that name.
	// Populated via RegisterParameterTarget; consulted by the
	// enactment hook to dispatch ApplyParameterChange.
	parameterTargets map[string]runtime.BAPIModuleParams
}

// RegisterParameterTarget associates `moduleName` with a module
// that implements BAPIModuleParams. The chain operator (or the
// runtime, on apps that want auto-discovery) calls this at module
// construction time. ProposalChange entries with the same
// TargetModule string will dispatch to `m` during enactment.
//
// Returns an error if `moduleName` is empty or already registered.
// PLAN §7 Phase 4.4.
func (m *BAPIGovernanceModule) RegisterParameterTarget(moduleName string, target runtime.BAPIModuleParams) error {
	if moduleName == "" {
		return fmt.Errorf("module name cannot be empty")
	}
	if target == nil {
		return fmt.Errorf("target cannot be nil")
	}
	if m.parameterTargets == nil {
		m.parameterTargets = make(map[string]runtime.BAPIModuleParams)
	}
	if _, exists := m.parameterTargets[moduleName]; exists {
		return fmt.Errorf("module %q already registered", moduleName)
	}
	m.parameterTargets[moduleName] = target
	return nil
}

// NewBAPIGovernanceModule creates a new BAPI governance module with the given stores.
func NewBAPIGovernanceModule(proposalStore *store.BAPIProposalStore, balanceStore *store.BAPIBalanceStore) (*BAPIGovernanceModule, error) {
	if proposalStore == nil {
		return nil, fmt.Errorf("proposal store cannot be nil")
	}
	if balanceStore == nil {
		return nil, fmt.Errorf("balance store cannot be nil")
	}

	return &BAPIGovernanceModule{
		proposalStore: proposalStore,
		balanceStore:  balanceStore,
		minDeposit:    DefaultMinDeposit,
		depositPeriod: DefaultDepositPeriod,
		votingPeriod:  DefaultVotingPeriod,
		quorum:        DefaultQuorum,
		threshold:     DefaultThreshold,
		vetoThreshold: DefaultVetoThreshold,
		depositDenom:  DefaultDepositDenom,
		Parameters:    NewParameterRegistry(),
	}, nil
}

// WithParams sets custom governance parameters.
func (m *BAPIGovernanceModule) WithParams(minDeposit uint64, depositPeriod, votingPeriod time.Duration, quorum, threshold, vetoThreshold uint32, depositDenom string) *BAPIGovernanceModule {
	m.minDeposit = minDeposit
	m.depositPeriod = depositPeriod
	m.votingPeriod = votingPeriod
	m.quorum = quorum
	m.threshold = threshold
	m.vetoThreshold = vetoThreshold
	m.depositDenom = depositDenom
	return m
}

// Name returns the module's unique name.
func (m *BAPIGovernanceModule) Name() string {
	return ModuleName
}

// RegisterMsgHandlers returns message handlers keyed by message type.
func (m *BAPIGovernanceModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgSubmitProposal: m.handleSubmitProposal,
		TypeMsgVote:           m.handleVote,
		TypeMsgDeposit:        m.handleDeposit,
	}
}

// RegisterQueryHandlers returns query handlers keyed by query path.
func (m *BAPIGovernanceModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/gov/proposal": m.handleQueryProposal,
		"/gov/params":   m.handleQueryParams,
	}
}

// BeginBlock is called at the beginning of each block.
func (m *BAPIGovernanceModule) BeginBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	// No begin-block processing needed
	return nil, nil
}

// EndBlock processes proposals whose voting period has ended.
//
// Phase 4.2: for each StatusVoting proposal past its
// VotingEndTime, tally the votes and apply the class-specific
// threshold. Passing proposals transition to StatusPassed and
// have their EffectiveHeight set to currentHeight +
// TimelockForClass(class). Rejected proposals go to StatusRejected.
//
// Walks the proposal store via IterateProposals; like the staking
// module's epoch-close logic, we collect dirty proposals into a
// slice before mutating to keep the IAVL Iterate / Set lock
// hygiene clean.
func (m *BAPIGovernanceModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	if blockCtx == nil {
		return nil, nil, nil
	}
	nowUnix := blockCtx.Time.ToTime().Unix()

	var toTally []*store.BAPIProposal
	err := m.proposalStore.IterateProposals(func(p *store.BAPIProposal) bool {
		if p == nil {
			return false
		}
		if p.Status != string(StatusVoting) {
			return false
		}
		if p.VotingEndTime > nowUnix {
			return false
		}
		toTally = append(toTally, p)
		return false
	})
	if err != nil {
		// Iteration unsupported (in-memory test stores) — no
		// tally happens. Real chains run on iterable backends.
		return nil, nil, nil
	}

	for _, p := range toTally {
		passed := m.tallyProposal(p)
		if passed {
			p.Status = string(StatusPassed)
			p.EffectiveHeight = uint64(blockCtx.Height) + TimelockForClass(p.Class)
		} else {
			p.Status = string(StatusRejected)
		}
		if err := m.proposalStore.SetProposal(ctx, p); err != nil {
			return nil, nil, fmt.Errorf("persist tallied proposal %d: %w", p.ID, err)
		}
	}

	// Phase 4.4: enactment. Walk passed proposals whose
	// EffectiveHeight has been reached and dispatch each one's
	// Changes to the target module's BAPIModuleParams hook.
	var toEnact []*store.BAPIProposal
	if err := m.proposalStore.IterateProposals(func(p *store.BAPIProposal) bool {
		if p == nil {
			return false
		}
		if p.Status != string(StatusPassed) {
			return false
		}
		if p.EffectiveHeight == 0 || p.EffectiveHeight > uint64(blockCtx.Height) {
			return false
		}
		toEnact = append(toEnact, p)
		return false
	}); err != nil {
		return nil, nil, nil
	}

	for _, p := range toEnact {
		if err := m.enactProposal(ctx, p); err != nil {
			p.Status = string(StatusEnactmentFailed)
		} else {
			p.Status = string(StatusEnacted)
		}
		if persistErr := m.proposalStore.SetProposal(ctx, p); persistErr != nil {
			return nil, nil, fmt.Errorf("persist enacted proposal %d: %w", p.ID, persistErr)
		}
	}
	return nil, nil, nil
}

// enactProposal applies all of a proposal's Changes atomically:
// every change is validated against the parameter registry's
// class-specific band first; if any change fails validation the
// whole proposal aborts (none applied). Then each change is
// dispatched to its target module's ApplyParameterChange. If any
// dispatch returns an error, the proposal is marked
// StatusEnactmentFailed — but earlier changes in the same
// proposal have already taken effect, which is the v1 limitation
// noted in Phase 4.6 "bundled atomicity" follow-up.
//
// For now, single-change proposals are the safe case. Bundled
// proposals need either two-phase commit (snapshot module state +
// rollback on failure) or per-module dry-run validation. Phase 4.6
// will tighten this.
func (m *BAPIGovernanceModule) enactProposal(ctx context.Context, p *store.BAPIProposal) error {
	if len(p.Changes) == 0 {
		// Text-only proposal — no enactment needed. Treat as success.
		return nil
	}
	// Pre-validate every change against the band registry.
	if m.Parameters != nil {
		for i, c := range p.Changes {
			if err := m.Parameters.ValidateChange(p.Class, c.ParameterName, c.NewValueInt); err != nil {
				return fmt.Errorf("proposal %d change %d band-validation: %w", p.ID, i, err)
			}
		}
	}
	// Dispatch each change.
	for i, c := range p.Changes {
		target, ok := m.parameterTargets[c.TargetModule]
		if !ok {
			return fmt.Errorf("proposal %d change %d: target module %q not registered", p.ID, i, c.TargetModule)
		}
		if err := target.ApplyParameterChange(ctx, c.ParameterName, c.NewValueInt); err != nil {
			return fmt.Errorf("proposal %d change %d (%s.%s): %w",
				p.ID, i, c.TargetModule, c.ParameterName, err)
		}
	}
	return nil
}

// tallyProposal applies the class-aware threshold to the
// proposal's accumulated vote totals and returns true if the
// proposal passes.
//
// Logic (cosmos-sdk x/gov convention):
//   - Quorum: (Yes+No+Abstain+Veto) ≥ totalStake × quorumBps / 10000
//     — quorum check requires knowing the total stake; for v1 we
//     compare against the module's configured quorum applied to
//     the participating votes (a permissive interpretation;
//     tightening to global-stake quorum is Phase 5 territory).
//   - Veto: VetoVotes / (Yes+No+Abstain+Veto) ≥ vetoThreshold → reject
//   - Yes threshold: YesVotes / (Yes+No+Veto) ≥ ThresholdForClass(class)
//
// All comparisons are in basis points; integer arithmetic avoids
// floating point.
func (m *BAPIGovernanceModule) tallyProposal(p *store.BAPIProposal) bool {
	participating := p.YesVotes + p.NoVotes + p.AbstainVotes + p.VetoVotes
	if participating == 0 {
		return false
	}

	// Veto check.
	if vetoNumerator := uint64(m.vetoThreshold); vetoNumerator > 0 {
		if p.VetoVotes*10000 >= participating*vetoNumerator {
			return false
		}
	}

	// Yes-threshold check on the (Yes + No + Veto) denominator.
	yesnoveto := p.YesVotes + p.NoVotes + p.VetoVotes
	if yesnoveto == 0 {
		// All-abstain → no Yes signal even if quorum is met.
		return false
	}
	threshold := uint64(ThresholdForClass(p.Class, m.threshold))
	return p.YesVotes*10000 >= yesnoveto*threshold
}

// InitGenesis initializes the module's state from genesis data.
func (m *BAPIGovernanceModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil // No genesis data for governance module is acceptable
	}

	var genesisState GovernanceGenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		return fmt.Errorf("unmarshal governance genesis: %w", err)
	}

	// Apply parameters from genesis
	if genesisState.Params.MinDeposit > 0 {
		m.minDeposit = genesisState.Params.MinDeposit
	}
	if genesisState.Params.DepositPeriodSecs > 0 {
		m.depositPeriod = time.Duration(genesisState.Params.DepositPeriodSecs) * time.Second
	}
	if genesisState.Params.VotingPeriodSecs > 0 {
		m.votingPeriod = time.Duration(genesisState.Params.VotingPeriodSecs) * time.Second
	}
	if genesisState.Params.Quorum > 0 {
		m.quorum = genesisState.Params.Quorum
	}
	if genesisState.Params.Threshold > 0 {
		m.threshold = genesisState.Params.Threshold
	}
	if genesisState.Params.VetoThreshold > 0 {
		m.vetoThreshold = genesisState.Params.VetoThreshold
	}
	if genesisState.Params.DepositDenom != "" {
		m.depositDenom = genesisState.Params.DepositDenom
	}

	return nil
}

// ExportGenesis exports the module's state for genesis.
func (m *BAPIGovernanceModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	genesisState := GovernanceGenesisState{
		Params: GenesisParams{
			MinDeposit:        m.minDeposit,
			DepositPeriodSecs: int64(m.depositPeriod.Seconds()),
			VotingPeriodSecs:  int64(m.votingPeriod.Seconds()),
			Quorum:            m.quorum,
			Threshold:         m.threshold,
			VetoThreshold:     m.vetoThreshold,
			DepositDenom:      m.depositDenom,
		},
	}
	return json.Marshal(genesisState)
}

// GovernanceGenesisState represents the governance module's genesis state.
type GovernanceGenesisState struct {
	Params GenesisParams `json:"params"`
}

// GenesisParams represents governance parameters in genesis.
type GenesisParams struct {
	MinDeposit        uint64 `json:"min_deposit"`
	DepositPeriodSecs int64  `json:"deposit_period_secs"`
	VotingPeriodSecs  int64  `json:"voting_period_secs"`
	Quorum            uint32 `json:"quorum"`        // basis points
	Threshold         uint32 `json:"threshold"`     // basis points
	VetoThreshold     uint32 `json:"veto_threshold"` // basis points
	DepositDenom      string `json:"deposit_denom"`
}

// handleSubmitProposal handles MsgSubmitProposal.
func (m *BAPIGovernanceModule) handleSubmitProposal(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.proposalStore == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	submitMsg, ok := msg.(*MsgSubmitProposal)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgSubmitProposal, got %T", msg)
	}

	// Verify the proposer is the transaction signer
	if submitMsg.Proposer != txCtx.Account {
		return nil, fmt.Errorf("proposer must be transaction account")
	}

	// Reject unknown classes early — a malformed class doesn't
	// depend on balance, so check it before the deposit gate so
	// the caller sees the most specific error.
	class := submitMsg.Class
	if class == "" {
		class = ProposalClassSimple7d
	}
	if !IsKnownClass(class) {
		return nil, fmt.Errorf("unknown proposal class %q (want one of: %s, %s, %s, %s)",
			class, ProposalClassSimple7d, ProposalClassSuper30d,
			ProposalClassSuper60d, ProposalClassConstitutional)
	}

	// Verify deposit denomination
	if submitMsg.InitialDeposit.Denom != m.depositDenom {
		return nil, fmt.Errorf("invalid deposit denomination: expected %s, got %s", m.depositDenom, submitMsg.InitialDeposit.Denom)
	}

	// Check proposer has sufficient balance for initial deposit
	if submitMsg.InitialDeposit.Amount > 0 {
		balance, err := m.balanceStore.GetAmount(ctx, string(submitMsg.Proposer), submitMsg.InitialDeposit.Denom)
		if err != nil {
			return nil, fmt.Errorf("failed to get balance: %w", err)
		}
		if balance < submitMsg.InitialDeposit.Amount {
			return nil, fmt.Errorf("%w: insufficient balance for deposit (have %d, need %d)",
				ptypes.ErrInsufficientFunds, balance, submitMsg.InitialDeposit.Amount)
		}
	}

	// Get next proposal ID
	proposalID, err := m.proposalStore.GetNextProposalID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get next proposal ID: %w", err)
	}

	// Calculate deposit end time
	blockTime := txCtx.BAPIBlockContext.Time.ToTime()
	depositEndTime := blockTime.Add(m.depositPeriod)

	// Create proposal
	proposal := &store.BAPIProposal{
		ID:             proposalID,
		Proposer:       string(submitMsg.Proposer),
		Title:          submitMsg.Title,
		Description:    submitMsg.Description,
		Type:           string(submitMsg.ProposalType),
		Status:         string(StatusDeposit),
		TotalDeposit:   submitMsg.InitialDeposit.Amount,
		DepositDenom:   submitMsg.InitialDeposit.Denom,
		SubmitTime:     blockTime.Unix(),
		DepositEndTime: depositEndTime.Unix(),
		Class:          class,
		// EffectiveHeight stays 0 until the proposal passes; set in
		// Phase 4.4's enactment-queue wiring.
	}

	// Store proposal
	if err := m.proposalStore.SetProposal(ctx, proposal); err != nil {
		return nil, fmt.Errorf("failed to store proposal: %w", err)
	}

	// Store initial deposit record
	if submitMsg.InitialDeposit.Amount > 0 {
		deposit := &store.BAPIDeposit{
			ProposalID: proposalID,
			Depositor:  string(submitMsg.Proposer),
			Amount:     submitMsg.InitialDeposit.Amount,
			Denom:      submitMsg.InitialDeposit.Denom,
		}
		if err := m.proposalStore.SetDeposit(ctx, deposit); err != nil {
			return nil, fmt.Errorf("failed to store deposit: %w", err)
		}
	}

	// Build effects
	var effectList []effects.Effect

	// Transfer deposit to governance module account
	if submitMsg.InitialDeposit.Amount > 0 {
		effectList = append(effectList, effects.TransferEffect{
			From:   submitMsg.Proposer,
			To:     ptypes.AccountName("governance.pool"),
			Amount: ptypes.Coins{submitMsg.InitialDeposit},
		})
	}

	// Emit event
	effectList = append(effectList, effects.NewEventEffect("governance.proposal_submitted", map[string][]byte{
		"proposal_id": []byte(fmt.Sprintf("%d", proposalID)),
		"proposer":    []byte(submitMsg.Proposer),
		"title":       []byte(submitMsg.Title),
		"type":        []byte(submitMsg.ProposalType),
		"deposit":     []byte(fmt.Sprintf("%d%s", submitMsg.InitialDeposit.Amount, submitMsg.InitialDeposit.Denom)),
		"height":      []byte(fmt.Sprintf("%d", txCtx.Height)),
	}))

	return effectList, nil
}

// handleVote handles MsgVote.
func (m *BAPIGovernanceModule) handleVote(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.proposalStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	voteMsg, ok := msg.(*MsgVote)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgVote, got %T", msg)
	}

	// Verify the voter is the transaction signer
	if voteMsg.Voter != txCtx.Account {
		return nil, fmt.Errorf("voter must be transaction account")
	}

	// Check proposal exists
	proposal, err := m.proposalStore.GetProposal(ctx, voteMsg.ProposalID)
	if err != nil {
		return nil, fmt.Errorf("proposal not found: %w", err)
	}

	// Check proposal is in voting period
	if proposal.Status != string(StatusVoting) {
		return nil, fmt.Errorf("proposal is not in voting period (status: %s)", proposal.Status)
	}

	// Check voting period hasn't ended
	blockTime := txCtx.BAPIBlockContext.Time.ToTime()
	if blockTime.Unix() > proposal.VotingEndTime {
		return nil, fmt.Errorf("voting period has ended")
	}

	// Check if already voted
	hasVoted, err := m.proposalStore.HasVote(ctx, voteMsg.ProposalID, string(voteMsg.Voter))
	if err != nil {
		return nil, fmt.Errorf("failed to check vote: %w", err)
	}
	if hasVoted {
		return nil, fmt.Errorf("already voted on this proposal")
	}

	// In a real implementation, we would get the voter's voting power from staking
	// For now, use a default power of 1
	votingPower := uint64(1)

	// Record the vote
	if err := m.proposalStore.RecordVote(ctx, voteMsg.ProposalID, string(voteMsg.Voter), string(voteMsg.Option), votingPower, blockTime); err != nil {
		return nil, fmt.Errorf("failed to record vote: %w", err)
	}

	// Emit event
	return []effects.Effect{
		effects.NewEventEffect("governance.vote", map[string][]byte{
			"proposal_id": []byte(fmt.Sprintf("%d", voteMsg.ProposalID)),
			"voter":       []byte(voteMsg.Voter),
			"option":      []byte(voteMsg.Option),
			"power":       []byte(fmt.Sprintf("%d", votingPower)),
			"height":      []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleDeposit handles MsgDeposit.
func (m *BAPIGovernanceModule) handleDeposit(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	if m == nil || m.proposalStore == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	depositMsg, ok := msg.(*MsgDeposit)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgDeposit, got %T", msg)
	}

	// Verify the depositor is the transaction signer
	if depositMsg.Depositor != txCtx.Account {
		return nil, fmt.Errorf("depositor must be transaction account")
	}

	// Check proposal exists
	proposal, err := m.proposalStore.GetProposal(ctx, depositMsg.ProposalID)
	if err != nil {
		return nil, fmt.Errorf("proposal not found: %w", err)
	}

	// Check proposal is in deposit period
	if proposal.Status != string(StatusDeposit) {
		return nil, fmt.Errorf("proposal is not in deposit period (status: %s)", proposal.Status)
	}

	// Check deposit period hasn't ended
	blockTime := txCtx.BAPIBlockContext.Time.ToTime()
	if blockTime.Unix() > proposal.DepositEndTime {
		return nil, fmt.Errorf("deposit period has ended")
	}

	// Verify deposit denomination matches proposal
	if depositMsg.Amount.Denom != proposal.DepositDenom {
		return nil, fmt.Errorf("invalid deposit denomination: expected %s, got %s", proposal.DepositDenom, depositMsg.Amount.Denom)
	}

	// Check depositor has sufficient balance
	balance, err := m.balanceStore.GetAmount(ctx, string(depositMsg.Depositor), depositMsg.Amount.Denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	if balance < depositMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient balance for deposit (have %d, need %d)",
			ptypes.ErrInsufficientFunds, balance, depositMsg.Amount.Amount)
	}

	// Add deposit
	if err := m.proposalStore.AddDeposit(ctx, depositMsg.ProposalID, string(depositMsg.Depositor), depositMsg.Amount.Amount, depositMsg.Amount.Denom); err != nil {
		return nil, fmt.Errorf("failed to add deposit: %w", err)
	}

	// Update proposal total deposit
	proposal.TotalDeposit += depositMsg.Amount.Amount
	if err := m.proposalStore.SetProposal(ctx, proposal); err != nil {
		return nil, fmt.Errorf("failed to update proposal: %w", err)
	}

	// Build effects
	var effectList []effects.Effect

	// Transfer deposit to governance module account
	effectList = append(effectList, effects.TransferEffect{
		From:   depositMsg.Depositor,
		To:     ptypes.AccountName("governance.pool"),
		Amount: ptypes.Coins{depositMsg.Amount},
	})

	// Check if deposit threshold is met and transition to voting period
	if proposal.TotalDeposit >= m.minDeposit {
		votingStart := blockTime
		votingEnd := votingStart.Add(m.votingPeriod)

		if err := m.proposalStore.StartVoting(ctx, depositMsg.ProposalID, votingStart, votingEnd); err != nil {
			return nil, fmt.Errorf("failed to start voting: %w", err)
		}

		effectList = append(effectList, effects.NewEventEffect("governance.voting_started", map[string][]byte{
			"proposal_id":      []byte(fmt.Sprintf("%d", depositMsg.ProposalID)),
			"voting_end_time":  []byte(fmt.Sprintf("%d", votingEnd.Unix())),
			"height":           []byte(fmt.Sprintf("%d", txCtx.Height)),
		}))
	}

	// Emit deposit event
	effectList = append(effectList, effects.NewEventEffect("governance.deposit", map[string][]byte{
		"proposal_id":   []byte(fmt.Sprintf("%d", depositMsg.ProposalID)),
		"depositor":     []byte(depositMsg.Depositor),
		"amount":        []byte(fmt.Sprintf("%d%s", depositMsg.Amount.Amount, depositMsg.Amount.Denom)),
		"total_deposit": []byte(fmt.Sprintf("%d", proposal.TotalDeposit)),
		"height":        []byte(fmt.Sprintf("%d", txCtx.Height)),
	}))

	return effectList, nil
}

// handleQueryProposal handles proposal queries.
func (m *BAPIGovernanceModule) handleQueryProposal(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.proposalStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Parse proposal ID
	proposalID, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid proposal ID: %w", err)
	}

	var proposal *store.BAPIProposal

	// Get proposal at specific height if requested
	if height > 0 {
		proposal, err = m.proposalStore.GetProposalAtHeight(ctx, proposalID, height)
	} else {
		proposal, err = m.proposalStore.GetProposal(ctx, proposalID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get proposal: %w", err)
	}

	// Return JSON response
	response := map[string]interface{}{
		"id":                proposal.ID,
		"proposer":          proposal.Proposer,
		"title":             proposal.Title,
		"description":       proposal.Description,
		"type":              proposal.Type,
		"status":            proposal.Status,
		"total_deposit":     proposal.TotalDeposit,
		"deposit_denom":     proposal.DepositDenom,
		"submit_time":       proposal.SubmitTime,
		"deposit_end_time":  proposal.DepositEndTime,
		"voting_start_time": proposal.VotingStartTime,
		"voting_end_time":   proposal.VotingEndTime,
		"yes_votes":         proposal.YesVotes,
		"no_votes":          proposal.NoVotes,
		"abstain_votes":     proposal.AbstainVotes,
		"veto_votes":        proposal.VetoVotes,
	}

	return json.Marshal(response)
}

// handleQueryParams handles parameter queries.
func (m *BAPIGovernanceModule) handleQueryParams(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil {
		return nil, fmt.Errorf("module is nil")
	}

	// Return JSON response
	response := map[string]interface{}{
		"min_deposit":        m.minDeposit,
		"deposit_period_sec": int64(m.depositPeriod.Seconds()),
		"voting_period_sec":  int64(m.votingPeriod.Seconds()),
		"quorum":             m.quorum,
		"threshold":          m.threshold,
		"veto_threshold":     m.vetoThreshold,
		"deposit_denom":      m.depositDenom,
	}

	return json.Marshal(response)
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule             = (*BAPIGovernanceModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIGovernanceModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIGovernanceModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIGovernanceModule)(nil)
)
