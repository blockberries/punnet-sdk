package effects

import (
	"bytes"
	"testing"
)

func TestDeleteEffect_Type(t *testing.T) {
	effect := DeleteEffect[string]{
		Store:    "test",
		StoreKey: []byte("key"),
	}

	if got := effect.Type(); got != EffectTypeDelete {
		t.Errorf("Type() = %v, want %v", got, EffectTypeDelete)
	}
}

func TestDeleteEffect_Validate(t *testing.T) {
	tests := []struct {
		name    string
		effect  DeleteEffect[string]
		wantErr bool
	}{
		{
			name: "valid delete effect",
			effect: DeleteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key"),
			},
			wantErr: false,
		},
		{
			name: "empty store name",
			effect: DeleteEffect[string]{
				Store:    "",
				StoreKey: []byte("key"),
			},
			wantErr: true,
		},
		{
			name: "empty key",
			effect: DeleteEffect[string]{
				Store:    "test",
				StoreKey: []byte{},
			},
			wantErr: true,
		},
		{
			name: "nil key",
			effect: DeleteEffect[string]{
				Store:    "test",
				StoreKey: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.effect.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteEffect_Dependencies(t *testing.T) {
	effect := DeleteEffect[string]{
		Store:    "account",
		StoreKey: []byte("alice"),
	}

	deps := effect.Dependencies()
	if len(deps) != 1 {
		t.Fatalf("Dependencies() returned %d dependencies, want 1", len(deps))
	}

	dep := deps[0]
	if dep.Type != DependencyTypeGeneric {
		t.Errorf("Dependency type = %v, want %v", dep.Type, DependencyTypeGeneric)
	}
	if dep.ReadOnly {
		t.Errorf("Dependency ReadOnly = true, want false")
	}

	expectedKey := []byte("account/alice")
	if !bytes.Equal(dep.Key, expectedKey) {
		t.Errorf("Dependency Key = %v, want %v", dep.Key, expectedKey)
	}
}

func TestDeleteEffect_Key(t *testing.T) {
	t.Run("simple key", func(t *testing.T) {
		effect := DeleteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
		}
		expected := []byte("test/key")
		key := effect.Key()
		if !bytes.Equal(key, expected) {
			t.Errorf("Key() = %v, want %v", key, expected)
		}
	})

	t.Run("nested store", func(t *testing.T) {
		effect := DeleteEffect[string]{
			Store:    "module.submodule",
			StoreKey: []byte("nested/key"),
		}
		expected := []byte("module.submodule/nested/key")
		key := effect.Key()
		if !bytes.Equal(key, expected) {
			t.Errorf("Key() = %v, want %v", key, expected)
		}
	})

	t.Run("binary key", func(t *testing.T) {
		effect := DeleteEffect[[]byte]{
			Store:    "data",
			StoreKey: []byte{0x01, 0x02, 0x03},
		}
		expected := append([]byte("data/"), []byte{0x01, 0x02, 0x03}...)
		key := effect.Key()
		if !bytes.Equal(key, expected) {
			t.Errorf("Key() = %v, want %v", key, expected)
		}
	})
}

func TestDeleteEffect_MultipleTypes(t *testing.T) {
	// Test that DeleteEffect works with different generic types
	t.Run("string type", func(t *testing.T) {
		effect := DeleteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
		}
		if err := effect.Validate(); err != nil {
			t.Errorf("Validate() failed for string type: %v", err)
		}
	})

	t.Run("int type", func(t *testing.T) {
		effect := DeleteEffect[int]{
			Store:    "test",
			StoreKey: []byte("key"),
		}
		if err := effect.Validate(); err != nil {
			t.Errorf("Validate() failed for int type: %v", err)
		}
	})

	t.Run("struct type", func(t *testing.T) {
		type TestStruct struct {
			Field string
		}
		effect := DeleteEffect[TestStruct]{
			Store:    "test",
			StoreKey: []byte("key"),
		}
		if err := effect.Validate(); err != nil {
			t.Errorf("Validate() failed for struct type: %v", err)
		}
	})

	t.Run("pointer type", func(t *testing.T) {
		type TestStruct struct {
			Field string
		}
		effect := DeleteEffect[*TestStruct]{
			Store:    "test",
			StoreKey: []byte("key"),
		}
		if err := effect.Validate(); err != nil {
			t.Errorf("Validate() failed for pointer type: %v", err)
		}
	})
}

func TestDeleteEffect_Concurrent(t *testing.T) {
	// Test that multiple DeleteEffects can be created and validated concurrently
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			effect := DeleteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key"),
			}
			if err := effect.Validate(); err != nil {
				errChan <- err
			}
			_ = effect.Type()
			_ = effect.Key()
			_ = effect.Dependencies()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for errors
	close(errChan)
	for err := range errChan {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestDeleteEffect_KeyImmutability(t *testing.T) {
	// Test that modifying the returned key doesn't affect the effect
	effect := DeleteEffect[string]{
		Store:    "test",
		StoreKey: []byte("key"),
	}

	key1 := effect.Key()
	originalLen := len(key1)

	// Attempt to modify the returned key
	key1 = append(key1, []byte("/modified")...)

	// Get the key again
	key2 := effect.Key()

	if len(key2) != originalLen {
		t.Errorf("Key length changed after modification: got %d, want %d", len(key2), originalLen)
	}

	// Verify the effect's key wasn't modified
	if bytes.Equal(key2, key1) {
		t.Error("Effect key was modified by external append operation")
	}
}
