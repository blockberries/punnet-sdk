package module

import (
	"context"
	"testing"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/require"
)

// mockModule implements Module for testing
// nolint:unused // Reserved for future tests requiring a mock module
type mockModule struct {
	baseModule
}

func TestValidateModule(t *testing.T) {
	validHandler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	validQueryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	tests := []struct {
		name    string
		module  Module
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil module",
			module:  nil,
			wantErr: true,
			errMsg:  "module is nil",
		},
		{
			name: "empty name",
			module: &baseModule{
				name: "",
			},
			wantErr: true,
			errMsg:  "module name is empty",
		},
		{
			name: "valid module",
			module: &baseModule{
				name:         "test",
				dependencies: []string{"dep1", "dep2"},
				msgHandlers: map[string]MsgHandler{
					"/test.v1.Msg": validHandler,
				},
				queryHandlers: map[string]QueryHandler{
					"/test/query": validQueryHandler,
				},
			},
			wantErr: false,
		},
		{
			name: "empty dependency",
			module: &baseModule{
				name:         "test",
				dependencies: []string{""},
			},
			wantErr: true,
			errMsg:  "empty dependency",
		},
		{
			name: "self dependency",
			module: &baseModule{
				name:         "test",
				dependencies: []string{"test"},
			},
			wantErr: true,
			errMsg:  "depend on itself",
		},
		{
			name: "duplicate dependency",
			module: &baseModule{
				name:         "test",
				dependencies: []string{"dep1", "dep1"},
			},
			wantErr: true,
			errMsg:  "duplicate dependency",
		},
		{
			name: "empty message type",
			module: &baseModule{
				name: "test",
				msgHandlers: map[string]MsgHandler{
					"": validHandler,
				},
			},
			wantErr: true,
			errMsg:  "empty message type",
		},
		{
			name: "nil message handler",
			module: &baseModule{
				name: "test",
				msgHandlers: map[string]MsgHandler{
					"/test.v1.Msg": nil,
				},
			},
			wantErr: true,
			errMsg:  "nil handler",
		},
		{
			name: "empty query path",
			module: &baseModule{
				name: "test",
				queryHandlers: map[string]QueryHandler{
					"": validQueryHandler,
				},
			},
			wantErr: true,
			errMsg:  "empty path",
		},
		{
			name: "nil query handler",
			module: &baseModule{
				name: "test",
				queryHandlers: map[string]QueryHandler{
					"/test/query": nil,
				},
			},
			wantErr: true,
			errMsg:  "nil handler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModule(tt.module)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBaseModule_Name(t *testing.T) {
	m := &baseModule{name: "test"}
	require.Equal(t, "test", m.Name())

	// Nil safety
	var nilMod *baseModule
	require.Equal(t, "", nilMod.Name())
}

func TestBaseModule_Dependencies(t *testing.T) {
	deps := []string{"dep1", "dep2"}
	m := &baseModule{dependencies: deps}

	returned := m.Dependencies()
	require.Equal(t, deps, returned)

	// Verify defensive copy
	returned[0] = "modified"
	require.Equal(t, "dep1", m.dependencies[0])

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.Dependencies())

	// Nil dependencies
	m2 := &baseModule{dependencies: nil}
	require.Nil(t, m2.Dependencies())
}

func TestBaseModule_RegisterMsgHandlers(t *testing.T) {
	handler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	handlers := map[string]MsgHandler{
		"/test.v1.Msg": handler,
	}

	m := &baseModule{msgHandlers: handlers}
	returned := m.RegisterMsgHandlers()
	require.NotNil(t, returned)
	require.Len(t, returned, 1)
	require.NotNil(t, returned["/test.v1.Msg"])

	// Verify defensive copy
	returned["/new.v1.Msg"] = handler
	require.Len(t, m.msgHandlers, 1)

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.RegisterMsgHandlers())

	// Nil handlers
	m2 := &baseModule{msgHandlers: nil}
	require.Nil(t, m2.RegisterMsgHandlers())
}

func TestBaseModule_RegisterQueryHandlers(t *testing.T) {
	handler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	handlers := map[string]QueryHandler{
		"/test/query": handler,
	}

	m := &baseModule{queryHandlers: handlers}
	returned := m.RegisterQueryHandlers()
	require.NotNil(t, returned)
	require.Len(t, returned, 1)
	require.NotNil(t, returned["/test/query"])

	// Verify defensive copy
	returned["/new/query"] = handler
	require.Len(t, m.queryHandlers, 1)

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.RegisterQueryHandlers())

	// Nil handlers
	m2 := &baseModule{queryHandlers: nil}
	require.Nil(t, m2.RegisterQueryHandlers())
}

func TestBaseModule_BeginBlock(t *testing.T) {
	handler := func(ctx *runtime.Context) ([]effects.Effect, error) {
		return nil, nil
	}

	m := &baseModule{beginBlock: handler}
	require.NotNil(t, m.BeginBlock())

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.BeginBlock())

	// Nil handler
	m2 := &baseModule{beginBlock: nil}
	require.Nil(t, m2.BeginBlock())
}

func TestBaseModule_EndBlock(t *testing.T) {
	handler := func(ctx *runtime.Context) ([]effects.Effect, []types.ValidatorUpdate, error) {
		return nil, nil, nil
	}

	m := &baseModule{endBlock: handler}
	require.NotNil(t, m.EndBlock())

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.EndBlock())

	// Nil handler
	m2 := &baseModule{endBlock: nil}
	require.Nil(t, m2.EndBlock())
}

func TestBaseModule_InitGenesis(t *testing.T) {
	handler := func(ctx *runtime.Context, data []byte) error {
		return nil
	}

	m := &baseModule{initGenesis: handler}
	require.NotNil(t, m.InitGenesis())

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.InitGenesis())

	// Nil handler
	m2 := &baseModule{initGenesis: nil}
	require.Nil(t, m2.InitGenesis())
}

func TestBaseModule_ExportGenesis(t *testing.T) {
	handler := func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}

	m := &baseModule{exportGenesis: handler}
	require.NotNil(t, m.ExportGenesis())

	// Nil safety
	var nilMod *baseModule
	require.Nil(t, nilMod.ExportGenesis())

	// Nil handler
	m2 := &baseModule{exportGenesis: nil}
	require.Nil(t, m2.ExportGenesis())
}
