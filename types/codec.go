package types

import (
	"encoding/json"
	"fmt"
	"sync"
)

// messageRegistry maps type URLs to factory functions for concrete Message types.
var (
	messageRegistry   = make(map[string]func() Message)
	messageRegistryMu sync.RWMutex
)

// RegisterMessage registers a Message factory for the given type URL.
// Panics if the type URL is already registered or empty.
func RegisterMessage(typeURL string, factory func() Message) {
	if typeURL == "" {
		panic("types: RegisterMessage called with empty typeURL")
	}
	if factory == nil {
		panic("types: RegisterMessage called with nil factory")
	}

	messageRegistryMu.Lock()
	defer messageRegistryMu.Unlock()

	if _, exists := messageRegistry[typeURL]; exists {
		panic(fmt.Sprintf("types: message type %q already registered", typeURL))
	}
	messageRegistry[typeURL] = factory
}

// GetMessageFactory returns the factory function for a registered message type.
func GetMessageFactory(typeURL string) (func() Message, bool) {
	messageRegistryMu.RLock()
	defer messageRegistryMu.RUnlock()

	factory, ok := messageRegistry[typeURL]
	return factory, ok
}

// MessageEnvelope is the JSON wire format for a type-discriminated Message.
type MessageEnvelope struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// MarshalMessageJSON marshals a Message into a type-discriminated JSON envelope.
func MarshalMessageJSON(msg Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("cannot marshal nil message")
	}

	value, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message value: %w", err)
	}

	envelope := MessageEnvelope{
		Type:  msg.Type(),
		Value: value,
	}
	return json.Marshal(envelope)
}

// UnmarshalMessageJSON unmarshals a type-discriminated JSON envelope into a concrete Message.
func UnmarshalMessageJSON(data []byte) (Message, error) {
	var envelope MessageEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal message envelope: %w", err)
	}

	if envelope.Type == "" {
		return nil, fmt.Errorf("message envelope missing type field")
	}

	factory, ok := GetMessageFactory(envelope.Type)
	if !ok {
		return nil, fmt.Errorf("unknown message type: %s", envelope.Type)
	}

	msg := factory()
	if err := json.Unmarshal(envelope.Value, msg); err != nil {
		return nil, fmt.Errorf("unmarshal message value for type %s: %w", envelope.Type, err)
	}

	return msg, nil
}
