package governance

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

const (
	// ModuleName is the governance module's name.
	ModuleName = "governance"

	// Message type identifiers.
	TypeMsgSubmitProposal = "governance/MsgSubmitProposal"
	TypeMsgVote           = "governance/MsgVote"
	TypeMsgDeposit        = "governance/MsgDeposit"
)

// ProposalType identifies the type of proposal.
type ProposalType string

const (
	// ProposalTypeText is for text-only proposals.
	ProposalTypeText ProposalType = "text"

	// ProposalTypeParameter is for parameter change proposals.
	ProposalTypeParameter ProposalType = "parameter"

	// ProposalTypeSoftwareUpgrade is for software upgrade proposals.
	ProposalTypeSoftwareUpgrade ProposalType = "software_upgrade"
)

// ProposalStatus represents the status of a proposal.
type ProposalStatus string

const (
	// StatusDeposit indicates the proposal is in deposit period.
	StatusDeposit ProposalStatus = "deposit"

	// StatusVoting indicates the proposal is in voting period.
	StatusVoting ProposalStatus = "voting"

	// StatusPassed indicates the proposal passed.
	StatusPassed ProposalStatus = "passed"

	// StatusRejected indicates the proposal was rejected.
	StatusRejected ProposalStatus = "rejected"

	// StatusFailed indicates the proposal failed due to lack of quorum.
	StatusFailed ProposalStatus = "failed"
)

// VoteOption represents a voting option.
type VoteOption string

const (
	// VoteYes is a yes vote.
	VoteYes VoteOption = "yes"

	// VoteNo is a no vote.
	VoteNo VoteOption = "no"

	// VoteAbstain is an abstain vote.
	VoteAbstain VoteOption = "abstain"

	// VoteNoWithVeto is a no with veto vote.
	VoteNoWithVeto VoteOption = "no_with_veto"
)

// IsValid checks if the vote option is valid.
func (v VoteOption) IsValid() bool {
	switch v {
	case VoteYes, VoteNo, VoteAbstain, VoteNoWithVeto:
		return true
	default:
		return false
	}
}

// MsgSubmitProposal submits a new governance proposal.
type MsgSubmitProposal struct {
	// Proposer is the account submitting the proposal.
	Proposer types.AccountName `json:"proposer"`

	// Title is the proposal title.
	Title string `json:"title"`

	// Description is the proposal description.
	Description string `json:"description"`

	// ProposalType is the proposal type.
	ProposalType ProposalType `json:"proposal_type"`

	// InitialDeposit is the initial deposit for the proposal.
	InitialDeposit types.Coin `json:"initial_deposit"`
}

// Type returns the message type.
func (m *MsgSubmitProposal) Type() string {
	return TypeMsgSubmitProposal
}

// GetSigners returns the signers.
func (m *MsgSubmitProposal) GetSigners() []types.AccountName {
	return []types.AccountName{m.Proposer}
}

// ValidateBasic performs stateless validation.
func (m *MsgSubmitProposal) ValidateBasic() error {
	if !m.Proposer.IsValid() {
		return fmt.Errorf("invalid proposer account: %s", m.Proposer)
	}
	if m.Title == "" {
		return fmt.Errorf("proposal title cannot be empty")
	}
	if len(m.Title) > 256 {
		return fmt.Errorf("proposal title too long (max 256 characters)")
	}
	if m.Description == "" {
		return fmt.Errorf("proposal description cannot be empty")
	}
	if len(m.Description) > 10000 {
		return fmt.Errorf("proposal description too long (max 10000 characters)")
	}
	if m.ProposalType == "" {
		return fmt.Errorf("proposal type cannot be empty")
	}
	if !m.InitialDeposit.IsValid() {
		return fmt.Errorf("invalid initial deposit")
	}
	return nil
}

// MsgVote votes on a governance proposal.
type MsgVote struct {
	// Voter is the account casting the vote.
	Voter types.AccountName `json:"voter"`

	// ProposalID is the proposal to vote on.
	ProposalID uint64 `json:"proposal_id"`

	// Option is the vote option.
	Option VoteOption `json:"option"`
}

// Type returns the message type.
func (m *MsgVote) Type() string {
	return TypeMsgVote
}

// GetSigners returns the signers.
func (m *MsgVote) GetSigners() []types.AccountName {
	return []types.AccountName{m.Voter}
}

// ValidateBasic performs stateless validation.
func (m *MsgVote) ValidateBasic() error {
	if !m.Voter.IsValid() {
		return fmt.Errorf("invalid voter account: %s", m.Voter)
	}
	if m.ProposalID == 0 {
		return fmt.Errorf("proposal ID cannot be zero")
	}
	if !m.Option.IsValid() {
		return fmt.Errorf("invalid vote option: %s", m.Option)
	}
	return nil
}

// MsgDeposit deposits tokens on a governance proposal.
type MsgDeposit struct {
	// Depositor is the account making the deposit.
	Depositor types.AccountName `json:"depositor"`

	// ProposalID is the proposal to deposit on.
	ProposalID uint64 `json:"proposal_id"`

	// Amount is the deposit amount.
	Amount types.Coin `json:"amount"`
}

// Type returns the message type.
func (m *MsgDeposit) Type() string {
	return TypeMsgDeposit
}

// GetSigners returns the signers.
func (m *MsgDeposit) GetSigners() []types.AccountName {
	return []types.AccountName{m.Depositor}
}

// ValidateBasic performs stateless validation.
func (m *MsgDeposit) ValidateBasic() error {
	if !m.Depositor.IsValid() {
		return fmt.Errorf("invalid depositor account: %s", m.Depositor)
	}
	if m.ProposalID == 0 {
		return fmt.Errorf("proposal ID cannot be zero")
	}
	if !m.Amount.IsValid() {
		return fmt.Errorf("invalid deposit amount")
	}
	if !m.Amount.IsPositive() {
		return fmt.Errorf("deposit amount must be positive")
	}
	return nil
}
