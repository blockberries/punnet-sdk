package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/blockberries/punnet-sdk/types"
)

// GenesisState represents the genesis state of the application
type GenesisState struct {
	// ChainID is the blockchain identifier
	ChainID string `json:"chain_id"`

	// GenesisTime is the genesis block time
	GenesisTime time.Time `json:"genesis_time"`

	// InitialHeight is the initial block height
	InitialHeight uint64 `json:"initial_height"`

	// Validators are the initial validator set
	Validators []types.ValidatorUpdate `json:"validators"`

	// AppState contains module-specific genesis data
	// Map of module name to module genesis JSON
	AppState map[string]json.RawMessage `json:"app_state"`
}

// ValidateBasic performs basic validation of genesis state
func (g *GenesisState) ValidateBasic() error {
	if g == nil {
		return fmt.Errorf("genesis state is nil")
	}

	if g.ChainID == "" {
		return fmt.Errorf("chain ID cannot be empty")
	}

	if g.GenesisTime.IsZero() {
		return fmt.Errorf("genesis time cannot be zero")
	}

	if g.InitialHeight == 0 {
		return fmt.Errorf("initial height cannot be zero")
	}

	// Validate validators
	if len(g.Validators) == 0 {
		return fmt.Errorf("must have at least one validator")
	}

	for i, val := range g.Validators {
		if len(val.PubKey) == 0 {
			return fmt.Errorf("validator %d has empty public key", i)
		}
		if val.Power <= 0 {
			return fmt.Errorf("validator %d has non-positive power: %d", i, val.Power)
		}
	}

	return nil
}

// initGenesis initializes the application from genesis state
func (app *Application) initGenesis(ctx context.Context, validators []types.ValidatorUpdate, appStateBytes []byte) error {
	if app == nil {
		return ErrApplicationNil
	}

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	// Parse genesis state
	var genesisState GenesisState
	if len(appStateBytes) > 0 {
		if err := json.Unmarshal(appStateBytes, &genesisState); err != nil {
			return fmt.Errorf("failed to unmarshal genesis state: %w", err)
		}
	} else {
		// Use default genesis if none provided
		genesisState = GenesisState{
			ChainID:       app.chainID,
			GenesisTime:   time.Now(),
			InitialHeight: 1,
			Validators:    validators,
			AppState:      make(map[string]json.RawMessage),
		}
	}

	// Use provided validators if genesis doesn't have them
	if len(genesisState.Validators) == 0 {
		genesisState.Validators = validators
	}

	// Validate genesis state
	if err := genesisState.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid genesis state: %w", err)
	}

	// Verify chain ID matches
	if genesisState.ChainID != app.chainID {
		return fmt.Errorf("chain ID mismatch: expected %s, got %s", app.chainID, genesisState.ChainID)
	}

	// Create genesis block header
	header := NewBlockHeader(
		genesisState.InitialHeight,
		genesisState.GenesisTime,
		genesisState.ChainID,
		nil, // No proposer for genesis
	)

	// Create execution context with system account
	execCtx, err := NewContext(ctx, header, "system")
	if err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}

	// Get all modules and sort for deterministic initialization
	modules := app.router.Modules()
	sortedModules := make([]Module, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	// Initialize each module
	for _, mod := range sortedModules {
		initGenesis := mod.InitGenesis()
		if initGenesis == nil {
			continue
		}

		// Get module genesis data
		moduleGenesis, exists := genesisState.AppState[mod.Name()]
		if !exists {
			// Use empty genesis if not provided
			moduleGenesis = json.RawMessage("{}")
		}

		// Initialize module
		if err := initGenesis(execCtx, moduleGenesis); err != nil {
			return fmt.Errorf("module %s InitGenesis failed: %w", mod.Name(), err)
		}
	}

	// Store initial validator set (if staking module exists)
	// The staking module should handle this in its InitGenesis
	// For now, we just validate that validators were provided

	return nil
}

// ExportGenesis exports the current application state as genesis
func (app *Application) ExportGenesis(ctx context.Context) (*GenesisState, error) {
	if app == nil {
		return nil, ErrApplicationNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	// Get current height and time
	app.mu.RLock()
	header := app.currentHeader
	app.mu.RUnlock()

	var height uint64
	var timestamp time.Time
	if header != nil {
		height = header.Height
		timestamp = header.Time
	} else {
		height = uint64(app.stateStore.Version())
		timestamp = time.Now()
	}

	// Get all modules and sort for deterministic export
	modules := app.router.Modules()
	sortedModules := make([]Module, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	// Export each module's state
	appState := make(map[string]json.RawMessage)
	for _, mod := range sortedModules {
		exportGenesis := mod.ExportGenesis()
		if exportGenesis == nil {
			continue
		}

		// Export module state
		moduleState, err := exportGenesis(ctx)
		if err != nil {
			return nil, fmt.Errorf("module %s ExportGenesis failed: %w", mod.Name(), err)
		}

		if len(moduleState) > 0 {
			appState[mod.Name()] = json.RawMessage(moduleState)
		}
	}

	genesis := &GenesisState{
		ChainID:       app.chainID,
		GenesisTime:   timestamp,
		InitialHeight: height,
		Validators:    []types.ValidatorUpdate{}, // TODO: Export current validator set
		AppState:      appState,
	}

	return genesis, nil
}
