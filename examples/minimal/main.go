package main

import (
	"context"
	"fmt"
	"log"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/modules/auth"
	"github.com/blockberries/punnet-sdk/modules/bank"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// MinimalApp demonstrates a minimal Punnet SDK application with auth and bank modules
func main() {
	fmt.Println("=== Punnet SDK Minimal Example ===")
	fmt.Println()

	// Step 1: Create backing store
	fmt.Println("1. Creating memory backing store...")
	backing := store.NewMemoryStore()

	// Step 2: Create capability manager
	fmt.Println("2. Creating capability manager...")
	capManager := capability.NewCapabilityManager(backing)

	// Step 3: Register modules
	fmt.Println("3. Registering modules...")
	if err := capManager.RegisterModule("auth"); err != nil {
		log.Fatalf("Failed to register auth module: %v", err)
	}
	if err := capManager.RegisterModule("bank"); err != nil {
		log.Fatalf("Failed to register bank module: %v", err)
	}

	// Step 4: Grant capabilities
	fmt.Println("4. Granting capabilities...")
	accountCap, err := capManager.GrantAccountCapability("auth")
	if err != nil {
		log.Fatalf("Failed to grant account capability: %v", err)
	}

	balanceCap, err := capManager.GrantBalanceCapability("bank")
	if err != nil {
		log.Fatalf("Failed to grant balance capability: %v", err)
	}

	// Step 5: Create modules
	fmt.Println("5. Creating modules...")
	authMod, err := auth.CreateModule(accountCap)
	if err != nil {
		log.Fatalf("Failed to create auth module: %v", err)
	}
	fmt.Printf("   - Auth module: %s\n", authMod.Name())

	bankMod, err := bank.CreateModule(balanceCap)
	if err != nil {
		log.Fatalf("Failed to create bank module: %v", err)
	}
	fmt.Printf("   - Bank module: %s\n", bankMod.Name())

	// Step 6: Demonstrate account creation
	fmt.Println()
	fmt.Println("6. Creating accounts...")
	ctx := context.Background()

	alice, err := accountCap.CreateAccount(ctx, "alice", []byte("alice-pubkey-12345"))
	if err != nil {
		log.Fatalf("Failed to create alice account: %v", err)
	}
	fmt.Printf("   - Created account: %s (nonce: %d)\n", alice.Name, alice.Nonce)

	bob, err := accountCap.CreateAccount(ctx, "bob", []byte("bob-pubkey-67890"))
	if err != nil {
		log.Fatalf("Failed to create bob account: %v", err)
	}
	fmt.Printf("   - Created account: %s (nonce: %d)\n", bob.Name, bob.Nonce)

	// Step 7: Set initial balances
	fmt.Println()
	fmt.Println("7. Setting initial balances...")
	if err := balanceCap.SetBalance(ctx, "alice", "token", 1000); err != nil {
		log.Fatalf("Failed to set alice balance: %v", err)
	}
	fmt.Println("   - Alice: 1000 token")

	if err := balanceCap.SetBalance(ctx, "bob", "token", 500); err != nil {
		log.Fatalf("Failed to set bob balance: %v", err)
	}
	fmt.Println("   - Bob: 500 token")

	// Step 8: Query balances
	fmt.Println()
	fmt.Println("8. Querying balances...")
	aliceBalance, err := balanceCap.GetBalance(ctx, "alice", "token")
	if err != nil {
		log.Fatalf("Failed to get alice balance: %v", err)
	}
	fmt.Printf("   - Alice: %d token\n", aliceBalance)

	bobBalance, err := balanceCap.GetBalance(ctx, "bob", "token")
	if err != nil {
		log.Fatalf("Failed to get bob balance: %v", err)
	}
	fmt.Printf("   - Bob: %d token\n", bobBalance)

	// Step 9: Demonstrate transfer
	fmt.Println()
	fmt.Println("9. Transferring 200 token from Alice to Bob...")
	if err := balanceCap.Transfer(ctx, "alice", "bob", "token", 200); err != nil {
		log.Fatalf("Failed to transfer: %v", err)
	}

	// Step 10: Query balances after transfer
	fmt.Println()
	fmt.Println("10. Querying balances after transfer...")
	aliceBalance, err = balanceCap.GetBalance(ctx, "alice", "token")
	if err != nil {
		log.Fatalf("Failed to get alice balance: %v", err)
	}
	fmt.Printf("    - Alice: %d token (was 1000, sent 200)\n", aliceBalance)

	bobBalance, err = balanceCap.GetBalance(ctx, "bob", "token")
	if err != nil {
		log.Fatalf("Failed to get bob balance: %v", err)
	}
	fmt.Printf("    - Bob: %d token (was 500, received 200)\n", bobBalance)

	// Step 11: Demonstrate message handling
	fmt.Println()
	fmt.Println("11. Demonstrating message handling...")
	charlie := types.AccountName("charlie")
	charlieKey := []byte("charlie-pubkey-abcde")
	charlieAuthority := types.NewAuthority(1, charlieKey, 1)

	// Create a message (demonstrates message structure)
	_ = &auth.MsgCreateAccount{
		Name:      charlie,
		PubKey:    charlieKey,
		Authority: charlieAuthority,
	}

	// Get message handlers from module
	authHandlers := authMod.RegisterMsgHandlers()
	createHandler := authHandlers[auth.TypeMsgCreateAccount]

	// Note: In a real application, you would create a proper runtime context
	// For this example, we'll just show the module structure
	fmt.Printf("    - Auth module has %d message handlers\n", len(authHandlers))
	fmt.Printf("    - Bank module has %d message handlers\n", len(bankMod.RegisterMsgHandlers()))
	fmt.Printf("    - Message type: %s\n", auth.TypeMsgCreateAccount)

	if createHandler != nil {
		fmt.Println("    - Handler registered successfully")
	}

	// Step 12: Summary
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Println("This example demonstrated:")
	fmt.Println("  1. Creating a backing store")
	fmt.Println("  2. Setting up the capability manager")
	fmt.Println("  3. Registering modules (auth, bank)")
	fmt.Println("  4. Granting capabilities to modules")
	fmt.Println("  5. Creating accounts")
	fmt.Println("  6. Managing balances")
	fmt.Println("  7. Transferring tokens")
	fmt.Println("  8. Module message handlers")
	fmt.Println()
	fmt.Println("Key Punnet SDK concepts:")
	fmt.Println("  - Capability-based security: Modules only access state through granted capabilities")
	fmt.Println("  - Named accounts: Human-readable account names (e.g., 'alice', 'bob')")
	fmt.Println("  - Effect-based execution: Handlers return effects, not direct mutations")
	fmt.Println("  - Module composition: Independent modules work together via capabilities")
	fmt.Println()
	fmt.Println("=== Example Complete ===")
}
