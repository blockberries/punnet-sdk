package integration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/modules/auth"
	"github.com/blockberries/punnet-sdk/modules/bank"
	"github.com/blockberries/punnet-sdk/modules/governance"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	sdktypes "github.com/blockberries/punnet-sdk/types"
)

// bapiTestEnv encapsulates the BAPI test environment
type bapiTestEnv struct {
	ctx            context.Context
	stateStore     statestore.StateStore
	app            *runtime.BAPIApplication
	accountStore   *store.BAPIAccountStore
	balanceStore   *store.BAPIBalanceStore
	validatorStore *store.BAPIValidatorStore
	proposalStore  *store.BAPIProposalStore
}

// setupBAPITestEnv creates a BAPI test environment with all modules
func setupBAPITestEnv(t *testing.T) *bapiTestEnv {
	t.Helper()

	// Create in-memory IAVL state store
	ss, err := statestore.NewMemoryIAVLStore(1000)
	if err != nil {
		t.Fatalf("failed to create state store: %v", err)
	}

	// Create typed stores
	accountStore := store.NewBAPIAccountStore(ss)
	balanceStore := store.NewBAPIBalanceStore(ss)
	validatorStore := store.NewBAPIValidatorStore(ss)
	proposalStore := store.NewBAPIProposalStore(ss)

	// Create modules
	authMod, err := auth.NewBAPIAuthModule(accountStore)
	if err != nil {
		t.Fatalf("failed to create auth module: %v", err)
	}

	bankMod, err := bank.NewBAPIBankModule(balanceStore)
	if err != nil {
		t.Fatalf("failed to create bank module: %v", err)
	}

	stakingMod, err := staking.NewBAPIStakingModule(validatorStore, balanceStore)
	if err != nil {
		t.Fatalf("failed to create staking module: %v", err)
	}

	govMod, err := governance.NewBAPIGovernanceModule(proposalStore, balanceStore)
	if err != nil {
		t.Fatalf("failed to create governance module: %v", err)
	}

	// Create application
	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "test-chain-1",
		StateStore: ss,
		Modules: []runtime.BAPIModule{
			authMod,
			bankMod,
			stakingMod,
			govMod,
		},
	})
	if err != nil {
		t.Fatalf("failed to create application: %v", err)
	}

	return &bapiTestEnv{
		ctx:            context.Background(),
		stateStore:     ss,
		app:            app,
		accountStore:   accountStore,
		balanceStore:   balanceStore,
		validatorStore: validatorStore,
		proposalStore:  proposalStore,
	}
}

// TestBAPIApplicationCreation tests creating a BAPI application
func TestBAPIApplicationCreation(t *testing.T) {
	env := setupBAPITestEnv(t)

	if env.app.ChainID() != "test-chain-1" {
		t.Errorf("expected chain_id 'test-chain-1', got %s", env.app.ChainID())
	}

	router := env.app.Router()
	modules := router.Modules()

	// Verify all modules are registered
	moduleNames := make(map[string]bool)
	for _, mod := range modules {
		moduleNames[mod.Name()] = true
	}

	expectedModules := []string{"auth", "bank", "staking", "governance"}
	for _, name := range expectedModules {
		if !moduleNames[name] {
			t.Errorf("module %s not registered", name)
		}
	}
}

// TestBAPIAccountOperations tests account creation and retrieval via BAPI stores
func TestBAPIAccountOperations(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create account directly in store (simulating genesis or message execution)
	pubKey := make([]byte, 32)
	copy(pubKey, "alice-pubkey-12345678901234567")

	alice := sdktypes.NewAccount("alice", pubKey)
	if err := env.accountStore.Set(env.ctx, alice); err != nil {
		t.Fatalf("failed to set alice account: %v", err)
	}

	// Retrieve account
	retrieved, err := env.accountStore.Get(env.ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get alice account: %v", err)
	}

	if string(retrieved.Name) != "alice" {
		t.Errorf("expected name alice, got %s", retrieved.Name)
	}

	// Check account existence
	exists, err := env.accountStore.Has(env.ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check alice existence: %v", err)
	}
	if !exists {
		t.Error("alice should exist")
	}

	// Check non-existent account
	exists, err = env.accountStore.Has(env.ctx, "nonexistent")
	if err != nil {
		t.Fatalf("failed to check nonexistent account: %v", err)
	}
	if exists {
		t.Error("nonexistent account should not exist")
	}
}

// TestBAPIBalanceOperations tests balance operations via BAPI stores
func TestBAPIBalanceOperations(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Set balances
	if err := env.balanceStore.Set(env.ctx, "alice", "stake", 1000); err != nil {
		t.Fatalf("failed to set alice stake balance: %v", err)
	}

	if err := env.balanceStore.Set(env.ctx, "alice", "token", 500); err != nil {
		t.Fatalf("failed to set alice token balance: %v", err)
	}

	// Get balances
	stakeBalance, err := env.balanceStore.GetAmount(env.ctx, "alice", "stake")
	if err != nil {
		t.Fatalf("failed to get alice stake balance: %v", err)
	}
	if stakeBalance != 1000 {
		t.Errorf("expected stake balance 1000, got %d", stakeBalance)
	}

	tokenBalance, err := env.balanceStore.GetAmount(env.ctx, "alice", "token")
	if err != nil {
		t.Fatalf("failed to get alice token balance: %v", err)
	}
	if tokenBalance != 500 {
		t.Errorf("expected token balance 500, got %d", tokenBalance)
	}

	// Test add balance
	_, err = env.balanceStore.AddAmount(env.ctx, "alice", "stake", 200)
	if err != nil {
		t.Fatalf("failed to add to alice stake balance: %v", err)
	}

	stakeBalance, err = env.balanceStore.GetAmount(env.ctx, "alice", "stake")
	if err != nil {
		t.Fatalf("failed to get updated alice stake balance: %v", err)
	}
	if stakeBalance != 1200 {
		t.Errorf("expected stake balance 1200, got %d", stakeBalance)
	}

	// Test subtract balance
	_, err = env.balanceStore.SubAmount(env.ctx, "alice", "stake", 300)
	if err != nil {
		t.Fatalf("failed to subtract from alice stake balance: %v", err)
	}

	stakeBalance, err = env.balanceStore.GetAmount(env.ctx, "alice", "stake")
	if err != nil {
		t.Fatalf("failed to get final alice stake balance: %v", err)
	}
	if stakeBalance != 900 {
		t.Errorf("expected stake balance 900, got %d", stakeBalance)
	}
}

// TestBAPIValidatorOperations tests validator operations via BAPI stores
func TestBAPIValidatorOperations(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create validator pubkey
	pubKey := make([]byte, 32)
	copy(pubKey, "validator-pubkey-123456789012")

	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           1000,
		Commission:      500, // 5%
		Description:     "Test validator",
		TotalDelegation: 500,
	}

	if err := env.validatorStore.SetValidator(env.ctx, validator); err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	// Retrieve validator
	retrieved, err := env.validatorStore.GetValidator(env.ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to get validator: %v", err)
	}

	if retrieved.Power != 1000 {
		t.Errorf("expected power 1000, got %d", retrieved.Power)
	}
	if retrieved.Description != "Test validator" {
		t.Errorf("expected description 'Test validator', got %s", retrieved.Description)
	}

	// Update power
	if err := env.validatorStore.UpdatePower(env.ctx, pubKey, 1500); err != nil {
		t.Fatalf("failed to update power: %v", err)
	}

	retrieved, err = env.validatorStore.GetValidator(env.ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to get updated validator: %v", err)
	}
	if retrieved.Power != 1500 {
		t.Errorf("expected power 1500, got %d", retrieved.Power)
	}
}

// TestBAPIProposalOperations tests governance proposal operations
func TestBAPIProposalOperations(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Get next proposal ID
	proposalID, err := env.proposalStore.GetNextProposalID(env.ctx)
	if err != nil {
		t.Fatalf("failed to get next proposal ID: %v", err)
	}
	if proposalID != 1 {
		t.Errorf("expected first proposal ID 1, got %d", proposalID)
	}

	// Create proposal
	now := time.Now().UTC()
	proposal := &store.BAPIProposal{
		ID:             proposalID,
		Proposer:       "alice",
		Title:          "Test Proposal",
		Description:    "This is a test proposal",
		Type:           "text",
		Status:         "deposit",
		TotalDeposit:   1000,
		DepositDenom:   "stake",
		SubmitTime:     now.Unix(),
		DepositEndTime: now.Add(14 * 24 * time.Hour).Unix(),
	}

	if err := env.proposalStore.SetProposal(env.ctx, proposal); err != nil {
		t.Fatalf("failed to set proposal: %v", err)
	}

	// Retrieve proposal
	retrieved, err := env.proposalStore.GetProposal(env.ctx, proposalID)
	if err != nil {
		t.Fatalf("failed to get proposal: %v", err)
	}

	if retrieved.Title != "Test Proposal" {
		t.Errorf("expected title 'Test Proposal', got %s", retrieved.Title)
	}
	if retrieved.Status != "deposit" {
		t.Errorf("expected status 'deposit', got %s", retrieved.Status)
	}

	// Start voting
	votingStart := now.Add(1 * time.Hour)
	votingEnd := votingStart.Add(14 * 24 * time.Hour)
	if err := env.proposalStore.StartVoting(env.ctx, proposalID, votingStart, votingEnd); err != nil {
		t.Fatalf("failed to start voting: %v", err)
	}

	retrieved, err = env.proposalStore.GetProposal(env.ctx, proposalID)
	if err != nil {
		t.Fatalf("failed to get proposal after voting start: %v", err)
	}
	if retrieved.Status != "voting" {
		t.Errorf("expected status 'voting', got %s", retrieved.Status)
	}

	// Record vote
	if err := env.proposalStore.RecordVote(env.ctx, proposalID, "alice", "yes", 100, now); err != nil {
		t.Fatalf("failed to record vote: %v", err)
	}

	retrieved, err = env.proposalStore.GetProposal(env.ctx, proposalID)
	if err != nil {
		t.Fatalf("failed to get proposal after vote: %v", err)
	}
	if retrieved.YesVotes != 100 {
		t.Errorf("expected yes votes 100, got %d", retrieved.YesVotes)
	}

	// Try to vote again (should fail)
	err = env.proposalStore.RecordVote(env.ctx, proposalID, "alice", "no", 100, now)
	if err == nil {
		t.Error("expected error for duplicate vote")
	}
}

// TestBAPIModuleHandlers tests message handlers through the router
func TestBAPIModuleHandlers(t *testing.T) {
	env := setupBAPITestEnv(t)

	router := env.app.Router()

	// Count handlers per module
	for _, mod := range router.Modules() {
		handlers := mod.RegisterMsgHandlers()
		queries := mod.RegisterQueryHandlers()

		t.Logf("Module %s: %d msg handlers, %d query handlers", mod.Name(), len(handlers), len(queries))

		if len(handlers) == 0 {
			t.Errorf("module %s has no message handlers", mod.Name())
		}
		if len(queries) == 0 {
			t.Errorf("module %s has no query handlers", mod.Name())
		}
	}
}

// TestBAPIQueryHandlers tests query handlers directly
func TestBAPIQueryHandlers(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Setup: create account and balance
	pubKey := make([]byte, 32)
	copy(pubKey, "alice-pubkey-12345678901234567")
	alice := sdktypes.NewAccount("alice", pubKey)
	if err := env.accountStore.Set(env.ctx, alice); err != nil {
		t.Fatalf("failed to set alice account: %v", err)
	}

	if err := env.balanceStore.Set(env.ctx, "alice", "stake", 1000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Test auth query handler
	router := env.app.Router()
	for _, mod := range router.Modules() {
		if mod.Name() == "auth" {
			queries := mod.RegisterQueryHandlers()
			accountQuery := queries["/auth/account"]
			if accountQuery == nil {
				t.Fatal("auth module has no /auth/account query handler")
			}

			result, err := accountQuery(env.ctx, []byte("alice"), 0)
			if err != nil {
				t.Fatalf("account query failed: %v", err)
			}

			var accountResp map[string]interface{}
			if err := json.Unmarshal(result, &accountResp); err != nil {
				t.Fatalf("failed to unmarshal account response: %v", err)
			}

			if accountResp["name"] != "alice" {
				t.Errorf("expected name alice in response, got %v", accountResp["name"])
			}
		}

		if mod.Name() == "bank" {
			queries := mod.RegisterQueryHandlers()
			balanceQuery := queries["/bank/balance"]
			if balanceQuery == nil {
				t.Fatal("bank module has no /bank/balance query handler")
			}

			result, err := balanceQuery(env.ctx, []byte("alice/stake"), 0)
			if err != nil {
				t.Fatalf("balance query failed: %v", err)
			}

			// Bank module returns just the balance amount as a number
			var balance uint64
			if err := json.Unmarshal(result, &balance); err != nil {
				t.Fatalf("failed to unmarshal balance response: %v", err)
			}

			if balance != 1000 {
				t.Errorf("expected balance 1000, got %d", balance)
			}
		}

		if mod.Name() == "governance" {
			queries := mod.RegisterQueryHandlers()
			paramsQuery := queries["/gov/params"]
			if paramsQuery == nil {
				t.Fatal("governance module has no /gov/params query handler")
			}

			result, err := paramsQuery(env.ctx, nil, 0)
			if err != nil {
				t.Fatalf("params query failed: %v", err)
			}

			var paramsResp map[string]interface{}
			if err := json.Unmarshal(result, &paramsResp); err != nil {
				t.Fatalf("failed to unmarshal params response: %v", err)
			}

			if paramsResp["deposit_denom"] != "stake" {
				t.Errorf("expected deposit_denom stake, got %v", paramsResp["deposit_denom"])
			}
		}
	}
}

// TestBAPIGenesisHandlers tests genesis initialization
func TestBAPIGenesisHandlers(t *testing.T) {
	env := setupBAPITestEnv(t)

	router := env.app.Router()

	// Test that modules can handle empty genesis
	for _, mod := range router.Modules() {
		if init, ok := mod.(runtime.BAPIGenesisInitializer); ok {
			err := init.InitGenesis(env.ctx, nil)
			if err != nil {
				t.Errorf("module %s failed to init empty genesis: %v", mod.Name(), err)
			}
		}
	}

	// Test governance genesis with params
	// Note: InitGenesis processes proposals array, not module params
	// Module params are set at construction time via WithParams()
	for _, mod := range router.Modules() {
		if mod.Name() == "governance" {
			if init, ok := mod.(runtime.BAPIGenesisInitializer); ok {
				// Empty genesis should work
				err := init.InitGenesis(env.ctx, []byte("{}"))
				if err != nil {
					t.Fatalf("governance init with empty genesis failed: %v", err)
				}
			}
		}
	}
}

// TestBAPIEndToEndTransfer tests an end-to-end transfer flow
func TestBAPIEndToEndTransfer(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Setup accounts
	pubKey1 := make([]byte, 32)
	copy(pubKey1, "alice-pubkey-12345678901234567")
	alice := sdktypes.NewAccount("alice", pubKey1)
	if err := env.accountStore.Set(env.ctx, alice); err != nil {
		t.Fatalf("failed to set alice: %v", err)
	}

	pubKey2 := make([]byte, 32)
	copy(pubKey2, "bob-pubkey-123456789012345678")
	bob := sdktypes.NewAccount("bob", pubKey2)
	if err := env.accountStore.Set(env.ctx, bob); err != nil {
		t.Fatalf("failed to set bob: %v", err)
	}

	// Set initial balances
	if err := env.balanceStore.Set(env.ctx, "alice", "stake", 1000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Verify initial state
	aliceBalance, _ := env.balanceStore.GetAmount(env.ctx, "alice", "stake")
	bobBalance, _ := env.balanceStore.GetAmount(env.ctx, "bob", "stake")

	if aliceBalance != 1000 {
		t.Errorf("expected alice initial balance 1000, got %d", aliceBalance)
	}
	if bobBalance != 0 {
		t.Errorf("expected bob initial balance 0, got %d", bobBalance)
	}

	// Simulate transfer using the Transfer method
	if err := env.balanceStore.Transfer(env.ctx, "alice", "bob", "stake", 100); err != nil {
		t.Fatalf("failed to transfer: %v", err)
	}

	// Verify final state
	aliceBalance, _ = env.balanceStore.GetAmount(env.ctx, "alice", "stake")
	bobBalance, _ = env.balanceStore.GetAmount(env.ctx, "bob", "stake")

	if aliceBalance != 900 {
		t.Errorf("expected alice final balance 900, got %d", aliceBalance)
	}
	if bobBalance != 100 {
		t.Errorf("expected bob final balance 100, got %d", bobBalance)
	}
}

// TestBAPIEndToEndStaking tests an end-to-end staking flow
func TestBAPIEndToEndStaking(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create validator pubkey
	valPubKey := make([]byte, 32)
	copy(valPubKey, "validator-pubkey-123456789012")

	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: valPubKey,
		},
		Power:           0,
		Commission:      500, // 5%
		Description:     "Test validator",
		TotalDelegation: 0,
	}
	if err := env.validatorStore.SetValidator(env.ctx, validator); err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Create delegator account and balance
	pubKey := make([]byte, 32)
	copy(pubKey, "alice-pubkey-12345678901234567")
	alice := sdktypes.NewAccount("alice", pubKey)
	if err := env.accountStore.Set(env.ctx, alice); err != nil {
		t.Fatalf("failed to set alice: %v", err)
	}
	if err := env.balanceStore.Set(env.ctx, "alice", "stake", 10000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Create delegation using the Delegate method
	if err := env.validatorStore.Delegate(env.ctx, "alice", valPubKey, 1000); err != nil {
		t.Fatalf("failed to delegate: %v", err)
	}

	// Update validator voting power
	if err := env.validatorStore.UpdatePower(env.ctx, valPubKey, 1000); err != nil {
		t.Fatalf("failed to update power: %v", err)
	}

	// Verify state
	val, _ := env.validatorStore.GetValidator(env.ctx, valPubKey)
	if val.Power != 1000 {
		t.Errorf("expected power 1000, got %d", val.Power)
	}
	if val.TotalDelegation != 1000 {
		t.Errorf("expected total delegation 1000, got %d", val.TotalDelegation)
	}

	del, _ := env.validatorStore.GetDelegation(env.ctx, "alice", valPubKey)
	if del.Amount != 1000 {
		t.Errorf("expected delegation amount 1000, got %d", del.Amount)
	}
}

// TestBAPIEndToEndGovernance tests an end-to-end governance flow
func TestBAPIEndToEndGovernance(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create proposer account and balance
	pubKey := make([]byte, 32)
	copy(pubKey, "alice-pubkey-12345678901234567")
	alice := sdktypes.NewAccount("alice", pubKey)
	if err := env.accountStore.Set(env.ctx, alice); err != nil {
		t.Fatalf("failed to set alice: %v", err)
	}
	if err := env.balanceStore.Set(env.ctx, "alice", "stake", 100000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Create voter account
	pubKey2 := make([]byte, 32)
	copy(pubKey2, "bob-pubkey-123456789012345678")
	bob := sdktypes.NewAccount("bob", pubKey2)
	if err := env.accountStore.Set(env.ctx, bob); err != nil {
		t.Fatalf("failed to set bob: %v", err)
	}
	if err := env.balanceStore.Set(env.ctx, "bob", "stake", 50000); err != nil {
		t.Fatalf("failed to set bob balance: %v", err)
	}

	// Get proposal ID
	proposalID, err := env.proposalStore.GetNextProposalID(env.ctx)
	if err != nil {
		t.Fatalf("failed to get next proposal ID: %v", err)
	}

	// Create proposal
	now := time.Now().UTC()
	proposal := &store.BAPIProposal{
		ID:             proposalID,
		Proposer:       "alice",
		Title:          "Increase block size",
		Description:    "Proposal to increase the maximum block size from 1MB to 2MB",
		Type:           "parameter",
		Status:         "deposit",
		TotalDeposit:   10000,
		DepositDenom:   "stake",
		SubmitTime:     now.Unix(),
		DepositEndTime: now.Add(14 * 24 * time.Hour).Unix(),
	}
	if err := env.proposalStore.SetProposal(env.ctx, proposal); err != nil {
		t.Fatalf("failed to create proposal: %v", err)
	}

	// Verify proposal exists
	exists, _ := env.proposalStore.HasProposal(env.ctx, proposalID)
	if !exists {
		t.Fatal("proposal should exist")
	}

	// Add deposit to reach minimum
	if err := env.proposalStore.AddDeposit(env.ctx, proposalID, "bob", 990000, "stake"); err != nil {
		t.Fatalf("failed to add deposit: %v", err)
	}

	// Start voting
	votingStart := now.Add(1 * time.Second)
	votingEnd := votingStart.Add(14 * 24 * time.Hour)
	if err := env.proposalStore.StartVoting(env.ctx, proposalID, votingStart, votingEnd); err != nil {
		t.Fatalf("failed to start voting: %v", err)
	}

	// Record votes
	if err := env.proposalStore.RecordVote(env.ctx, proposalID, "alice", "yes", 100000, now); err != nil {
		t.Fatalf("failed to record alice vote: %v", err)
	}
	if err := env.proposalStore.RecordVote(env.ctx, proposalID, "bob", "yes", 50000, now); err != nil {
		t.Fatalf("failed to record bob vote: %v", err)
	}

	// Verify vote tallies
	retrieved, err := env.proposalStore.GetProposal(env.ctx, proposalID)
	if err != nil {
		t.Fatalf("failed to get proposal: %v", err)
	}

	if retrieved.YesVotes != 150000 {
		t.Errorf("expected yes votes 150000, got %d", retrieved.YesVotes)
	}
	if retrieved.TotalVotingPower() != 150000 {
		t.Errorf("expected total voting power 150000, got %d", retrieved.TotalVotingPower())
	}
	if retrieved.Status != "voting" {
		t.Errorf("expected status voting, got %s", retrieved.Status)
	}
}

// TestBAPIMultiModuleInteraction tests that modules work together correctly
func TestBAPIMultiModuleInteraction(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Step 1: Create accounts
	for _, name := range []string{"alice", "bob", "validator1"} {
		pubKey := make([]byte, 32)
		copy(pubKey, name+"-pubkey-123456789012")
		account := sdktypes.NewAccount(sdktypes.AccountName(name), pubKey)
		if err := env.accountStore.Set(env.ctx, account); err != nil {
			t.Fatalf("failed to create account %s: %v", name, err)
		}
	}

	// Step 2: Set initial balances
	balances := map[string]uint64{
		"alice":      100000,
		"bob":        50000,
		"validator1": 1000000,
	}
	for account, amount := range balances {
		if err := env.balanceStore.Set(env.ctx, account, "stake", amount); err != nil {
			t.Fatalf("failed to set balance for %s: %v", account, err)
		}
	}

	// Step 3: Create validator and delegate
	valPubKey := make([]byte, 32)
	copy(valPubKey, "validator1-pubkey-1234567890")
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: valPubKey,
		},
		Power:           0,
		Commission:      500,
		Description:     "Validator 1",
		TotalDelegation: 0,
	}
	if err := env.validatorStore.SetValidator(env.ctx, validator); err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Self delegation
	if err := env.validatorStore.Delegate(env.ctx, "validator1", valPubKey, 500000); err != nil {
		t.Fatalf("failed to set self delegation: %v", err)
	}

	// Alice delegates
	if err := env.validatorStore.Delegate(env.ctx, "alice", valPubKey, 50000); err != nil {
		t.Fatalf("failed to set alice delegation: %v", err)
	}

	// Update validator power
	if err := env.validatorStore.UpdatePower(env.ctx, valPubKey, 550000); err != nil {
		t.Fatalf("failed to update power: %v", err)
	}

	// Step 4: Create and vote on proposal
	proposalID, _ := env.proposalStore.GetNextProposalID(env.ctx)
	now := time.Now().UTC()
	proposal := &store.BAPIProposal{
		ID:              proposalID,
		Proposer:        "alice",
		Title:           "Test Multi-Module",
		Description:     "Testing multi-module interaction",
		Type:            "text",
		Status:          "voting",
		TotalDeposit:    1000000,
		DepositDenom:    "stake",
		SubmitTime:      now.Unix(),
		DepositEndTime:  now.Unix(),
		VotingStartTime: now.Unix(),
		VotingEndTime:   now.Add(24 * time.Hour).Unix(),
	}
	if err := env.proposalStore.SetProposal(env.ctx, proposal); err != nil {
		t.Fatalf("failed to create proposal: %v", err)
	}

	// Votes based on delegation amounts
	if err := env.proposalStore.RecordVote(env.ctx, proposalID, "validator1", "yes", 500000, now); err != nil {
		t.Fatalf("failed to record validator vote: %v", err)
	}
	if err := env.proposalStore.RecordVote(env.ctx, proposalID, "alice", "yes", 50000, now); err != nil {
		t.Fatalf("failed to record alice vote: %v", err)
	}

	// Verify final state
	p, _ := env.proposalStore.GetProposal(env.ctx, proposalID)
	if p.YesVotes != 550000 {
		t.Errorf("expected yes votes 550000, got %d", p.YesVotes)
	}

	val, _ := env.validatorStore.GetValidator(env.ctx, valPubKey)
	if val.Power != 550000 {
		t.Errorf("expected validator power 550000, got %d", val.Power)
	}
	if val.TotalDelegation != 550000 {
		t.Errorf("expected total delegation 550000, got %d", val.TotalDelegation)
	}

	// All modules working together successfully
	t.Log("Multi-module integration test passed")
}

// TestBAPITransferInsufficientFunds tests transfer with insufficient funds
func TestBAPITransferInsufficientFunds(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Set alice balance
	if err := env.balanceStore.Set(env.ctx, "alice", "stake", 100); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Try to transfer more than alice has
	err := env.balanceStore.Transfer(env.ctx, "alice", "bob", "stake", 200)
	if err == nil {
		t.Error("expected error for insufficient funds")
	}

	// Verify alice balance unchanged
	balance, _ := env.balanceStore.GetAmount(env.ctx, "alice", "stake")
	if balance != 100 {
		t.Errorf("expected balance 100, got %d", balance)
	}
}

// TestBAPIValidatorDelegationUndelegation tests delegation and undelegation
func TestBAPIValidatorDelegationUndelegation(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create validator
	valPubKey := make([]byte, 32)
	copy(valPubKey, "validator-pubkey-123456789012")

	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: valPubKey,
		},
		Power:           0,
		Commission:      500,
		Description:     "Test validator",
		TotalDelegation: 0,
	}
	if err := env.validatorStore.SetValidator(env.ctx, validator); err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Delegate 1000
	if err := env.validatorStore.Delegate(env.ctx, "alice", valPubKey, 1000); err != nil {
		t.Fatalf("failed to delegate: %v", err)
	}

	// Verify delegation
	del, _ := env.validatorStore.GetDelegation(env.ctx, "alice", valPubKey)
	if del.Amount != 1000 {
		t.Errorf("expected delegation 1000, got %d", del.Amount)
	}

	val, _ := env.validatorStore.GetValidator(env.ctx, valPubKey)
	if val.TotalDelegation != 1000 {
		t.Errorf("expected total delegation 1000, got %d", val.TotalDelegation)
	}

	// Undelegate 400
	if err := env.validatorStore.Undelegate(env.ctx, "alice", valPubKey, 400); err != nil {
		t.Fatalf("failed to undelegate: %v", err)
	}

	// Verify after undelegation
	del, _ = env.validatorStore.GetDelegation(env.ctx, "alice", valPubKey)
	if del.Amount != 600 {
		t.Errorf("expected delegation 600, got %d", del.Amount)
	}

	val, _ = env.validatorStore.GetValidator(env.ctx, valPubKey)
	if val.TotalDelegation != 600 {
		t.Errorf("expected total delegation 600, got %d", val.TotalDelegation)
	}

	// Try to undelegate more than delegated
	err := env.validatorStore.Undelegate(env.ctx, "alice", valPubKey, 700)
	if err == nil {
		t.Error("expected error for insufficient delegation")
	}
}

// TestBAPIProposalWithProof tests proposal retrieval
// Note: Merkle proofs require committed state, so we just test retrieval
func TestBAPIProposalWithProof(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create proposal
	proposalID, _ := env.proposalStore.GetNextProposalID(env.ctx)
	now := time.Now().UTC()
	proposal := &store.BAPIProposal{
		ID:             proposalID,
		Proposer:       "alice",
		Title:          "Test Proposal With Proof",
		Description:    "Testing retrieval",
		Type:           "text",
		Status:         "deposit",
		TotalDeposit:   1000,
		DepositDenom:   "stake",
		SubmitTime:     now.Unix(),
		DepositEndTime: now.Add(14 * 24 * time.Hour).Unix(),
	}
	if err := env.proposalStore.SetProposal(env.ctx, proposal); err != nil {
		t.Fatalf("failed to create proposal: %v", err)
	}

	// Get proposal (without proof since state isn't committed)
	retrieved, err := env.proposalStore.GetProposal(env.ctx, proposalID)
	if err != nil {
		t.Fatalf("failed to get proposal: %v", err)
	}

	if retrieved.Title != "Test Proposal With Proof" {
		t.Errorf("expected title 'Test Proposal With Proof', got %s", retrieved.Title)
	}
}

// TestBAPIValidatorJailing tests validator jailing
func TestBAPIValidatorJailing(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create validator
	valPubKey := make([]byte, 32)
	copy(valPubKey, "validator-pubkey-123456789012")

	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: valPubKey,
		},
		Power:      1000,
		Jailed:     false,
		Commission: 500,
	}
	if err := env.validatorStore.SetValidator(env.ctx, validator); err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Jail validator
	if err := env.validatorStore.JailValidator(env.ctx, valPubKey); err != nil {
		t.Fatalf("failed to jail validator: %v", err)
	}

	// Verify jailed
	val, _ := env.validatorStore.GetValidator(env.ctx, valPubKey)
	if !val.Jailed {
		t.Error("expected validator to be jailed")
	}
	if val.Power != 0 {
		t.Errorf("expected jailed validator power 0, got %d", val.Power)
	}

	// Unjail validator
	if err := env.validatorStore.UnjailValidator(env.ctx, valPubKey); err != nil {
		t.Fatalf("failed to unjail validator: %v", err)
	}

	// Verify unjailed
	val, _ = env.validatorStore.GetValidator(env.ctx, valPubKey)
	if val.Jailed {
		t.Error("expected validator to be unjailed")
	}
}

// TestBAPIHexEncodedValidatorPubKey tests hex encoding of validator pubkeys
func TestBAPIHexEncodedValidatorPubKey(t *testing.T) {
	env := setupBAPITestEnv(t)

	// Create validator with known pubkey
	valPubKey := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	expectedHex := hex.EncodeToString(valPubKey)

	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: valPubKey,
		},
		Power: 1000,
	}
	if err := env.validatorStore.SetValidator(env.ctx, validator); err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Create delegation (which uses hex-encoded pubkey)
	if err := env.validatorStore.Delegate(env.ctx, "alice", valPubKey, 500); err != nil {
		t.Fatalf("failed to delegate: %v", err)
	}

	// Retrieve delegation and verify hex encoding
	del, _ := env.validatorStore.GetDelegation(env.ctx, "alice", valPubKey)
	if del.ValidatorPubKey != expectedHex {
		t.Errorf("expected validator pubkey %s, got %s", expectedHex, del.ValidatorPubKey)
	}
}
