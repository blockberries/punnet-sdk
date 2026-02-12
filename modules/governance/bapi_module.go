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

// EndBlock is called at the end of each block.
// It processes proposals that have ended their deposit or voting periods.
func (m *BAPIGovernanceModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	// In a full implementation, we would iterate over proposals and:
	// 1. Move proposals from deposit to voting period if deposit threshold met
	// 2. Tally votes and finalize proposals whose voting period has ended
	// For now, return empty updates
	return nil, nil, nil
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
