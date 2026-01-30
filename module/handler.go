package module

import (
	"github.com/blockberries/punnet-sdk/runtime"
)

// Re-export handler types from runtime to maintain API compatibility
// This avoids circular imports while keeping handlers accessible from the module package

type (
	// MsgHandler handles a message and returns effects
	MsgHandler = runtime.MsgHandler

	// QueryHandler handles a query and returns the result
	QueryHandler = runtime.QueryHandler

	// BeginBlocker is called at the beginning of each block
	BeginBlocker = runtime.BeginBlocker

	// EndBlocker is called at the end of each block
	EndBlocker = runtime.EndBlocker

	// InitGenesis initializes the module's state from genesis data
	InitGenesis = runtime.InitGenesis

	// ExportGenesis exports the module's state for genesis
	ExportGenesis = runtime.ExportGenesis
)
