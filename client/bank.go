package client

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// BankClient provides bank module query methods.
type BankClient struct {
	client *Client
}

// Bank returns a bank module client.
func (c *Client) Bank() *BankClient {
	return &BankClient{client: c}
}

// BalanceResponse contains balance information.
type BalanceResponse struct {
	Account string `json:"account"`
	Denom   string `json:"denom"`
	Amount  uint64 `json:"amount"`
}

// AllBalancesResponse contains all balances for an account.
type AllBalancesResponse struct {
	Account  string       `json:"account"`
	Balances []types.Coin `json:"balances"`
}

// SupplyResponse contains total supply for a denomination.
type SupplyResponse struct {
	Denom  string `json:"denom"`
	Amount uint64 `json:"amount"`
}

// Balance retrieves the balance of a specific denomination for an account.
func (b *BankClient) Balance(ctx context.Context, account types.AccountName, denom string) (*BalanceResponse, error) {
	return b.BalanceAtHeight(ctx, account, denom, 0)
}

// BalanceAtHeight retrieves a balance at a specific block height.
func (b *BankClient) BalanceAtHeight(ctx context.Context, account types.AccountName, denom string, height int64) (*BalanceResponse, error) {
	if !account.IsValid() {
		return nil, fmt.Errorf("invalid account name: %s", account)
	}
	if denom == "" {
		return nil, fmt.Errorf("denom cannot be empty")
	}

	// Server handler expects "account/denom" format
	data := []byte(fmt.Sprintf("%s/%s", account, denom))

	var resp BalanceResponse
	if err := b.client.QueryJSON(ctx, "/bank/balance", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// AllBalances retrieves all balances for an account.
func (b *BankClient) AllBalances(ctx context.Context, account types.AccountName) (*AllBalancesResponse, error) {
	return b.AllBalancesAtHeight(ctx, account, 0)
}

// AllBalancesAtHeight retrieves all balances at a specific block height.
func (b *BankClient) AllBalancesAtHeight(ctx context.Context, account types.AccountName, height int64) (*AllBalancesResponse, error) {
	if !account.IsValid() {
		return nil, fmt.Errorf("invalid account name: %s", account)
	}

	// Server handler expects raw account name bytes
	data := []byte(account)

	var resp AllBalancesResponse
	if err := b.client.QueryJSON(ctx, "/bank/balances", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// TotalSupply retrieves the total supply of a denomination.
func (b *BankClient) TotalSupply(ctx context.Context, denom string) (*SupplyResponse, error) {
	return b.TotalSupplyAtHeight(ctx, denom, 0)
}

// TotalSupplyAtHeight retrieves the total supply at a specific block height.
func (b *BankClient) TotalSupplyAtHeight(ctx context.Context, denom string, height int64) (*SupplyResponse, error) {
	if denom == "" {
		return nil, fmt.Errorf("denom cannot be empty")
	}

	// Server handler expects raw denom bytes
	data := []byte(denom)

	var resp SupplyResponse
	if err := b.client.QueryJSON(ctx, "/bank/supply", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
