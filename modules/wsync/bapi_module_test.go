package wsync

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSource returns fixed values for the checkpoint source.
type stubSource struct {
	appHash Hash32
	vsHash  Hash32
}

func (s *stubSource) LatestAppHash() Hash32             { return s.appHash }
func (s *stubSource) ActiveValidatorSetHash() Hash32    { return s.vsHash }

func newFixture(t *testing.T, src CheckpointSource) (*BAPIWsyncModule, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	mod, err := NewBAPIWsyncModule(ss)
	require.NoError(t, err)
	if src != nil {
		mod.WithCheckpointSource(src)
	}
	require.NoError(t, mod.InitGenesis(context.Background(), nil))
	return mod, context.Background()
}

func signedCheckpoint(t *testing.T, height uint64, app, vs Hash32, signers []ed25519.PrivateKey, validatorSet map[string]uint64) *Checkpoint {
	t.Helper()
	ck := &Checkpoint{
		Height:           height,
		AppHash:          app,
		ValidatorSetHash: vs,
	}
	signed := ck.SignedContent()
	for _, priv := range signers {
		pub := priv.Public().(ed25519.PublicKey)
		ck.Attestations = append(ck.Attestations, Attestation{
			ValidatorPubKey: []byte(pub),
			Signature:       ed25519.Sign(priv, signed),
			VotingPower:     validatorSet[string(pub)],
		})
	}
	return ck
}

// TestEndBlock_EmitsCheckpointAtHourlyBoundary: a module wired
// with a CheckpointSource creates an unsigned checkpoint at every
// HourlyBlocks-multiple height.
func TestEndBlock_EmitsCheckpointAtHourlyBoundary(t *testing.T) {
	app := Hash32{0x01}
	vs := Hash32{0x02}
	mod, ctx := newFixture(t, &stubSource{appHash: app, vsHash: vs})

	// Mid-hour: no checkpoint.
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks - 1})
	require.NoError(t, err)
	ck, err := mod.GetCheckpoint(ctx, HourlyBlocks-1)
	require.Error(t, err, "no checkpoint stored at non-hourly height")
	_ = ck

	// On boundary: checkpoint emitted.
	_, _, err = mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks})
	require.NoError(t, err)
	ck, err = mod.GetCheckpoint(ctx, HourlyBlocks)
	require.NoError(t, err)
	require.NotNil(t, ck)
	assert.Equal(t, HourlyBlocks, ck.Height)
	assert.Equal(t, app, ck.AppHash)
	assert.Equal(t, vs, ck.ValidatorSetHash)
	assert.Empty(t, ck.Attestations, "unsigned at emit time; gossip layer attaches later")
}

// TestEndBlock_NoCheckpointWithoutSource: a module without a
// CheckpointSource is a no-op even at hourly boundaries.
func TestEndBlock_NoCheckpointWithoutSource(t *testing.T) {
	mod, ctx := newFixture(t, nil)
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks})
	require.NoError(t, err)
	_, err = mod.GetCheckpoint(ctx, HourlyBlocks)
	assert.Error(t, err, "no source ⇒ no checkpoint")
}

// TestEndBlock_PrunesOlderThanUnbondingPeriod: after enough
// blocks pass that a checkpoint is older than the unbonding
// period, the rolling-window prune removes it.
func TestEndBlock_PrunesOlderThanUnbondingPeriod(t *testing.T) {
	mod, ctx := newFixture(t, &stubSource{})
	// Force a checkpoint at height HourlyBlocks (= 3600).
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks})
	require.NoError(t, err)
	ck, _ := mod.GetCheckpoint(ctx, HourlyBlocks)
	require.NotNil(t, ck)

	// Run EndBlock at a height well past UnbondingPeriodBlocks.
	farFuture := UnbondingPeriodBlocks + HourlyBlocks + 1
	_, _, err = mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: farFuture})
	require.NoError(t, err)

	_, err = mod.GetCheckpoint(ctx, HourlyBlocks)
	assert.Error(t, err, "checkpoint older than unbonding period must be pruned")
}

// TestAddAttestation_RoundTrip: a signature is appended and
// queryable.
func TestAddAttestation_RoundTrip(t *testing.T) {
	mod, ctx := newFixture(t, &stubSource{appHash: Hash32{0xa}})
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks})
	require.NoError(t, err)

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	ck, _ := mod.GetCheckpoint(ctx, HourlyBlocks)
	sig := ed25519.Sign(priv, ck.SignedContent())

	require.NoError(t, mod.AddAttestation(ctx, HourlyBlocks, Attestation{
		ValidatorPubKey: []byte(pub),
		Signature:       sig,
		VotingPower:     100,
	}))

	ck, _ = mod.GetCheckpoint(ctx, HourlyBlocks)
	require.Len(t, ck.Attestations, 1)
}

// TestAddAttestation_BadSignatureRejected.
func TestAddAttestation_BadSignatureRejected(t *testing.T) {
	mod, ctx := newFixture(t, &stubSource{})
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks})
	require.NoError(t, err)

	pub, _, _ := ed25519.GenerateKey(nil)
	err = mod.AddAttestation(ctx, HourlyBlocks, Attestation{
		ValidatorPubKey: []byte(pub),
		Signature:       make([]byte, ed25519.SignatureSize),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

// TestAddAttestation_DuplicatesDropped.
func TestAddAttestation_DuplicatesDropped(t *testing.T) {
	mod, ctx := newFixture(t, &stubSource{})
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: HourlyBlocks})
	require.NoError(t, err)

	pub, priv, _ := ed25519.GenerateKey(nil)
	ck, _ := mod.GetCheckpoint(ctx, HourlyBlocks)
	sig := ed25519.Sign(priv, ck.SignedContent())
	att := Attestation{ValidatorPubKey: []byte(pub), Signature: sig, VotingPower: 100}

	require.NoError(t, mod.AddAttestation(ctx, HourlyBlocks, att))
	require.NoError(t, mod.AddAttestation(ctx, HourlyBlocks, att))

	ck, _ = mod.GetCheckpoint(ctx, HourlyBlocks)
	assert.Len(t, ck.Attestations, 1, "duplicate attestation silently dropped")
}

// TestVerifyCheckpoint_HappyPath: a checkpoint signed by ≥2/3
// stake validates.
func TestVerifyCheckpoint_HappyPath(t *testing.T) {
	// 3 validators with equal power 100 each → total 300.
	// 2/3 → require ≥ 200 signed.
	var keys []ed25519.PrivateKey
	validatorSet := make(map[string]uint64)
	for i := 0; i < 3; i++ {
		pub, priv, _ := ed25519.GenerateKey(nil)
		keys = append(keys, priv)
		validatorSet[string(pub)] = 100
	}

	vs := ComputeValidatorSetHash(validatorSet)
	app := Hash32{0xa, 0xb, 0xc}
	ck := signedCheckpoint(t, HourlyBlocks, app, vs, keys[:2], validatorSet) // 200/300 = 66.7%
	require.NoError(t, VerifyCheckpoint(ck, validatorSet, 0))
}

// TestVerifyCheckpoint_InsufficientStake.
func TestVerifyCheckpoint_InsufficientStake(t *testing.T) {
	var keys []ed25519.PrivateKey
	validatorSet := make(map[string]uint64)
	for i := 0; i < 3; i++ {
		pub, priv, _ := ed25519.GenerateKey(nil)
		keys = append(keys, priv)
		validatorSet[string(pub)] = 100
	}
	vs := ComputeValidatorSetHash(validatorSet)
	ck := signedCheckpoint(t, HourlyBlocks, Hash32{}, vs, keys[:1], validatorSet) // only 100/300
	err := VerifyCheckpoint(ck, validatorSet, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient stake")
}

// TestVerifyCheckpoint_Expired: a checkpoint older than the
// unbonding period from the current tip is rejected.
func TestVerifyCheckpoint_Expired(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	vs := map[string]uint64{string(pub): 100}
	vsHash := ComputeValidatorSetHash(vs)
	ck := signedCheckpoint(t, HourlyBlocks, Hash32{}, vsHash, []ed25519.PrivateKey{priv}, vs)

	currentHeight := HourlyBlocks + UnbondingPeriodBlocks + 1
	err := VerifyCheckpoint(ck, vs, currentHeight)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// TestVerifyCheckpoint_ValidatorSetHashMismatch: a checkpoint
// whose declared ValidatorSetHash doesn't match the supplied set
// is rejected.
func TestVerifyCheckpoint_ValidatorSetHashMismatch(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	vs := map[string]uint64{string(pub): 100}
	ck := signedCheckpoint(t, HourlyBlocks, Hash32{}, Hash32{0xff}, []ed25519.PrivateKey{priv}, vs)
	err := VerifyCheckpoint(ck, vs, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validator-set hash mismatch")
}

// TestVerifyCheckpoint_NonSetSignerRejected: an attestation from
// a validator not in the supplied set fails verification.
func TestVerifyCheckpoint_NonSetSignerRejected(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	vs := map[string]uint64{string(pub): 100}
	vsHash := ComputeValidatorSetHash(vs)

	// Sign with a DIFFERENT key.
	_, outsider, _ := ed25519.GenerateKey(nil)
	ck := signedCheckpoint(t, HourlyBlocks, Hash32{}, vsHash, []ed25519.PrivateKey{outsider}, vs)
	// Patch the attestation to use the outsider's pub key (since
	// signedCheckpoint looked it up by key indirectly).
	outsiderPub := outsider.Public().(ed25519.PublicKey)
	ck.Attestations[0].ValidatorPubKey = []byte(outsiderPub)

	err := VerifyCheckpoint(ck, vs, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in active set")
	_ = priv
}

// TestComputeValidatorSetHash_Deterministic verifies the digest
// is independent of map iteration order.
func TestComputeValidatorSetHash_Deterministic(t *testing.T) {
	vs := map[string]uint64{
		"alice":   100,
		"bob":     200,
		"charlie": 50,
	}
	h1 := ComputeValidatorSetHash(vs)
	for i := 0; i < 10; i++ {
		assert.Equal(t, h1, ComputeValidatorSetHash(vs), "hash deterministic across map-iter randomisation")
	}
}
