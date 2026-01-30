package auth

import (
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

func TestMsgCreateAccount_Type(t *testing.T) {
	msg := &MsgCreateAccount{}
	if got := msg.Type(); got != TypeMsgCreateAccount {
		t.Errorf("Type() = %v, want %v", got, TypeMsgCreateAccount)
	}
}

func TestMsgCreateAccount_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgCreateAccount
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgCreateAccount{
				Name:   "alice",
				PubKey: []byte("test-pubkey"),
				Authority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "empty account name",
			msg: &MsgCreateAccount{
				Name:   "",
				PubKey: []byte("test-pubkey"),
				Authority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid account name",
			msg: &MsgCreateAccount{
				Name:   "ALICE",
				PubKey: []byte("test-pubkey"),
				Authority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			wantErr: true,
		},
		{
			name: "empty public key",
			msg: &MsgCreateAccount{
				Name:   "alice",
				PubKey: []byte{},
				Authority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid authority - zero threshold",
			msg: &MsgCreateAccount{
				Name:   "alice",
				PubKey: []byte("test-pubkey"),
				Authority: types.Authority{
					Threshold:      0,
					KeyWeights:     map[string]uint64{"test-pubkey": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
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

func TestMsgCreateAccount_GetSigners(t *testing.T) {
	msg := &MsgCreateAccount{
		Name: "alice",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgCreateAccount_GetSigners_Nil(t *testing.T) {
	var msg *MsgCreateAccount
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}

func TestMsgUpdateAuthority_Type(t *testing.T) {
	msg := &MsgUpdateAuthority{}
	if got := msg.Type(); got != TypeMsgUpdateAuthority {
		t.Errorf("Type() = %v, want %v", got, TypeMsgUpdateAuthority)
	}
}

func TestMsgUpdateAuthority_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgUpdateAuthority
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgUpdateAuthority{
				Name: "alice",
				NewAuthority: types.Authority{
					Threshold:      2,
					KeyWeights:     map[string]uint64{"key1": 1, "key2": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "empty account name",
			msg: &MsgUpdateAuthority{
				Name: "",
				NewAuthority: types.Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{"key1": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid authority",
			msg: &MsgUpdateAuthority{
				Name: "alice",
				NewAuthority: types.Authority{
					Threshold:      0,
					KeyWeights:     map[string]uint64{"key1": 1},
					AccountWeights: make(map[types.AccountName]uint64),
				},
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

func TestMsgUpdateAuthority_GetSigners(t *testing.T) {
	msg := &MsgUpdateAuthority{
		Name: "alice",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgUpdateAuthority_GetSigners_Nil(t *testing.T) {
	var msg *MsgUpdateAuthority
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}

func TestMsgDeleteAccount_Type(t *testing.T) {
	msg := &MsgDeleteAccount{}
	if got := msg.Type(); got != TypeMsgDeleteAccount {
		t.Errorf("Type() = %v, want %v", got, TypeMsgDeleteAccount)
	}
}

func TestMsgDeleteAccount_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgDeleteAccount
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgDeleteAccount{
				Name: "alice",
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "empty account name",
			msg: &MsgDeleteAccount{
				Name: "",
			},
			wantErr: true,
		},
		{
			name: "invalid account name",
			msg: &MsgDeleteAccount{
				Name: "ALICE",
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

func TestMsgDeleteAccount_GetSigners(t *testing.T) {
	msg := &MsgDeleteAccount{
		Name: "alice",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgDeleteAccount_GetSigners_Nil(t *testing.T) {
	var msg *MsgDeleteAccount
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}
