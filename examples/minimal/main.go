// Package main demonstrates a minimal Punnet SDK application using BAPI modules.
// This example shows how to create and configure an application with auth, bank,
// staking, and governance modules using the new BAPI-based architecture.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/modules/auth"
	"github.com/blockberries/punnet-sdk/modules/bank"
	"github.com/blockberries/punnet-sdk/modules/governance"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func main() {
	fmt.Println("=== Punnet SDK BAPI Example ===")
	fmt.Println()

	// Step 1: Create backing StateStore (in-memory IAVL for example)
	fmt.Println("1. Creating StateStore...")
	ss, err := statestore.NewMemoryIAVLStore(1000)
	if err != nil {
		log.Fatalf("Failed to create state store: %v", err)
	}
	defer ss.Close()
	fmt.Println("   - In-memory IAVL store created")

	// Step 2: Create typed stores for each domain
	fmt.Println()
	fmt.Println("2. Creating typed stores...")
	accountStore := store.NewBAPIAccountStore(ss)
	balanceStore := store.NewBAPIBalanceStore(ss)
	validatorStore := store.NewBAPIValidatorStore(ss)
	proposalStore := store.NewBAPIProposalStore(ss)
	fmt.Println("   - AccountStore created")
	fmt.Println("   - BalanceStore created")
	fmt.Println("   - ValidatorStore created")
	fmt.Println("   - ProposalStore created")

	// Step 3: Create BAPI modules
	fmt.Println()
	fmt.Println("3. Creating BAPI modules...")

	authMod, err := auth.NewBAPIAuthModule(accountStore)
	if err != nil {
		log.Fatalf("Failed to create auth module: %v", err)
	}
	fmt.Printf("   - %s module created\n", authMod.Name())

	bankMod, err := bank.NewBAPIBankModule(balanceStore)
	if err != nil {
		log.Fatalf("Failed to create bank module: %v", err)
	}
	fmt.Printf("   - %s module created\n", bankMod.Name())

	stakingMod, err := staking.NewBAPIStakingModule(validatorStore, balanceStore)
	if err != nil {
		log.Fatalf("Failed to create staking module: %v", err)
	}
	fmt.Printf("   - %s module created\n", stakingMod.Name())

	govMod, err := governance.NewBAPIGovernanceModule(proposalStore, balanceStore)
	if err != nil {
		log.Fatalf("Failed to create governance module: %v", err)
	}
	fmt.Printf("   - %s module created\n", govMod.Name())

	// Step 4: Create BAPI Application
	fmt.Println()
	fmt.Println("4. Creating BAPI Application...")
	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "example-chain-1",
		StateStore: ss,
		Modules: []runtime.BAPIModule{
			authMod,
			bankMod,
			stakingMod,
			govMod,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}
	fmt.Printf("   - Application created for chain: %s\n", app.ChainID())

	// Step 5: Show registered handlers
	fmt.Println()
	fmt.Println("5. Registered message handlers:")
	router := app.Router()
	for _, mod := range router.Modules() {
		handlers := mod.RegisterMsgHandlers()
		fmt.Printf("   - %s: %d handlers\n", mod.Name(), len(handlers))
		for msgType := range handlers {
			fmt.Printf("     * %s\n", msgType)
		}
	}

	// Step 6: Show query handlers
	fmt.Println()
	fmt.Println("6. Registered query handlers:")
	for _, mod := range router.Modules() {
		queries := mod.RegisterQueryHandlers()
		fmt.Printf("   - %s: %d queries\n", mod.Name(), len(queries))
		for path := range queries {
			fmt.Printf("     * %s\n", path)
		}
	}

	// Step 7: Direct store operations (for setup, bypassing transaction flow)
	fmt.Println()
	fmt.Println("7. Setting up genesis state...")
	ctx := context.Background()

	// Create accounts using helper function
	aliceAccount := types.NewAccount(types.AccountName("alice"), []byte("alice-pubkey-12345678901234567890"))
	if err := accountStore.Set(ctx, aliceAccount); err != nil {
		log.Fatalf("Failed to create alice account: %v", err)
	}
	fmt.Println("   - Created account: alice")

	bobAccount := types.NewAccount(types.AccountName("bob"), []byte("bob-pubkey-12345678901234567890ab"))
	if err := accountStore.Set(ctx, bobAccount); err != nil {
		log.Fatalf("Failed to create bob account: %v", err)
	}
	fmt.Println("   - Created account: bob")

	// Set initial balances
	if err := balanceStore.Set(ctx, "alice", "stake", 1000000); err != nil {
		log.Fatalf("Failed to set alice balance: %v", err)
	}
	fmt.Println("   - Set alice balance: 1,000,000 stake")

	if err := balanceStore.Set(ctx, "bob", "stake", 500000); err != nil {
		log.Fatalf("Failed to set bob balance: %v", err)
	}
	fmt.Println("   - Set bob balance: 500,000 stake")

	// Step 8: Query balances
	fmt.Println()
	fmt.Println("8. Querying state...")

	aliceBalance, err := balanceStore.GetAmount(ctx, "alice", "stake")
	if err != nil {
		log.Fatalf("Failed to get alice balance: %v", err)
	}
	fmt.Printf("   - Alice balance: %d stake\n", aliceBalance)

	bobBalance, err := balanceStore.GetAmount(ctx, "bob", "stake")
	if err != nil {
		log.Fatalf("Failed to get bob balance: %v", err)
	}
	fmt.Printf("   - Bob balance: %d stake\n", bobBalance)

	// Step 9: Demonstrate direct query through application
	fmt.Println()
	fmt.Println("9. Querying through application...")

	// Query governance params
	govParamsHandler := govMod.RegisterQueryHandlers()["/gov/params"]
	if govParamsHandler != nil {
		result, err := govParamsHandler(ctx, nil, 0)
		if err != nil {
			log.Fatalf("Failed to query gov params: %v", err)
		}
		var params map[string]interface{}
		if err := json.Unmarshal(result, &params); err != nil {
			log.Fatalf("Failed to parse gov params: %v", err)
		}
		fmt.Printf("   - Governance min_deposit: %v\n", params["min_deposit"])
		fmt.Printf("   - Governance deposit_denom: %v\n", params["deposit_denom"])
	}

	// Step 10: Demonstrate message validation
	fmt.Println()
	fmt.Println("10. Message validation examples...")

	// Valid send message
	sendMsg := &bank.MsgSend{
		From:   types.AccountName("alice"),
		To:     types.AccountName("bob"),
		Amount: types.Coin{Denom: "stake", Amount: 100},
	}
	if err := sendMsg.ValidateBasic(); err != nil {
		fmt.Printf("    - MsgSend validation failed: %v\n", err)
	} else {
		fmt.Printf("    - MsgSend %s->%s %d%s: valid\n",
			sendMsg.From, sendMsg.To, sendMsg.Amount.Amount, sendMsg.Amount.Denom)
	}

	// Valid proposal submission
	proposalMsg := &governance.MsgSubmitProposal{
		Proposer:     types.AccountName("alice"),
		Title:        "Test Proposal",
		Description:  "This is a test proposal for demonstration",
		ProposalType: governance.ProposalTypeText,
		InitialDeposit: types.Coin{
			Denom:  "stake",
			Amount: 1000,
		},
	}
	if err := proposalMsg.ValidateBasic(); err != nil {
		fmt.Printf("    - MsgSubmitProposal validation failed: %v\n", err)
	} else {
		fmt.Printf("    - MsgSubmitProposal '%s': valid\n", proposalMsg.Title)
	}

	// Step 11: Summary
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Println()
	fmt.Println("This example demonstrated the BAPI-based Punnet SDK:")
	fmt.Println()
	fmt.Println("Architecture:")
	fmt.Println("  - StateStore: blockberry IAVL-backed merkle tree")
	fmt.Println("  - TypedStores: BAPIAccountStore, BAPIBalanceStore, etc.")
	fmt.Println("  - BAPIModules: Handlers return Effects, not direct mutations")
	fmt.Println("  - BAPIApplication: Implements BAPI Lifecycle interface")
	fmt.Println()
	fmt.Println("Registered Modules:")
	fmt.Println("  - auth: Account creation, authority management, nonce tracking")
	fmt.Println("  - bank: Token transfers, balance queries")
	fmt.Println("  - staking: Validator registration, delegation")
	fmt.Println("  - governance: Proposals, voting, deposits")
	fmt.Println()
	fmt.Println("Key Features:")
	fmt.Println("  - Effect-based execution: Handlers return Effects")
	fmt.Println("  - Dependency injection: Stores are injected into modules")
	fmt.Println("  - Named accounts: Human-readable names (alice, bob)")
	fmt.Println("  - BAPI Lifecycle: Handshake, CheckTx, ExecuteBlock, Commit, Query")
	fmt.Println()
	fmt.Println("Integration with Raspberry:")
	fmt.Println("  - BAPIApplication implements raspberry's Application interface")
	fmt.Println("  - Use node.WithApplication(app) when creating raspberry node")
	fmt.Println()
	fmt.Println("=== Example Complete ===")
}
