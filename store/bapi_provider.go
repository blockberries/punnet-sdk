package store

import (
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/di"
)

// BAPIStoreProvider implements di.StoreProvider and provides all typed stores.
// It is the single point of registration for dependency injection.
type BAPIStoreProvider struct {
	stateStore     statestore.StateStore
	accountStore   *BAPIAccountStore
	balanceStore   *BAPIBalanceStore
	validatorStore *BAPIValidatorStore
	paramsStore    *BAPIParamsStore
}

// NewBAPIStoreProvider creates a new store provider with all typed stores.
func NewBAPIStoreProvider(stateStore statestore.StateStore) *BAPIStoreProvider {
	return &BAPIStoreProvider{
		stateStore:     stateStore,
		accountStore:   NewBAPIAccountStore(stateStore),
		balanceStore:   NewBAPIBalanceStore(stateStore),
		validatorStore: NewBAPIValidatorStore(stateStore),
		paramsStore:    NewBAPIParamsStore(stateStore),
	}
}

// StateStore returns the raw state store.
func (p *BAPIStoreProvider) StateStore() statestore.StateStore {
	return p.stateStore
}

// AccountStore returns the account store.
func (p *BAPIStoreProvider) AccountStore() any {
	return p.accountStore
}

// BalanceStore returns the balance store.
func (p *BAPIStoreProvider) BalanceStore() any {
	return p.balanceStore
}

// ValidatorStore returns the validator store.
func (p *BAPIStoreProvider) ValidatorStore() any {
	return p.validatorStore
}

// ParamsStore returns the params store.
func (p *BAPIStoreProvider) ParamsStore() any {
	return p.paramsStore
}

// GetAccountStore returns the typed account store.
func (p *BAPIStoreProvider) GetAccountStore() *BAPIAccountStore {
	return p.accountStore
}

// GetBalanceStore returns the typed balance store.
func (p *BAPIStoreProvider) GetBalanceStore() *BAPIBalanceStore {
	return p.balanceStore
}

// GetValidatorStore returns the typed validator store.
func (p *BAPIStoreProvider) GetValidatorStore() *BAPIValidatorStore {
	return p.validatorStore
}

// GetParamsStore returns the typed params store.
func (p *BAPIStoreProvider) GetParamsStore() *BAPIParamsStore {
	return p.paramsStore
}

// NotifyRawWrite is called by the effect executor after a raw state-store
// write so the typed stores can update their in-memory key indices. The
// `fullKey` is the raw state-store key — i.e. it includes the typed store's
// prefix (e.g. "accounts/alice", "balances/alice/stake"). Unknown prefixes
// are silently ignored, which makes the call safe to invoke from a generic
// write path.
//
// This is the bridge between the effect executor (which writes raw bytes
// for cramberry-serialized WriteEffect[T] values, bypassing the typed
// stores) and the typed stores that own the in-memory iteration index.
func (p *BAPIStoreProvider) NotifyRawWrite(fullKey []byte) {
	if p == nil || len(fullKey) == 0 {
		return
	}
	if rel, ok := stripPrefix(fullKey, "accounts/"); ok {
		p.accountStore.TrackKey(rel)
		return
	}
	if rel, ok := stripPrefix(fullKey, "balances/"); ok {
		p.balanceStore.TrackKey(rel)
		return
	}
}

// NotifyRawDelete is the delete-side counterpart to NotifyRawWrite.
func (p *BAPIStoreProvider) NotifyRawDelete(fullKey []byte) {
	if p == nil || len(fullKey) == 0 {
		return
	}
	if rel, ok := stripPrefix(fullKey, "accounts/"); ok {
		p.accountStore.UntrackKey(rel)
		return
	}
	if rel, ok := stripPrefix(fullKey, "balances/"); ok {
		p.balanceStore.UntrackKey(rel)
		return
	}
}

// stripPrefix returns the suffix of `key` after `prefix` if `key` starts with
// `prefix`, and reports false otherwise.
func stripPrefix(key []byte, prefix string) (string, bool) {
	if len(key) < len(prefix) {
		return "", false
	}
	if string(key[:len(prefix)]) != prefix {
		return "", false
	}
	return string(key[len(prefix):]), true
}

// Verify BAPIStoreProvider implements the DI interfaces.
var _ di.StateStoreProvider = (*BAPIStoreProvider)(nil)
var _ di.AccountStoreProvider = (*BAPIStoreProvider)(nil)
var _ di.BalanceStoreProvider = (*BAPIStoreProvider)(nil)
var _ di.ValidatorStoreProvider = (*BAPIStoreProvider)(nil)
var _ di.ParamsStoreProvider = (*BAPIStoreProvider)(nil)

// RegisterWithContainer registers all store providers with the DI container.
func (p *BAPIStoreProvider) RegisterWithContainer(c *di.Container) error {
	// Register the provider under each interface type
	if err := c.RegisterInterface((*di.StateStoreProvider)(nil), p); err != nil {
		return err
	}
	if err := c.RegisterInterface((*di.AccountStoreProvider)(nil), p); err != nil {
		return err
	}
	if err := c.RegisterInterface((*di.BalanceStoreProvider)(nil), p); err != nil {
		return err
	}
	if err := c.RegisterInterface((*di.ValidatorStoreProvider)(nil), p); err != nil {
		return err
	}
	if err := c.RegisterInterface((*di.ParamsStoreProvider)(nil), p); err != nil {
		return err
	}

	// Also register the typed stores directly for convenience
	if err := c.Register(p.accountStore); err != nil {
		return err
	}
	if err := c.Register(p.balanceStore); err != nil {
		return err
	}
	if err := c.Register(p.validatorStore); err != nil {
		return err
	}
	if err := c.Register(p.paramsStore); err != nil {
		return err
	}

	return nil
}
