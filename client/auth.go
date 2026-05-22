package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// AuthClient provides auth module query methods.
type AuthClient struct {
	client *Client
}

// Auth returns an auth module client.
func (c *Client) Auth() *AuthClient {
	return &AuthClient{client: c}
}

// AccountResponse contains account information.
type AccountResponse struct {
	Name      string          `json:"name"`
	Nonce     uint64          `json:"nonce"`
	Authority json.RawMessage `json:"authority"`
}

// Account retrieves an account by name.
func (a *AuthClient) Account(ctx context.Context, name types.AccountName) (*AccountResponse, error) {
	return a.AccountAtHeight(ctx, name, 0)
}

// AccountAtHeight retrieves an account at a specific block height.
func (a *AuthClient) AccountAtHeight(ctx context.Context, name types.AccountName, height int64) (*AccountResponse, error) {
	if !name.IsValid() {
		return nil, fmt.Errorf("invalid account name: %s", name)
	}

	// Send raw account name bytes - the server handler expects plain bytes
	data := []byte(name)

	var resp AccountResponse
	if err := a.client.QueryJSON(ctx, "/auth/account", data, height, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// AccountExists checks if an account exists.
func (a *AuthClient) AccountExists(ctx context.Context, name types.AccountName) (bool, error) {
	_, err := a.Account(ctx, name)
	if err != nil {
		if qe, ok := err.(*QueryError); ok && qe.Code == 1 {
			// Account not found
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Nonce retrieves the current nonce for an account.
func (a *AuthClient) Nonce(ctx context.Context, name types.AccountName) (uint64, error) {
	account, err := a.Account(ctx, name)
	if err != nil {
		return 0, err
	}
	return account.Nonce, nil
}
