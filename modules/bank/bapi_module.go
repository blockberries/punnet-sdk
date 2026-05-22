package bank

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// BAPIBankModule provides token transfer functionality for BAPI-based applications.
// It implements runtime.BAPIModule and runtime.BAPIGenesisInitializer.
type BAPIBankModule struct {
	balanceStore *store.BAPIBalanceStore
}

// NewBAPIBankModule creates a new BAPI bank module with the given balance store.
func NewBAPIBankModule(balanceStore *store.BAPIBalanceStore) (*BAPIBankModule, error) {
	if balanceStore == nil {
		return nil, fmt.Errorf("balance store cannot be nil")
	}

	return &BAPIBankModule{
		balanceStore: balanceStore,
	}, nil
}

// Name returns the module's unique name.
func (m *BAPIBankModule) Name() string {
	return ModuleName
}

// RegisterMsgHandlers returns message handlers keyed by message type.
func (m *BAPIBankModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgSend:      m.handleSend,
		TypeMsgMultiSend: m.handleMultiSend,
	}
}

// RegisterQueryHandlers returns query handlers keyed by query path.
func (m *BAPIBankModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/bank/balance":      m.handleQueryBalance,
		"/bank/all_balances": m.handleQueryAllBalances,
	}
}

// InitGenesis initializes the module's state from genesis data.
func (m *BAPIBankModule) InitGenesis(ctx context.Context, data []byte) error {
	if len(data) == 0 {
		return nil // No genesis data for bank module is acceptable
	}

	var genesisState BankGenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		return fmt.Errorf("unmarshal bank genesis: %w", err)
	}

	// Set all genesis balances
	for _, balance := range genesisState.Balances {
		if err := m.balanceStore.Set(ctx, balance.Account, balance.Denom, balance.Amount); err != nil {
			return fmt.Errorf("set genesis balance for %s/%s: %w", balance.Account, balance.Denom, err)
		}
	}

	return nil
}

// ExportGenesis exports the module's state for genesis.
func (m *BAPIBankModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	// For now, return empty state - full export would require iteration
	genesisState := BankGenesisState{
		Balances: []GenesisBalance{},
	}
	return json.Marshal(genesisState)
}

// BankGenesisState represents the bank module's genesis state.
type BankGenesisState struct {
	Balances []GenesisBalance `json:"balances"`
}

// GenesisBalance represents a balance entry in genesis.
type GenesisBalance struct {
	Account string `json:"account"`
	Denom   string `json:"denom"`
	Amount  uint64 `json:"amount"`
}

// handleSend handles MsgSend.
func (m *BAPIBankModule) handleSend(ctx context.Context, txCtx *runtime.BAPITxContext, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	sendMsg, ok := msg.(*MsgSend)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgSend, got %T", msg)
	}

	// Verify the sender is the transaction signer
	if sendMsg.From != txCtx.Account {
		return nil, fmt.Errorf("sender must be transaction account")
	}

	// Check sender has sufficient balance
	balance, err := m.balanceStore.GetAmount(ctx, string(sendMsg.From), sendMsg.Amount.Denom)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	if balance < sendMsg.Amount.Amount {
		return nil, fmt.Errorf("%w: insufficient balance for %s (have %d, need %d)",
			types.ErrInsufficientFunds, sendMsg.Amount.Denom, balance, sendMsg.Amount.Amount)
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
			"height": []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	}, nil
}

// handleMultiSend handles MsgMultiSend.
func (m *BAPIBankModule) handleMultiSend(ctx context.Context, txCtx *runtime.BAPITxContext, msg types.Message) ([]effects.Effect, error) {
	if m == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}
	if txCtx == nil {
		return nil, fmt.Errorf("transaction context is nil")
	}

	multiSendMsg, ok := msg.(*MsgMultiSend)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgMultiSend, got %T", msg)
	}

	// Verify all senders are authorized (at least one must match the transaction account)
	signers := multiSendMsg.GetSigners()
	found := false
	for _, signer := range signers {
		if signer == txCtx.Account {
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
			balance, err := m.balanceStore.GetAmount(ctx, string(input.Address), coin.Denom)
			if err != nil {
				return nil, fmt.Errorf("failed to get balance for %s: %w", input.Address, err)
			}

			if balance < coin.Amount {
				return nil, fmt.Errorf("%w: insufficient balance for %s denom %s (have %d, need %d)",
					types.ErrInsufficientFunds, input.Address, coin.Denom, balance, coin.Amount)
			}
		}
	}

	// Create transfer effects for each input->output combination
	var transferEffects []effects.Effect

	// Build a map of total outputs needed per account and denom
	outputNeeds := make(map[string]map[string]uint64) // account -> denom -> amount
	for _, output := range multiSendMsg.Outputs {
		if outputNeeds[string(output.Address)] == nil {
			outputNeeds[string(output.Address)] = make(map[string]uint64)
		}
		for _, coin := range output.Coins {
			outputNeeds[string(output.Address)][coin.Denom] += coin.Amount
		}
	}

	// Sort recipient account names so the effect order is deterministic.
	// Iterating outputNeeds directly used Go map order, which differs run-to-run
	// and produces different effect lists on different validators for the same
	// MsgMultiSend — a consensus split (PLAN T1-7). Snapshotting the keys once
	// (instead of inside the inner loop) is sufficient: every iteration of the
	// outer loop sees the same key ordering even as the inner loop mutates
	// `outputNeeds[acct][coin.Denom]`.
	sortedRecipients := make([]string, 0, len(outputNeeds))
	for acct := range outputNeeds {
		sortedRecipients = append(sortedRecipients, acct)
	}
	sort.Strings(sortedRecipients)

	// Create transfers from inputs to outputs
	for _, input := range multiSendMsg.Inputs {
		for _, coin := range input.Coins {
			// Find outputs that need this denomination and transfer to them
			remaining := coin.Amount
			for _, acct := range sortedRecipients {
				denoms := outputNeeds[acct]
				if needed, ok := denoms[coin.Denom]; ok && needed > 0 {
					// Transfer what we can
					toTransfer := remaining
					if toTransfer > needed {
						toTransfer = needed
					}

					transferEffects = append(transferEffects, effects.TransferEffect{
						From:   input.Address,
						To:     types.AccountName(acct),
						Amount: types.Coins{{Denom: coin.Denom, Amount: toTransfer}},
					})

					outputNeeds[acct][coin.Denom] -= toTransfer
					remaining -= toTransfer

					if remaining == 0 {
						break
					}
				}
			}
		}
	}

	// Add event effect
	transferEffects = append(transferEffects,
		effects.NewEventEffect("bank.multi_send", map[string][]byte{
			"num_inputs":  []byte(fmt.Sprintf("%d", len(multiSendMsg.Inputs))),
			"num_outputs": []byte(fmt.Sprintf("%d", len(multiSendMsg.Outputs))),
			"height":      []byte(fmt.Sprintf("%d", txCtx.Height)),
		}),
	)

	return transferEffects, nil
}

// handleQueryBalance handles balance queries.
func (m *BAPIBankModule) handleQueryBalance(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	// Expect format: "account/denom"
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

	var balance uint64
	var err error

	// Get balance at specific height if requested
	if height > 0 {
		balanceObj, err := m.balanceStore.GetAtHeight(ctx, string(account), denom, height)
		if err != nil {
			return nil, fmt.Errorf("failed to get balance: %w", err)
		}
		balance = balanceObj.Amount
	} else {
		balance, err = m.balanceStore.GetAmount(ctx, string(account), denom)
		if err != nil {
			return nil, fmt.Errorf("failed to get balance: %w", err)
		}
	}

	return json.Marshal(balance)
}

// AllBalancesResponse is the JSON response shape for /bank/all_balances. It is
// exported so external callers (e.g. RPC tests, integration suites) can decode
// the response without re-deriving the format. PLAN B2-4.
type AllBalancesResponse struct {
	Account  string         `json:"account"`
	Balances []GenesisBalance `json:"balances"`
}

// handleQueryAllBalances returns every denom balance for the requested
// account. It uses the in-memory key index maintained by BAPIBalanceStore
// (TrackedKeys) to enumerate all "<account>/<denom>" keys under the balances
// prefix, then filters to the requested account. PLAN B2-4 replaces the prior
// placeholder which returned a hard-coded note explaining iteration was not
// supported.
//
// Empty `data` means "list balances for every known account" — useful for
// genesis export and tests. A non-empty `data` is interpreted as an account
// name (matching the prior contract) and only balances belonging to that
// account are returned.
func (m *BAPIBankModule) handleQueryAllBalances(ctx context.Context, data []byte, height int64) ([]byte, error) {
	if m == nil || m.balanceStore == nil {
		return nil, fmt.Errorf("module or store is nil")
	}

	var filterAccount types.AccountName
	if len(data) > 0 {
		filterAccount = types.AccountName(data)
		if !filterAccount.IsValid() {
			return nil, fmt.Errorf("%w: invalid account name", types.ErrInvalidAccount)
		}
	}

	// TrackedKeys is pre-sorted, so the response is deterministic.
	keys := m.balanceStore.TrackedKeys()
	balances := make([]GenesisBalance, 0, len(keys))

	for _, k := range keys {
		// Keys are formatted as "account/denom" by balanceKey().
		account, denom, ok := splitBalanceKey(k)
		if !ok {
			continue
		}
		if filterAccount != "" && types.AccountName(account) != filterAccount {
			continue
		}
		amount, err := m.balanceStore.GetAmount(ctx, account, denom)
		if err != nil {
			return nil, fmt.Errorf("get balance %s/%s: %w", account, denom, err)
		}
		// Skip zero balances — the tracker may retain a key whose row was
		// rewritten to 0 by SubAmount. This matches the existing zero-
		// balance semantics in BAPIBalanceStore.Get.
		if amount == 0 {
			continue
		}
		balances = append(balances, GenesisBalance{
			Account: account,
			Denom:   denom,
			Amount:  amount,
		})
	}

	return json.Marshal(AllBalancesResponse{
		Account:  string(filterAccount),
		Balances: balances,
	})
}

// splitBalanceKey reverses balanceKey: it splits "account/denom" on the first
// '/'. Returns the account, denom, and a success flag.
func splitBalanceKey(k string) (string, string, bool) {
	idx := strings.IndexByte(k, '/')
	if idx <= 0 || idx == len(k)-1 {
		return "", "", false
	}
	return k[:idx], k[idx+1:], true
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule             = (*BAPIBankModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIBankModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIBankModule)(nil)
)
