package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/require"
)

// For tests, we need a concrete module implementation
// We'll define it inline to avoid circular imports
type testModule struct {
	name          string
	msgHandlers   map[string]MsgHandler
	queryHandlers map[string]QueryHandler
}

func (m *testModule) Name() string                                   { return m.name }
func (m *testModule) RegisterMsgHandlers() map[string]MsgHandler     { return m.msgHandlers }
func (m *testModule) RegisterQueryHandlers() map[string]QueryHandler { return m.queryHandlers }
func (m *testModule) BeginBlock() BeginBlocker                       { return nil }
func (m *testModule) EndBlock() EndBlocker                           { return nil }
func (m *testModule) InitGenesis() InitGenesis                       { return nil }
func (m *testModule) ExportGenesis() ExportGenesis                   { return nil }

// mockMessage implements types.Message for testing
type mockMessage struct {
	msgType string
	signers []types.AccountName
}

func (m mockMessage) Type() string {
	return m.msgType
}

func (m mockMessage) ValidateBasic() error {
	return nil
}

func (m mockMessage) GetSigners() []types.AccountName {
	return m.signers
}

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	require.NotNil(t, r)
	require.Equal(t, 0, r.MsgHandlerCount())
	require.Equal(t, 0, r.QueryHandlerCount())
	require.Equal(t, 0, r.ModuleCount())
}

func TestRouter_RegisterModule(t *testing.T) {
	r := NewRouter()

	msgHandler := func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	queryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	mod := &testModule{
		name: "test",
		msgHandlers: map[string]MsgHandler{
			"/test.v1.Msg": msgHandler,
		},
		queryHandlers: map[string]QueryHandler{
			"/test/query": queryHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	require.Equal(t, 1, r.MsgHandlerCount())
	require.Equal(t, 1, r.QueryHandlerCount())
	require.Equal(t, 1, r.ModuleCount())
}

func TestRouter_RegisterModule_Nil(t *testing.T) {
	r := NewRouter()
	err := r.RegisterModule(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

func TestRouter_RegisterModule_DuplicateMsgHandler(t *testing.T) {
	r := NewRouter()

	msgHandler := func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	mod1 := &testModule{
		name: "mod1",
		msgHandlers: map[string]MsgHandler{
			"/test.v1.Msg": msgHandler,
		},
	}

	mod2 := &testModule{
		name: "mod2",
		msgHandlers: map[string]MsgHandler{
			"/test.v1.Msg": msgHandler,
		},
	}

	err := r.RegisterModule(mod1)
	require.NoError(t, err)

	// Should fail because of duplicate message type
	err = r.RegisterModule(mod2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestRouter_RegisterModule_DuplicateQueryHandler(t *testing.T) {
	r := NewRouter()

	queryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	mod1 := &testModule{
		name: "mod1",
		queryHandlers: map[string]QueryHandler{
			"/test/query": queryHandler,
		},
	}

	mod2 := &testModule{
		name: "mod2",
		queryHandlers: map[string]QueryHandler{
			"/test/query": queryHandler,
		},
	}

	err := r.RegisterModule(mod1)
	require.NoError(t, err)

	// Should fail because of duplicate query path
	err = r.RegisterModule(mod2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestRouter_RouteMsg(t *testing.T) {
	r := NewRouter()

	called := false
	msgHandler := func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
		called = true
		return []effects.Effect{
			effects.NewEventEffect("test", map[string][]byte{"key": []byte("value")}),
		}, nil
	}

	mod := &testModule{
		name: "test",
		msgHandlers: map[string]MsgHandler{
			"/test.v1.Msg": msgHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	// Create context
	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	ctx, err := NewContext(context.Background(), header, types.AccountName("alice"))
	require.NoError(t, err)

	// Create message
	msg := mockMessage{
		msgType: "/test.v1.Msg",
		signers: []types.AccountName{"alice"},
	}

	// Route message
	effs, err := r.RouteMsg(ctx, msg)
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, effs, 1)
}

func TestRouter_RouteMsg_NotFound(t *testing.T) {
	r := NewRouter()

	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	ctx, err := NewContext(context.Background(), header, types.AccountName("alice"))
	require.NoError(t, err)

	msg := mockMessage{
		msgType: "/unknown.v1.Msg",
		signers: []types.AccountName{"alice"},
	}

	_, err = r.RouteMsg(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "handler not found")
}

func TestRouter_RouteMsg_NilContext(t *testing.T) {
	r := NewRouter()

	msg := mockMessage{
		msgType: "/test.v1.Msg",
		signers: []types.AccountName{"alice"},
	}

	_, err := r.RouteMsg(nil, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context")
}

func TestRouter_RouteMsg_NilMessage(t *testing.T) {
	r := NewRouter()

	header := NewBlockHeader(100, time.Now(), "test-chain", []byte("proposer"))
	ctx, err := NewContext(context.Background(), header, types.AccountName("alice"))
	require.NoError(t, err)

	_, err = r.RouteMsg(ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message")
}

func TestRouter_RouteQuery(t *testing.T) {
	r := NewRouter()

	called := false
	queryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		called = true
		return []byte("result"), nil
	}

	mod := &testModule{
		name: "test",
		queryHandlers: map[string]QueryHandler{
			"/test/query": queryHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	// Route query
	result, err := r.RouteQuery(context.Background(), "/test/query", []byte("data"))
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []byte("result"), result)
}

func TestRouter_RouteQuery_NotFound(t *testing.T) {
	r := NewRouter()

	_, err := r.RouteQuery(context.Background(), "/unknown/query", []byte("data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "query handler not found")
}

func TestRouter_RouteQuery_NilContext(t *testing.T) {
	r := NewRouter()

	_, err := r.RouteQuery(nil, "/test/query", []byte("data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "context")
}

func TestRouter_RouteQuery_EmptyPath(t *testing.T) {
	r := NewRouter()

	_, err := r.RouteQuery(context.Background(), "", []byte("data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "path")
}

func TestRouter_HasMsgHandler(t *testing.T) {
	r := NewRouter()

	msgHandler := func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	mod := &testModule{
		name: "test",
		msgHandlers: map[string]MsgHandler{
			"/test.v1.Msg": msgHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	require.True(t, r.HasMsgHandler("/test.v1.Msg"))
	require.False(t, r.HasMsgHandler("/unknown.v1.Msg"))
}

func TestRouter_HasMsgHandler_Nil(t *testing.T) {
	var r *Router
	require.False(t, r.HasMsgHandler("/test.v1.Msg"))
}

func TestRouter_HasQueryHandler(t *testing.T) {
	r := NewRouter()

	queryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	mod := &testModule{
		name: "test",
		queryHandlers: map[string]QueryHandler{
			"/test/query": queryHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	require.True(t, r.HasQueryHandler("/test/query"))
	require.False(t, r.HasQueryHandler("/unknown/query"))
}

func TestRouter_HasQueryHandler_Nil(t *testing.T) {
	var r *Router
	require.False(t, r.HasQueryHandler("/test/query"))
}

func TestRouter_Modules(t *testing.T) {
	r := NewRouter()

	mod1 := &testModule{name: "mod1"}
	mod2 := &testModule{name: "mod2"}

	err := r.RegisterModule(mod1)
	require.NoError(t, err)

	err = r.RegisterModule(mod2)
	require.NoError(t, err)

	modules := r.Modules()
	require.Len(t, modules, 2)

	// Verify defensive copy
	modules[0] = nil
	require.Equal(t, 2, r.ModuleCount())
}

func TestRouter_Modules_Nil(t *testing.T) {
	var r *Router
	require.Nil(t, r.Modules())
}

func TestRouter_MsgTypes(t *testing.T) {
	r := NewRouter()

	msgHandler := func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	mod := &testModule{
		name: "test",
		msgHandlers: map[string]MsgHandler{
			"/test.v1.Msg2": msgHandler,
			"/test.v1.Msg1": msgHandler,
			"/test.v1.Msg3": msgHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	types := r.MsgTypes()
	require.Len(t, types, 3)

	// Verify sorted order
	require.Equal(t, []string{"/test.v1.Msg1", "/test.v1.Msg2", "/test.v1.Msg3"}, types)
}

func TestRouter_MsgTypes_Nil(t *testing.T) {
	var r *Router
	require.Nil(t, r.MsgTypes())
}

func TestRouter_QueryPaths(t *testing.T) {
	r := NewRouter()

	queryHandler := func(ctx context.Context, path string, data []byte) ([]byte, error) {
		return nil, nil
	}

	mod := &testModule{
		name: "test",
		queryHandlers: map[string]QueryHandler{
			"/test/query2": queryHandler,
			"/test/query1": queryHandler,
			"/test/query3": queryHandler,
		},
	}

	err := r.RegisterModule(mod)
	require.NoError(t, err)

	paths := r.QueryPaths()
	require.Len(t, paths, 3)

	// Verify sorted order
	require.Equal(t, []string{"/test/query1", "/test/query2", "/test/query3"}, paths)
}

func TestRouter_QueryPaths_Nil(t *testing.T) {
	var r *Router
	require.Nil(t, r.QueryPaths())
}

func TestRouter_ThreadSafety(t *testing.T) {
	r := NewRouter()

	msgHandler := func(ctx *Context, msg types.Message) ([]effects.Effect, error) {
		return nil, nil
	}

	mod1 := &testModule{
		name: "mod1",
		msgHandlers: map[string]MsgHandler{
			"/mod1.v1.Msg": msgHandler,
		},
	}

	mod2 := &testModule{
		name: "mod2",
		msgHandlers: map[string]MsgHandler{
			"/mod2.v1.Msg": msgHandler,
		},
	}

	// Register modules concurrently
	done := make(chan bool, 2)

	go func() {
		r.RegisterModule(mod1)
		done <- true
	}()

	go func() {
		r.RegisterModule(mod2)
		done <- true
	}()

	<-done
	<-done

	require.Equal(t, 2, r.MsgHandlerCount())
	require.Equal(t, 2, r.ModuleCount())
}

func TestRouter_NilSafety(t *testing.T) {
	var r *Router

	// All methods should handle nil safely
	err := r.RegisterModule(nil)
	require.Error(t, err)

	_, err = r.RouteMsg(nil, nil)
	require.Error(t, err)

	_, err = r.RouteQuery(nil, "", nil)
	require.Error(t, err)

	require.False(t, r.HasMsgHandler("/test.v1.Msg"))
	require.False(t, r.HasQueryHandler("/test/query"))
	require.Equal(t, 0, r.MsgHandlerCount())
	require.Equal(t, 0, r.QueryHandlerCount())
	require.Equal(t, 0, r.ModuleCount())
	require.Nil(t, r.Modules())
	require.Nil(t, r.MsgTypes())
	require.Nil(t, r.QueryPaths())
}
