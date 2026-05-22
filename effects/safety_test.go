package effects

import (
	"bytes"
	"testing"
)

// TestSliceAliasing_WriteEffectFullKey verifies fullKey creates defensive copy
func TestSliceAliasing_WriteEffectFullKey(t *testing.T) {
	effect := WriteEffect[string]{
		Store:    "test",
		StoreKey: []byte("key"),
	}

	key1 := effect.Key()
	key2 := effect.Key()

	// Modify key1
	if len(key1) > 0 {
		key1[0] = 'X'
	}

	// key2 should be unaffected
	if bytes.Equal(key1, key2) {
		t.Errorf("fullKey() is not creating defensive copies - slice aliasing detected")
	}

	t.Log("✓ WriteEffect.fullKey() creates defensive copy")
}

// TestSliceAliasing_ReadEffectFullKey verifies fullKey creates defensive copy
func TestSliceAliasing_ReadEffectFullKey(t *testing.T) {
	var dest string
	effect := ReadEffect[string]{
		Store:    "test",
		StoreKey: []byte("key"),
		Dest:     &dest,
	}

	key1 := effect.Key()
	key2 := effect.Key()

	// Modify key1
	if len(key1) > 0 {
		key1[0] = 'X'
	}

	// key2 should be unaffected
	if bytes.Equal(key1, key2) {
		t.Errorf("fullKey() is not creating defensive copies - slice aliasing detected")
	}

	t.Log("✓ ReadEffect.fullKey() creates defensive copy")
}

// TestSliceAliasing_DeleteEffectFullKey verifies fullKey creates defensive copy
func TestSliceAliasing_DeleteEffectFullKey(t *testing.T) {
	effect := DeleteEffect[string]{
		Store:    "test",
		StoreKey: []byte("key"),
	}

	key1 := effect.Key()
	key2 := effect.Key()

	// Modify key1
	if len(key1) > 0 {
		key1[0] = 'X'
	}

	// key2 should be unaffected
	if bytes.Equal(key1, key2) {
		t.Errorf("fullKey() is not creating defensive copies - slice aliasing detected")
	}

	t.Log("✓ DeleteEffect.fullKey() creates defensive copy")
}

// TestMapAliasing_NewEventEffect verifies defensive copy of attributes
func TestMapAliasing_NewEventEffect(t *testing.T) {
	originalAttrs := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
	}

	effect := NewEventEffect("test", originalAttrs)

	// Modify original map
	originalAttrs["key1"] = []byte("corrupted")
	originalAttrs["key3"] = []byte("new")

	// Effect should be unaffected
	if string(effect.Attributes["key1"]) == "corrupted" {
		t.Errorf("NewEventEffect did not create defensive copy of map")
	}
	if _, exists := effect.Attributes["key3"]; exists {
		t.Errorf("NewEventEffect did not create defensive copy - external additions visible")
	}

	t.Log("✓ NewEventEffect creates defensive copy of attributes map")
}

// TestMapAliasing_EventEffectAttributeValues verifies deep copy of attribute values
func TestMapAliasing_EventEffectAttributeValues(t *testing.T) {
	value := []byte("original")
	originalAttrs := map[string][]byte{
		"key": value,
	}

	effect := NewEventEffect("test", originalAttrs)

	// Modify original byte slice
	value[0] = 'X'

	// Effect's value should be unaffected
	if effect.Attributes["key"][0] == 'X' {
		t.Errorf("NewEventEffect did not deep copy attribute byte slices")
	}

	t.Log("✓ NewEventEffect deep copies attribute byte slices")
}

// TestNilCheck_CollectorMethods tests nil checks in Collector
func TestNilCheck_CollectorMethods(t *testing.T) {
	var c *Collector = nil

	// Test Add
	err := c.Add(WriteEffect[string]{Store: "test", StoreKey: []byte("key")})
	if err == nil || err.Error() != "collector is nil" {
		t.Errorf("Collector.Add() should return error for nil receiver, got: %v", err)
	}

	// Test AddMultiple
	err = c.AddMultiple([]Effect{})
	if err == nil || err.Error() != "collector is nil" {
		t.Errorf("Collector.AddMultiple() should return error for nil receiver, got: %v", err)
	}

	// Test Collect (should return nil, not panic)
	result := c.Collect()
	if result != nil {
		t.Errorf("Collector.Collect() should return nil for nil receiver, got: %v", result)
	}

	// Test Count (should return 0, not panic)
	count := c.Count()
	if count != 0 {
		t.Errorf("Collector.Count() should return 0 for nil receiver, got: %d", count)
	}

	// Test Clear (should not panic)
	c.Clear()

	t.Log("✓ All Collector methods handle nil receiver safely")
}

// TestNilCheck_ConflictError tests nil checks in Conflict.Error
func TestNilCheck_ConflictError(t *testing.T) {
	var c *Conflict = nil

	// Should not panic
	msg := c.Error()
	if msg != "nil conflict" {
		t.Errorf("Conflict.Error() should return 'nil conflict' for nil receiver, got: %s", msg)
	}

	// Test with nil effects
	c = &Conflict{
		Type:    ConflictTypeWriteWrite,
		Effect1: nil,
		Effect2: WriteEffect[string]{Store: "test", StoreKey: []byte("key")},
		Key:     []byte("test/key"),
	}

	msg = c.Error()
	if msg == "" {
		t.Errorf("Conflict.Error() should handle nil effects gracefully")
	}

	t.Log("✓ Conflict.Error() handles nil safely")
}

