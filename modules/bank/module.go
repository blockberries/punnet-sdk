package bank

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/module"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/types"
)

// Module name
const ModuleName = "bank"

// BankModule provides token transfer functionality
type BankModule struct {
	balanceCap capability.BalanceCapability
}

// NewBankModule creates a new bank module with the given capability
func NewBankModule(balanceCap capability.BalanceCapability) (*BankModule, error) {
	if balanceCap == nil {
		return nil, fmt.Errorf("balance capability cannot be nil")
	}

	return &BankModule{
		balanceCap: balanceCap,
	}, nil
}

// CreateModule creates the bank module using the module builder
func CreateModule(balanceCap capability.BalanceCapability) (module.Module, error) {
	if balanceCap == nil {
		return nil, fmt.Errorf("balance capability cannot be nil")
	}

	bankMod, err := NewBankModule(balanceCap)
	if err != nil {
		return nil, fmt.Errorf("failed to create bank module: %w", err)
	}

	return module.NewModuleBuilder(ModuleName).
		WithMsgHandler(TypeMsgSend, bankMod.handleSend).
		WithMsgHandler(TypeMsgMultiSend, bankMod.handleMultiSend).
		WithQueryHandler("/balance", bankMod.handleQueryBalance).
		WithQueryHandler("/all_balances", bankMod.handleQueryAllBalances).
		Build()
}

// handleSend handles MsgSend
func (m *BankModule) handleSend(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.balanceCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	sendMsg, ok := msg.(*MsgSend)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgSend")
	}

	// Verify the sender is the transaction signer
	if sendMsg.From != ctx.Account() {
		return nil, fmt.Errorf("sender must be transaction account")
	}

	// Check sender has sufficient balance
	balance, err := m.balanceCap.GetBalance(ctx.Context(), sendMsg.From, sendMsg.Amount.Denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	if balance < sendMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient balance for %s", types.ErrInsufficientFunds, sendMsg.Amount.Denom)
	}

	// Return transfer effect
	return []effects.Effect{
		effects.TransferEffect{
			From:   sendMsg.From,
			To:     sendMsg.To,
			Amount: types.NewCoins(sendMsg.Amount),
		},
		effects.NewEventEffect("bank.send", map[string][]byte{
			"from":   []byte(sendMsg.From),
			"to":     []byte(sendMsg.To),
			"denom":  []byte(sendMsg.Amount.Denom),
			"amount": []byte(fmt.Sprintf("%d", sendMsg.Amount.Amount)),
			"height": []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	}, nil
}

// handleMultiSend handles MsgMultiSend
func (m *BankModule) handleMultiSend(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.balanceCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	multiSendMsg, ok := msg.(*MsgMultiSend)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgMultiSend")
	}

	// Verify all senders are authorized
	signers := multiSendMsg.GetSigners()
	found := false
	for _, signer := range signers {
		if signer == ctx.Account() {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("transaction account must be one of the senders")
	}

	// Check all senders have sufficient balances
	for _, input := range multiSendMsg.Inputs {
		for _, coin := range input.Coins {
			balance, err := m.balanceCap.GetBalance(ctx.Context(), input.Address, coin.Denom)
			if err != nil {
				return nil, fmt.Errorf("failed to get balance for %s: %w", input.Address, err)
			}

			if balance < coin.Amount {
				return nil, fmt.Errorf("%w: insufficient balance for %s denom %s",
					types.ErrInsufficientFunds, input.Address, coin.Denom)
			}
		}
	}

	// Create transfer effects for each input-output pair
	var transferEffects []effects.Effect

	// First subtract from all inputs
	for _, input := range multiSendMsg.Inputs {
		for _, coin := range input.Coins {
			// Create a write effect for balance subtraction
			transferEffects = append(transferEffects,
				effects.WriteEffect[uint64]{
					Store:    "balance_sub",
					StoreKey: []byte(fmt.Sprintf("%s/%s", input.Address, coin.Denom)),
					Value:    coin.Amount,
				},
			)
		}
	}

	// Then add to all outputs
	for _, output := range multiSendMsg.Outputs {
		for _, coin := range output.Coins {
			transferEffects = append(transferEffects,
				effects.WriteEffect[uint64]{
					Store:    "balance_add",
					StoreKey: []byte(fmt.Sprintf("%s/%s", output.Address, coin.Denom)),
					Value:    coin.Amount,
				},
			)
		}
	}

	// Add event effect
	transferEffects = append(transferEffects,
		effects.NewEventEffect("bank.multi_send", map[string][]byte{
			"num_inputs":  []byte(fmt.Sprintf("%d", len(multiSendMsg.Inputs))),
			"num_outputs": []byte(fmt.Sprintf("%d", len(multiSendMsg.Outputs))),
			"height":      []byte(fmt.Sprintf("%d", ctx.BlockHeight())),
		}),
	)

	return transferEffects, nil
}

// QueryBalanceRequest is the request for balance query
type QueryBalanceRequest struct {
	Account types.AccountName `json:"account"`
	Denom   string            `json:"denom"`
}

// QueryBalanceResponse is the response for balance query
type QueryBalanceResponse struct {
	Balance uint64 `json:"balance"`
}

// handleQueryBalance handles balance queries
func (m *BankModule) handleQueryBalance(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.balanceCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	// TODO: Proper deserialization
	// For now, expect format: "account/denom"
	parts := splitOnce(string(data), '/')
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid query format: expected account/denom")
	}

	account := types.AccountName(parts[0])
	denom := parts[1]

	if !account.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	if denom == "" {
		return nil, fmt.Errorf("denom cannot be empty")
	}

	balance, err := m.balanceCap.GetBalance(ctx, account, denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// TODO: Proper serialization
	return []byte(fmt.Sprintf("%d", balance)), nil
}

// QueryAllBalancesRequest is the request for all balances query
type QueryAllBalancesRequest struct {
	Account types.AccountName `json:"account"`
}

// QueryAllBalancesResponse is the response for all balances query
type QueryAllBalancesResponse struct {
	Balances types.Coins `json:"balances"`
}

// handleQueryAllBalances handles all balances queries
func (m *BankModule) handleQueryAllBalances(ctx context.Context, path string, data []byte) ([]byte, error) {
	if m == nil || m.balanceCap == nil {
		return nil, fmt.Errorf("module or capability is nil")
	}

	// For now, treat data as account name
	account := types.AccountName(data)
	if !account.IsValid() {
		return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
	}

	balances, err := m.balanceCap.GetAccountBalances(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}

	// TODO: Proper serialization
	return []byte(balances.String()), nil
}

// splitOnce splits a string on the first occurrence of a separator
func splitOnce(s string, sep rune) []string {
	for i, c := range s {
		if c == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
