package bank

import (
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

// TestMsgMultiSend_OverflowProtection verifies overflow detection in validation
func TestMsgMultiSend_OverflowProtection(t *testing.T) {
	// Create inputs that would overflow if not protected
	maxUint := ^uint64(0)

	msg := &MsgMultiSend{
		Inputs: []Input{
			{
				Address: "alice",
				Coins:   types.Coins{{Denom: "uatom", Amount: maxUint - 100}},
			},
			{
				Address: "bob",
				Coins:   types.Coins{{Denom: "uatom", Amount: 200}},
			},
		},
		Outputs: []Output{
			{
				Address: "charlie",
				Coins:   types.Coins{{Denom: "uatom", Amount: 100}},
			},
		},
	}

	err := msg.ValidateBasic()
	if err == nil {
		t.Fatal("Expected overflow error, got nil")
	}

	if err.Error() != "input total overflow for denom uatom" {
		t.Errorf("Expected overflow error, got: %v", err)
	}

	t.Log("✓ MsgMultiSend detects overflow in input totals")
}

// TestMsgMultiSend_OutputOverflowProtection verifies output overflow detection
func TestMsgMultiSend_OutputOverflowProtection(t *testing.T) {
	maxUint := ^uint64(0)

	msg := &MsgMultiSend{
		Inputs: []Input{
			{
				Address: "alice",
				Coins:   types.Coins{{Denom: "uatom", Amount: 100}},
			},
		},
		Outputs: []Output{
			{
				Address: "charlie",
				Coins:   types.Coins{{Denom: "uatom", Amount: maxUint - 50}},
			},
			{
				Address: "dave",
				Coins:   types.Coins{{Denom: "uatom", Amount: 100}},
			},
		},
	}

	err := msg.ValidateBasic()
	if err == nil {
		t.Fatal("Expected overflow error, got nil")
	}

	if err.Error() != "output total overflow for denom uatom" {
		t.Errorf("Expected overflow error, got: %v", err)
	}

	t.Log("✓ MsgMultiSend detects overflow in output totals")
}

// TestMsgMultiSend_ValidWithLargeAmounts verifies large valid amounts work
func TestMsgMultiSend_ValidWithLargeAmounts(t *testing.T) {
	largeAmount := uint64(1_000_000_000_000) // 1 trillion

	msg := &MsgMultiSend{
		Inputs: []Input{
			{
				Address: "alice",
				Coins:   types.Coins{{Denom: "uatom", Amount: largeAmount}},
			},
			{
				Address: "bob",
				Coins:   types.Coins{{Denom: "uatom", Amount: largeAmount}},
			},
		},
		Outputs: []Output{
			{
				Address: "charlie",
				Coins:   types.Coins{{Denom: "uatom", Amount: largeAmount * 2}},
			},
		},
	}

	err := msg.ValidateBasic()
	if err != nil {
		t.Fatalf("Valid message should not error: %v", err)
	}

	t.Log("✓ MsgMultiSend handles large valid amounts correctly")
}
