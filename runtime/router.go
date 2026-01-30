package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/types"
)

var (
	// ErrRouterNil is returned when a router is nil
	ErrRouterNil = fmt.Errorf("router is nil")

	// ErrHandlerNotFound is returned when no handler is found for a message type
	ErrHandlerNotFound = fmt.Errorf("handler not found")

	// ErrQueryHandlerNotFound is returned when no handler is found for a query path
	ErrQueryHandlerNotFound = fmt.Errorf("query handler not found")
)

// Module is the minimal interface required by the router
// This avoids circular imports with the module package
type Module interface {
	// Name returns the module's name
	Name() string

	// RegisterMsgHandlers registers message type handlers
	RegisterMsgHandlers() map[string]MsgHandler

	// RegisterQueryHandlers registers query path handlers
	RegisterQueryHandlers() map[string]QueryHandler

	// BeginBlock is called at the start of each block
	BeginBlock() BeginBlocker

	// EndBlock is called at the end of each block
	EndBlock() EndBlocker

	// InitGenesis initializes module state from genesis
	InitGenesis() InitGenesis

	// ExportGenesis exports module state for genesis
	ExportGenesis() ExportGenesis
}

// Router routes messages and queries to their respective handlers
// It maintains a mapping of message types to handlers and query paths to handlers
type Router struct {
	mu sync.RWMutex

	// msgHandlers maps message type to handler
	msgHandlers map[string]MsgHandler

	// queryHandlers maps query path to handler
	queryHandlers map[string]QueryHandler

	// modules stores registered modules for lifecycle management
	modules []Module
}

// NewRouter creates a new router
func NewRouter() *Router {
	return &Router{
		msgHandlers:   make(map[string]MsgHandler),
		queryHandlers: make(map[string]QueryHandler),
		modules:       make([]Module, 0),
	}
}

// RegisterModule registers all handlers from a module
func (r *Router) RegisterModule(m Module) error {
	if r == nil {
		return ErrRouterNil
	}

	if m == nil {
		return fmt.Errorf("module cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Register message handlers
	msgHandlers := m.RegisterMsgHandlers()
	if msgHandlers != nil {
		for msgType, handler := range msgHandlers {
			if msgType == "" {
				return fmt.Errorf("empty message type in module %s", m.Name())
			}
			if handler == nil {
				return fmt.Errorf("nil handler for message type %s in module %s", msgType, m.Name())
			}

			// Check for duplicate
			if _, exists := r.msgHandlers[msgType]; exists {
				return fmt.Errorf("duplicate handler for message type %s", msgType)
			}

			r.msgHandlers[msgType] = handler
		}
	}

	// Register query handlers
	queryHandlers := m.RegisterQueryHandlers()
	if queryHandlers != nil {
		for path, handler := range queryHandlers {
			if path == "" {
				return fmt.Errorf("empty query path in module %s", m.Name())
			}
			if handler == nil {
				return fmt.Errorf("nil handler for query path %s in module %s", path, m.Name())
			}

			// Check for duplicate
			if _, exists := r.queryHandlers[path]; exists {
				return fmt.Errorf("duplicate handler for query path %s", path)
			}

			r.queryHandlers[path] = handler
		}
	}

	// Store module reference
	r.modules = append(r.modules, m)

	return nil
}

// RouteMsg routes a message to its handler and returns the effects
func (r *Router) RouteMsg(ctx *Context, msg types.Message) ([]effects.Effect, error) {
	if r == nil {
		return nil, ErrRouterNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}

	msgType := msg.Type()
	if msgType == "" {
		return nil, fmt.Errorf("message type cannot be empty")
	}

	r.mu.RLock()
	handler, exists := r.msgHandlers[msgType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrHandlerNotFound, msgType)
	}

	// Call the handler
	return handler(ctx, msg)
}

// RouteQuery routes a query to its handler and returns the result
func (r *Router) RouteQuery(ctx context.Context, path string, data []byte) ([]byte, error) {
	if r == nil {
		return nil, ErrRouterNil
	}

	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if path == "" {
		return nil, fmt.Errorf("query path cannot be empty")
	}

	r.mu.RLock()
	handler, exists := r.queryHandlers[path]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrQueryHandlerNotFound, path)
	}

	// Call the handler
	return handler(ctx, path, data)
}

// HasMsgHandler checks if a handler exists for a message type
func (r *Router) HasMsgHandler(msgType string) bool {
	if r == nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.msgHandlers[msgType]
	return exists
}

// HasQueryHandler checks if a handler exists for a query path
func (r *Router) HasQueryHandler(path string) bool {
	if r == nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.queryHandlers[path]
	return exists
}

// MsgHandlerCount returns the number of registered message handlers
func (r *Router) MsgHandlerCount() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.msgHandlers)
}

// QueryHandlerCount returns the number of registered query handlers
func (r *Router) QueryHandlerCount() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.queryHandlers)
}

// ModuleCount returns the number of registered modules
func (r *Router) ModuleCount() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.modules)
}

// Modules returns all registered modules
func (r *Router) Modules() []Module {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return defensive copy
	modules := make([]Module, len(r.modules))
	copy(modules, r.modules)
	return modules
}

// MsgTypes returns all registered message types (sorted for determinism)
func (r *Router) MsgTypes() []string {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.msgHandlers))
	for msgType := range r.msgHandlers {
		types = append(types, msgType)
	}

	// Sort for deterministic ordering
	sort.Strings(types)
	return types
}

// QueryPaths returns all registered query paths (sorted for determinism)
func (r *Router) QueryPaths() []string {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	paths := make([]string, 0, len(r.queryHandlers))
	for path := range r.queryHandlers {
		paths = append(paths, path)
	}

	// Sort for deterministic ordering
	sort.Strings(paths)
	return paths
}
