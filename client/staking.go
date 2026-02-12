package client

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// StakingClient provides staking module query methods.
type StakingClient struct {
	client *Client
}

// Staking returns a staking module client.
func (c *Client) Staking() *StakingClient {
	return &StakingClient{client: c}
}

// ValidatorResponse contains validator information.
type ValidatorResponse struct {
	Name             string `json:"name"`
	PubKey           string `json:"pub_key"`
	VotingPower      uint64 `json:"voting_power"`
	Commission       uint64 `json:"commission"`      // basis points (1/100th of percent)
	Description      string `json:"description"`
	Status           string `json:"status"`          // bonded, unbonding, unbonded
	Jailed           bool   `json:"jailed"`
	UnbondingHeight  int64  `json:"unbonding_height,omitempty"`
	SelfDelegation   uint64 `json:"self_delegation"`
	TotalDelegation  uint64 `json:"total_delegation"`
}

// DelegationResponse contains delegation information.
type DelegationResponse struct {
	Delegator string `json:"delegator"`
	Validator string `json:"validator"`
	Amount    uint64 `json:"amount"`
	Denom     string `json:"denom"`
}

// UnbondingDelegationResponse contains unbonding delegation information.
type UnbondingDelegationResponse struct {
	Delegator        string `json:"delegator"`
	Validator        string `json:"validator"`
	Amount           uint64 `json:"amount"`
	Denom            string `json:"denom"`
	CompletionHeight int64  `json:"completion_height"`
}

// StakingParamsResponse contains staking parameters.
type StakingParamsResponse struct {
	BondDenom         string `json:"bond_denom"`
	UnbondingPeriod   int64  `json:"unbonding_period"`
	MaxValidators     uint32 `json:"max_validators"`
	MinSelfDelegation uint64 `json:"min_self_delegation"`
}

// Validator retrieves a validator by name.
func (s *StakingClient) Validator(ctx context.Context, name types.AccountName) (*ValidatorResponse, error) {
	return s.ValidatorAtHeight(ctx, name, 0)
}

// ValidatorAtHeight retrieves a validator at a specific block height.
func (s *StakingClient) ValidatorAtHeight(ctx context.Context, name types.AccountName, height int64) (*ValidatorResponse, error) {
	if !name.IsValid() {
		return nil, fmt.Errorf("invalid validator name: %s", name)
	}

	data, err := encodeQueryData(map[string]string{"name": string(name)})
	if err != nil {
		return nil, fmt.Errorf("encode query data: %w", err)
	}

	var resp ValidatorResponse
	if err := s.client.QueryJSON(ctx, "/staking/validator", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// Validators retrieves all validators.
func (s *StakingClient) Validators(ctx context.Context) ([]ValidatorResponse, error) {
	return s.ValidatorsAtHeight(ctx, 0)
}

// ValidatorsAtHeight retrieves all validators at a specific block height.
func (s *StakingClient) ValidatorsAtHeight(ctx context.Context, height int64) ([]ValidatorResponse, error) {
	var resp []ValidatorResponse
	if err := s.client.QueryJSON(ctx, "/staking/validators", nil, height, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Delegation retrieves a delegation.
func (s *StakingClient) Delegation(ctx context.Context, delegator, validator types.AccountName) (*DelegationResponse, error) {
	return s.DelegationAtHeight(ctx, delegator, validator, 0)
}

// DelegationAtHeight retrieves a delegation at a specific block height.
func (s *StakingClient) DelegationAtHeight(ctx context.Context, delegator, validator types.AccountName, height int64) (*DelegationResponse, error) {
	if !delegator.IsValid() {
		return nil, fmt.Errorf("invalid delegator name: %s", delegator)
	}
	if !validator.IsValid() {
		return nil, fmt.Errorf("invalid validator name: %s", validator)
	}

	data, err := encodeQueryData(map[string]string{
		"delegator": string(delegator),
		"validator": string(validator),
	})
	if err != nil {
		return nil, fmt.Errorf("encode query data: %w", err)
	}

	var resp DelegationResponse
	if err := s.client.QueryJSON(ctx, "/staking/delegation", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// DelegatorDelegations retrieves all delegations for a delegator.
func (s *StakingClient) DelegatorDelegations(ctx context.Context, delegator types.AccountName) ([]DelegationResponse, error) {
	return s.DelegatorDelegationsAtHeight(ctx, delegator, 0)
}

// DelegatorDelegationsAtHeight retrieves all delegations for a delegator at a specific height.
func (s *StakingClient) DelegatorDelegationsAtHeight(ctx context.Context, delegator types.AccountName, height int64) ([]DelegationResponse, error) {
	if !delegator.IsValid() {
		return nil, fmt.Errorf("invalid delegator name: %s", delegator)
	}

	data, err := encodeQueryData(map[string]string{"delegator": string(delegator)})
	if err != nil {
		return nil, fmt.Errorf("encode query data: %w", err)
	}

	var resp []DelegationResponse
	if err := s.client.QueryJSON(ctx, "/staking/delegations", data, height, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// UnbondingDelegations retrieves all unbonding delegations for a delegator.
func (s *StakingClient) UnbondingDelegations(ctx context.Context, delegator types.AccountName) ([]UnbondingDelegationResponse, error) {
	if !delegator.IsValid() {
		return nil, fmt.Errorf("invalid delegator name: %s", delegator)
	}

	data, err := encodeQueryData(map[string]string{"delegator": string(delegator)})
	if err != nil {
		return nil, fmt.Errorf("encode query data: %w", err)
	}

	var resp []UnbondingDelegationResponse
	if err := s.client.QueryJSON(ctx, "/staking/unbonding_delegations", data, 0, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// Params retrieves staking parameters.
func (s *StakingClient) Params(ctx context.Context) (*StakingParamsResponse, error) {
	var resp StakingParamsResponse
	if err := s.client.QueryJSON(ctx, "/staking/params", nil, 0, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
