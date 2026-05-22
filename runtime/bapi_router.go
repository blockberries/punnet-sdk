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

// BAPIEvidenceHandler is implemented by modules that wish to react to
// Byzantine evidence delivered in a FinalizedBlock — typically the staking
// module, which slashes the offending validator. The handler returns
// effects (e.g. a WriteEffect updating the validator's power) that the
// application executes between BeginBlock and tx execution so the resulting
// state changes are visible to txs and to EndBlock's ValidatorUpdate
// emission. See PLAN C2.
type BAPIEvidenceHandler interface {
	BAPIModule
	ProcessEvidence(ctx context.Context, blockCtx *BAPIBlockContext, evidence types.Evidence) ([]effects.Effect, error)
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

// BAPIModuleParams is the optional hook for modules whose parameters
// can be changed via governance proposals. The governance module's
// enactment loop (PLAN §7 Phase 4.4) walks every passed-and-due
// proposal at EndBlock, looks up each change's TargetModule, and
// invokes ApplyParameterChange.
//
// Modules typically register their parameter names with the
// governance ParameterRegistry at construction time. The registry
// gates the value against the proposal's class-specific band
// BEFORE this hook fires, so handlers only need to validate
// module-specific structural invariants (cross-parameter
// constraints, deferred state migrations, etc.).
//
// Returning an error from ApplyParameterChange marks the proposal
// as StatusEnactmentFailed (sticky — the chain does not retry).
type BAPIModuleParams interface {
	BAPIModule
	ApplyParameterChange(ctx context.Context, name string, newValue int64) error
}

// BAPIMempoolObserver is the optional hook for modules that consume
// the bapi.MempoolObserver events. The BAPIApplication implements
// bapi.MempoolObserver itself (declared via CapMempoolObserver in the
// handshake response) and fans the events out to every module that
// implements this interface. Used by the participation tracker
// (PLAN §7 Phase 3.3) to count leader_blocks and batches_certified.
//
// Modules that don't care about these events simply don't implement
// the interface; the runtime skips them.
type BAPIMempoolObserver interface {
	BAPIModule
	OnBatchCertified(ctx context.Context, ev types.BatchCertifiedEvent) error
	OnBlockConstructed(ctx context.Context, ev types.BlockConstructedEvent) error
}

// BAPITokenomicsConsumer is the optional hook for modules that need
// chain-wide tokenomics parameters at genesis time
// (TotalSupply, bootstrap validators + their per-validator BL share,
// vest-start height). The runtime invokes it once per module after
// the regular InitGenesis loop and after protocol-account seeding,
// when TokenomicsGenesis is present. The hook fires even when
// BootstrapValidators is empty — modules that just need TotalSupply
// (e.g. the staking module's validator-register bond) still get
// called.
//
// PerValidatorShare and VestStartHeight are zero when there are no
// bootstrap validators.
type BAPITokenomicsConsumer interface {
	BAPIModule
	ConsumeTokenomics(ctx context.Context, p TokenomicsParams) error
}

// TokenomicsParams is the payload to BAPITokenomicsConsumer.
type TokenomicsParams struct {
	TotalSupply         uint64
	BootstrapValidators []BootstrapValidator
	PerValidatorShare   uint64
	VestStartHeight     uint64
}

// BAPIParamsUpdater is implemented by modules that can change consensus
// params during a block — e.g. a governance module enacting a passed
// proposal. The runtime walks all such modules at EndBlock (in
// name-sorted order, after the regular EndBlock hooks). At most one
// module per block may return a non-nil result; if a second module
// also returns non-nil, the runtime rejects the block as conflicting.
// The returned `*types.ConsensusParams` REPLACES the chain's current
// params wholesale — modules that want to change one field must read
// the current params first and copy through unmodified fields.
type BAPIParamsUpdater interface {
	BAPIModule
	ParamsUpdate(ctx context.Context, blockCtx *BAPIBlockContext) (*types.ConsensusParams, error)
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
