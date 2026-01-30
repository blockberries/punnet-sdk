package runtime

import (
	"context"
	"fmt"
	"sort"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/types"
)

// callBeginBlockers calls BeginBlock on all registered modules
func (app *Application) callBeginBlockers(ctx context.Context, header *BlockHeader) error {
	if app == nil {
		return ErrApplicationNil
	}

	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if header == nil {
		return fmt.Errorf("block header cannot be nil")
	}

	// Get all modules (defensive copy already made by Router.Modules())
	modules := app.router.Modules()
	if len(modules) == 0 {
		return nil
	}

	// Sort modules by name for deterministic execution order
	sortedModules := make([]Module, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	// Create execution context with empty account (system context)
	execCtx, err := NewContext(ctx, header, "system")
	if err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}

	// Collect all effects from BeginBlock hooks
	var allEffects []effects.Effect

	for _, mod := range sortedModules {
		beginBlocker := mod.BeginBlock()
		if beginBlocker == nil {
			continue
		}

		// Call BeginBlock hook
		moduleEffects, err := beginBlocker(execCtx)
		if err != nil {
			return fmt.Errorf("module %s BeginBlock failed: %w", mod.Name(), err)
		}

		// Collect effects
		if len(moduleEffects) > 0 {
			allEffects = append(allEffects, moduleEffects...)
		}
	}

	// Execute all collected effects
	if len(allEffects) > 0 {
		_, err := app.effectExecutor.Execute(allEffects)
		if err != nil {
			return fmt.Errorf("BeginBlock effect execution failed: %w", err)
		}
	}

	return nil
}

// callEndBlockers calls EndBlock on all registered modules
func (app *Application) callEndBlockers(ctx context.Context, header *BlockHeader) (*types.EndBlockResult, error) {
	if app == nil {
		return nil, ErrApplicationNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if header == nil {
		return nil, fmt.Errorf("block header cannot be nil")
	}

	// Get all modules (defensive copy already made by Router.Modules())
	modules := app.router.Modules()
	if len(modules) == 0 {
		return &types.EndBlockResult{}, nil
	}

	// Sort modules by name for deterministic execution order
	sortedModules := make([]Module, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	// Create execution context with empty account (system context)
	execCtx, err := NewContext(ctx, header, "system")
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	// Collect all effects and validator updates
	var allEffects []effects.Effect
	var allValidatorUpdates []types.ValidatorUpdate
	var allEvents []types.Event

	for _, mod := range sortedModules {
		endBlocker := mod.EndBlock()
		if endBlocker == nil {
			continue
		}

		// Call EndBlock hook
		moduleEffects, validatorUpdates, err := endBlocker(execCtx)
		if err != nil {
			return nil, fmt.Errorf("module %s EndBlock failed: %w", mod.Name(), err)
		}

		// Collect effects
		if len(moduleEffects) > 0 {
			allEffects = append(allEffects, moduleEffects...)
		}

		// Collect validator updates
		if len(validatorUpdates) > 0 {
			allValidatorUpdates = append(allValidatorUpdates, validatorUpdates...)
		}
	}

	// Execute all collected effects
	if len(allEffects) > 0 {
		execResult, err := app.effectExecutor.Execute(allEffects)
		if err != nil {
			return nil, fmt.Errorf("EndBlock effect execution failed: %w", err)
		}

		// Convert execution events to types.Event
		for _, event := range execResult.Events {
			txEvent := types.NewEvent(event.Type)
			for key, value := range event.Attributes {
				txEvent.AddAttribute(key, value)
			}
			allEvents = append(allEvents, txEvent)
		}
	}

	// Deduplicate validator updates (last update wins)
	validatorUpdates := deduplicateValidatorUpdates(allValidatorUpdates)

	return &types.EndBlockResult{
		ValidatorUpdates: validatorUpdates,
		Events:           allEvents,
	}, nil
}

// deduplicateValidatorUpdates removes duplicate validator updates
// keeping only the last update for each validator (by public key)
func deduplicateValidatorUpdates(updates []types.ValidatorUpdate) []types.ValidatorUpdate {
	if len(updates) == 0 {
		return nil
	}

	// Use map to track last update for each validator
	updateMap := make(map[string]types.ValidatorUpdate)
	keyOrder := make([]string, 0, len(updates))

	for _, update := range updates {
		key := string(update.PubKey)
		if _, exists := updateMap[key]; !exists {
			keyOrder = append(keyOrder, key)
		}
		updateMap[key] = update
	}

	// Build result maintaining order of first appearance
	result := make([]types.ValidatorUpdate, 0, len(updateMap))
	for _, key := range keyOrder {
		result = append(result, updateMap[key])
	}

	return result
}
