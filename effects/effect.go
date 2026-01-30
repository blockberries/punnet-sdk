package effects

import (
	"fmt"
)

// EffectType represents the type of effect
type EffectType uint8

const (
	// EffectTypeRead indicates a read operation
	EffectTypeRead EffectType = iota

	// EffectTypeWrite indicates a write operation
	EffectTypeWrite

	// EffectTypeTransfer indicates a token transfer
	EffectTypeTransfer

	// EffectTypeEvent indicates an event emission
	EffectTypeEvent

	// EffectTypeDelete indicates a deletion operation
	EffectTypeDelete
)

// String returns the string representation of EffectType
func (t EffectType) String() string {
	switch t {
	case EffectTypeRead:
		return "read"
	case EffectTypeWrite:
		return "write"
	case EffectTypeTransfer:
		return "transfer"
	case EffectTypeEvent:
		return "event"
	case EffectTypeDelete:
		return "delete"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// DependencyType represents the type of dependency
type DependencyType uint8

const (
	// DependencyTypeAccount indicates an account dependency
	DependencyTypeAccount DependencyType = iota

	// DependencyTypeBalance indicates a balance dependency
	DependencyTypeBalance

	// DependencyTypeValidator indicates a validator dependency
	DependencyTypeValidator

	// DependencyTypeGeneric indicates a generic key-value dependency
	DependencyTypeGeneric
)

// Dependency represents a dependency on state
type Dependency struct {
	// Type is the dependency type
	Type DependencyType

	// Key is the state key this depends on
	Key []byte

	// ReadOnly indicates if this is a read-only dependency
	ReadOnly bool
}

// Effect is the interface that all effects must implement
type Effect interface {
	// Type returns the effect type
	Type() EffectType

	// Validate performs validation of the effect
	Validate() error

	// Dependencies returns the dependencies of this effect
	Dependencies() []Dependency

	// Key returns the primary key affected by this effect (for conflict detection)
	Key() []byte
}

// Collector collects effects from message handlers
type Collector struct {
	effects []Effect
}

// NewCollector creates a new effect collector
func NewCollector() *Collector {
	return &Collector{
		effects: make([]Effect, 0),
	}
}

// Add adds an effect to the collector
func (c *Collector) Add(effect Effect) error {
	if c == nil {
		return fmt.Errorf("collector is nil")
	}
	if effect == nil {
		return fmt.Errorf("cannot add nil effect")
	}

	if err := effect.Validate(); err != nil {
		return fmt.Errorf("invalid effect: %w", err)
	}

	c.effects = append(c.effects, effect)
	return nil
}

// AddMultiple adds multiple effects to the collector
func (c *Collector) AddMultiple(effects []Effect) error {
	if c == nil {
		return fmt.Errorf("collector is nil")
	}
	for i, effect := range effects {
		if err := c.Add(effect); err != nil {
			return fmt.Errorf("effect %d: %w", i, err)
		}
	}
	return nil
}

// Collect returns all collected effects and clears the collector
func (c *Collector) Collect() []Effect {
	if c == nil {
		return nil
	}
	result := c.effects
	c.effects = make([]Effect, 0)
	return result
}

// Count returns the number of collected effects
func (c *Collector) Count() int {
	if c == nil {
		return 0
	}
	return len(c.effects)
}

// Clear clears all collected effects
func (c *Collector) Clear() {
	if c == nil {
		return
	}
	c.effects = make([]Effect, 0)
}

// ConflictType represents the type of conflict between effects
type ConflictType uint8

const (
	// ConflictTypeNone indicates no conflict
	ConflictTypeNone ConflictType = iota

	// ConflictTypeReadWrite indicates a read-write conflict
	ConflictTypeReadWrite

	// ConflictTypeWriteWrite indicates a write-write conflict
	ConflictTypeWriteWrite
)

// String returns the string representation of ConflictType
func (t ConflictType) String() string {
	switch t {
	case ConflictTypeNone:
		return "none"
	case ConflictTypeReadWrite:
		return "read-write"
	case ConflictTypeWriteWrite:
		return "write-write"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// Conflict represents a conflict between two effects
type Conflict struct {
	// Type is the conflict type
	Type ConflictType

	// Effect1 is the first conflicting effect
	Effect1 Effect

	// Effect2 is the second conflicting effect
	Effect2 Effect

	// Key is the conflicting key
	Key []byte
}

// Error returns the error message for the conflict
func (c *Conflict) Error() string {
	if c == nil {
		return "nil conflict"
	}
	if c.Effect1 == nil || c.Effect2 == nil {
		return fmt.Sprintf("%s conflict on key %x with nil effect", c.Type, c.Key)
	}
	return fmt.Sprintf("%s conflict on key %x between %s and %s",
		c.Type, c.Key, c.Effect1.Type(), c.Effect2.Type())
}

// DetectConflict detects if two effects conflict
func DetectConflict(e1, e2 Effect) *Conflict {
	if e1 == nil || e2 == nil {
		return nil
	}

	key1 := e1.Key()
	key2 := e2.Key()

	// No conflict if keys are different
	if string(key1) != string(key2) {
		return nil
	}

	// Determine conflict type based on effect types
	isWrite1 := e1.Type() == EffectTypeWrite || e1.Type() == EffectTypeTransfer || e1.Type() == EffectTypeDelete
	isWrite2 := e2.Type() == EffectTypeWrite || e2.Type() == EffectTypeTransfer || e2.Type() == EffectTypeDelete

	if isWrite1 && isWrite2 {
		return &Conflict{
			Type:    ConflictTypeWriteWrite,
			Effect1: e1,
			Effect2: e2,
			Key:     key1,
		}
	}

	isRead1 := e1.Type() == EffectTypeRead
	isRead2 := e2.Type() == EffectTypeRead

	if (isRead1 && isWrite2) || (isWrite1 && isRead2) {
		return &Conflict{
			Type:    ConflictTypeReadWrite,
			Effect1: e1,
			Effect2: e2,
			Key:     key1,
		}
	}

	return nil
}

// KeyString returns a string representation of a key for use in maps
func KeyString(key []byte) string {
	return string(key)
}

// ValidateEffects validates a list of effects
func ValidateEffects(effects []Effect) error {
	for i, effect := range effects {
		if effect == nil {
			return fmt.Errorf("effect %d is nil", i)
		}
		if err := effect.Validate(); err != nil {
			return fmt.Errorf("effect %d: %w", i, err)
		}
	}
	return nil
}
