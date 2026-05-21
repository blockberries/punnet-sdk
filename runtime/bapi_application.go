package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

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

	// anteHandlers run in registration order after authorization
	// validation and before message handlers in executeTx. Empty by
	// default; populated by RegisterAnteHandler. PLAN §7 Phase 0.4
	// adds the framework; Phase 1.4 wires the fee handler.
	anteHandlers []AnteHandler

	// tokenomicsGenesis is the chain-wide tokenomics parameters
	// parsed from BAPIGenesisState.Tokenomics at genesis. nil for
	// non-tokenomics chains; downstream modules consult via the
	// TokenomicsGenesis() accessor. PLAN §7 Phase 0.5.
	tokenomicsGenesis *TokenomicsGenesis

	// Typed stores (created from StateStore)
	accountStore   *store.BAPIAccountStore
	balanceStore   *store.BAPIBalanceStore
	validatorStore *store.BAPIValidatorStore
	paramsStore    *store.BAPIParamsStore

	// State
	currentBlock *types.FinalizedBlock
	appHash      types.AppHash
	chainID      string

	// Pending nonce tracking for mempool.
	// Allows multiple in-flight TXs from the same account by tracking
	// the next expected nonce per account across CheckTx calls.
	// Cleared on Commit when committed state catches up.
	pendingNonceMu sync.Mutex
	pendingNonces  map[ptypes.AccountName]uint64

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

	// Restart with existing state — use the app's current latest version.
	// We intentionally do NOT call LoadVersion(requestedHeight) because the
	// IAVL version may differ from the block height (e.g., if earlier bugs
	// created extra versions). The app's latest version is the correct state.
	appVersion := a.stateStore.Version()

	if appVersion <= 0 {
		// App has no state despite engine reporting existing blocks.
		// Fall back to genesis initialization if provided.
		if req.Genesis != nil {
			if err := a.initGenesis(ctx, req.Genesis); err != nil {
				return types.HandshakeResponse{}, fmt.Errorf("init genesis on restart: %w", err)
			}
			hash, version, err := a.stateStore.Commit()
			if err != nil {
				return types.HandshakeResponse{}, fmt.Errorf("commit genesis on restart: %w", err)
			}
			copy(a.appHash[:], hash)
			a.lastCommitHeight = uint64(version)
			return types.HandshakeResponse{
				LastBlock:    nil,
				AppHash:      &a.appHash,
				Capabilities: 0,
			}, nil
		}
		return types.HandshakeResponse{}, fmt.Errorf(
			"restart requested but app has no state (version 0) and no genesis provided")
	}

	rootHash := a.stateStore.RootHash()
	copy(a.appHash[:], rootHash)
	a.lastCommitHeight = uint64(appVersion)

	var blockHash types.Hash
	copy(blockHash[:], rootHash)

	return types.HandshakeResponse{
		LastBlock: &types.BlockID{
			Height: uint64(appVersion),
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

	// Validate authorization with pending nonce support.
	// This allows multiple in-flight TXs from the same account by tracking
	// the next expected nonce across CheckTx calls.
	if err := a.checkTxAuthorization(ctx, ptx, mctx); err != nil {
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

	// Snapshot consensus params for this block so modules (e.g. the
	// staking slasher) can read per-block-policy values like
	// slash-fraction. A missed read is non-fatal — modules fall back to
	// compiled-in defaults — but logging the failure surfaces a
	// configuration regression.
	var paramsSnapshot *types.ConsensusParams
	if a.paramsStore != nil {
		p, err := a.paramsStore.Get(ctx)
		if err == nil {
			paramsSnapshot = p
		}
	}

	// Create block context
	blockCtx := &BAPIBlockContext{
		Height:   block.Height,
		Time:     block.Time,
		Proposer: block.Proposer,
		ChainID:  a.chainID,
		Params:   paramsSnapshot,
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

	// Sort transactions by (account, nonce) to ensure correct execution order.
	// TXs from DAG mempool batches may arrive out of nonce order when
	// different workers batch TXs from the same account separately.
	orderedTxs := a.orderTxsByNonce(block.Txs)

	// Execute all transactions
	txOutcomes := make([]types.TxOutcome, len(orderedTxs))
	var blockEvents []types.Event
	blockEvents = append(blockEvents, beginBlockEvents...)

	for i, tx := range orderedTxs {
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

	// Reset pending nonces. After commit, the committed state has the
	// authoritative nonces. New CheckTx calls will read from committed state.
	a.pendingNonceMu.Lock()
	a.pendingNonces = nil
	a.pendingNonceMu.Unlock()

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

	// Determine height for query. Historical queries (req.Height set to
	// a past version) aren't yet supported at the handler level —
	// handlers read from a.stateStore which is always at the latest
	// committed version. Surfacing a height parameter that handlers
	// silently ignore would mislead clients into thinking they got
	// historical state; reject such requests up front. Per-key
	// historical reads ARE available via TypedStore.GetAtHeight; a
	// future query handler can opt in to that.
	currentVersion := a.stateStore.Version()
	var queryHeight int64
	if req.Height != nil {
		queryHeight = int64(*req.Height)
		if queryHeight != 0 && queryHeight != currentVersion {
			return types.StateQueryResult{
				Code: 5,
				Info: fmt.Sprintf("historical queries not supported: requested height %d, current %d", queryHeight, currentVersion),
			}, nil
		}
	}
	queryHeight = currentVersion

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

// orderTxsByNonce sorts transactions so that TXs from the same account are
// executed in nonce order. TXs from different accounts retain their relative
// order (stable sort). This is necessary because the DAG mempool may place
// TXs from one account into different batches, and batch ordering doesn't
// guarantee nonce ordering.
func (a *BAPIApplication) orderTxsByNonce(txs []types.Tx) []types.Tx {
	if len(txs) <= 1 {
		return txs
	}

	type decoded struct {
		raw     types.Tx
		account ptypes.AccountName
		nonce   uint64
		ok      bool
	}

	items := make([]decoded, len(txs))
	for i, tx := range txs {
		ptx, err := a.decodeTx(tx)
		if err != nil {
			items[i] = decoded{raw: tx}
			continue
		}
		items[i] = decoded{raw: tx, account: ptx.Account, nonce: ptx.Nonce, ok: true}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].ok || !items[j].ok {
			return false
		}
		if items[i].account != items[j].account {
			return false // preserve original order across accounts
		}
		return items[i].nonce < items[j].nonce
	})

	result := make([]types.Tx, len(items))
	for i, item := range items {
		result[i] = item.raw
	}
	return result
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

// checkTxAuthorization validates authorization for CheckTx with pending nonce support.
// Unlike validateAuthorization (used in ExecuteBlock which checks strictly against
// committed state), this method tracks pending nonces so that multiple TXs from the
// same account can be accepted into the mempool with sequential nonces.
//
// MempoolContext semantics:
//
//   - MempoolFirstSeen: the TX must arrive at the gate with nonce ==
//     max(committed, pending). On accept, the pending nonce is bumped
//     so the next first-seen sees the correct slot.
//
//   - MempoolRevalidation: the TX is being re-checked (typically after a
//     new block committed). The original first-seen already bumped
//     pending; we must NOT re-bump or pending drifts upward on every
//     revalidation. The check is "is this TX's nonce still applicable
//     given current committed state?" — i.e., committed ≤ tx.Nonce <
//     pending. If tx.Nonce < committed, the block already executed
//     this TX and the mempool should evict it (treat as nonce
//     mismatch). If tx.Nonce ≥ pending, the TX was never accepted in
//     the first place (also a mismatch).
//
// Either way, accept is conditional on the nonce check — drops
// mismatched TXs at the gate so they never reach the block builder.
func (a *BAPIApplication) checkTxAuthorization(ctx context.Context, tx *ptypes.Transaction, mctx types.MempoolContext) error {
	// Get account from committed state
	account, err := a.accountStore.Get(ctx, tx.Account)
	if err != nil {
		if err == store.ErrNotFound {
			return fmt.Errorf("account not found: %s", tx.Account)
		}
		return fmt.Errorf("get account: %w", err)
	}

	// Determine expected nonce: max(committed, pending)
	committedNonce := account.Nonce

	a.pendingNonceMu.Lock()
	pendingNonce, hasPending := a.pendingNonces[tx.Account]
	expectedNonce := committedNonce
	if hasPending && pendingNonce > expectedNonce {
		expectedNonce = pendingNonce
	}

	if mctx == types.MempoolRevalidation {
		// Revalidation: the mempool is asking "is this TX still
		// applicable given current committed state?" Accept any TX
		// whose nonce hasn't already been executed. Don't compare
		// against pending — pendingNonces is cleared on every Commit,
		// so a TX still in the mempool post-commit would otherwise
		// be falsely rejected. Don't bump either — pending is the
		// first-seen admission counter, not a per-TX tracker.
		if tx.Nonce < committedNonce {
			a.pendingNonceMu.Unlock()
			return fmt.Errorf("nonce mismatch: revalidation tx.Nonce=%d < committed=%d (already executed)",
				tx.Nonce, committedNonce)
		}
		a.pendingNonceMu.Unlock()
	} else {
		// MempoolFirstSeen (and any unknown value, treated as
		// first-seen for forward compat): require strict match against
		// the next free slot, then bump.
		if tx.Nonce != expectedNonce {
			a.pendingNonceMu.Unlock()
			return fmt.Errorf("nonce mismatch: expected %d, got %d", expectedNonce, tx.Nonce)
		}
		if a.pendingNonces == nil {
			a.pendingNonces = make(map[ptypes.AccountName]uint64)
		}
		a.pendingNonces[tx.Account] = expectedNonce + 1
		a.pendingNonceMu.Unlock()
	}

	// Verify authorization (signatures, weights, threshold).
	// Set account nonce to match TX nonce so VerifyAuthorization's internal
	// nonce check passes -- we already validated it against pending state above.
	accountCopy := *account
	accountCopy.Nonce = tx.Nonce
	if err := tx.VerifyAuthorization(&accountCopy, a); err != nil {
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

	// AnteHandler stage. PLAN §7 Phase 0.4: run any registered ante
	// handlers (fee deduction, spam guards, ...) between authorization
	// and message dispatch. The first handler returning an error
	// aborts the tx; subsequent handlers don't run.
	//
	// Today, AnteHandler effects are folded into the same atomic batch
	// as the message-handler effects below — a handler failure rolls
	// back the AnteHandler too. Phase 1.5 will split the commit so
	// fees persist when the message handler aborts (spec §3's
	// "failed-execution txs still pay").
	a.mu.RLock()
	anteChain := a.anteHandlers
	a.mu.RUnlock()
	for _, ante := range anteChain {
		anteEffects, err := ante(ctx, txCtx, ptx)
		if err != nil {
			return types.TxOutcome{
				Index: index,
				Code:  20, // distinct from handler errors so apps can tell them apart
				Info:  fmt.Sprintf("ante: %v", err),
			}, nil
		}
		allEffects = append(allEffects, anteEffects...)
	}

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

	// Tokenomics genesis: validate and cache so downstream modules
	// (bank for protocol-account seeding, mint for VRP, etc.) can
	// consult the same parsed values. Optional — nil skips
	// tokenomics enforcement. PLAN §7 Phase 0.5.
	if genesisState.Tokenomics != nil {
		if err := genesisState.Tokenomics.ValidateBasic(); err != nil {
			return fmt.Errorf("invalid tokenomics genesis: %w", err)
		}
		a.mu.Lock()
		a.tokenomicsGenesis = genesisState.Tokenomics
		a.mu.Unlock()
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

// ExportGenesis produces a `types.GenesisDoc` reflecting the
// application's current committed state — chain ID, consensus params,
// validator set, and each module's app-level genesis state. The
// resulting doc can be used to bootstrap a fresh chain identical (at
// the BAPI layer) to this one's tip.
//
// Determinism: modules are walked in name-sorted order; the validator
// set is iterated in the staking store's deterministic order then
// sorted by hex-encoded pubkey. As long as every module's
// ExportGenesis is itself deterministic, the resulting doc bytes are
// byte-identical across nodes that ran the same blocks.
//
// Returns an error if any module's ExportGenesis fails or if the
// per-module JSON cannot be merged into the application-level state.
// The validator set falls back to an empty list if the underlying
// store doesn't implement Iterable (test in-memory backend).
func (a *BAPIApplication) ExportGenesis(ctx context.Context) (*types.GenesisDoc, error) {
	if a == nil {
		return nil, fmt.Errorf("application is nil")
	}

	// 1. Snapshot consensus params.
	var params types.ConsensusParams
	if a.paramsStore != nil {
		if p, err := a.paramsStore.Get(ctx); err == nil && p != nil {
			params = *p
		}
	}

	// 2. Snapshot the current validator set. We walk the staking
	// module's validator store (the source of truth for power /
	// jail status) — this means we work even when the engine's
	// consensus-side set isn't directly addressable from here.
	var validators []types.ValidatorUpdate
	if a.validatorStore != nil {
		_ = a.validatorStore.IterateValidators(func(v *store.BAPIValidator) bool {
			// Skip jailed / zero-power validators: they should not
			// be in the new chain's active set on restart. If the
			// operator wants them back, they unjail post-genesis.
			if v.Jailed || v.Power == 0 {
				return false
			}
			validators = append(validators, types.ValidatorUpdate{
				PubKey: v.PubKey,
				Power:  v.Power,
			})
			return false
		})
		// Defensive sort so we don't depend on the underlying iteration
		// order being stable forever.
		sort.Slice(validators, func(i, j int) bool {
			return string(validators[i].PubKey.Data) < string(validators[j].PubKey.Data)
		})
	}

	// 3. Aggregate each module's ExportGenesis output into BAPIGenesisState.
	modules := a.router.Modules()
	sortedModules := make([]BAPIModule, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})
	moduleState := make(map[string]json.RawMessage)
	for _, mod := range sortedModules {
		exporter, ok := mod.(BAPIGenesisExporter)
		if !ok {
			continue
		}
		raw, err := exporter.ExportGenesis(ctx)
		if err != nil {
			return nil, fmt.Errorf("module %s ExportGenesis: %w", mod.Name(), err)
		}
		if len(raw) == 0 {
			continue
		}
		moduleState[mod.Name()] = json.RawMessage(raw)
	}
	appState, err := json.Marshal(BAPIGenesisState{Modules: moduleState})
	if err != nil {
		return nil, fmt.Errorf("marshal app state: %w", err)
	}

	// 4. Assemble the GenesisDoc. Pull height/time from the current
	// header if we're inside a block; otherwise use the state-store
	// version and wall-clock time as a best-effort floor.
	a.mu.RLock()
	header := a.currentBlock
	a.mu.RUnlock()
	var height uint64
	var genTime types.Timestamp
	if header != nil {
		height = header.Height
		genTime = header.Time
	} else if a.stateStore != nil {
		height = uint64(a.stateStore.Version())
		genTime = types.TimeToTimestamp(time.Now())
	}

	return &types.GenesisDoc{
		ChainID:         a.chainID,
		GenesisTime:     genTime,
		InitialHeight:   height,
		ConsensusParams: params,
		Validators:      validators,
		AppState:        appState,
	}, nil
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

	// Collect consensus-params updates from modules implementing
	// BAPIParamsUpdater. Walked in name-sorted order (same as the
	// EndBlock hooks above) so multi-module behavior is deterministic.
	// At most one module may emit a non-nil update per block; a second
	// non-nil update is rejected with an error so silent overwrites
	// don't produce divergent chain state.
	var paramsUpdate *types.ConsensusParams
	var paramsUpdater string
	for _, mod := range sortedModules {
		updater, ok := mod.(BAPIParamsUpdater)
		if !ok {
			continue
		}
		next, err := updater.ParamsUpdate(ctx, blockCtx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("module %s ParamsUpdate: %w", mod.Name(), err)
		}
		if next == nil {
			continue
		}
		if paramsUpdate != nil {
			return nil, nil, nil, fmt.Errorf("conflicting params updates: %s and %s both emitted at height %d",
				paramsUpdater, mod.Name(), blockCtx.Height)
		}
		paramsUpdate = next
		paramsUpdater = mod.Name()
	}

	// Persist the update so the next block sees it via the params
	// snapshot taken in processBlock. Without this, the change is
	// returned to the consumer but never observed by the next block.
	if paramsUpdate != nil && a.paramsStore != nil {
		if err := a.paramsStore.Set(ctx, paramsUpdate); err != nil {
			return nil, nil, nil, fmt.Errorf("persist params update from %s: %w", paramsUpdater, err)
		}
	}

	return validatorUpdates, paramsUpdate, events, nil
}

// processEvidence processes misbehavior evidence.
//
// Every module that implements BAPIEvidenceHandler gets a chance to act on
// the evidence — typically the staking module, which slashes the offending
// validator. We collect the effects from all handlers, then execute them
// through the same effects executor that handles tx and block-lifecycle
// effects, so consensus-critical state changes (validator power, jail
// status, …) follow the same effects-not-mutations invariant the rest of
// the SDK enforces (PLAN C2). Modules are iterated in name-sorted order
// for determinism, matching runBeginBlockHooks / runEndBlockHooks.
func (a *BAPIApplication) processEvidence(ctx context.Context, blockCtx *BAPIBlockContext, evidence types.Evidence) error {
	modules := a.router.Modules()
	sortedModules := make([]BAPIModule, len(modules))
	copy(sortedModules, modules)
	sort.Slice(sortedModules, func(i, j int) bool {
		return sortedModules[i].Name() < sortedModules[j].Name()
	})

	var allEffects []effects.Effect
	for _, mod := range sortedModules {
		handler, ok := mod.(BAPIEvidenceHandler)
		if !ok {
			continue
		}
		modEffects, err := handler.ProcessEvidence(ctx, blockCtx, evidence)
		if err != nil {
			return fmt.Errorf("module %s ProcessEvidence: %w", mod.Name(), err)
		}
		allEffects = append(allEffects, modEffects...)
	}

	if len(allEffects) == 0 {
		return nil
	}

	if _, err := a.effectExecutor.Execute(allEffects); err != nil {
		return fmt.Errorf("execute evidence effects: %w", err)
	}
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
//
// Params is the ConsensusParams snapshot at the start of this block.
// nil when the application hasn't been initialized with a params store
// (e.g. very early in tests); callers should fall back to compiled-in
// defaults in that case.
type BAPIBlockContext struct {
	Height   uint64
	Time     types.Timestamp
	Proposer types.ValidatorAddress
	ChainID  string
	Params   *types.ConsensusParams
}

// BAPITxContext contains transaction-level execution context.
type BAPITxContext struct {
	*BAPIBlockContext
	Account ptypes.AccountName
	TxIndex uint32
}

// BAPIGenesisState represents the genesis app state structure.
type BAPIGenesisState struct {
	// Modules carries per-module genesis blobs, keyed by module
	// name. Each value is opaque to the runtime; the owning module's
	// InitGenesis hook decodes it.
	Modules map[string]json.RawMessage `json:"modules"`

	// Tokenomics carries chain-wide tokenomics parameters that
	// must be known at chain construction (total supply, bootstrap
	// validators). Optional — nil means the runtime does not
	// enforce the tokenomics supply model. PLAN §7 Phase 0.5.
	//
	// When set, ValidateBasic is invoked from initGenesis before
	// any module genesis blob is processed; downstream modules
	// (bank, mint, distribution, staking) consult these values
	// via BAPIApplication.TokenomicsGenesis().
	Tokenomics *TokenomicsGenesis `json:"tokenomics,omitempty"`
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

// TokenomicsGenesis returns the parsed TokenomicsGenesis from the
// chain's initial app state, or nil if the chain wasn't initialised
// with one (non-tokenomics chains). Downstream modules — bank for
// protocol-account seeding, mint for VRP draining, distribution for
// epoch accounting — consult this to wire genesis state consistently.
//
// Safe for concurrent reads; the value is set once at initGenesis
// and never mutated thereafter. PLAN §7 Phase 0.5.
func (a *BAPIApplication) TokenomicsGenesis() *TokenomicsGenesis {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tokenomicsGenesis
}

// RegisterAnteHandler appends a handler to the AnteHandler chain.
// The chain runs in registration order in executeTx, after
// authorization validation and before module message handlers.
//
// Apps typically register all their AnteHandlers during construction
// (before any block is executed). Registering at runtime is permitted
// but not encouraged: the AnteHandler set is part of the deterministic
// state-transition function, so a node that flips the chain mid-life
// will diverge from peers that didn't.
//
// PLAN §7 Phase 0.4 / Phase 1.4.
func (a *BAPIApplication) RegisterAnteHandler(handler AnteHandler) {
	if a == nil || handler == nil {
		return
	}
	a.mu.Lock()
	a.anteHandlers = append(a.anteHandlers, handler)
	a.mu.Unlock()
}
