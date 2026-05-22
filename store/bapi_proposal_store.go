package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/blockberries/blockberry/pkg/statestore"
)

// BAPIProposal represents a governance proposal.
type BAPIProposal struct {
	// ID is the unique proposal identifier.
	ID uint64 `cramberry:"1"`

	// Proposer is the account that submitted the proposal.
	Proposer string `cramberry:"2"`

	// Title is the proposal title.
	Title string `cramberry:"3"`

	// Description is the proposal description.
	Description string `cramberry:"4"`

	// Type is the proposal type (text, parameter, software_upgrade).
	Type string `cramberry:"5"`

	// Status is the current proposal status.
	Status string `cramberry:"6"`

	// TotalDeposit is the total deposited amount.
	TotalDeposit uint64 `cramberry:"7"`

	// DepositDenom is the denomination of deposits.
	DepositDenom string `cramberry:"8"`

	// SubmitTime is when the proposal was submitted (Unix timestamp).
	SubmitTime int64 `cramberry:"9"`

	// DepositEndTime is when deposit period ends (Unix timestamp).
	DepositEndTime int64 `cramberry:"10"`

	// VotingStartTime is when voting period starts (Unix timestamp, 0 if not started).
	VotingStartTime int64 `cramberry:"11"`

	// VotingEndTime is when voting period ends (Unix timestamp, 0 if not started).
	VotingEndTime int64 `cramberry:"12"`

	// YesVotes is the total voting power that voted yes.
	YesVotes uint64 `cramberry:"13"`

	// NoVotes is the total voting power that voted no.
	NoVotes uint64 `cramberry:"14"`

	// AbstainVotes is the total voting power that abstained.
	AbstainVotes uint64 `cramberry:"15"`

	// VetoVotes is the total voting power that voted no with veto.
	VetoVotes uint64 `cramberry:"16"`
}

// SubmitTimeAsTime returns the submit time as a time.Time.
func (p *BAPIProposal) SubmitTimeAsTime() time.Time {
	return time.Unix(p.SubmitTime, 0).UTC()
}

// DepositEndTimeAsTime returns the deposit end time as a time.Time.
func (p *BAPIProposal) DepositEndTimeAsTime() time.Time {
	return time.Unix(p.DepositEndTime, 0).UTC()
}

// VotingStartTimeAsTime returns the voting start time as a time.Time.
func (p *BAPIProposal) VotingStartTimeAsTime() time.Time {
	if p.VotingStartTime == 0 {
		return time.Time{}
	}
	return time.Unix(p.VotingStartTime, 0).UTC()
}

// VotingEndTimeAsTime returns the voting end time as a time.Time.
func (p *BAPIProposal) VotingEndTimeAsTime() time.Time {
	if p.VotingEndTime == 0 {
		return time.Time{}
	}
	return time.Unix(p.VotingEndTime, 0).UTC()
}

// TotalVotingPower returns the total voting power that voted.
func (p *BAPIProposal) TotalVotingPower() uint64 {
	return p.YesVotes + p.NoVotes + p.AbstainVotes + p.VetoVotes
}

// BAPIVote represents a vote on a proposal.
type BAPIVote struct {
	// ProposalID is the proposal being voted on.
	ProposalID uint64 `cramberry:"1"`

	// Voter is the account that voted.
	Voter string `cramberry:"2"`

	// Option is the vote option (yes, no, abstain, no_with_veto).
	Option string `cramberry:"3"`

	// VotingPower is the voter's power at the time of voting.
	VotingPower uint64 `cramberry:"4"`

	// VoteTime is when the vote was cast (Unix timestamp).
	VoteTime int64 `cramberry:"5"`
}

// BAPIDeposit represents a deposit on a proposal.
type BAPIDeposit struct {
	// ProposalID is the proposal being deposited on.
	ProposalID uint64 `cramberry:"1"`

	// Depositor is the account that deposited.
	Depositor string `cramberry:"2"`

	// Amount is the deposited amount.
	Amount uint64 `cramberry:"3"`

	// Denom is the deposit denomination.
	Denom string `cramberry:"4"`
}

// BAPIProposalStore provides typed access to governance proposal data.
type BAPIProposalStore struct {
	proposals *TypedStore[*BAPIProposal]
	votes     *TypedStore[*BAPIVote]
	deposits  *TypedStore[*BAPIDeposit]
	metadata  *TypedStore[*ProposalMetadata]
}

// ProposalMetadata stores module-level metadata.
type ProposalMetadata struct {
	// NextProposalID is the next proposal ID to be used.
	NextProposalID uint64 `cramberry:"1"`
}

// NewBAPIProposalStore creates a new proposal store backed by blockberry's StateStore.
func NewBAPIProposalStore(store statestore.StateStore) *BAPIProposalStore {
	return &BAPIProposalStore{
		proposals: NewTypedStore[*BAPIProposal](store, "proposals/"),
		votes:     NewTypedStore[*BAPIVote](store, "votes/"),
		deposits:  NewTypedStore[*BAPIDeposit](store, "deposits/"),
		metadata:  NewTypedStore[*ProposalMetadata](store, "gov_metadata/"),
	}
}

// proposalKey creates the key for a proposal.
func proposalKey(id uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, id)
	return string(buf)
}

// voteKey creates the key for a vote.
func voteKey(proposalID uint64, voter string) string {
	return proposalKey(proposalID) + "/" + voter
}

// depositKey creates the key for a deposit.
func depositKey(proposalID uint64, depositor string) string {
	return proposalKey(proposalID) + "/" + depositor
}

// GetNextProposalID returns the next proposal ID and increments it.
func (s *BAPIProposalStore) GetNextProposalID(ctx context.Context) (uint64, error) {
	meta, err := s.metadata.Get(ctx, "next_id")
	if err == ErrNotFound {
		// Initialize with ID 1
		meta = &ProposalMetadata{NextProposalID: 1}
	} else if err != nil {
		return 0, fmt.Errorf("get metadata: %w", err)
	}

	id := meta.NextProposalID
	meta.NextProposalID++

	if err := s.metadata.Set(ctx, "next_id", meta); err != nil {
		return 0, fmt.Errorf("set metadata: %w", err)
	}

	return id, nil
}

// GetProposal retrieves a proposal by ID.
func (s *BAPIProposalStore) GetProposal(ctx context.Context, id uint64) (*BAPIProposal, error) {
	if id == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}
	return s.proposals.Get(ctx, proposalKey(id))
}

// SetProposal stores a proposal.
func (s *BAPIProposalStore) SetProposal(ctx context.Context, proposal *BAPIProposal) error {
	if proposal == nil {
		return ErrInvalidValue
	}
	if proposal.ID == 0 {
		return fmt.Errorf("proposal ID cannot be zero")
	}
	return s.proposals.Set(ctx, proposalKey(proposal.ID), proposal)
}

// DeleteProposal removes a proposal.
func (s *BAPIProposalStore) DeleteProposal(ctx context.Context, id uint64) error {
	if id == 0 {
		return fmt.Errorf("proposal ID cannot be zero")
	}
	return s.proposals.Delete(ctx, proposalKey(id))
}

// HasProposal checks if a proposal exists.
func (s *BAPIProposalStore) HasProposal(ctx context.Context, id uint64) (bool, error) {
	if id == 0 {
		return false, fmt.Errorf("proposal ID cannot be zero")
	}
	return s.proposals.Has(ctx, proposalKey(id))
}

// IterateProposals walks every proposal in the store, invoking fn for
// each. Return true from fn to stop iteration.
//
// Returns ErrIterationUnsupported if the underlying StateStore is not
// iterable. Iteration order is the proposal store's underlying tree
// order (ascending byte order over zero-padded proposal IDs).
func (s *BAPIProposalStore) IterateProposals(fn func(p *BAPIProposal) bool) error {
	return s.proposals.IterateRelative(func(_ string, p *BAPIProposal) bool {
		return fn(p)
	})
}

// GetVote retrieves a vote.
func (s *BAPIProposalStore) GetVote(ctx context.Context, proposalID uint64, voter string) (*BAPIVote, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}
	if voter == "" {
		return nil, fmt.Errorf("voter cannot be empty")
	}
	return s.votes.Get(ctx, voteKey(proposalID, voter))
}

// SetVote stores a vote.
func (s *BAPIProposalStore) SetVote(ctx context.Context, vote *BAPIVote) error {
	if vote == nil {
		return ErrInvalidValue
	}
	if vote.ProposalID == 0 {
		return fmt.Errorf("proposal ID cannot be zero")
	}
	if vote.Voter == "" {
		return fmt.Errorf("voter cannot be empty")
	}
	return s.votes.Set(ctx, voteKey(vote.ProposalID, vote.Voter), vote)
}

// HasVote checks if a vote exists.
func (s *BAPIProposalStore) HasVote(ctx context.Context, proposalID uint64, voter string) (bool, error) {
	if proposalID == 0 {
		return false, fmt.Errorf("proposal ID cannot be zero")
	}
	if voter == "" {
		return false, fmt.Errorf("voter cannot be empty")
	}
	return s.votes.Has(ctx, voteKey(proposalID, voter))
}

// GetDeposit retrieves a deposit.
func (s *BAPIProposalStore) GetDeposit(ctx context.Context, proposalID uint64, depositor string) (*BAPIDeposit, error) {
	if proposalID == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}
	if depositor == "" {
		return nil, fmt.Errorf("depositor cannot be empty")
	}

	deposit, err := s.deposits.Get(ctx, depositKey(proposalID, depositor))
	if err == ErrNotFound {
		// Return zero deposit instead of error
		return &BAPIDeposit{
			ProposalID: proposalID,
			Depositor:  depositor,
			Amount:     0,
		}, nil
	}
	return deposit, err
}

// SetDeposit stores a deposit.
func (s *BAPIProposalStore) SetDeposit(ctx context.Context, deposit *BAPIDeposit) error {
	if deposit == nil {
		return ErrInvalidValue
	}
	if deposit.ProposalID == 0 {
		return fmt.Errorf("proposal ID cannot be zero")
	}
	if deposit.Depositor == "" {
		return fmt.Errorf("depositor cannot be empty")
	}
	return s.deposits.Set(ctx, depositKey(deposit.ProposalID, deposit.Depositor), deposit)
}

// AddDeposit adds to a deposit amount.
func (s *BAPIProposalStore) AddDeposit(ctx context.Context, proposalID uint64, depositor string, amount uint64, denom string) error {
	deposit, err := s.GetDeposit(ctx, proposalID, depositor)
	if err != nil && err != ErrNotFound {
		return fmt.Errorf("get deposit: %w", err)
	}

	newAmount := deposit.Amount + amount
	if newAmount < deposit.Amount {
		return fmt.Errorf("deposit overflow")
	}

	deposit.Amount = newAmount
	deposit.Denom = denom
	return s.SetDeposit(ctx, deposit)
}

// GetProposalWithProof retrieves a proposal with a Merkle proof.
func (s *BAPIProposalStore) GetProposalWithProof(ctx context.Context, id uint64) (*BAPIProposal, *statestore.Proof, error) {
	if id == 0 {
		return nil, nil, fmt.Errorf("proposal ID cannot be zero")
	}
	return s.proposals.GetWithProof(ctx, proposalKey(id))
}

// GetProposalAtHeight retrieves a proposal at a specific block height.
func (s *BAPIProposalStore) GetProposalAtHeight(ctx context.Context, id uint64, height int64) (*BAPIProposal, error) {
	if id == 0 {
		return nil, fmt.Errorf("proposal ID cannot be zero")
	}
	return s.proposals.GetAtHeight(ctx, proposalKey(id), height)
}

// UpdateProposalStatus updates a proposal's status.
func (s *BAPIProposalStore) UpdateProposalStatus(ctx context.Context, id uint64, status string) error {
	proposal, err := s.GetProposal(ctx, id)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}

	proposal.Status = status
	return s.SetProposal(ctx, proposal)
}

// StartVoting transitions a proposal from deposit to voting period.
func (s *BAPIProposalStore) StartVoting(ctx context.Context, id uint64, votingStart, votingEnd time.Time) error {
	proposal, err := s.GetProposal(ctx, id)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}

	if proposal.Status != "deposit" {
		return fmt.Errorf("proposal not in deposit period")
	}

	proposal.Status = "voting"
	proposal.VotingStartTime = votingStart.Unix()
	proposal.VotingEndTime = votingEnd.Unix()

	return s.SetProposal(ctx, proposal)
}

// RecordVote records a vote on a proposal.
func (s *BAPIProposalStore) RecordVote(ctx context.Context, proposalID uint64, voter string, option string, power uint64, voteTime time.Time) error {
	// Check if already voted
	hasVote, err := s.HasVote(ctx, proposalID, voter)
	if err != nil {
		return fmt.Errorf("check vote: %w", err)
	}
	if hasVote {
		return fmt.Errorf("already voted on this proposal")
	}

	// Store the vote
	vote := &BAPIVote{
		ProposalID:  proposalID,
		Voter:       voter,
		Option:      option,
		VotingPower: power,
		VoteTime:    voteTime.Unix(),
	}
	if err := s.SetVote(ctx, vote); err != nil {
		return fmt.Errorf("set vote: %w", err)
	}

	// Update proposal vote counts
	proposal, err := s.GetProposal(ctx, proposalID)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}

	switch option {
	case "yes":
		proposal.YesVotes += power
	case "no":
		proposal.NoVotes += power
	case "abstain":
		proposal.AbstainVotes += power
	case "no_with_veto":
		proposal.VetoVotes += power
	default:
		return fmt.Errorf("invalid vote option: %s", option)
	}

	return s.SetProposal(ctx, proposal)
}
