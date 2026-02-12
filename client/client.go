// Package client provides type-safe RPC client helpers for querying Punnet SDK applications.
// It wraps raspberry's /abci_query endpoint with convenient methods for each module.
package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client provides methods for querying a Punnet SDK application via RPC.
type Client struct {
	// endpoint is the base URL of the RPC server (e.g., "http://localhost:26657")
	endpoint string

	// httpClient is used for making HTTP requests
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// New creates a new Client with the given RPC endpoint.
func New(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ABCIQueryResponse represents the response from an ABCI query.
type ABCIQueryResponse struct {
	Result ABCIQueryResult `json:"result"`
}

// ABCIQueryResult contains the query result data.
type ABCIQueryResult struct {
	Response ABCIResponse `json:"response"`
}

// ABCIResponse contains the actual response fields.
type ABCIResponse struct {
	Code      uint32 `json:"code"`
	Log       string `json:"log"`
	Info      string `json:"info"`
	Index     int64  `json:"index"`
	Key       []byte `json:"key"`
	Value     []byte `json:"value"`
	ProofOps  []byte `json:"proofOps,omitempty"`
	Height    string `json:"height"`
	Codespace string `json:"codespace"`
}

// QueryError represents an error from a query.
type QueryError struct {
	Code      uint32
	Log       string
	Codespace string
}

func (e *QueryError) Error() string {
	if e.Log != "" {
		return fmt.Sprintf("query failed (code %d): %s", e.Code, e.Log)
	}
	return fmt.Sprintf("query failed with code %d", e.Code)
}

// Query performs a raw ABCI query.
func (c *Client) Query(ctx context.Context, path string, data []byte, height int64) (*ABCIResponse, error) {
	// Build query URL
	u, err := url.Parse(c.endpoint + "/abci_query")
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("path", fmt.Sprintf("%q", path))
	if len(data) > 0 {
		q.Set("data", fmt.Sprintf("0x%s", hex.EncodeToString(data)))
	}
	if height > 0 {
		q.Set("height", strconv.FormatInt(height, 10))
	}
	u.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queryResp ABCIQueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	response := &queryResp.Result.Response

	// Check for application-level errors
	if response.Code != 0 {
		return nil, &QueryError{
			Code:      response.Code,
			Log:       response.Log,
			Codespace: response.Codespace,
		}
	}

	return response, nil
}

// QueryJSON performs a query and unmarshals the JSON response into v.
func (c *Client) QueryJSON(ctx context.Context, path string, data []byte, height int64, v any) error {
	resp, err := c.Query(ctx, path, data, height)
	if err != nil {
		return err
	}

	if len(resp.Value) == 0 {
		return fmt.Errorf("empty response value")
	}

	if err := json.Unmarshal(resp.Value, v); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

// BroadcastTxSync broadcasts a transaction and waits for CheckTx result.
func (c *Client) BroadcastTxSync(ctx context.Context, tx []byte) (*BroadcastTxResponse, error) {
	return c.broadcastTx(ctx, "broadcast_tx_sync", tx)
}

// BroadcastTxAsync broadcasts a transaction without waiting for result.
func (c *Client) BroadcastTxAsync(ctx context.Context, tx []byte) (*BroadcastTxResponse, error) {
	return c.broadcastTx(ctx, "broadcast_tx_async", tx)
}

// BroadcastTxCommit broadcasts a transaction and waits for it to be committed.
func (c *Client) BroadcastTxCommit(ctx context.Context, tx []byte) (*BroadcastTxCommitResponse, error) {
	u, err := url.Parse(c.endpoint + "/broadcast_tx_commit")
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("tx", fmt.Sprintf("0x%s", hex.EncodeToString(tx)))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result BroadcastTxCommitResponse `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result.Result, nil
}

func (c *Client) broadcastTx(ctx context.Context, method string, tx []byte) (*BroadcastTxResponse, error) {
	u, err := url.Parse(c.endpoint + "/" + method)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("tx", fmt.Sprintf("0x%s", hex.EncodeToString(tx)))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result BroadcastTxResponse `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result.Result, nil
}

// BroadcastTxResponse contains the response from broadcast_tx_sync/async.
type BroadcastTxResponse struct {
	Code      uint32 `json:"code"`
	Data      []byte `json:"data"`
	Log       string `json:"log"`
	Codespace string `json:"codespace"`
	Hash      string `json:"hash"`
}

// BroadcastTxCommitResponse contains the response from broadcast_tx_commit.
type BroadcastTxCommitResponse struct {
	CheckTx   TxResponse `json:"check_tx"`
	DeliverTx TxResponse `json:"deliver_tx"`
	Hash      string     `json:"hash"`
	Height    string     `json:"height"`
}

// TxResponse contains transaction execution results.
type TxResponse struct {
	Code      uint32 `json:"code"`
	Data      []byte `json:"data"`
	Log       string `json:"log"`
	Info      string `json:"info"`
	GasWanted int64  `json:"gas_wanted"`
	GasUsed   int64  `json:"gas_used"`
	Events    []Event `json:"events"`
	Codespace string `json:"codespace"`
}

// Event represents an event emitted during transaction execution.
type Event struct {
	Type       string           `json:"type"`
	Attributes []EventAttribute `json:"attributes"`
}

// EventAttribute is a key-value pair in an event.
type EventAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Index bool   `json:"index"`
}

// Status returns the node status.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result StatusResponse `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result.Result, nil
}

// StatusResponse contains node status information.
type StatusResponse struct {
	NodeInfo      NodeInfo      `json:"node_info"`
	SyncInfo      SyncInfo      `json:"sync_info"`
	ValidatorInfo ValidatorInfo `json:"validator_info"`
}

// NodeInfo contains information about the node.
type NodeInfo struct {
	ProtocolVersion ProtocolVersion `json:"protocol_version"`
	ID              string          `json:"id"`
	ListenAddr      string          `json:"listen_addr"`
	Network         string          `json:"network"`
	Version         string          `json:"version"`
	Channels        string          `json:"channels"`
	Moniker         string          `json:"moniker"`
}

// ProtocolVersion contains protocol version information.
type ProtocolVersion struct {
	P2P   string `json:"p2p"`
	Block string `json:"block"`
	App   string `json:"app"`
}

// SyncInfo contains synchronization status.
type SyncInfo struct {
	LatestBlockHash     string    `json:"latest_block_hash"`
	LatestAppHash       string    `json:"latest_app_hash"`
	LatestBlockHeight   string    `json:"latest_block_height"`
	LatestBlockTime     time.Time `json:"latest_block_time"`
	EarliestBlockHash   string    `json:"earliest_block_hash"`
	EarliestAppHash     string    `json:"earliest_app_hash"`
	EarliestBlockHeight string    `json:"earliest_block_height"`
	EarliestBlockTime   time.Time `json:"earliest_block_time"`
	CatchingUp          bool      `json:"catching_up"`
}

// ValidatorInfo contains validator information for this node.
type ValidatorInfo struct {
	Address     string `json:"address"`
	PubKey      PubKey `json:"pub_key"`
	VotingPower string `json:"voting_power"`
}

// PubKey represents a public key.
type PubKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Health checks if the node is healthy.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/health", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("node unhealthy: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Block retrieves a block at the given height.
func (c *Client) Block(ctx context.Context, height int64) (*BlockResponse, error) {
	u, err := url.Parse(c.endpoint + "/block")
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	if height > 0 {
		q := u.Query()
		q.Set("height", strconv.FormatInt(height, 10))
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result BlockResponse `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result.Result, nil
}

// BlockResponse contains block data.
type BlockResponse struct {
	BlockID BlockID `json:"block_id"`
	Block   Block   `json:"block"`
}

// BlockID identifies a block.
type BlockID struct {
	Hash  string        `json:"hash"`
	Parts PartSetHeader `json:"parts"`
}

// PartSetHeader contains part set metadata.
type PartSetHeader struct {
	Total int    `json:"total"`
	Hash  string `json:"hash"`
}

// Block contains block data.
type Block struct {
	Header     BlockHeader `json:"header"`
	Data       BlockData   `json:"data"`
	Evidence   Evidence    `json:"evidence"`
	LastCommit Commit      `json:"last_commit"`
}

// BlockHeader contains block header fields.
type BlockHeader struct {
	Version            Version   `json:"version"`
	ChainID            string    `json:"chain_id"`
	Height             string    `json:"height"`
	Time               time.Time `json:"time"`
	LastBlockID        BlockID   `json:"last_block_id"`
	LastCommitHash     string    `json:"last_commit_hash"`
	DataHash           string    `json:"data_hash"`
	ValidatorsHash     string    `json:"validators_hash"`
	NextValidatorsHash string    `json:"next_validators_hash"`
	ConsensusHash      string    `json:"consensus_hash"`
	AppHash            string    `json:"app_hash"`
	LastResultsHash    string    `json:"last_results_hash"`
	EvidenceHash       string    `json:"evidence_hash"`
	ProposerAddress    string    `json:"proposer_address"`
}

// Version contains block version info.
type Version struct {
	Block string `json:"block"`
	App   string `json:"app"`
}

// BlockData contains transaction data.
type BlockData struct {
	Txs []string `json:"txs"`
}

// Evidence contains evidence of misbehavior.
type Evidence struct {
	Evidence []any `json:"evidence"`
}

// Commit contains the last commit info.
type Commit struct {
	Height     string      `json:"height"`
	Round      int         `json:"round"`
	BlockID    BlockID     `json:"block_id"`
	Signatures []Signature `json:"signatures"`
}

// Signature contains a commit signature.
type Signature struct {
	BlockIDFlag      int       `json:"block_id_flag"`
	ValidatorAddress string    `json:"validator_address"`
	Timestamp        time.Time `json:"timestamp"`
	Signature        string    `json:"signature"`
}

// encodeQueryData encodes query data for the URL.
func encodeQueryData(data any) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	switch v := data.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(data)
	}
}

