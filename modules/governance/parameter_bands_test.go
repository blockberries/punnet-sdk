package governance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParameterRegistry_RegisterAndGet covers the basic happy path:
// register a band, read it back.
func TestParameterRegistry_RegisterAndGet(t *testing.T) {
	r := NewParameterRegistry()
	band := ParameterBand{
		Name: "max_batches_per_block",
		SoftMin: 10, SoftMax: 200,
		HardMin: 5, HardMax: 500,
	}
	require.NoError(t, r.Register(band))

	got, ok := r.Get("max_batches_per_block")
	require.True(t, ok)
	assert.Equal(t, band, got)
}

// TestParameterRegistry_RejectsMalformedBand verifies the
// pre-flight checks: empty name, inverted bands, hard not
// containing soft.
func TestParameterRegistry_RejectsMalformedBand(t *testing.T) {
	cases := []struct {
		name     string
		band     ParameterBand
		wantText string
	}{
		{"empty name", ParameterBand{SoftMin: 1, SoftMax: 2, HardMin: 1, HardMax: 2}, "name cannot be empty"},
		{"soft inverted", ParameterBand{Name: "p", SoftMin: 10, SoftMax: 5, HardMin: 1, HardMax: 100}, "SoftMin"},
		{"hard inverted", ParameterBand{Name: "p", SoftMin: 1, SoftMax: 10, HardMin: 100, HardMax: 5}, "HardMin"},
		{"hard above soft min", ParameterBand{Name: "p", SoftMin: 0, SoftMax: 10, HardMin: 5, HardMax: 100}, "hard band must contain soft"},
		{"hard below soft max", ParameterBand{Name: "p", SoftMin: 0, SoftMax: 100, HardMin: 0, HardMax: 50}, "hard band must contain soft"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewParameterRegistry()
			err := r.Register(tc.band)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantText)
		})
	}
}

// TestParameterRegistry_RejectsDuplicate confirms that
// double-registration is a programmer error.
func TestParameterRegistry_RejectsDuplicate(t *testing.T) {
	r := NewParameterRegistry()
	band := ParameterBand{Name: "p", SoftMin: 1, SoftMax: 2, HardMin: 1, HardMax: 2}
	require.NoError(t, r.Register(band))
	require.Error(t, r.Register(band))
}

// TestParameterRegistry_Replace lets tests override a registered
// band without going through the duplicate-rejection gate.
func TestParameterRegistry_Replace(t *testing.T) {
	r := NewParameterRegistry()
	require.NoError(t, r.Register(ParameterBand{Name: "p", SoftMin: 1, SoftMax: 2, HardMin: 0, HardMax: 100}))
	require.NoError(t, r.Replace(ParameterBand{Name: "p", SoftMin: 10, SoftMax: 20, HardMin: 0, HardMax: 100}))
	got, _ := r.Get("p")
	assert.Equal(t, int64(10), got.SoftMin)
}

// TestValidateChange_ClassRules is the Phase 4.3 contract: simple
// stays within soft, super may exit soft to within hard,
// constitutional may exit hard.
func TestValidateChange_ClassRules(t *testing.T) {
	r := NewParameterRegistry()
	require.NoError(t, r.Register(ParameterBand{
		Name:    "max_validators",
		SoftMin: 50, SoftMax: 200,
		HardMin: 21, HardMax: 300,
	}))

	cases := []struct {
		class    string
		newVal   int64
		wantErr  bool
	}{
		// Simple7d
		{ProposalClassSimple7d, 100, false}, // within soft
		{ProposalClassSimple7d, 50, false},  // soft boundary
		{ProposalClassSimple7d, 200, false}, // soft boundary
		{ProposalClassSimple7d, 49, true},   // below soft
		{ProposalClassSimple7d, 201, true},  // above soft
		// Super30d
		{ProposalClassSuper30d, 100, false}, // within soft (allowed)
		{ProposalClassSuper30d, 21, false},  // hard boundary
		{ProposalClassSuper30d, 300, false}, // hard boundary
		{ProposalClassSuper30d, 20, true},   // below hard
		{ProposalClassSuper30d, 301, true},  // above hard
		// Super60d (same hard band as Super30d)
		{ProposalClassSuper60d, 21, false},
		{ProposalClassSuper60d, 20, true},
		// Constitutional — anything goes
		{ProposalClassConstitutional, 100_000, false},
		{ProposalClassConstitutional, -5, false},
	}
	for _, tc := range cases {
		err := r.ValidateChange(tc.class, "max_validators", tc.newVal)
		if tc.wantErr {
			assert.Error(t, err, "class=%s value=%d should reject", tc.class, tc.newVal)
		} else {
			assert.NoError(t, err, "class=%s value=%d should accept", tc.class, tc.newVal)
		}
	}
}

// TestValidateChange_UnknownParameterRejected: every
// governance-tunable parameter must be in the registry. Unknown
// names are rejected to prevent silent typos in proposal text.
func TestValidateChange_UnknownParameterRejected(t *testing.T) {
	r := NewParameterRegistry()
	err := r.ValidateChange(ProposalClassSimple7d, "not_registered", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in governance registry")
}

// TestValidateChange_UnknownClassRejected: a malformed class
// string is itself a structural error.
func TestValidateChange_UnknownClassRejected(t *testing.T) {
	r := NewParameterRegistry()
	require.NoError(t, r.Register(ParameterBand{Name: "p", SoftMin: 0, SoftMax: 10, HardMin: 0, HardMax: 10}))
	err := r.ValidateChange("freeform-poll", "p", 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown proposal class")
}
