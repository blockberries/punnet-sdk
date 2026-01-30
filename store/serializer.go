package store

import (
	"encoding/json"
	"fmt"
)

// JSONSerializer implements Serializer using JSON encoding
type JSONSerializer[T any] struct{}

// NewJSONSerializer creates a new JSON serializer
func NewJSONSerializer[T any]() *JSONSerializer[T] {
	return &JSONSerializer[T]{}
}

// Marshal converts an object to JSON bytes
func (s *JSONSerializer[T]) Marshal(obj T) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("serializer is nil")
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("json marshal failed: %w", err)
	}
	return data, nil
}

// Unmarshal converts JSON bytes to an object
func (s *JSONSerializer[T]) Unmarshal(data []byte) (T, error) {
	var obj T
	if s == nil {
		return obj, fmt.Errorf("serializer is nil")
	}
	if len(data) == 0 {
		return obj, fmt.Errorf("data is empty")
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return obj, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return obj, nil
}
