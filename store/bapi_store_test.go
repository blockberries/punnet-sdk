package store

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/di"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestStateStore creates an in-memory state store for testing.
func createTestStateStore(t *testing.T) statestore.StateStore {
	store, err := statestore.NewMemoryIAVLStore(1000) // cacheSize=1000 for testing
	require.NoError(t, err)
	return store
}

func TestTypedStore_CRUD(t *testing.T) {
	ss := createTestStateStore(t)
	store := NewTypedStore[*testValue](ss, "test/")
	ctx := context.Background()

	// Create
	val := &testValue{Name: "alice", Value: 42}
	err := store.Set(ctx, "key1", val)
	require.NoError(t, err)

	// Read
	got, err := store.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Name)
	assert.Equal(t, 42, got.Value)

	// Update
	val.Value = 100
	err = store.Set(ctx, "key1", val)
	require.NoError(t, err)

	got, err = store.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, 100, got.Value)

	// Delete
	err = store.Delete(ctx, "key1")
	require.NoError(t, err)

	_, err = store.Get(ctx, "key1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestTypedStore_Has(t *testing.T) {
	ss := createTestStateStore(t)
	store := NewTypedStore[*testValue](ss, "test/")
	ctx := context.Background()

	has, err := store.Has(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, has)

	err = store.Set(ctx, "exists", &testValue{Name: "test"})
	require.NoError(t, err)

	has, err = store.Has(ctx, "exists")
	require.NoError(t, err)
	assert.True(t, has)
}

type testValue struct {
	Name  string `cramberry:"1"`
	Value int    `cramberry:"2"`
}

func TestBAPIAccountStore(t *testing.T) {
	ss := createTestStateStore(t)
	store := NewBAPIAccountStore(ss)
	ctx := context.Background()

	// Create account
	account := &ptypes.Account{
		Name: "alice",
		Authority: ptypes.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{"key1": 1},
			AccountWeights: make(map[ptypes.AccountName]uint64),
		},
		Nonce:     0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.Set(ctx, account)
	require.NoError(t, err)

	// Get account
	got, err := store.Get(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, ptypes.AccountName("alice"), got.Name)
	assert.Equal(t, uint64(0), got.Nonce)

	// Increment nonce
	err = store.IncrementNonce(ctx, "alice")
	require.NoError(t, err)

	nonce, err := store.GetNonce(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), nonce)

	// Has account
	has, err := store.Has(ctx, "alice")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = store.Has(ctx, "bob")
	require.NoError(t, err)
	assert.False(t, has)

	// Delete account
	err = store.Delete(ctx, "alice")
	require.NoError(t, err)

	_, err = store.Get(ctx, "alice")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestBAPIBalanceStore(t *testing.T) {
	ss := createTestStateStore(t)
	store := NewBAPIBalanceStore(ss)
	ctx := context.Background()

	// Get returns zero balance for missing entries
	balance, err := store.Get(ctx, "alice", "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), balance.Amount)

	// Set balance
	err = store.Set(ctx, "alice", "stake", 1000)
	require.NoError(t, err)

	balance, err = store.Get(ctx, "alice", "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), balance.Amount)

	// Add amount
	newAmount, err := store.AddAmount(ctx, "alice", "stake", 500)
	require.NoError(t, err)
	assert.Equal(t, uint64(1500), newAmount)

	// Sub amount
	newAmount, err = store.SubAmount(ctx, "alice", "stake", 300)
	require.NoError(t, err)
	assert.Equal(t, uint64(1200), newAmount)

	// Sub insufficient - should fail
	_, err = store.SubAmount(ctx, "alice", "stake", 2000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient balance")

	// Transfer
	err = store.Set(ctx, "bob", "stake", 100)
	require.NoError(t, err)

	err = store.Transfer(ctx, "alice", "bob", "stake", 200)
	require.NoError(t, err)

	aliceBalance, _ := store.GetAmount(ctx, "alice", "stake")
	bobBalance, _ := store.GetAmount(ctx, "bob", "stake")
	assert.Equal(t, uint64(1000), aliceBalance)
	assert.Equal(t, uint64(300), bobBalance)
}

func TestBAPIValidatorStore(t *testing.T) {
	ss := createTestStateStore(t)
	store := NewBAPIValidatorStore(ss)
	ctx := context.Background()

	pubKey := []byte("validator-pubkey-1234567890")

	// Create validator
	validator := &BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power:           100,
		Jailed:          false,
		Description:     "Test validator",
		Commission:      500, // 5%
		TotalDelegation: 0,
	}

	err := store.SetValidator(ctx, validator)
	require.NoError(t, err)

	// Get validator
	got, err := store.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), got.Power)
	assert.Equal(t, "Test validator", got.Description)

	// Update power
	err = store.UpdatePower(ctx, pubKey, 200)
	require.NoError(t, err)

	got, err = store.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(200), got.Power)

	// Jail validator
	err = store.JailValidator(ctx, pubKey)
	require.NoError(t, err)

	got, err = store.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.True(t, got.Jailed)
	assert.Equal(t, uint64(0), got.Power)

	// Unjail validator
	err = store.UnjailValidator(ctx, pubKey)
	require.NoError(t, err)

	got, err = store.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.False(t, got.Jailed)

	// Delegate
	err = store.Delegate(ctx, "alice", pubKey, 1000)
	require.NoError(t, err)

	delegation, err := store.GetDelegation(ctx, "alice", pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), delegation.Amount)

	got, err = store.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), got.TotalDelegation)

	// Undelegate
	err = store.Undelegate(ctx, "alice", pubKey, 300)
	require.NoError(t, err)

	delegation, err = store.GetDelegation(ctx, "alice", pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(700), delegation.Amount)
}

func TestBAPIParamsStore(t *testing.T) {
	ss := createTestStateStore(t)
	store := NewBAPIParamsStore(ss)
	ctx := context.Background()

	// Set default params
	params := DefaultConsensusParams()
	err := store.Set(ctx, params)
	require.NoError(t, err)

	// Get params
	got, err := store.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, params.MaxBlockBytes, got.MaxBlockBytes)
	assert.Equal(t, params.MaxTxBytes, got.MaxTxBytes)

	// Update MaxBlockBytes
	err = store.UpdateMaxBlockBytes(ctx, 30000000)
	require.NoError(t, err)

	got, err = store.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(30000000), got.MaxBlockBytes)

	// Update MaxTxBytes
	err = store.UpdateMaxTxBytes(ctx, 2000000)
	require.NoError(t, err)

	got, err = store.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(2000000), got.MaxTxBytes)

	// Invalid: MaxTxBytes > MaxBlockBytes
	err = store.UpdateMaxTxBytes(ctx, 50000000)
	assert.Error(t, err)
}

func TestBAPIStoreProvider(t *testing.T) {
	ss := createTestStateStore(t)
	provider := NewBAPIStoreProvider(ss)

	assert.NotNil(t, provider.StateStore())
	assert.NotNil(t, provider.GetAccountStore())
	assert.NotNil(t, provider.GetBalanceStore())
	assert.NotNil(t, provider.GetValidatorStore())
	assert.NotNil(t, provider.GetParamsStore())

	// Test DI registration
	container := di.New()
	err := provider.RegisterWithContainer(container)
	require.NoError(t, err)

	// Verify we can resolve the registered stores
	var accountStore *BAPIAccountStore
	err = container.Resolve(&accountStore)
	require.NoError(t, err)
	assert.NotNil(t, accountStore)

	var balanceStore *BAPIBalanceStore
	err = container.Resolve(&balanceStore)
	require.NoError(t, err)
	assert.NotNil(t, balanceStore)

	var validatorStore *BAPIValidatorStore
	err = container.Resolve(&validatorStore)
	require.NoError(t, err)
	assert.NotNil(t, validatorStore)

	var paramsStore *BAPIParamsStore
	err = container.Resolve(&paramsStore)
	require.NoError(t, err)
	assert.NotNil(t, paramsStore)
}
