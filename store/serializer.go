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
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("json marshal failed: %w", err)
	}
	return data, nil
}

// Unmarshal converts JSON bytes to an object
func (s *JSONSerializer[T]) Unmarshal(data []byte) (T, error) {
	var obj T
	if err := json.Unmarshal(data, &obj); err != nil {
		return obj, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return obj, nil
}
