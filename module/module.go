package module

import (
	"errors"
	"fmt"
)

var (
	// ErrModuleNil is returned when a module is nil
	ErrModuleNil = errors.New("module is nil")

	// ErrModuleNameEmpty is returned when a module name is empty
	ErrModuleNameEmpty = errors.New("module name is empty")

	// ErrHandlerNotFound is returned when a message handler is not found
	ErrHandlerNotFound = errors.New("handler not found")

	// ErrQueryNotFound is returned when a query handler is not found
	ErrQueryNotFound = errors.New("query handler not found")

	// ErrInvalidDependency is returned when a dependency is invalid
	ErrInvalidDependency = errors.New("invalid dependency")

	// ErrModuleNotFound is returned when a module is not registered
	ErrModuleNotFound = errors.New("module not found")

	// ErrDuplicateModule is returned when a module is already registered
	ErrDuplicateModule = errors.New("module already registered")
)

// Module is the interface that all modules must implement
// Modules are the building blocks of a Punnet SDK application
type Module interface {
	// Name returns the module's name (must be unique in an application)
	Name() string

	// Dependencies returns the names of modules this module depends on
	// These modules must be registered before this module
	Dependencies() []string

	// RegisterMsgHandlers registers message type handlers
	// Returns a map of message type to handler function
	RegisterMsgHandlers() map[string]MsgHandler

	// RegisterQueryHandlers registers query path handlers
	// Returns a map of query path to handler function
	RegisterQueryHandlers() map[string]QueryHandler

	// BeginBlock is called at the start of each block (optional)
	// Return nil if the module doesn't need to run logic at block start
	BeginBlock() BeginBlocker

	// EndBlock is called at the end of each block (optional)
	// Return nil if the module doesn't need to run logic at block end
	EndBlock() EndBlocker

	// InitGenesis initializes module state from genesis (optional)
	// Return nil if the module doesn't need genesis initialization
	InitGenesis() InitGenesis

	// ExportGenesis exports module state for genesis (optional)
	// Return nil if the module doesn't need to export state
	ExportGenesis() ExportGenesis
}

// ValidateModule performs validation on a module
func ValidateModule(m Module) error {
	if m == nil {
		return ErrModuleNil
	}

	name := m.Name()
	if name == "" {
		return ErrModuleNameEmpty
	}

	// Validate dependencies
	deps := m.Dependencies()
	if deps != nil {
		// Check for duplicate dependencies
		seen := make(map[string]bool)
		for _, dep := range deps {
			if dep == "" {
				return fmt.Errorf("%w: empty dependency name", ErrInvalidDependency)
			}
			if dep == name {
				return fmt.Errorf("%w: module cannot depend on itself", ErrInvalidDependency)
			}
			if seen[dep] {
				return fmt.Errorf("%w: duplicate dependency %s", ErrInvalidDependency, dep)
			}
			seen[dep] = true
		}
	}

	// Validate message handlers
	msgHandlers := m.RegisterMsgHandlers()
	if msgHandlers != nil {
		for msgType, handler := range msgHandlers {
			if msgType == "" {
				return fmt.Errorf("empty message type in handler map")
			}
			if handler == nil {
				return fmt.Errorf("nil handler for message type %s", msgType)
			}
		}
	}

	// Validate query handlers
	queryHandlers := m.RegisterQueryHandlers()
	if queryHandlers != nil {
		for path, handler := range queryHandlers {
			if path == "" {
				return fmt.Errorf("empty path in query handler map")
			}
			if handler == nil {
				return fmt.Errorf("nil handler for query path %s", path)
			}
		}
	}

	return nil
}

// baseModule is a minimal implementation of Module for embedding
type baseModule struct {
	name         string
	dependencies []string
	msgHandlers  map[string]MsgHandler
	queryHandlers map[string]QueryHandler
	beginBlock   BeginBlocker
	endBlock     EndBlocker
	initGenesis  InitGenesis
	exportGenesis ExportGenesis
}

// Name returns the module name
func (m *baseModule) Name() string {
	if m == nil {
		return ""
	}
	return m.name
}

// Dependencies returns the module dependencies
func (m *baseModule) Dependencies() []string {
	if m == nil || m.dependencies == nil {
		return nil
	}

	// Return defensive copy
	deps := make([]string, len(m.dependencies))
	copy(deps, m.dependencies)
	return deps
}

// RegisterMsgHandlers returns the message handlers
func (m *baseModule) RegisterMsgHandlers() map[string]MsgHandler {
	if m == nil || m.msgHandlers == nil {
		return nil
	}

	// Return defensive copy
	handlers := make(map[string]MsgHandler, len(m.msgHandlers))
	for k, v := range m.msgHandlers {
		handlers[k] = v
	}
	return handlers
}

// RegisterQueryHandlers returns the query handlers
func (m *baseModule) RegisterQueryHandlers() map[string]QueryHandler {
	if m == nil || m.queryHandlers == nil {
		return nil
	}

	// Return defensive copy
	handlers := make(map[string]QueryHandler, len(m.queryHandlers))
	for k, v := range m.queryHandlers {
		handlers[k] = v
	}
	return handlers
}

// BeginBlock returns the begin block handler
func (m *baseModule) BeginBlock() BeginBlocker {
	if m == nil {
		return nil
	}
	return m.beginBlock
}

// EndBlock returns the end block handler
func (m *baseModule) EndBlock() EndBlocker {
	if m == nil {
		return nil
	}
	return m.endBlock
}

// InitGenesis returns the init genesis handler
func (m *baseModule) InitGenesis() InitGenesis {
	if m == nil {
		return nil
	}
	return m.initGenesis
}

// ExportGenesis returns the export genesis handler
func (m *baseModule) ExportGenesis() ExportGenesis {
	if m == nil {
		return nil
	}
	return m.exportGenesis
}
