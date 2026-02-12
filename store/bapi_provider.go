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
