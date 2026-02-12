package runtime

import (
	"context"
	"sync"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// BAPIMsgHandler handles a message and returns effects.
// This is the new handler signature for BAPI-based applications.
type BAPIMsgHandler func(ctx context.Context, txCtx *BAPITxContext, msg ptypes.Message) ([]effects.Effect, error)

// BAPIQueryHandler handles a query and returns the result.
type BAPIQueryHandler func(ctx context.Context, data []byte, height int64) ([]byte, error)

// BAPIBeginBlocker is called at the beginning of each block.
type BAPIBeginBlocker func(ctx context.Context, blockCtx *BAPIBlockContext) ([]effects.Effect, error)

// BAPIEndBlocker is called at the end of each block.
type BAPIEndBlocker func(ctx context.Context, blockCtx *BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error)

// BAPIInitGenesis initializes the module's state from genesis data.
type BAPIInitGenesis func(ctx context.Context, data []byte) error

// BAPIExportGenesis exports the module's state for genesis.
type BAPIExportGenesis func(ctx context.Context) ([]byte, error)

// BAPIModule is the interface for modules in the new BAPI-based application.
type BAPIModule interface {
	// Name returns the module's unique name.
	Name() string

	// RegisterMsgHandlers returns message handlers keyed by message type.
	RegisterMsgHandlers() map[string]BAPIMsgHandler

	// RegisterQueryHandlers returns query handlers keyed by query path.
	RegisterQueryHandlers() map[string]BAPIQueryHandler
}

// BAPIBlockProcessor is implemented by modules that need block lifecycle hooks.
type BAPIBlockProcessor interface {
	BAPIModule
	BeginBlock(ctx context.Context, blockCtx *BAPIBlockContext) ([]effects.Effect, error)
	EndBlock(ctx context.Context, blockCtx *BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error)
}

// BAPIGenesisInitializer is implemented by modules that need genesis initialization.
type BAPIGenesisInitializer interface {
	BAPIModule
	InitGenesis(ctx context.Context, data []byte) error
}

// BAPIGenesisExporter is implemented by modules that can export genesis state.
type BAPIGenesisExporter interface {
	BAPIModule
	ExportGenesis(ctx context.Context) ([]byte, error)
}

// BAPIRouter routes messages and queries to handlers.
type BAPIRouter struct {
	mu            sync.RWMutex
	msgHandlers   map[string]BAPIMsgHandler
	queryHandlers map[string]BAPIQueryHandler
	modules       []BAPIModule
}

// NewBAPIRouter creates a new BAPI router.
func NewBAPIRouter() *BAPIRouter {
	return &BAPIRouter{
		msgHandlers:   make(map[string]BAPIMsgHandler),
		queryHandlers: make(map[string]BAPIQueryHandler),
		modules:       make([]BAPIModule, 0),
	}
}

// RegisterModule registers a module's handlers with the router.
func (r *BAPIRouter) RegisterModule(mod BAPIModule) error {
	if mod == nil {
		return ErrModuleNil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Register message handlers
	for msgType, handler := range mod.RegisterMsgHandlers() {
		if msgType == "" {
			continue
		}
		if handler == nil {
			continue
		}
		r.msgHandlers[msgType] = handler
	}

	// Register query handlers
	for path, handler := range mod.RegisterQueryHandlers() {
		if path == "" {
			continue
		}
		if handler == nil {
			continue
		}
		r.queryHandlers[path] = handler
	}

	r.modules = append(r.modules, mod)

	return nil
}

// GetMsgHandler returns the handler for a message type.
func (r *BAPIRouter) GetMsgHandler(msgType string) BAPIMsgHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.msgHandlers[msgType]
}

// GetQueryHandler returns the handler for a query path.
func (r *BAPIRouter) GetQueryHandler(path string) BAPIQueryHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try exact match first
	if handler, ok := r.queryHandlers[path]; ok {
		return handler
	}

	// Try prefix matching
	for registeredPath, handler := range r.queryHandlers {
		if len(path) >= len(registeredPath) && path[:len(registeredPath)] == registeredPath {
			return handler
		}
	}

	return nil
}

// Modules returns all registered modules.
func (r *BAPIRouter) Modules() []BAPIModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return defensive copy
	modules := make([]BAPIModule, len(r.modules))
	copy(modules, r.modules)
	return modules
}

// ErrModuleNil is returned when a module is nil.
var ErrModuleNil = ErrApplicationNil
