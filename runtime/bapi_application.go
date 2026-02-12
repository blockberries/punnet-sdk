package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/blockberries/bapi"
	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/di"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// Verify BAPIApplication implements bapi.Lifecycle
var _ bapi.Lifecycle = (*BAPIApplication)(nil)

// BAPIApplication implements the BAPI Lifecycle interface.
// It coordinates transaction execution, module lifecycle, and state management
// using the new BAPI paradigm.
type BAPIApplication struct {
	mu sync.RWMutex

	// Injected dependencies
	stateStore statestore.StateStore

	// Internal components
	container      *di.Container
	router         *BAPIRouter
	effectExecutor *effects.BAPIExecutor

	// Typed stores (created from StateStore)
	accountStore   *store.BAPIAccountStore
	balanceStore   *store.BAPIBalanceStore
	validatorStore *store.BAPIValidatorStore
	paramsStore    *store.BAPIParamsStore

	// State
	currentBlock *types.FinalizedBlock
	appHash      types.AppHash
	chainID      string

	// Metrics
	lastCommitHeight uint64
}

// BAPIApplicationConfig configures the BAPI application.
type BAPIApplicationConfig struct {
	// ChainID is the blockchain identifier
	ChainID string

	// StateStore is the backing avlberry store
	StateStore statestore.StateStore

	// Modules are the modules to register
	Modules []BAPIModule
}

// NewBAPIApplication creates a new BAPI application.
func NewBAPIApplication(config BAPIApplicationConfig) (*BAPIApplication, error) {
	if config.ChainID == "" {
		return nil, fmt.Errorf("chain ID cannot be empty")
	}

	if config.StateStore == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}

	// Create DI container
	container := di.New()

	// Create typed stores using the StateStore
	storeProvider := store.NewBAPIStoreProvider(config.StateStore)

	// Register store provider with container
	if err := storeProvider.RegisterWithContainer(container); err != nil {
		return nil, fmt.Errorf("register store provider: %w", err)
	}

	// Create effect executor
	executor := effects.NewBAPIExecutor(storeProvider)

	// Create router
	router := NewBAPIRouter()

	app := &BAPIApplication{
		stateStore:     config.StateStore,
		container:      container,
		router:         router,
		effectExecutor: executor,
		accountStore:   storeProvider.GetAccountStore(),
		balanceStore:   storeProvider.GetBalanceStore(),
		validatorStore: storeProvider.GetValidatorStore(),
		paramsStore:    storeProvider.GetParamsStore(),
		chainID:        config.ChainID,
	}

	// Register and inject dependencies into all modules
	for _, mod := range config.Modules {
		// Inject dependencies into module
		if err := container.InjectDependencies(mod); err != nil {
			return nil, fmt.Errorf("inject dependencies into module %s: %w", mod.Name(), err)
		}

		// Register handlers with router
		if err := router.RegisterModule(mod); err != nil {
			return nil, fmt.Errorf("register module handlers %s: %w", mod.Name(), err)
		}
	}

	return app, nil
}

// Handshake is called once on every startup (cold start or restart).
func (a *BAPIApplication) Handshake(ctx context.Context, req types.HandshakeRequest) (types.HandshakeResponse, error) {
	if a == nil {
		return types.HandshakeResponse{}, fmt.Errorf("application is nil")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if req.LastCommitted == nil {
		// Genesis initialization
		if req.Genesis == nil {
			return types.HandshakeResponse{}, fmt.Errorf("genesis required for fresh start")
		}

		// Initialize from genesis
		if err := a.initGenesis(ctx, req.Genesis); err != nil {
			return types.HandshakeResponse{}, fmt.Errorf("init genesis: %w", err)
		}

		// Commit genesis state
		hash, version, err := a.stateStore.Commit()
		if err != nil {
			return types.HandshakeResponse{}, fmt.Errorf("commit genesis: %w", err)
		}
		copy(a.appHash[:], hash)
		a.lastCommitHeight = uint64(version)

		return types.HandshakeResponse{
			LastBlock:    nil,
			AppHash:      &a.appHash,
			Capabilities: 0, // Lifecycle only for now
		}, nil
	}

	// Restart with existing state
	appVersion := a.stateStore.Version()
	requestedHeight := int64(req.LastCommitted.Height)

	if appVersion != requestedHeight {
		// Try to load the requested version
		if err := a.stateStore.LoadVersion(requestedHeight); err != nil {
			return types.HandshakeResponse{}, fmt.Errorf(
				"cannot load version %d, have %d: %w", requestedHeight, appVersion, err)
		}
	}

	rootHash := a.stateStore.RootHash()
	copy(a.appHash[:], rootHash)
	a.lastCommitHeight = uint64(a.stateStore.Version())

	var blockHash types.Hash
	copy(blockHash[:], rootHash)

	return types.HandshakeResponse{
		LastBlock: &types.BlockID{
			Height: uint64(a.stateStore.Version()),
			Hash:   blockHash,
		},
		AppHash:      &a.appHash,
		Capabilities: 0,
	}, nil
}

// CheckTx gate-checks a transaction before it enters the mempool.
func (a *BAPIApplication) CheckTx(ctx context.Context, tx types.Tx, mctx types.MempoolContext) (types.GateVerdict, error) {
	if a == nil {
		return types.GateVerdict{Code: 1, Info: "application is nil"}, nil
	}

	// Decode transaction
	ptx, err := a.decodeTx(tx)
	if err != nil {
		return types.GateVerdict{
			Code: 1,
			Info: fmt.Sprintf("decode error: %v", err),
		}, nil
	}

	// Validate basic transaction structure
	if err := ptx.ValidateBasic(); err != nil {
		return types.GateVerdict{
			Code: 2,
			Info: fmt.Sprintf("validation failed: %v", err),
		}, nil
	}

	// Validate authorization (signatures, nonces)
	if err := a.validateAuthorization(ctx, ptx); err != nil {
		return types.GateVerdict{
			Code: 3,
			Info: fmt.Sprintf("authorization failed: %v", err),
		}, nil
	}

	// Route to module for message-specific validation
	for _, msg := range ptx.Messages {
		handler := a.router.GetMsgHandler(msg.Type())
		if handler == nil {
			return types.GateVerdict{
				Code: 4,
				Info: fmt.Sprintf("unknown message type: %s", msg.Type()),
			}, nil
		}
	}

	return types.GateVerdict{
		Code:     0,
		Priority: 0, // Fixed priority as per spec
		Sender:   string(ptx.Account),
	}, nil
}

// ExecuteBlock deterministically executes a finalized block.
func (a *BAPIApplication) ExecuteBlock(ctx context.Context, block types.FinalizedBlock) (types.BlockOutcome, error) {
	if a == nil {
		return types.BlockOutcome{}, fmt.Errorf("application is nil")
	}

	a.mu.Lock()
	a.currentBlock = &block
	a.mu.Unlock()

	// Create block context
	blockCtx := &BAPIBlockContext{
		Height:   block.Height,
		Time:     block.Time,
		Proposer: block.Proposer,
		ChainID:  a.chainID,
	}

	// Run module begin-block hooks
	beginBlockEvents, err := a.runBeginBlockHooks(ctx, blockCtx)
	if err != nil {
		return types.BlockOutcome{}, fmt.Errorf("begin block hooks: %w", err)
	}

	// Process evidence (slashing)
	for _, evidence := range block.Evidence {
		if err := a.processEvidence(ctx, blockCtx, evidence); err != nil {
			return types.BlockOutcome{}, fmt.Errorf("process evidence: %w", err)
		}
	}

	// Execute all transactions
	txOutcomes := make([]types.TxOutcome, len(block.Txs))
	var blockEvents []types.Event
	blockEvents = append(blockEvents, beginBlockEvents...)

	for i, tx := range block.Txs {
		outcome, events := a.executeTx(ctx, blockCtx, tx, uint32(i))
		txOutcomes[i] = outcome
		blockEvents = append(blockEvents, events...)
	}

	// Run module end-block hooks (validator updates, etc.)
	validatorUpdates, paramsUpdate, endBlockEvents, err := a.runEndBlockHooks(ctx, blockCtx)
	if err != nil {
		return types.BlockOutcome{}, fmt.Errorf("end block hooks: %w", err)
	}
	blockEvents = append(blockEvents, endBlockEvents...)

	// Compute working hash (will be finalized on Commit)
	workingHash := a.stateStore.RootHash()
	copy(a.appHash[:], workingHash)

	return types.BlockOutcome{
		TxOutcomes:       txOutcomes,
		BlockEvents:      blockEvents,
		AppHash:          a.appHash,
		ValidatorUpdates: validatorUpdates,
		ParamsUpdate:     paramsUpdate,
	}, nil
}

// Commit persists all state changes from the last ExecuteBlock.
func (a *BAPIApplication) Commit(ctx context.Context) (types.CommitResult, error) {
	if a == nil {
		return types.CommitResult{}, fmt.Errorf("application is nil")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Persist all state changes
	hash, version, err := a.stateStore.Commit()
	if err != nil {
		return types.CommitResult{}, fmt.Errorf("commit state: %w", err)
	}

	// Update app hash
	copy(a.appHash[:], hash)
	a.lastCommitHeight = uint64(version)

	// Clear current block
	a.currentBlock = nil

	// Return retain height (keep last 100 blocks for queries)
	retainHeight := uint64(0)
	if version > 100 {
		retainHeight = uint64(version - 100)
	}

	return types.CommitResult{
		RetainHeight: retainHeight,
	}, nil
}

// Query reads application state.
func (a *BAPIApplication) Query(ctx context.Context, req types.StateQuery) (types.StateQueryResult, error) {
	if a == nil {
		return types.StateQueryResult{Code: 1, Info: "application is nil"}, nil
	}

	// Parse query path
	path := string(req.Path)
	if path == "" {
		return types.StateQueryResult{Code: 1, Info: "empty query path"}, nil
	}

	// Determine height for query
	var queryHeight int64
	if req.Height != nil {
		queryHeight = int64(*req.Height)
	} else {
		queryHeight = a.stateStore.Version()
	}

	// Route query to handler
	handler := a.router.GetQueryHandler(path)
	if handler == nil {
		return types.StateQueryResult{
			Code: 2,
			Info: fmt.Sprintf("unknown query path: %s", path),
		}, nil
	}

	// Execute query
	result, err := handler(ctx, req.Data, queryHeight)
	if err != nil {
		return types.StateQueryResult{
			Code: 3,
			Info: fmt.Sprintf("query failed: %v", err),
		}, nil
	}

	// Build response
	response := types.StateQueryResult{
		Code:   0,
		Key:    req.Data,
		Value:  result,
		Height: uint64(queryHeight),
	}

	// Add proof if requested
	if req.Prove {
		proof, err := a.generateProof(ctx, req.Data)
		if err != nil {
			return types.StateQueryResult{
				Code: 4,
				Info: fmt.Sprintf("proof generation failed: %v", err),
			}, nil
		}
		response.Proof = proof
	}

	return response, nil
}

// decodeTx decodes a raw transaction into a punnet-sdk Transaction.
func (a *BAPIApplication) decodeTx(tx types.Tx) (*ptypes.Transaction, error) {
	var ptx ptypes.Transaction
	if err := json.Unmarshal(tx, &ptx); err != nil {
		return nil, fmt.Errorf("unmarshal transaction: %w", err)
	}
	return &ptx, nil
}

// validateAuthorization validates transaction authorization.
func (a *BAPIApplication) validateAuthorization(ctx context.Context, tx *ptypes.Transaction) error {
	// Get account
	account, err := a.accountStore.Get(ctx, tx.Account)
	if err != nil {
		if err == store.ErrNotFound {
			return fmt.Errorf("account not found: %s", tx.Account)
		}
		return fmt.Errorf("get account: %w", err)
	}

	// Verify nonce
	if tx.Nonce != account.Nonce {
		return fmt.Errorf("nonce mismatch: expected %d, got %d", account.Nonce, tx.Nonce)
	}

	// Verify authorization (signatures, weights, threshold)
	if err := tx.VerifyAuthorization(account, a); err != nil {
		return fmt.Errorf("verify authorization: %w", err)
	}

	return nil
}

// GetAccount implements types.AccountGetter for authorization verification.
func (a *BAPIApplication) GetAccount(name ptypes.AccountName) (*ptypes.Account, error) {
	return a.accountStore.Get(context.Background(), name)
}

// executeTx executes a single transaction.
func (a *BAPIApplication) executeTx(ctx context.Context, blockCtx *BAPIBlockContext, tx types.Tx, index uint32) (types.TxOutcome, []types.Event) {
	// Decode
	ptx, err := a.decodeTx(tx)
	if err != nil {
		return types.TxOutcome{
			Index: index,
			Code:  1,
			Info:  fmt.Sprintf("decode error: %v", err),
		}, nil
	}

	// Validate authorization
	if err := a.validateAuthorization(ctx, ptx); err != nil {
		return types.TxOutcome{
			Index: index,
			Code:  2,
			Info:  fmt.Sprintf("authorization failed: %v", err),
		}, nil
	}

	// Create transaction context
	txCtx := &BAPITxContext{
		BAPIBlockContext: blockCtx,
		Account:          ptx.Account,
		TxIndex:          index,
	}

	// Collect effects from all messages
	var allEffects []effects.Effect

	for _, msg := range ptx.Messages {
		// Get handler
		handler := a.router.GetMsgHandler(msg.Type())
		if handler == nil {
			return types.TxOutcome{
				Index: index,
				Code:  3,
				Info:  fmt.Sprintf("unknown message type: %s", msg.Type()),
			}, nil
		}

		// Execute handler - collect effects
		msgEffects, err := handler(ctx, txCtx, msg)
		if err != nil {
			return types.TxOutcome{
				Index: index,
				Code:  4,
				Info:  fmt.Sprintf("handler error: %v", err),
			}, nil
		}

		allEffects = append(allEffects, msgEffects...)
	}

	// Validate all effects
	if err := a.effectExecutor.ValidateAll(allEffects); err != nil {
		return types.TxOutcome{
			Index: index,
			Code:  5,
			Info:  fmt.Sprintf("effect validation: %v", err),
		}, nil
	}

	// Execute all effects atomically
	execResult, err := a.effectExecutor.Execute(allEffects)
	if err != nil {
		return types.TxOutcome{
			Index: index,
			Code:  6,
			Info:  fmt.Sprintf("effect execution: %v", err),
		}, nil
	}

	// Increment account nonce
	if err := a.accountStore.IncrementNonce(ctx, ptx.Account); err != nil {
		return types.TxOutcome{
			Index: index,
			Code:  7,
			Info:  fmt.Sprintf("increment nonce: %v", err),
		}, nil
	}

	// Convert execution events to BAPI events
	bapiEvents := make([]types.Event, len(execResult.Events))
	for i, event := range execResult.Events {
		bapiEvents[i] = convertToBAPIEvent(event)
	}

	return types.TxOutcome{
		Index:  index,
		Code:   0,
		Events: bapiEvents,
	}, bapiEvents
}

// initGenesis initializes the application from genesis.
func (a *BAPIApplication) initGenesis(ctx context.Context, genesis *types.GenesisDoc) error {
	// Store chain ID
	a.chainID = genesis.ChainID

	// Store initial consensus params
	if err := a.paramsStore.Set(ctx, &genesis.ConsensusParams); err != nil {
		return fmt.Errorf("set consensus params: %w", err)
	}

	// Parse application state
	var genesisState BAPIGenesisState
	if len(genesis.AppState) > 0 {
		if err := json.Unmarshal(genesis.AppState, &genesisState); err != nil {
			return fmt.Errorf("unmarshal app state: %w", err)
		}
	}

	// Get all modules and sort for deterministic initialization
	modules := a.router.Modules()
	sortedModules := make([]BAPIModule, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	// Initialize each module
	for _, mod := range sortedModules {
		initializer, ok := mod.(BAPIGenesisInitializer)
		if !ok {
			continue
		}

		// Get module genesis data
		moduleGenesis, exists := genesisState.Modules[mod.Name()]
		if !exists {
			moduleGenesis = json.RawMessage("{}")
		}

		// Initialize module
		if err := initializer.InitGenesis(ctx, moduleGenesis); err != nil {
			return fmt.Errorf("init genesis for %s: %w", mod.Name(), err)
		}
	}

	// Store initial validators
	for _, val := range genesis.Validators {
		validator := &store.BAPIValidator{
			PubKey: val.PubKey,
			Power:  val.Power,
		}
		if err := a.validatorStore.SetValidator(ctx, validator); err != nil {
			return fmt.Errorf("set initial validator: %w", err)
		}
	}

	return nil
}

// runBeginBlockHooks runs all module begin-block hooks.
func (a *BAPIApplication) runBeginBlockHooks(ctx context.Context, blockCtx *BAPIBlockContext) ([]types.Event, error) {
	modules := a.router.Modules()
	sortedModules := make([]BAPIModule, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	var allEffects []effects.Effect
	for _, mod := range sortedModules {
		processor, ok := mod.(BAPIBlockProcessor)
		if !ok {
			continue
		}

		blockEffects, err := processor.BeginBlock(ctx, blockCtx)
		if err != nil {
			return nil, fmt.Errorf("module %s BeginBlock: %w", mod.Name(), err)
		}

		allEffects = append(allEffects, blockEffects...)
	}

	// Execute all effects
	if len(allEffects) == 0 {
		return nil, nil
	}

	execResult, err := a.effectExecutor.Execute(allEffects)
	if err != nil {
		return nil, fmt.Errorf("execute begin block effects: %w", err)
	}

	// Convert events
	events := make([]types.Event, len(execResult.Events))
	for i, event := range execResult.Events {
		events[i] = convertToBAPIEvent(event)
	}

	return events, nil
}

// runEndBlockHooks runs all module end-block hooks.
func (a *BAPIApplication) runEndBlockHooks(ctx context.Context, blockCtx *BAPIBlockContext) ([]types.ValidatorUpdate, *types.ConsensusParams, []types.Event, error) {
	modules := a.router.Modules()
	sortedModules := make([]BAPIModule, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	var allEffects []effects.Effect
	var allValidatorUpdates []types.ValidatorUpdate

	for _, mod := range sortedModules {
		processor, ok := mod.(BAPIBlockProcessor)
		if !ok {
			continue
		}

		blockEffects, validatorUpdates, err := processor.EndBlock(ctx, blockCtx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("module %s EndBlock: %w", mod.Name(), err)
		}

		allEffects = append(allEffects, blockEffects...)
		allValidatorUpdates = append(allValidatorUpdates, validatorUpdates...)
	}

	// Execute all effects
	var events []types.Event
	if len(allEffects) > 0 {
		execResult, err := a.effectExecutor.Execute(allEffects)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("execute end block effects: %w", err)
		}

		for _, event := range execResult.Events {
			events = append(events, convertToBAPIEvent(event))
		}
	}

	// Deduplicate validator updates
	validatorUpdates := deduplicateBAPIValidatorUpdates(allValidatorUpdates)

	// Check for consensus params updates
	var paramsUpdate *types.ConsensusParams
	// TODO: Implement consensus params update collection from modules

	return validatorUpdates, paramsUpdate, events, nil
}

// processEvidence processes misbehavior evidence.
func (a *BAPIApplication) processEvidence(ctx context.Context, blockCtx *BAPIBlockContext, evidence types.Evidence) error {
	// TODO: Implement evidence processing (slashing)
	// For now, just log the evidence
	return nil
}

// generateProof generates a Merkle proof for a key.
func (a *BAPIApplication) generateProof(ctx context.Context, key []byte) (*types.MerkleProof, error) {
	// Get proof from state store
	proof, err := a.stateStore.GetProof(key)
	if err != nil {
		return nil, fmt.Errorf("get proof: %w", err)
	}

	if proof == nil {
		return nil, nil
	}

	// Convert to BAPI MerkleProof format
	// The statestore.Proof has ProofBytes which contains ICS23 commitment proof
	bapiProof := &types.MerkleProof{
		Ops: []types.ProofOp{
			{
				Type: "ics23:iavl",
				Key:  proof.Key,
				Data: proof.ProofBytes,
			},
		},
	}

	return bapiProof, nil
}

// convertToBAPIEvent converts an internal event to a BAPI event.
func convertToBAPIEvent(event effects.Event) types.Event {
	attrs := make([]types.EventAttribute, 0, len(event.Attributes))
	for key, value := range event.Attributes {
		attrs = append(attrs, types.EventAttribute{
			Key:   key,
			Value: string(value),
		})
	}

	// Sort attributes for determinism
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Key < attrs[j].Key
	})

	return types.Event{
		Kind:       event.Type,
		Attributes: attrs,
	}
}

// deduplicateBAPIValidatorUpdates removes duplicate validator updates.
func deduplicateBAPIValidatorUpdates(updates []types.ValidatorUpdate) []types.ValidatorUpdate {
	if len(updates) == 0 {
		return nil
	}

	updateMap := make(map[string]types.ValidatorUpdate)
	keyOrder := make([]string, 0, len(updates))

	for _, update := range updates {
		key := string(update.PubKey.Data)
		if _, exists := updateMap[key]; !exists {
			keyOrder = append(keyOrder, key)
		}
		updateMap[key] = update
	}

	result := make([]types.ValidatorUpdate, 0, len(updateMap))
	for _, key := range keyOrder {
		result = append(result, updateMap[key])
	}

	return result
}

// BAPIBlockContext contains block-level execution context.
type BAPIBlockContext struct {
	Height   uint64
	Time     types.Timestamp
	Proposer types.ValidatorAddress
	ChainID  string
}

// BAPITxContext contains transaction-level execution context.
type BAPITxContext struct {
	*BAPIBlockContext
	Account ptypes.AccountName
	TxIndex uint32
}

// BAPIGenesisState represents the genesis app state structure.
type BAPIGenesisState struct {
	Modules map[string]json.RawMessage `json:"modules"`
}

// ChainID returns the chain identifier.
func (a *BAPIApplication) ChainID() string {
	if a == nil {
		return ""
	}
	return a.chainID
}

// StateStore returns the underlying state store.
func (a *BAPIApplication) StateStore() statestore.StateStore {
	if a == nil {
		return nil
	}
	return a.stateStore
}

// Container returns the DI container.
func (a *BAPIApplication) Container() *di.Container {
	if a == nil {
		return nil
	}
	return a.container
}

// Router returns the BAPI router.
func (a *BAPIApplication) Router() *BAPIRouter {
	if a == nil {
		return nil
	}
	return a.router
}
