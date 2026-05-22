package staking

import (
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTombstone_SecondSlashTombstones pins the double-slash rule
// (Phase 2.7): a validator that is already Jailed when a new slash
// fires gets the permanent Tombstoned flag.
func TestTombstone_SecondSlashTombstones(t *testing.T) {
	f := newSlashFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xe1

	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           1000,
		TotalDelegation: 1000,
	}))
	require.NoError(t, f.bs.Set(f.ctx, "staking.pool", "stake", 2000))

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}

	// First slash: jailed but not tombstoned.
	effs, err := f.mod.ProcessEvidence(f.ctx, &runtime.BAPIBlockContext{Height: 1}, ev)
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	v1, _ := f.vs.GetValidator(f.ctx, pubKey)
	assert.True(t, v1.Jailed, "first slash must jail")
	assert.False(t, v1.Tombstoned, "first slash must NOT tombstone")

	// Second slash on the same (now-jailed) validator: tombstone.
	effs, err = f.mod.ProcessEvidence(f.ctx, &runtime.BAPIBlockContext{Height: 2}, ev)
	require.NoError(t, err)
	_, err = f.exec.Execute(effs)
	require.NoError(t, err)

	v2, _ := f.vs.GetValidator(f.ctx, pubKey)
	assert.True(t, v2.Jailed, "still jailed after second slash")
	assert.True(t, v2.Tombstoned, "second slash tombstones permanently")
}

// TestTombstone_TombstonePersists confirms that once tombstoned a
// further slash leaves the Tombstoned flag set (never clears it).
func TestTombstone_TombstonePersists(t *testing.T) {
	f := newSlashFixture(t)
	pubKey := make([]byte, 32)
	pubKey[0] = 0xe2

	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           1000,
		TotalDelegation: 1000,
		Jailed:          true,
		Tombstoned:      true,
	}))
	require.NoError(t, f.bs.Set(f.ctx, "staking.pool", "stake", 1000))

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, _ := f.mod.ProcessEvidence(f.ctx, &runtime.BAPIBlockContext{Height: 1}, ev)
	_, err := f.exec.Execute(effs)
	require.NoError(t, err)

	v, _ := f.vs.GetValidator(f.ctx, pubKey)
	assert.True(t, v.Tombstoned)
}

// TestActiveSet_ExcludesJailedAndTombstoned verifies that the
// epoch-boundary refresh skips validators whose Jailed or
// Tombstoned bit is set, even if their Power is still > 0.
func TestActiveSet_ExcludesJailedAndTombstoned(t *testing.T) {
	f := newSlashFixture(t)
	pkActive := make([]byte, 32)
	pkActive[0] = 0xa1
	pkJailed := make([]byte, 32)
	pkJailed[0] = 0xa2
	pkTomb := make([]byte, 32)
	pkTomb[0] = 0xa3

	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pkActive},
		Power:  1000, TotalDelegation: 1000,
	}))
	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pkJailed},
		Power:  2000, TotalDelegation: 2000, Jailed: true,
	}))
	require.NoError(t, f.vs.SetValidator(f.ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pkTomb},
		Power:  3000, TotalDelegation: 3000, Jailed: true, Tombstoned: true,
	}))

	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 1, "only the active validator may enter the set")
	assert.Equal(t, pkActive, updates[0].PubKey.Data)
}
