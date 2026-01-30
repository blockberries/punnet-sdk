package staking

import (
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

func TestMsgCreateValidator_Type(t *testing.T) {
	msg := &MsgCreateValidator{}
	if got := msg.Type(); got != TypeMsgCreateValidator {
		t.Errorf("Type() = %v, want %v", got, TypeMsgCreateValidator)
	}
}

func TestMsgCreateValidator_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgCreateValidator
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgCreateValidator{
				Delegator:    "alice",
				PubKey:       []byte("validator-pubkey"),
				InitialPower: 100,
				Commission:   500,
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "invalid delegator",
			msg: &MsgCreateValidator{
				Delegator:    "",
				PubKey:       []byte("validator-pubkey"),
				InitialPower: 100,
				Commission:   500,
			},
			wantErr: true,
		},
		{
			name: "empty public key",
			msg: &MsgCreateValidator{
				Delegator:    "alice",
				PubKey:       []byte{},
				InitialPower: 100,
				Commission:   500,
			},
			wantErr: true,
		},
		{
			name: "negative initial power",
			msg: &MsgCreateValidator{
				Delegator:    "alice",
				PubKey:       []byte("validator-pubkey"),
				InitialPower: -1,
				Commission:   500,
			},
			wantErr: true,
		},
		{
			name: "commission exceeds 100%",
			msg: &MsgCreateValidator{
				Delegator:    "alice",
				PubKey:       []byte("validator-pubkey"),
				InitialPower: 100,
				Commission:   10001,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMsgCreateValidator_GetSigners(t *testing.T) {
	msg := &MsgCreateValidator{
		Delegator: "alice",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgCreateValidator_GetSigners_Nil(t *testing.T) {
	var msg *MsgCreateValidator
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}

func TestMsgDelegate_Type(t *testing.T) {
	msg := &MsgDelegate{}
	if got := msg.Type(); got != TypeMsgDelegate {
		t.Errorf("Type() = %v, want %v", got, TypeMsgDelegate)
	}
}

func TestMsgDelegate_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgDelegate
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("stake", 100),
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "invalid delegator",
			msg: &MsgDelegate{
				Delegator: "",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("stake", 100),
			},
			wantErr: true,
		},
		{
			name: "empty validator",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: []byte{},
				Amount:    types.NewCoin("stake", 100),
			},
			wantErr: true,
		},
		{
			name: "zero amount",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("stake", 0),
			},
			wantErr: true,
		},
		{
			name: "invalid denom",
			msg: &MsgDelegate{
				Delegator: "alice",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("", 100),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMsgDelegate_GetSigners(t *testing.T) {
	msg := &MsgDelegate{
		Delegator: "alice",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgDelegate_GetSigners_Nil(t *testing.T) {
	var msg *MsgDelegate
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}

func TestMsgUndelegate_Type(t *testing.T) {
	msg := &MsgUndelegate{}
	if got := msg.Type(); got != TypeMsgUndelegate {
		t.Errorf("Type() = %v, want %v", got, TypeMsgUndelegate)
	}
}

func TestMsgUndelegate_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgUndelegate
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("stake", 100),
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "invalid delegator",
			msg: &MsgUndelegate{
				Delegator: "",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("stake", 100),
			},
			wantErr: true,
		},
		{
			name: "empty validator",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: []byte{},
				Amount:    types.NewCoin("stake", 100),
			},
			wantErr: true,
		},
		{
			name: "zero amount",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("stake", 0),
			},
			wantErr: true,
		},
		{
			name: "invalid denom",
			msg: &MsgUndelegate{
				Delegator: "alice",
				Validator: []byte("validator-pubkey"),
				Amount:    types.NewCoin("", 100),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMsgUndelegate_GetSigners(t *testing.T) {
	msg := &MsgUndelegate{
		Delegator: "alice",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgUndelegate_GetSigners_Nil(t *testing.T) {
	var msg *MsgUndelegate
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}
