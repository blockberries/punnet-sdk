package module

import (
	"fmt"
)

// ModuleBuilder provides a fluent API for building modules
// It uses the builder pattern for ergonomic module construction
type ModuleBuilder struct {
	module *baseModule
	err    error // tracks errors during building
}

// NewModuleBuilder creates a new module builder
func NewModuleBuilder(name string) *ModuleBuilder {
	if name == "" {
		return &ModuleBuilder{
			err: fmt.Errorf("module name cannot be empty"),
		}
	}

	return &ModuleBuilder{
		module: &baseModule{
			name:         name,
			dependencies: make([]string, 0),
			msgHandlers:  make(map[string]MsgHandler),
			queryHandlers: make(map[string]QueryHandler),
		},
		err: nil,
	}
}

// WithDependency adds a dependency to the module
// Dependencies are loaded in the order they are registered
func (b *ModuleBuilder) WithDependency(moduleName string) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if moduleName == "" {
		b.err = fmt.Errorf("dependency name cannot be empty")
		return b
	}

	if moduleName == b.module.name {
		b.err = fmt.Errorf("module cannot depend on itself")
		return b
	}

	// Check for duplicate
	for _, dep := range b.module.dependencies {
		if dep == moduleName {
			b.err = fmt.Errorf("duplicate dependency: %s", moduleName)
			return b
		}
	}

	b.module.dependencies = append(b.module.dependencies, moduleName)
	return b
}

// WithDependencies adds multiple dependencies
func (b *ModuleBuilder) WithDependencies(moduleNames ...string) *ModuleBuilder {
	if b == nil {
		return nil
	}

	for _, name := range moduleNames {
		b = b.WithDependency(name)
		if b.err != nil {
			return b
		}
	}

	return b
}

// WithMsgHandler registers a message handler
func (b *ModuleBuilder) WithMsgHandler(msgType string, handler MsgHandler) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if msgType == "" {
		b.err = fmt.Errorf("message type cannot be empty")
		return b
	}

	if handler == nil {
		b.err = fmt.Errorf("handler cannot be nil for message type %s", msgType)
		return b
	}

	// Check for duplicate
	if _, exists := b.module.msgHandlers[msgType]; exists {
		b.err = fmt.Errorf("duplicate message handler for type: %s", msgType)
		return b
	}

	b.module.msgHandlers[msgType] = handler
	return b
}

// WithMsgHandlers registers multiple message handlers
func (b *ModuleBuilder) WithMsgHandlers(handlers map[string]MsgHandler) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if handlers == nil {
		return b
	}

	for msgType, handler := range handlers {
		b = b.WithMsgHandler(msgType, handler)
		if b.err != nil {
			return b
		}
	}

	return b
}

// WithQueryHandler registers a query handler
func (b *ModuleBuilder) WithQueryHandler(path string, handler QueryHandler) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if path == "" {
		b.err = fmt.Errorf("query path cannot be empty")
		return b
	}

	if handler == nil {
		b.err = fmt.Errorf("handler cannot be nil for query path %s", path)
		return b
	}

	// Check for duplicate
	if _, exists := b.module.queryHandlers[path]; exists {
		b.err = fmt.Errorf("duplicate query handler for path: %s", path)
		return b
	}

	b.module.queryHandlers[path] = handler
	return b
}

// WithQueryHandlers registers multiple query handlers
func (b *ModuleBuilder) WithQueryHandlers(handlers map[string]QueryHandler) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if handlers == nil {
		return b
	}

	for path, handler := range handlers {
		b = b.WithQueryHandler(path, handler)
		if b.err != nil {
			return b
		}
	}

	return b
}

// WithBeginBlocker sets the begin block handler
func (b *ModuleBuilder) WithBeginBlocker(handler BeginBlocker) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if handler == nil {
		b.err = fmt.Errorf("begin block handler cannot be nil")
		return b
	}

	if b.module.beginBlock != nil {
		b.err = fmt.Errorf("begin block handler already set")
		return b
	}

	b.module.beginBlock = handler
	return b
}

// WithEndBlocker sets the end block handler
func (b *ModuleBuilder) WithEndBlocker(handler EndBlocker) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if handler == nil {
		b.err = fmt.Errorf("end block handler cannot be nil")
		return b
	}

	if b.module.endBlock != nil {
		b.err = fmt.Errorf("end block handler already set")
		return b
	}

	b.module.endBlock = handler
	return b
}

// WithInitGenesis sets the init genesis handler
func (b *ModuleBuilder) WithInitGenesis(handler InitGenesis) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if handler == nil {
		b.err = fmt.Errorf("init genesis handler cannot be nil")
		return b
	}

	if b.module.initGenesis != nil {
		b.err = fmt.Errorf("init genesis handler already set")
		return b
	}

	b.module.initGenesis = handler
	return b
}

// WithExportGenesis sets the export genesis handler
func (b *ModuleBuilder) WithExportGenesis(handler ExportGenesis) *ModuleBuilder {
	if b == nil {
		return nil
	}

	if b.err != nil {
		return b
	}

	if handler == nil {
		b.err = fmt.Errorf("export genesis handler cannot be nil")
		return b
	}

	if b.module.exportGenesis != nil {
		b.err = fmt.Errorf("export genesis handler already set")
		return b
	}

	b.module.exportGenesis = handler
	return b
}

// Build constructs the module and validates it
func (b *ModuleBuilder) Build() (Module, error) {
	if b == nil {
		return nil, fmt.Errorf("builder is nil")
	}

	// Return any accumulated error
	if b.err != nil {
		return nil, b.err
	}

	// Validate the module
	if err := ValidateModule(b.module); err != nil {
		return nil, fmt.Errorf("module validation failed: %w", err)
	}

	return b.module, nil
}
