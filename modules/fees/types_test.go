package fees

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeeSchedule_Validate(t *testing.T) {
	t.Run("nil schedule", func(t *testing.T) {
		var s *FeeSchedule
		assert.Error(t, s.Validate())
	})

	t.Run("empty op_fees is valid", func(t *testing.T) {
		s := &FeeSchedule{}
		assert.NoError(t, s.Validate())
	})

	t.Run("sorted entries are valid", func(t *testing.T) {
		s := &FeeSchedule{
			OpFees: []OpFeeEntry{
				{MessageType: "/a", Amount: 1},
				{MessageType: "/b", Amount: 2},
			},
		}
		assert.NoError(t, s.Validate())
	})

	t.Run("unsorted entries rejected", func(t *testing.T) {
		s := &FeeSchedule{
			OpFees: []OpFeeEntry{
				{MessageType: "/b", Amount: 1},
				{MessageType: "/a", Amount: 2},
			},
		}
		err := s.Validate()
		require := assert.Error(t, err)
		_ = require
		assert.Contains(t, err.Error(), "not sorted")
	})

	t.Run("duplicate entries rejected (equal is not strictly increasing)", func(t *testing.T) {
		s := &FeeSchedule{
			OpFees: []OpFeeEntry{
				{MessageType: "/a", Amount: 1},
				{MessageType: "/a", Amount: 2},
			},
		}
		assert.Error(t, s.Validate())
	})

	t.Run("empty message_type rejected", func(t *testing.T) {
		s := &FeeSchedule{
			OpFees: []OpFeeEntry{
				{MessageType: "", Amount: 1},
			},
		}
		err := s.Validate()
		require := assert.Error(t, err)
		_ = require
		assert.Contains(t, err.Error(), "empty")
	})
}

func TestFeeSchedule_OpFee(t *testing.T) {
	s := &FeeSchedule{
		OpFees: []OpFeeEntry{
			{MessageType: "/a", Amount: 10},
			{MessageType: "/m", Amount: 20},
			{MessageType: "/z", Amount: 30},
		},
	}

	t.Run("hit middle", func(t *testing.T) {
		amount, ok := s.OpFee("/m")
		assert.True(t, ok)
		assert.Equal(t, uint64(20), amount)
	})

	t.Run("hit first", func(t *testing.T) {
		amount, ok := s.OpFee("/a")
		assert.True(t, ok)
		assert.Equal(t, uint64(10), amount)
	})

	t.Run("hit last", func(t *testing.T) {
		amount, ok := s.OpFee("/z")
		assert.True(t, ok)
		assert.Equal(t, uint64(30), amount)
	})

	t.Run("miss before first", func(t *testing.T) {
		amount, ok := s.OpFee("/0")
		assert.False(t, ok)
		assert.Equal(t, uint64(0), amount)
	})

	t.Run("miss between", func(t *testing.T) {
		amount, ok := s.OpFee("/b")
		assert.False(t, ok)
		assert.Equal(t, uint64(0), amount)
	})

	t.Run("miss after last", func(t *testing.T) {
		amount, ok := s.OpFee("/zz")
		assert.False(t, ok)
		assert.Equal(t, uint64(0), amount)
	})

	t.Run("nil schedule miss", func(t *testing.T) {
		var s *FeeSchedule
		amount, ok := s.OpFee("/a")
		assert.False(t, ok)
		assert.Equal(t, uint64(0), amount)
	})
}

func TestSortedOpFees(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		assert.Nil(t, SortedOpFees(nil))
		assert.Nil(t, SortedOpFees([]OpFeeEntry{}))
	})

	t.Run("sorts a defensive copy", func(t *testing.T) {
		in := []OpFeeEntry{
			{MessageType: "/z", Amount: 1},
			{MessageType: "/a", Amount: 2},
			{MessageType: "/m", Amount: 3},
		}
		out := SortedOpFees(in)

		assert.Equal(t, "/z", in[0].MessageType, "original slice must be untouched")

		require := assert.Len(t, out, 3)
		_ = require
		assert.Equal(t, "/a", out[0].MessageType)
		assert.Equal(t, "/m", out[1].MessageType)
		assert.Equal(t, "/z", out[2].MessageType)
	})
}
