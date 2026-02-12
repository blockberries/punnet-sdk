package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/types"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:26657")
	if c.endpoint != "http://localhost:26657" {
		t.Errorf("expected endpoint http://localhost:26657, got %s", c.endpoint)
	}
	if c.httpClient.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", c.httpClient.Timeout)
	}
}

func TestNewWithOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 30 * time.Second}
	c := New("http://localhost:26657",
		WithTimeout(5*time.Second),
		WithHTTPClient(customClient),
	)
	if c.httpClient != customClient {
		t.Error("expected custom HTTP client")
	}
}

func TestQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/abci_query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		path := r.URL.Query().Get("path")
		if path != `"/auth/account"` {
			t.Errorf("unexpected path param: %s", path)
		}

		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code:   0,
					Value:  []byte(`{"name":"alice","nonce":5}`),
					Height: "100",
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	resp, err := c.Query(context.Background(), "/auth/account", []byte(`{"name":"alice"}`), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	if resp.Height != "100" {
		t.Errorf("expected height 100, got %s", resp.Height)
	}
}

func TestQueryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code: 1,
					Log:  "account not found",
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	_, err := c.Query(context.Background(), "/auth/account", nil, 0)
	if err == nil {
		t.Fatal("expected error")
	}

	qe, ok := err.(*QueryError)
	if !ok {
		t.Fatalf("expected QueryError, got %T", err)
	}
	if qe.Code != 1 {
		t.Errorf("expected code 1, got %d", qe.Code)
	}
	if qe.Log != "account not found" {
		t.Errorf("expected 'account not found', got %s", qe.Log)
	}
}

func TestQueryJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code:  0,
					Value: []byte(`{"account":"alice","denom":"stake","amount":1000}`),
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	var balance BalanceResponse
	err := c.QueryJSON(context.Background(), "/bank/balance", nil, 0, &balance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balance.Account != "alice" {
		t.Errorf("expected account alice, got %s", balance.Account)
	}
	if balance.Amount != 1000 {
		t.Errorf("expected amount 1000, got %d", balance.Amount)
	}
}

func TestBroadcastTxSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/broadcast_tx_sync" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		txHex := r.URL.Query().Get("tx")
		if txHex != "0x"+hex.EncodeToString([]byte("test-tx")) {
			t.Errorf("unexpected tx: %s", txHex)
		}

		resp := struct {
			Result BroadcastTxResponse `json:"result"`
		}{
			Result: BroadcastTxResponse{
				Code: 0,
				Hash: "ABC123",
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	resp, err := c.BroadcastTxSync(context.Background(), []byte("test-tx"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	if resp.Hash != "ABC123" {
		t.Errorf("expected hash ABC123, got %s", resp.Hash)
	}
}

func TestStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := struct {
			Result StatusResponse `json:"result"`
		}{
			Result: StatusResponse{
				NodeInfo: NodeInfo{
					Network: "test-chain",
					Moniker: "test-node",
				},
				SyncInfo: SyncInfo{
					LatestBlockHeight: "100",
					CatchingUp:        false,
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	status, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.NodeInfo.Network != "test-chain" {
		t.Errorf("expected network test-chain, got %s", status.NodeInfo.Network)
	}
	if status.SyncInfo.LatestBlockHeight != "100" {
		t.Errorf("expected height 100, got %s", status.SyncInfo.LatestBlockHeight)
	}
}

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL)
	err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHealthUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("service unavailable"))
	}))
	defer server.Close()

	c := New(server.URL)
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error for unhealthy node")
	}
}

func TestAuthClient_Account(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code:  0,
					Value: []byte(`{"name":"alice","nonce":10}`),
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	account, err := c.Auth().Account(context.Background(), types.AccountName("alice"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.Name != "alice" {
		t.Errorf("expected name alice, got %s", account.Name)
	}
	if account.Nonce != 10 {
		t.Errorf("expected nonce 10, got %d", account.Nonce)
	}
}

func TestAuthClient_InvalidAccount(t *testing.T) {
	c := New("http://localhost:26657")
	_, err := c.Auth().Account(context.Background(), types.AccountName(""))
	if err == nil {
		t.Fatal("expected error for invalid account name")
	}
}

func TestBankClient_Balance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code:  0,
					Value: []byte(`{"account":"alice","denom":"stake","amount":5000}`),
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	balance, err := c.Bank().Balance(context.Background(), types.AccountName("alice"), "stake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balance.Amount != 5000 {
		t.Errorf("expected amount 5000, got %d", balance.Amount)
	}
}

func TestStakingClient_Validator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code:  0,
					Value: []byte(`{"name":"validator1","voting_power":100,"status":"bonded"}`),
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	validator, err := c.Staking().Validator(context.Background(), types.AccountName("validator1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if validator.Name != "validator1" {
		t.Errorf("expected name validator1, got %s", validator.Name)
	}
	if validator.VotingPower != 100 {
		t.Errorf("expected voting power 100, got %d", validator.VotingPower)
	}
}

func TestGovernanceClient_Proposal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ABCIQueryResponse{
			Result: ABCIQueryResult{
				Response: ABCIResponse{
					Code:  0,
					Value: []byte(`{"id":1,"proposer":"alice","title":"Test Proposal","status":"voting"}`),
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	proposal, err := c.Governance().Proposal(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proposal.ID != 1 {
		t.Errorf("expected ID 1, got %d", proposal.ID)
	}
	if proposal.Title != "Test Proposal" {
		t.Errorf("expected title 'Test Proposal', got %s", proposal.Title)
	}
}

func TestGovernanceClient_InvalidProposal(t *testing.T) {
	c := New("http://localhost:26657")
	_, err := c.Governance().Proposal(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error for proposal ID 0")
	}
}

func TestEncodeQueryData(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantErr  bool
		validate func([]byte) bool
	}{
		{
			name:    "nil input",
			input:   nil,
			wantErr: false,
			validate: func(b []byte) bool {
				return b == nil
			},
		},
		{
			name:    "byte slice",
			input:   []byte("test"),
			wantErr: false,
			validate: func(b []byte) bool {
				return string(b) == "test"
			},
		},
		{
			name:    "string",
			input:   "hello",
			wantErr: false,
			validate: func(b []byte) bool {
				return string(b) == "hello"
			},
		},
		{
			name:    "map",
			input:   map[string]string{"key": "value"},
			wantErr: false,
			validate: func(b []byte) bool {
				var m map[string]string
				if err := json.Unmarshal(b, &m); err != nil {
					return false
				}
				return m["key"] == "value"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeQueryData(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("encodeQueryData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.validate(got) {
				t.Errorf("encodeQueryData() validation failed for %v", got)
			}
		})
	}
}

func TestQueryError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  QueryError
		want string
	}{
		{
			name: "with log",
			err:  QueryError{Code: 1, Log: "not found"},
			want: "query failed (code 1): not found",
		},
		{
			name: "without log",
			err:  QueryError{Code: 2},
			want: "query failed with code 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("QueryError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}
