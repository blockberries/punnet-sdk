package bank

import (
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

func TestMsgSend_Type(t *testing.T) {
	msg := &MsgSend{}
	if got := msg.Type(); got != TypeMsgSend {
		t.Errorf("Type() = %v, want %v", got, TypeMsgSend)
	}
}

func TestMsgSend_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgSend
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgSend{
				From:   "alice",
				To:     "bob",
				Amount: types.NewCoin("token", 100),
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "invalid sender",
			msg: &MsgSend{
				From:   "",
				To:     "bob",
				Amount: types.NewCoin("token", 100),
			},
			wantErr: true,
		},
		{
			name: "invalid recipient",
			msg: &MsgSend{
				From:   "alice",
				To:     "",
				Amount: types.NewCoin("token", 100),
			},
			wantErr: true,
		},
		{
			name: "send to self",
			msg: &MsgSend{
				From:   "alice",
				To:     "alice",
				Amount: types.NewCoin("token", 100),
			},
			wantErr: true,
		},
		{
			name: "zero amount",
			msg: &MsgSend{
				From:   "alice",
				To:     "bob",
				Amount: types.NewCoin("token", 0),
			},
			wantErr: true,
		},
		{
			name: "invalid denom",
			msg: &MsgSend{
				From:   "alice",
				To:     "bob",
				Amount: types.NewCoin("", 100),
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

func TestMsgSend_GetSigners(t *testing.T) {
	msg := &MsgSend{
		From: "alice",
		To:   "bob",
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}

func TestMsgSend_GetSigners_Nil(t *testing.T) {
	var msg *MsgSend
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}

func TestInput_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		input   *Input
		wantErr bool
	}{
		{
			name: "valid input",
			input: &Input{
				Address: "alice",
				Coins:   types.NewCoins(types.NewCoin("token", 100)),
			},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name: "invalid address",
			input: &Input{
				Address: "",
				Coins:   types.NewCoins(types.NewCoin("token", 100)),
			},
			wantErr: true,
		},
		{
			name: "zero amount",
			input: &Input{
				Address: "alice",
				Coins:   types.NewCoins(types.NewCoin("token", 0)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOutput_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		output  *Output
		wantErr bool
	}{
		{
			name: "valid output",
			output: &Output{
				Address: "bob",
				Coins:   types.NewCoins(types.NewCoin("token", 100)),
			},
			wantErr: false,
		},
		{
			name:    "nil output",
			output:  nil,
			wantErr: true,
		},
		{
			name: "invalid address",
			output: &Output{
				Address: "",
				Coins:   types.NewCoins(types.NewCoin("token", 100)),
			},
			wantErr: true,
		},
		{
			name: "zero amount",
			output: &Output{
				Address: "bob",
				Coins:   types.NewCoins(types.NewCoin("token", 0)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.output.ValidateBasic()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBasic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMsgMultiSend_Type(t *testing.T) {
	msg := &MsgMultiSend{}
	if got := msg.Type(); got != TypeMsgMultiSend {
		t.Errorf("Type() = %v, want %v", got, TypeMsgMultiSend)
	}
}

func TestMsgMultiSend_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgMultiSend
		wantErr bool
	}{
		{
			name: "valid message",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
				Outputs: []Output{
					{
						Address: "bob",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
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
			name: "empty inputs",
			msg: &MsgMultiSend{
				Inputs: []Input{},
				Outputs: []Output{
					{
						Address: "bob",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty outputs",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
				Outputs: []Output{},
			},
			wantErr: true,
		},
		{
			name: "inputs != outputs",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
				Outputs: []Output{
					{
						Address: "bob",
						Coins:   types.NewCoins(types.NewCoin("token", 50)),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple inputs and outputs balanced",
			msg: &MsgMultiSend{
				Inputs: []Input{
					{
						Address: "alice",
						Coins:   types.NewCoins(types.NewCoin("token", 50)),
					},
					{
						Address: "bob",
						Coins:   types.NewCoins(types.NewCoin("token", 50)),
					},
				},
				Outputs: []Output{
					{
						Address: "charlie",
						Coins:   types.NewCoins(types.NewCoin("token", 100)),
					},
				},
			},
			wantErr: false,
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

func TestMsgMultiSend_GetSigners(t *testing.T) {
	msg := &MsgMultiSend{
		Inputs: []Input{
			{Address: "alice"},
			{Address: "bob"},
		},
	}

	signers := msg.GetSigners()
	if len(signers) != 2 {
		t.Errorf("GetSigners() returned %d signers, want 2", len(signers))
	}

	// Check both signers are present
	found := make(map[types.AccountName]bool)
	for _, signer := range signers {
		found[signer] = true
	}

	if !found["alice"] || !found["bob"] {
		t.Errorf("GetSigners() = %v, want [alice, bob]", signers)
	}
}

func TestMsgMultiSend_GetSigners_Nil(t *testing.T) {
	var msg *MsgMultiSend
	signers := msg.GetSigners()
	if signers != nil {
		t.Errorf("GetSigners() on nil message = %v, want nil", signers)
	}
}

func TestMsgMultiSend_GetSigners_Duplicates(t *testing.T) {
	msg := &MsgMultiSend{
		Inputs: []Input{
			{Address: "alice"},
			{Address: "alice"},
		},
	}

	signers := msg.GetSigners()
	if len(signers) != 1 {
		t.Errorf("GetSigners() with duplicate inputs returned %d signers, want 1", len(signers))
	}
	if signers[0] != "alice" {
		t.Errorf("GetSigners() = %v, want [alice]", signers)
	}
}
