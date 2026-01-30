package runtime

import (
	"context"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/types"
)

// MsgHandler handles a message and returns effects
// Handlers should not mutate state directly - instead, they return effects
// that describe their intent. The runtime collects and executes these effects.
type MsgHandler func(ctx *Context, msg types.Message) ([]effects.Effect, error)

// QueryHandler handles a query and returns the result
// Queries are read-only and do not produce effects
type QueryHandler func(ctx context.Context, path string, data []byte) ([]byte, error)

// BeginBlocker is called at the beginning of each block
// It can return effects that will be executed before any transactions
type BeginBlocker func(ctx *Context) ([]effects.Effect, error)

// EndBlocker is called at the end of each block
// It can return effects and validator updates
type EndBlocker func(ctx *Context) ([]effects.Effect, []types.ValidatorUpdate, error)

// InitGenesis initializes the module's state from genesis data
type InitGenesis func(ctx *Context, data []byte) error

// ExportGenesis exports the module's state for genesis
type ExportGenesis func(ctx context.Context) ([]byte, error)
