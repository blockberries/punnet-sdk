package di

import (
	"github.com/blockberries/blockberry/pkg/statestore"
)

// NeedsStateStore is implemented by modules that need raw state store access.
// This should be used sparingly - prefer typed stores.
type NeedsStateStore interface {
	SetStateStore(statestore.StateStore)
}

// NeedsAccountStore is implemented by modules that need account access.
type NeedsAccountStore interface {
	SetAccountStore(any)
}

// NeedsBalanceStore is implemented by modules that need balance access.
type NeedsBalanceStore interface {
	SetBalanceStore(any)
}

// NeedsValidatorStore is implemented by modules that need validator access.
type NeedsValidatorStore interface {
	SetValidatorStore(any)
}

// NeedsParamsStore is implemented by modules that need consensus params access.
type NeedsParamsStore interface {
	SetParamsStore(any)
}

// StateStoreProvider provides access to the raw state store.
type StateStoreProvider interface {
	StateStore() statestore.StateStore
}

// AccountStoreProvider provides access to the account store.
type AccountStoreProvider interface {
	AccountStore() any
}

// BalanceStoreProvider provides access to the balance store.
type BalanceStoreProvider interface {
	BalanceStore() any
}

// ValidatorStoreProvider provides access to the validator store.
type ValidatorStoreProvider interface {
	ValidatorStore() any
}

// ParamsStoreProvider provides access to the params store.
type ParamsStoreProvider interface {
	ParamsStore() any
}
