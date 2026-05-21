package types

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestFee_ValidateBasic(t *testing.T) {
	t.Run("zero fee is valid", func(t *testing.T) {
		f := &Fee{}
		if err := f.ValidateBasic(); err != nil {
			t.Errorf("zero Fee.ValidateBasic: %v", err)
		}
	})

	t.Run("non-zero valid fee", func(t *testing.T) {
		f := &Fee{
			OpFees: []OpFee{
				{MessageType: "/bank.MsgSend", Amount: 100},
				{MessageType: "/staking.MsgDelegate", Amount: 200},
			},
			ByteFee:  50,
			Priority: 10,
		}
		if err := f.ValidateBasic(); err != nil {
			t.Errorf("Fee.ValidateBasic: %v", err)
		}
	})

	t.Run("op_fees over cap rejected", func(t *testing.T) {
		ops := make([]OpFee, MaxOpFees+1)
		for i := range ops {
			ops[i] = OpFee{MessageType: "/bank.MsgSend." + string(rune('a'+i)), Amount: 1}
		}
		f := &Fee{OpFees: ops}
		err := f.ValidateBasic()
		if err == nil {
			t.Fatal("expected over-cap rejection")
		}
		if !strings.Contains(err.Error(), "exceeds max") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty message_type rejected", func(t *testing.T) {
		f := &Fee{OpFees: []OpFee{{MessageType: "", Amount: 1}}}
		if err := f.ValidateBasic(); err == nil || !strings.Contains(err.Error(), "empty") {
			t.Errorf("expected empty-message-type rejection, got %v", err)
		}
	})

	t.Run("duplicate message_type rejected", func(t *testing.T) {
		f := &Fee{OpFees: []OpFee{
			{MessageType: "/bank.MsgSend", Amount: 1},
			{MessageType: "/bank.MsgSend", Amount: 2},
		}}
		if err := f.ValidateBasic(); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Errorf("expected duplicate rejection, got %v", err)
		}
	})

	t.Run("negative priority rejected", func(t *testing.T) {
		f := &Fee{Priority: -1}
		if err := f.ValidateBasic(); err == nil || !strings.Contains(err.Error(), "non-negative") {
			t.Errorf("expected negative-priority rejection, got %v", err)
		}
	})

	t.Run("invalid payer rejected", func(t *testing.T) {
		f := &Fee{Payer: AccountName("invalid char $")}
		if err := f.ValidateBasic(); err == nil {
			t.Error("expected invalid-payer rejection")
		}
	})

	t.Run("empty payer is fine", func(t *testing.T) {
		f := &Fee{}
		if err := f.ValidateBasic(); err != nil {
			t.Errorf("empty Payer should be allowed (tx.Account pays): %v", err)
		}
	})

	t.Run("overflow in op_fees rejected", func(t *testing.T) {
		f := &Fee{OpFees: []OpFee{
			{MessageType: "/a", Amount: math.MaxUint64},
			{MessageType: "/b", Amount: 1},
		}}
		if err := f.ValidateBasic(); err == nil || !strings.Contains(err.Error(), "overflow") {
			t.Errorf("expected overflow rejection, got %v", err)
		}
	})

	t.Run("overflow across components rejected", func(t *testing.T) {
		f := &Fee{
			OpFees:  []OpFee{{MessageType: "/a", Amount: math.MaxUint64 - 5}},
			ByteFee: 10,
		}
		if err := f.ValidateBasic(); err == nil || !strings.Contains(err.Error(), "overflow") {
			t.Errorf("expected cross-component overflow rejection, got %v", err)
		}
	})

	t.Run("nil fee rejected", func(t *testing.T) {
		var f *Fee
		if err := f.ValidateBasic(); !errors.Is(err, ErrInvalidTransaction) {
			t.Errorf("expected ErrInvalidTransaction, got %v", err)
		}
	})
}

func TestFee_Total(t *testing.T) {
	t.Run("zero is zero", func(t *testing.T) {
		f := &Fee{}
		got, err := f.Total()
		if err != nil || got != 0 {
			t.Errorf("Total: got %d err=%v want 0 err=nil", got, err)
		}
	})

	t.Run("sums all three components", func(t *testing.T) {
		f := &Fee{
			OpFees: []OpFee{
				{MessageType: "/a", Amount: 100},
				{MessageType: "/b", Amount: 50},
			},
			ByteFee:  30,
			Priority: 20,
		}
		got, err := f.Total()
		if err != nil {
			t.Fatalf("Total: %v", err)
		}
		if got != 200 {
			t.Errorf("Total: got %d, want 200", got)
		}
	})

	t.Run("priority added correctly when large", func(t *testing.T) {
		f := &Fee{Priority: math.MaxInt64}
		got, err := f.Total()
		if err != nil {
			t.Fatalf("Total: %v", err)
		}
		if got != uint64(math.MaxInt64) {
			t.Errorf("Total: got %d, want %d", got, math.MaxInt64)
		}
	})
}

func TestFee_PayerOrAccount(t *testing.T) {
	acct := AccountName("alice")
	t.Run("falls back to account when Payer empty", func(t *testing.T) {
		f := &Fee{}
		if got := f.PayerOrAccount(acct); got != acct {
			t.Errorf("PayerOrAccount: got %q, want %q", got, acct)
		}
	})

	t.Run("returns Payer when set", func(t *testing.T) {
		paymaster := AccountName("paymaster")
		f := &Fee{Payer: paymaster}
		if got := f.PayerOrAccount(acct); got != paymaster {
			t.Errorf("PayerOrAccount: got %q, want %q", got, paymaster)
		}
	})
}
