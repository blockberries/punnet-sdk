package effects

import (
	"fmt"
)

// EventEffect represents an event emission effect
type EventEffect struct {
	// EventType is the event type
	EventType string

	// Attributes are the event attributes
	Attributes map[string][]byte
}

// NewEventEffect creates a new event effect with defensive copy of attributes
func NewEventEffect(eventType string, attributes map[string][]byte) EventEffect {
	// Create defensive copy of attributes map
	attrCopy := make(map[string][]byte, len(attributes))
	for k, v := range attributes {
		// Also copy the byte slices
		vCopy := make([]byte, len(v))
		copy(vCopy, v)
		attrCopy[k] = vCopy
	}

	return EventEffect{
		EventType:  eventType,
		Attributes: attrCopy,
	}
}

// Type returns the effect type
func (e EventEffect) Type() EffectType {
	return EffectTypeEvent
}

// Validate performs validation
func (e EventEffect) Validate() error {
	if e.EventType == "" {
		return fmt.Errorf("event type cannot be empty")
	}
	return nil
}

// Dependencies returns the dependencies (events have no dependencies)
func (e EventEffect) Dependencies() []Dependency {
	return []Dependency{}
}

// Key returns the primary key (events use a unique key based on type)
func (e EventEffect) Key() []byte {
	// Events don't conflict with each other, use unique key
	return []byte(fmt.Sprintf("event/%s", e.EventType))
}
