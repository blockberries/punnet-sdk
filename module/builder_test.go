package module

import (
	"context"
	"testing"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestNewModuleBuilder(t *testing.T) {
	builder := NewModuleBuilder("test")
	require.NotNil(t, builder)
	require.NotNil(t, builder.module)
	require.Equal(t, "test", builder.module.name)
	require.Nil(t, builder.err)
}

func TestNewModuleBuilder_EmptyName(t *testing.T) {
	builder := NewModuleBuilder("")
	require.NotNil(t, builder)
	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "empty")
}

func TestModuleBuilder_WithDependency(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependency("dep1").
		WithDependency("dep2")

	require.NoError(t, builder.err)
	require.Len(t, builder.module.dependencies, 2)
	require.Equal(t, "dep1", builder.module.dependencies[0])
	require.Equal(t, "dep2", builder.module.dependencies[1])
}

func TestModuleBuilder_WithDependency_Empty(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependency("")

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "empty")
}

func TestModuleBuilder_WithDependency_Self(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependency("test")

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "depend on itself")
}

func TestModuleBuilder_WithDependency_Duplicate(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependency("dep1").
		WithDependency("dep1")

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "duplicate")
}

func TestModuleBuilder_WithDependencies(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependencies("dep1", "dep2", "dep3")

	require.NoError(t, builder.err)
	require.Len(t, builder.module.dependencies, 3)
}

func TestModuleBuilder_WithDependencies_Error(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependencies("dep1", "", "dep3")

	require.Error(t, builder.err)
}

func TestModuleBuilder_WithMsgHandler(t *testing.T) {
	handler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithMsgHandler("/test.v1.Msg1", handler).
		WithMsgHandler("/test.v1.Msg2", handler)

	require.NoError(t, builder.err)
	require.Len(t, builder.module.msgHandlers, 2)
}

func TestModuleBuilder_WithMsgHandler_EmptyType(t *testing.T) {
	handler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithMsgHandler("", handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "empty")
}

func TestModuleBuilder_WithMsgHandler_NilHandler(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithMsgHandler("/test.v1.Msg", nil)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "nil")
}

func TestModuleBuilder_WithMsgHandler_Duplicate(t *testing.T) {
	handler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithMsgHandler("/test.v1.Msg", handler).
		WithMsgHandler("/test.v1.Msg", handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "duplicate")
}

func TestModuleBuilder_WithMsgHandlers(t *testing.T) {
	handler1 := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}
	handler2 := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	handlers := map[string]MsgHandler{
		"/test.v1.Msg1": handler1,
		"/test.v1.Msg2": handler2,
	}

	builder := NewModuleBuilder("test").
		WithMsgHandlers(handlers)

	require.NoError(t, builder.err)
	require.Len(t, builder.module.msgHandlers, 2)
}

func TestModuleBuilder_WithMsgHandlers_Nil(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithMsgHandlers(nil)

	require.NoError(t, builder.err)
	require.Empty(t, builder.module.msgHandlers)
}

func TestModuleBuilder_WithQueryHandler(t *testing.T) {
	handler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithQueryHandler("/test/query1", handler).
		WithQueryHandler("/test/query2", handler)

	require.NoError(t, builder.err)
	require.Len(t, builder.module.queryHandlers, 2)
}

func TestModuleBuilder_WithQueryHandler_EmptyPath(t *testing.T) {
	handler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithQueryHandler("", handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "empty")
}

func TestModuleBuilder_WithQueryHandler_NilHandler(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithQueryHandler("/test/query", nil)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "nil")
}

func TestModuleBuilder_WithQueryHandler_Duplicate(t *testing.T) {
	handler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithQueryHandler("/test/query", handler).
		WithQueryHandler("/test/query", handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "duplicate")
}

func TestModuleBuilder_WithQueryHandlers(t *testing.T) {
	handler1 := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}
	handler2 := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	handlers := map[string]QueryHandler{
		"/test/query1": handler1,
		"/test/query2": handler2,
	}

	builder := NewModuleBuilder("test").
		WithQueryHandlers(handlers)

	require.NoError(t, builder.err)
	require.Len(t, builder.module.queryHandlers, 2)
}

func TestModuleBuilder_WithQueryHandlers_Nil(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithQueryHandlers(nil)

	require.NoError(t, builder.err)
	require.Empty(t, builder.module.queryHandlers)
}

func TestModuleBuilder_WithBeginBlocker(t *testing.T) {
	handler := func(ctx *runtime.Context) ([]effects.Effect, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithBeginBlocker(handler)

	require.NoError(t, builder.err)
	require.NotNil(t, builder.module.beginBlock)
}

func TestModuleBuilder_WithBeginBlocker_Nil(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithBeginBlocker(nil)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "nil")
}

func TestModuleBuilder_WithBeginBlocker_Duplicate(t *testing.T) {
	handler := func(ctx *runtime.Context) ([]effects.Effect, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithBeginBlocker(handler).
		WithBeginBlocker(handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "already set")
}

func TestModuleBuilder_WithEndBlocker(t *testing.T) {
	handler := func(ctx *runtime.Context) ([]effects.Effect, []types.ValidatorUpdate, error) {
		return nil, nil, nil
	}

	builder := NewModuleBuilder("test").
		WithEndBlocker(handler)

	require.NoError(t, builder.err)
	require.NotNil(t, builder.module.endBlock)
}

func TestModuleBuilder_WithEndBlocker_Nil(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithEndBlocker(nil)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "nil")
}

func TestModuleBuilder_WithEndBlocker_Duplicate(t *testing.T) {
	handler := func(ctx *runtime.Context) ([]effects.Effect, []types.ValidatorUpdate, error) {
		return nil, nil, nil
	}

	builder := NewModuleBuilder("test").
		WithEndBlocker(handler).
		WithEndBlocker(handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "already set")
}

func TestModuleBuilder_WithInitGenesis(t *testing.T) {
	handler := func(ctx *runtime.Context, data []byte) error {
		return nil
	}

	builder := NewModuleBuilder("test").
		WithInitGenesis(handler)

	require.NoError(t, builder.err)
	require.NotNil(t, builder.module.initGenesis)
}

func TestModuleBuilder_WithInitGenesis_Nil(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithInitGenesis(nil)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "nil")
}

func TestModuleBuilder_WithInitGenesis_Duplicate(t *testing.T) {
	handler := func(ctx *runtime.Context, data []byte) error {
		return nil
	}

	builder := NewModuleBuilder("test").
		WithInitGenesis(handler).
		WithInitGenesis(handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "already set")
}

func TestModuleBuilder_WithExportGenesis(t *testing.T) {
	handler := func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithExportGenesis(handler)

	require.NoError(t, builder.err)
	require.NotNil(t, builder.module.exportGenesis)
}

func TestModuleBuilder_WithExportGenesis_Nil(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithExportGenesis(nil)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "nil")
}

func TestModuleBuilder_WithExportGenesis_Duplicate(t *testing.T) {
	handler := func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithExportGenesis(handler).
		WithExportGenesis(handler)

	require.Error(t, builder.err)
	require.Contains(t, builder.err.Error(), "already set")
}

func TestModuleBuilder_Build(t *testing.T) {
	handler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	module, err := NewModuleBuilder("test").
		WithDependency("dep1").
		WithMsgHandler("/test.v1.Msg", handler).
		Build()

	require.NoError(t, err)
	require.NotNil(t, module)
	require.Equal(t, "test", module.Name())
}

func TestModuleBuilder_Build_WithError(t *testing.T) {
	builder := NewModuleBuilder("test").
		WithDependency("")

	module, err := builder.Build()
	require.Error(t, err)
	require.Nil(t, module)
}

func TestModuleBuilder_Build_Nil(t *testing.T) {
	var builder *ModuleBuilder
	module, err := builder.Build()
	require.Error(t, err)
	require.Nil(t, module)
}

func TestModuleBuilder_FluentChaining(t *testing.T) {
	msgHandler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	queryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	beginBlock := func(ctx *runtime.Context) ([]effects.Effect, error) {
		return nil, nil
	}

	endBlock := func(ctx *runtime.Context) ([]effects.Effect, []types.ValidatorUpdate, error) {
		return nil, nil, nil
	}

	initGenesis := func(ctx *runtime.Context, data []byte) error {
		return nil
	}

	exportGenesis := func(ctx context.Context) ([]byte, error) {
		return nil, nil
	}

	// Test complete fluent chain
	module, err := NewModuleBuilder("test").
		WithDependencies("dep1", "dep2").
		WithMsgHandler("/test.v1.Msg1", msgHandler).
		WithMsgHandler("/test.v1.Msg2", msgHandler).
		WithQueryHandler("/test/query", queryHandler).
		WithBeginBlocker(beginBlock).
		WithEndBlocker(endBlock).
		WithInitGenesis(initGenesis).
		WithExportGenesis(exportGenesis).
		Build()

	require.NoError(t, err)
	require.NotNil(t, module)
	require.Equal(t, "test", module.Name())
	require.Len(t, module.Dependencies(), 2)
	require.Len(t, module.RegisterMsgHandlers(), 2)
	require.Len(t, module.RegisterQueryHandlers(), 1)
	require.NotNil(t, module.BeginBlock())
	require.NotNil(t, module.EndBlock())
	require.NotNil(t, module.InitGenesis())
	require.NotNil(t, module.ExportGenesis())
}

func TestModuleBuilder_NilSafety(t *testing.T) {
	var builder *ModuleBuilder

	// All methods should handle nil safely and return nil
	require.Nil(t, builder.WithDependency("dep"))
	require.Nil(t, builder.WithDependencies("dep1", "dep2"))
	require.Nil(t, builder.WithMsgHandler("/test.v1.Msg", nil))
	require.Nil(t, builder.WithMsgHandlers(nil))
	require.Nil(t, builder.WithQueryHandler("/test/query", nil))
	require.Nil(t, builder.WithQueryHandlers(nil))
	require.Nil(t, builder.WithBeginBlocker(nil))
	require.Nil(t, builder.WithEndBlocker(nil))
	require.Nil(t, builder.WithInitGenesis(nil))
	require.Nil(t, builder.WithExportGenesis(nil))
}

func TestModuleBuilder_ErrorPropagation(t *testing.T) {
	// Once an error occurs, all subsequent operations should preserve the error
	handler := func(ctx *runtime.Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	builder := NewModuleBuilder("test").
		WithDependency(""). // This will set an error
		WithMsgHandler("/test.v1.Msg", handler) // This should be a no-op due to error

	require.Error(t, builder.err)
	require.Empty(t, builder.module.msgHandlers) // Handler should not have been added
}
