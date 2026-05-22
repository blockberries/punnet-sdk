package client

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/blockberries/punnet-sdk/types"
)

// GovernanceClient provides governance module query methods.
type GovernanceClient struct {
	client *Client
}

// Governance returns a governance module client.
func (c *Client) Governance() *GovernanceClient {
	return &GovernanceClient{client: c}
}

// ProposalResponse contains proposal information.
type ProposalResponse struct {
	ID               uint64    `json:"id"`
	Proposer         string    `json:"proposer"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Type             string    `json:"type"`
	Status           string    `json:"status"`
	TotalDeposit     uint64    `json:"total_deposit"`
	DepositDenom     string    `json:"deposit_denom"`
	SubmitTime       time.Time `json:"submit_time"`
	DepositEndTime   time.Time `json:"deposit_end_time"`
	VotingStartTime  time.Time `json:"voting_start_time"`
	VotingEndTime    time.Time `json:"voting_end_time"`
	YesVotes         uint64    `json:"yes_votes"`
	NoVotes          uint64    `json:"no_votes"`
	AbstainVotes     uint64    `json:"abstain_votes"`
	VetoVotes        uint64    `json:"veto_votes"`
	TotalVotingPower uint64    `json:"total_voting_power"`
}

// VoteResponse contains vote information.
type VoteResponse struct {
	ProposalID  uint64    `json:"proposal_id"`
	Voter       string    `json:"voter"`
	Option      string    `json:"option"`
	VotingPower uint64    `json:"voting_power"`
	VoteTime    time.Time `json:"vote_time"`
}

// DepositResponse contains deposit information.
type DepositResponse struct {
	ProposalID uint64 `json:"proposal_id"`
	Depositor  string `json:"depositor"`
	Amount     uint64 `json:"amount"`
	Denom      string `json:"denom"`
}

// GovernanceParamsResponse contains governance parameters.
type GovernanceParamsResponse struct {
	MinDeposit    uint64        `json:"min_deposit"`
	DepositDenom  string        `json:"deposit_denom"`
	DepositPeriod time.Duration `json:"deposit_period"`
	VotingPeriod  time.Duration `json:"voting_period"`
	Quorum        uint64        `json:"quorum"`        // basis points
	Threshold     uint64        `json:"threshold"`     // basis points
	VetoThreshold uint64        `json:"veto_threshold"` // basis points
}

// TallyResponse contains tally information.
type TallyResponse struct {
	YesVotes         uint64 `json:"yes_votes"`
	NoVotes          uint64 `json:"no_votes"`
	AbstainVotes     uint64 `json:"abstain_votes"`
	VetoVotes        uint64 `json:"veto_votes"`
	TotalVotingPower uint64 `json:"total_voting_power"`
	QuorumReached    bool   `json:"quorum_reached"`
	Passed           bool   `json:"passed"`
	Vetoed           bool   `json:"vetoed"`
}

// Proposal retrieves a proposal by ID.
func (g *GovernanceClient) Proposal(ctx context.Context, id uint64) (*ProposalResponse, error) {
	return g.ProposalAtHeight(ctx, id, 0)
}

// ProposalAtHeight retrieves a proposal at a specific block height.
func (g *GovernanceClient) ProposalAtHeight(ctx context.Context, id uint64, height int64) (*ProposalResponse, error) {
	if id == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}

	// Server handler expects proposal ID as raw numeric string
	data := []byte(strconv.FormatUint(id, 10))

	var resp ProposalResponse
	if err := g.client.QueryJSON(ctx, "/gov/proposal", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Proposals retrieves all proposals.
func (g *GovernanceClient) Proposals(ctx context.Context) ([]ProposalResponse, error) {
	return g.ProposalsWithStatus(ctx, "")
}

// ProposalsWithStatus retrieves proposals filtered by status.
func (g *GovernanceClient) ProposalsWithStatus(ctx context.Context, status string) ([]ProposalResponse, error) {
	var data []byte
	if status != "" {
		data = []byte(status)
	}

	var resp []ProposalResponse
	if err := g.client.QueryJSON(ctx, "/gov/proposals", data, 0, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// Vote retrieves a vote on a proposal.
func (g *GovernanceClient) Vote(ctx context.Context, proposalID uint64, voter types.AccountName) (*VoteResponse, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}
	if !voter.IsValid() {
		return nil, fmt.Errorf("invalid voter name: %s", voter)
	}

	// Format: "proposalID/voter"
	data := []byte(fmt.Sprintf("%d/%s", proposalID, voter))

	var resp VoteResponse
	if err := g.client.QueryJSON(ctx, "/gov/vote", data, 0, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Votes retrieves all votes on a proposal.
func (g *GovernanceClient) Votes(ctx context.Context, proposalID uint64) ([]VoteResponse, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}

	data := []byte(strconv.FormatUint(proposalID, 10))

	var resp []VoteResponse
	if err := g.client.QueryJSON(ctx, "/gov/votes", data, 0, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// Deposit retrieves a deposit on a proposal.
func (g *GovernanceClient) Deposit(ctx context.Context, proposalID uint64, depositor types.AccountName) (*DepositResponse, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}
	if !depositor.IsValid() {
		return nil, fmt.Errorf("invalid depositor name: %s", depositor)
	}

	// Format: "proposalID/depositor"
	data := []byte(fmt.Sprintf("%d/%s", proposalID, depositor))

	var resp DepositResponse
	if err := g.client.QueryJSON(ctx, "/gov/deposit", data, 0, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Deposits retrieves all deposits on a proposal.
func (g *GovernanceClient) Deposits(ctx context.Context, proposalID uint64) ([]DepositResponse, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}

	data := []byte(strconv.FormatUint(proposalID, 10))

	var resp []DepositResponse
	if err := g.client.QueryJSON(ctx, "/gov/deposits", data, 0, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// Tally retrieves the current tally for a proposal.
func (g *GovernanceClient) Tally(ctx context.Context, proposalID uint64) (*TallyResponse, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}

	data := []byte(strconv.FormatUint(proposalID, 10))

	var resp TallyResponse
	if err := g.client.QueryJSON(ctx, "/gov/tally", data, 0, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Params retrieves governance parameters.
func (g *GovernanceClient) Params(ctx context.Context) (*GovernanceParamsResponse, error) {
	var resp GovernanceParamsResponse
	if err := g.client.QueryJSON(ctx, "/gov/params", nil, 0, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
