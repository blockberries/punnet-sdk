package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoin_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		coin  Coin
		valid bool
	}{
		{"valid coin", Coin{Denom: "uatom", Amount: 100}, true},
		{"empty denom", Coin{Denom: "", Amount: 100}, false},
		{"zero amount", Coin{Denom: "uatom", Amount: 0}, true},
		{"long denom", Coin{Denom: string(make([]byte, 65)), Amount: 100}, false},
		{"max denom", Coin{Denom: string(make([]byte, 64)), Amount: 100}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.coin.IsValid())
		})
	}
}

func TestCoin_IsZero(t *testing.T) {
	assert.True(t, Coin{Denom: "uatom", Amount: 0}.IsZero())
	assert.False(t, Coin{Denom: "uatom", Amount: 1}.IsZero())
}

func TestCoin_IsPositive(t *testing.T) {
	assert.False(t, Coin{Denom: "uatom", Amount: 0}.IsPositive())
	assert.True(t, Coin{Denom: "uatom", Amount: 1}.IsPositive())
}

func TestCoins_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		coins Coins
		valid bool
	}{
		{"empty", Coins{}, true},
		{"single coin", Coins{Coin{Denom: "uatom", Amount: 100}}, true},
		{"sorted coins", Coins{
			Coin{Denom: "stake", Amount: 100},
			Coin{Denom: "uatom", Amount: 50},
		}, true},
		{"unsorted coins", Coins{
			Coin{Denom: "uatom", Amount: 50},
			Coin{Denom: "stake", Amount: 100},
		}, false},
		{"duplicate denom", Coins{
			Coin{Denom: "uatom", Amount: 50},
			Coin{Denom: "uatom", Amount: 100},
		}, false},
		{"invalid coin", Coins{
			Coin{Denom: "", Amount: 100},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.coins.IsValid())
		})
	}
}

func TestCoins_AmountOf(t *testing.T) {
	coins := Coins{
		Coin{Denom: "stake", Amount: 100},
		Coin{Denom: "uatom", Amount: 50},
	}

	assert.Equal(t, uint64(100), coins.AmountOf("stake"))
	assert.Equal(t, uint64(50), coins.AmountOf("uatom"))
	assert.Equal(t, uint64(0), coins.AmountOf("missing"))
}

func TestCoins_Add(t *testing.T) {
	tests := []struct {
		name     string
		coins1   Coins
		coins2   Coins
		expected Coins
	}{
		{
			"add same denom",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "uatom", Amount: 50}},
			Coins{Coin{Denom: "uatom", Amount: 150}},
		},
		{
			"add different denoms",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "stake", Amount: 50}},
			Coins{
				Coin{Denom: "stake", Amount: 50},
				Coin{Denom: "uatom", Amount: 100},
			},
		},
		{
			"add empty",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{},
			Coins{Coin{Denom: "uatom", Amount: 100}},
		},
		{
			"add multiple",
			Coins{
				Coin{Denom: "stake", Amount: 100},
				Coin{Denom: "uatom", Amount: 50},
			},
			Coins{
				Coin{Denom: "stake", Amount: 25},
				Coin{Denom: "token", Amount: 75},
			},
			Coins{
				Coin{Denom: "stake", Amount: 125},
				Coin{Denom: "token", Amount: 75},
				Coin{Denom: "uatom", Amount: 50},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.coins1.Add(tt.coins2)
			assert.Equal(t, tt.expected, result)
			assert.True(t, result.IsValid())
		})
	}
}

func TestCoins_Sub(t *testing.T) {
	tests := []struct {
		name      string
		coins1    Coins
		coins2    Coins
		expected  Coins
		expectErr bool
	}{
		{
			"sub same denom",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "uatom", Amount: 50}},
			Coins{Coin{Denom: "uatom", Amount: 50}},
			false,
		},
		{
			"sub to zero",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{},
			false,
		},
		{
			"insufficient funds",
			Coins{Coin{Denom: "uatom", Amount: 50}},
			Coins{Coin{Denom: "uatom", Amount: 100}},
			nil,
			true,
		},
		{
			"sub different denom",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "stake", Amount: 50}},
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.coins1.Sub(tt.coins2)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
				assert.True(t, result.IsValid())
			}
		})
	}
}

func TestCoins_IsAllGTE(t *testing.T) {
	tests := []struct {
		name     string
		coins1   Coins
		coins2   Coins
		expected bool
	}{
		{
			"greater",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "uatom", Amount: 50}},
			true,
		},
		{
			"equal",
			Coins{Coin{Denom: "uatom", Amount: 100}},
			Coins{Coin{Denom: "uatom", Amount: 100}},
			true,
		},
		{
			"less",
			Coins{Coin{Denom: "uatom", Amount: 50}},
			Coins{Coin{Denom: "uatom", Amount: 100}},
			false,
		},
		{
			"missing denom",
			Coins{Coin{Denom: "stake", Amount: 100}},
			Coins{Coin{Denom: "uatom", Amount: 50}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.coins1.IsAllGTE(tt.coins2))
		})
	}
}
