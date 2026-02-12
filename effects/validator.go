package effects

import (
	"fmt"
)

// EffectValidator validates effects and detects conflicts.
type EffectValidator struct {
	// strict enables strict validation mode (fails on any conflict)
	strict bool
}

// NewEffectValidator creates a new effect validator.
func NewEffectValidator(strict bool) *EffectValidator {
	return &EffectValidator{
		strict: strict,
	}
}

// ValidationResult contains the result of validating a batch of effects.
type ValidationResult struct {
	// Valid indicates if all effects are valid
	Valid bool

	// Errors contains validation errors by effect index
	Errors map[int]error

	// Conflicts contains detected conflicts
	Conflicts []*Conflict

	// WriteKeys tracks all keys that will be written
	WriteKeys map[string]int // key -> effect index

	// ReadKeys tracks all keys that will be read
	ReadKeys map[string]int // key -> effect index
}

// NewValidationResult creates a new validation result.
func NewValidationResult() *ValidationResult {
	return &ValidationResult{
		Valid:     true,
		Errors:    make(map[int]error),
		Conflicts: make([]*Conflict, 0),
		WriteKeys: make(map[string]int),
		ReadKeys:  make(map[string]int),
	}
}

// AddError adds a validation error for an effect.
func (r *ValidationResult) AddError(index int, err error) {
	r.Valid = false
	r.Errors[index] = err
}

// AddConflict adds a conflict to the result.
func (r *ValidationResult) AddConflict(conflict *Conflict) {
	r.Valid = false
	r.Conflicts = append(r.Conflicts, conflict)
}

// Error returns a combined error message for all validation issues.
func (r *ValidationResult) Error() error {
	if r.Valid {
		return nil
	}

	errMsg := "effect validation failed:"

	for idx, err := range r.Errors {
		errMsg += fmt.Sprintf("\n  effect %d: %v", idx, err)
	}

	for _, conflict := range r.Conflicts {
		errMsg += fmt.Sprintf("\n  conflict: %s", conflict.Error())
	}

	return fmt.Errorf("%s", errMsg)
}

// ValidateEffect validates a single effect.
func (v *EffectValidator) ValidateEffect(effect Effect) error {
	if effect == nil {
		return fmt.Errorf("effect is nil")
	}
	return effect.Validate()
}

// ValidateBatch validates a batch of effects and detects conflicts.
func (v *EffectValidator) ValidateBatch(effects []Effect) *ValidationResult {
	result := NewValidationResult()

	// First pass: validate each effect individually
	for i, effect := range effects {
		if effect == nil {
			result.AddError(i, fmt.Errorf("effect is nil"))
			continue
		}

		if err := effect.Validate(); err != nil {
			result.AddError(i, err)
			continue
		}

		// Track key access
		key := KeyString(effect.Key())
		effectType := effect.Type()

		isWrite := effectType == EffectTypeWrite ||
			effectType == EffectTypeDelete ||
			effectType == EffectTypeTransfer

		if isWrite {
			// Check for write-write conflict
			if prevIdx, exists := result.WriteKeys[key]; exists {
				result.AddConflict(&Conflict{
					Type:    ConflictTypeWriteWrite,
					Effect1: effects[prevIdx],
					Effect2: effect,
					Key:     effect.Key(),
				})
			} else {
				result.WriteKeys[key] = i
			}

			// Check for read-write conflict (write after read)
			if prevIdx, exists := result.ReadKeys[key]; exists && v.strict {
				result.AddConflict(&Conflict{
					Type:    ConflictTypeReadWrite,
					Effect1: effects[prevIdx],
					Effect2: effect,
					Key:     effect.Key(),
				})
			}
		} else if effectType == EffectTypeRead {
			// Check for write-read conflict (read after write)
			if prevIdx, exists := result.WriteKeys[key]; exists && v.strict {
				result.AddConflict(&Conflict{
					Type:    ConflictTypeReadWrite,
					Effect1: effects[prevIdx],
					Effect2: effect,
					Key:     effect.Key(),
				})
			} else {
				result.ReadKeys[key] = i
			}
		}
	}

	return result
}

// ValidateForExecution validates effects specifically for execution.
// This performs additional checks beyond basic validation.
func (v *EffectValidator) ValidateForExecution(effects []Effect) error {
	if len(effects) == 0 {
		return nil
	}

	// Validate batch
	result := v.ValidateBatch(effects)
	if !result.Valid {
		return result.Error()
	}

	// Additional execution-specific validation
	for i, effect := range effects {
		switch e := effect.(type) {
		case TransferEffect:
			// Verify transfer amounts don't exceed safe limits
			for _, coin := range e.Amount {
				if coin.Amount > MaxTransferAmount {
					return fmt.Errorf("effect %d: transfer amount %d exceeds safe limit", i, coin.Amount)
				}
			}
		}
	}

	return nil
}

// GroupByKey groups effects by their primary key.
// This is useful for parallel execution planning.
func GroupByKey(effects []Effect) map[string][]Effect {
	groups := make(map[string][]Effect)

	for _, effect := range effects {
		key := KeyString(effect.Key())
		groups[key] = append(groups[key], effect)
	}

	return groups
}

// FindConflictingPairs finds all pairs of conflicting effects.
// Returns a list of conflicts.
func FindConflictingPairs(effects []Effect) []*Conflict {
	var conflicts []*Conflict

	for i := 0; i < len(effects); i++ {
		for j := i + 1; j < len(effects); j++ {
			if conflict := DetectConflict(effects[i], effects[j]); conflict != nil {
				conflicts = append(conflicts, conflict)
			}
		}
	}

	return conflicts
}

// CanExecuteInParallel determines if two effects can be executed in parallel.
func CanExecuteInParallel(e1, e2 Effect) bool {
	// Events can always run in parallel
	if e1.Type() == EffectTypeEvent || e2.Type() == EffectTypeEvent {
		return true
	}

	// Different keys can run in parallel
	if KeyString(e1.Key()) != KeyString(e2.Key()) {
		// Also check dependencies
		deps1 := e1.Dependencies()
		deps2 := e2.Dependencies()

		for _, d1 := range deps1 {
			for _, d2 := range deps2 {
				if KeyString(d1.Key) == KeyString(d2.Key) {
					// Conflict if either is a write
					if !d1.ReadOnly || !d2.ReadOnly {
						return false
					}
				}
			}
		}
		return true
	}

	// Same key - only reads can run in parallel
	return e1.Type() == EffectTypeRead && e2.Type() == EffectTypeRead
}

// BuildExecutionPlan creates an execution plan that respects dependencies.
// Returns a list of effect batches that can be executed in parallel.
func BuildExecutionPlan(effects []Effect) [][]Effect {
	if len(effects) == 0 {
		return nil
	}

	var plan [][]Effect
	remaining := make([]Effect, len(effects))
	copy(remaining, effects)

	for len(remaining) > 0 {
		// Find effects that can run in parallel in this batch
		var batch []Effect
		var nextRemaining []Effect

		for _, effect := range remaining {
			canAdd := true

			// Check if this effect conflicts with any effect in the current batch
			for _, batchEffect := range batch {
				if !CanExecuteInParallel(effect, batchEffect) {
					canAdd = false
					break
				}
			}

			if canAdd {
				batch = append(batch, effect)
			} else {
				nextRemaining = append(nextRemaining, effect)
			}
		}

		if len(batch) == 0 {
			// This shouldn't happen, but if it does, just add one effect
			batch = append(batch, remaining[0])
			nextRemaining = remaining[1:]
		}

		plan = append(plan, batch)
		remaining = nextRemaining
	}

	return plan
}
