package governance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestStateStore(t *testing.T) statestore.StateStore {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	return ss
}

func createTestModule(t *testing.T) (*BAPIGovernanceModule, *store.BAPIProposalStore, *store.BAPIBalanceStore) {
	stateStore := createTestStateStore(t)
	proposalStore := store.NewBAPIProposalStore(stateStore)
	balanceStore := store.NewBAPIBalanceStore(stateStore)

	mod, err := NewBAPIGovernanceModule(proposalStore, balanceStore)
	require.NoError(t, err)

	return mod, proposalStore, balanceStore
}

func createTestTxContext(account string, height uint64, blockTime time.Time) *runtime.BAPITxContext {
	return &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height: height,
			Time: types.Timestamp{
				Seconds: blockTime.Unix(),
				Nanos:   int32(blockTime.Nanosecond()),
			},
			ChainID: "test-chain",
		},
		Account: ptypes.AccountName(account),
		TxIndex: 0,
	}
}

func TestNewBAPIGovernanceModule(t *testing.T) {
	t.Run("creates module with valid stores", func(t *testing.T) {
		stateStore := createTestStateStore(t)
		proposalStore := store.NewBAPIProposalStore(stateStore)
		balanceStore := store.NewBAPIBalanceStore(stateStore)

		mod, err := NewBAPIGovernanceModule(proposalStore, balanceStore)
		require.NoError(t, err)
		assert.NotNil(t, mod)
		assert.Equal(t, ModuleName, mod.Name())
	})

	t.Run("fails with nil proposal store", func(t *testing.T) {
		stateStore := createTestStateStore(t)
		balanceStore := store.NewBAPIBalanceStore(stateStore)

		mod, err := NewBAPIGovernanceModule(nil, balanceStore)
		assert.Error(t, err)
		assert.Nil(t, mod)
	})

	t.Run("fails with nil balance store", func(t *testing.T) {
		stateStore := createTestStateStore(t)
		proposalStore := store.NewBAPIProposalStore(stateStore)

		mod, err := NewBAPIGovernanceModule(proposalStore, nil)
		assert.Error(t, err)
		assert.Nil(t, mod)
	})
}

func TestBAPIGovernanceModule_RegisterMsgHandlers(t *testing.T) {
	mod, _, _ := createTestModule(t)

	handlers := mod.RegisterMsgHandlers()
	assert.Contains(t, handlers, TypeMsgSubmitProposal)
	assert.Contains(t, handlers, TypeMsgVote)
	assert.Contains(t, handlers, TypeMsgDeposit)
}

func TestBAPIGovernanceModule_RegisterQueryHandlers(t *testing.T) {
	mod, _, _ := createTestModule(t)

	handlers := mod.RegisterQueryHandlers()
	assert.Contains(t, handlers, "/gov/proposal")
	assert.Contains(t, handlers, "/gov/params")
}

func TestBAPIGovernanceModule_SubmitProposal(t *testing.T) {
	ctx := context.Background()

	t.Run("submits proposal with initial deposit", func(t *testing.T) {
		mod, proposalStore, balanceStore := createTestModule(t)

		// Set up balance for proposer
		err := balanceStore.Set(ctx, "alice", "stake", 1000000)
		require.NoError(t, err)

		txCtx := createTestTxContext("alice", 100, time.Now())

		msg := &MsgSubmitProposal{
			Proposer:     ptypes.AccountName("alice"),
			Title:        "Test Proposal",
			Description:  "This is a test proposal",
			ProposalType: ProposalTypeText,
			InitialDeposit: ptypes.Coin{
				Denom:  "stake",
				Amount: 1000,
			},
		}

		handlers := mod.RegisterMsgHandlers()
		effects, err := handlers[TypeMsgSubmitProposal](ctx, txCtx, msg)
		require.NoError(t, err)
		assert.NotEmpty(t, effects)

		// Verify proposal was created
		proposal, err := proposalStore.GetProposal(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, "alice", proposal.Proposer)
		assert.Equal(t, "Test Proposal", proposal.Title)
		assert.Equal(t, string(StatusDeposit), proposal.Status)
		assert.Equal(t, uint64(1000), proposal.TotalDeposit)
	})

	t.Run("fails with insufficient balance", func(t *testing.T) {
		mod, _, balanceStore := createTestModule(t)

		// Set up insufficient balance
		err := balanceStore.Set(ctx, "alice", "stake", 100)
		require.NoError(t, err)

		txCtx := createTestTxContext("alice", 100, time.Now())

		msg := &MsgSubmitProposal{
			Proposer:     ptypes.AccountName("alice"),
			Title:        "Test Proposal",
			Description:  "This is a test proposal",
			ProposalType: ProposalTypeText,
			InitialDeposit: ptypes.Coin{
				Denom:  "stake",
				Amount: 1000000,
			},
		}

		handlers := mod.RegisterMsgHandlers()
		_, err = handlers[TypeMsgSubmitProposal](ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient")
	})

	t.Run("fails with wrong sender", func(t *testing.T) {
		mod, _, _ := createTestModule(t)

		txCtx := createTestTxContext("bob", 100, time.Now())

		msg := &MsgSubmitProposal{
			Proposer:     ptypes.AccountName("alice"),
			Title:        "Test Proposal",
			Description:  "This is a test proposal",
			ProposalType: ProposalTypeText,
			InitialDeposit: ptypes.Coin{
				Denom:  "stake",
				Amount: 0,
			},
		}

		handlers := mod.RegisterMsgHandlers()
		_, err := handlers[TypeMsgSubmitProposal](ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "proposer must be transaction account")
	})
}

func TestBAPIGovernanceModule_Vote(t *testing.T) {
	ctx := context.Background()

	t.Run("casts vote on voting proposal", func(t *testing.T) {
		mod, proposalStore, balanceStore := createTestModule(t)

		// Set up balance
		err := balanceStore.Set(ctx, "alice", "stake", 1000000)
		require.NoError(t, err)

		// Create proposal and move to voting period
		proposal := &store.BAPIProposal{
			ID:              1,
			Proposer:        "alice",
			Title:           "Test",
			Description:     "Test",
			Type:            "text",
			Status:          string(StatusVoting),
			TotalDeposit:    DefaultMinDeposit,
			DepositDenom:    "stake",
			SubmitTime:      time.Now().Unix(),
			VotingStartTime: time.Now().Unix(),
			VotingEndTime:   time.Now().Add(14 * 24 * time.Hour).Unix(),
		}
		err = proposalStore.SetProposal(ctx, proposal)
		require.NoError(t, err)

		txCtx := createTestTxContext("alice", 100, time.Now())

		msg := &MsgVote{
			Voter:      ptypes.AccountName("alice"),
			ProposalID: 1,
			Option:     VoteYes,
		}

		handlers := mod.RegisterMsgHandlers()
		effects, err := handlers[TypeMsgVote](ctx, txCtx, msg)
		require.NoError(t, err)
		assert.NotEmpty(t, effects)

		// Verify vote was recorded
		hasVote, err := proposalStore.HasVote(ctx, 1, "alice")
		require.NoError(t, err)
		assert.True(t, hasVote)
	})

	t.Run("fails on non-voting proposal", func(t *testing.T) {
		mod, proposalStore, _ := createTestModule(t)

		// Create proposal in deposit period
		proposal := &store.BAPIProposal{
			ID:           1,
			Proposer:     "alice",
			Title:        "Test",
			Description:  "Test",
			Type:         "text",
			Status:       string(StatusDeposit),
			TotalDeposit: 0,
			DepositDenom: "stake",
			SubmitTime:   time.Now().Unix(),
		}
		err := proposalStore.SetProposal(ctx, proposal)
		require.NoError(t, err)

		txCtx := createTestTxContext("alice", 100, time.Now())

		msg := &MsgVote{
			Voter:      ptypes.AccountName("alice"),
			ProposalID: 1,
			Option:     VoteYes,
		}

		handlers := mod.RegisterMsgHandlers()
		_, err = handlers[TypeMsgVote](ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in voting period")
	})

	t.Run("fails on double vote", func(t *testing.T) {
		mod, proposalStore, _ := createTestModule(t)

		// Create proposal in voting period
		proposal := &store.BAPIProposal{
			ID:              1,
			Proposer:        "alice",
			Title:           "Test",
			Description:     "Test",
			Type:            "text",
			Status:          string(StatusVoting),
			TotalDeposit:    DefaultMinDeposit,
			DepositDenom:    "stake",
			VotingStartTime: time.Now().Unix(),
			VotingEndTime:   time.Now().Add(14 * 24 * time.Hour).Unix(),
		}
		err := proposalStore.SetProposal(ctx, proposal)
		require.NoError(t, err)

		// Record first vote
		err = proposalStore.RecordVote(ctx, 1, "alice", "yes", 1, time.Now())
		require.NoError(t, err)

		txCtx := createTestTxContext("alice", 100, time.Now())

		msg := &MsgVote{
			Voter:      ptypes.AccountName("alice"),
			ProposalID: 1,
			Option:     VoteNo,
		}

		handlers := mod.RegisterMsgHandlers()
		_, err = handlers[TypeMsgVote](ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already voted")
	})
}

func TestBAPIGovernanceModule_Deposit(t *testing.T) {
	ctx := context.Background()

	t.Run("deposits on proposal", func(t *testing.T) {
		mod, proposalStore, balanceStore := createTestModule(t)

		// Set up balance
		err := balanceStore.Set(ctx, "bob", "stake", 1000000)
		require.NoError(t, err)

		// Create proposal in deposit period
		proposal := &store.BAPIProposal{
			ID:             1,
			Proposer:       "alice",
			Title:          "Test",
			Description:    "Test",
			Type:           "text",
			Status:         string(StatusDeposit),
			TotalDeposit:   1000,
			DepositDenom:   "stake",
			SubmitTime:     time.Now().Unix(),
			DepositEndTime: time.Now().Add(14 * 24 * time.Hour).Unix(),
		}
		err = proposalStore.SetProposal(ctx, proposal)
		require.NoError(t, err)

		txCtx := createTestTxContext("bob", 100, time.Now())

		msg := &MsgDeposit{
			Depositor:  ptypes.AccountName("bob"),
			ProposalID: 1,
			Amount: ptypes.Coin{
				Denom:  "stake",
				Amount: 5000,
			},
		}

		handlers := mod.RegisterMsgHandlers()
		effects, err := handlers[TypeMsgDeposit](ctx, txCtx, msg)
		require.NoError(t, err)
		assert.NotEmpty(t, effects)

		// Verify deposit was recorded
		deposit, err := proposalStore.GetDeposit(ctx, 1, "bob")
		require.NoError(t, err)
		assert.Equal(t, uint64(5000), deposit.Amount)
	})

	t.Run("transitions to voting when deposit threshold met", func(t *testing.T) {
		mod, proposalStore, balanceStore := createTestModule(t)
		mod.minDeposit = 10000

		// Set up balance
		err := balanceStore.Set(ctx, "bob", "stake", 100000000)
		require.NoError(t, err)

		// Create proposal with partial deposit
		proposal := &store.BAPIProposal{
			ID:             1,
			Proposer:       "alice",
			Title:          "Test",
			Description:    "Test",
			Type:           "text",
			Status:         string(StatusDeposit),
			TotalDeposit:   5000,
			DepositDenom:   "stake",
			SubmitTime:     time.Now().Unix(),
			DepositEndTime: time.Now().Add(14 * 24 * time.Hour).Unix(),
		}
		err = proposalStore.SetProposal(ctx, proposal)
		require.NoError(t, err)

		txCtx := createTestTxContext("bob", 100, time.Now())

		// Deposit enough to meet threshold
		msg := &MsgDeposit{
			Depositor:  ptypes.AccountName("bob"),
			ProposalID: 1,
			Amount: ptypes.Coin{
				Denom:  "stake",
				Amount: 5000, // Total will be 10000
			},
		}

		handlers := mod.RegisterMsgHandlers()
		effects, err := handlers[TypeMsgDeposit](ctx, txCtx, msg)
		require.NoError(t, err)
		assert.NotEmpty(t, effects)

		// Verify proposal transitioned to voting
		updatedProposal, err := proposalStore.GetProposal(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, string(StatusVoting), updatedProposal.Status)
		assert.Greater(t, updatedProposal.VotingStartTime, int64(0))
		assert.Greater(t, updatedProposal.VotingEndTime, int64(0))
	})

	t.Run("fails with wrong denomination", func(t *testing.T) {
		mod, proposalStore, balanceStore := createTestModule(t)

		// Set up balance
		err := balanceStore.Set(ctx, "bob", "atom", 1000000)
		require.NoError(t, err)

		// Create proposal
		proposal := &store.BAPIProposal{
			ID:             1,
			Proposer:       "alice",
			Title:          "Test",
			Description:    "Test",
			Type:           "text",
			Status:         string(StatusDeposit),
			TotalDeposit:   0,
			DepositDenom:   "stake",
			SubmitTime:     time.Now().Unix(),
			DepositEndTime: time.Now().Add(14 * 24 * time.Hour).Unix(),
		}
		err = proposalStore.SetProposal(ctx, proposal)
		require.NoError(t, err)

		txCtx := createTestTxContext("bob", 100, time.Now())

		msg := &MsgDeposit{
			Depositor:  ptypes.AccountName("bob"),
			ProposalID: 1,
			Amount: ptypes.Coin{
				Denom:  "atom",
				Amount: 5000,
			},
		}

		handlers := mod.RegisterMsgHandlers()
		_, err = handlers[TypeMsgDeposit](ctx, txCtx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid deposit denomination")
	})
}

func TestBAPIGovernanceModule_QueryProposal(t *testing.T) {
	ctx := context.Background()
	mod, proposalStore, _ := createTestModule(t)

	// Create proposal
	proposal := &store.BAPIProposal{
		ID:          1,
		Proposer:    "alice",
		Title:       "Test Proposal",
		Description: "Description",
		Type:        "text",
		Status:      "deposit",
	}
	err := proposalStore.SetProposal(ctx, proposal)
	require.NoError(t, err)

	handlers := mod.RegisterQueryHandlers()
	result, err := handlers["/gov/proposal"](ctx, []byte("1"), 0)
	require.NoError(t, err)

	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	assert.Equal(t, float64(1), response["id"])
	assert.Equal(t, "alice", response["proposer"])
	assert.Equal(t, "Test Proposal", response["title"])
}

func TestBAPIGovernanceModule_QueryParams(t *testing.T) {
	ctx := context.Background()
	mod, _, _ := createTestModule(t)

	handlers := mod.RegisterQueryHandlers()
	result, err := handlers["/gov/params"](ctx, nil, 0)
	require.NoError(t, err)

	var response map[string]interface{}
	err = json.Unmarshal(result, &response)
	require.NoError(t, err)

	assert.Equal(t, float64(DefaultMinDeposit), response["min_deposit"])
	assert.Equal(t, DefaultDepositDenom, response["deposit_denom"])
}

func TestBAPIGovernanceModule_Genesis(t *testing.T) {
	ctx := context.Background()

	t.Run("exports genesis", func(t *testing.T) {
		mod, _, _ := createTestModule(t)

		data, err := mod.ExportGenesis(ctx)
		require.NoError(t, err)

		var state GovernanceGenesisState
		err = json.Unmarshal(data, &state)
		require.NoError(t, err)

		assert.Equal(t, DefaultMinDeposit, state.Params.MinDeposit)
	})

	t.Run("imports genesis", func(t *testing.T) {
		mod, _, _ := createTestModule(t)

		state := GovernanceGenesisState{
			Params: GenesisParams{
				MinDeposit:        5000000,
				DepositPeriodSecs: 604800,
				VotingPeriodSecs:  604800,
				Quorum:            4000,
				Threshold:         6000,
				VetoThreshold:     3000,
				DepositDenom:      "atom",
			},
		}

		data, err := json.Marshal(state)
		require.NoError(t, err)

		err = mod.InitGenesis(ctx, data)
		require.NoError(t, err)

		assert.Equal(t, uint64(5000000), mod.minDeposit)
		assert.Equal(t, "atom", mod.depositDenom)
		assert.Equal(t, uint32(4000), mod.quorum)
	})

	t.Run("handles empty genesis", func(t *testing.T) {
		mod, _, _ := createTestModule(t)

		err := mod.InitGenesis(ctx, nil)
		require.NoError(t, err)

		// Should keep defaults
		assert.Equal(t, DefaultMinDeposit, mod.minDeposit)
	})
}

func TestBAPIGovernanceModule_BlockLifecycle(t *testing.T) {
	ctx := context.Background()
	mod, _, _ := createTestModule(t)

	blockCtx := &runtime.BAPIBlockContext{
		Height:  100,
		ChainID: "test-chain",
	}

	t.Run("begin block returns empty effects", func(t *testing.T) {
		effects, err := mod.BeginBlock(ctx, blockCtx)
		require.NoError(t, err)
		assert.Empty(t, effects)
	})

	t.Run("end block returns empty updates", func(t *testing.T) {
		effects, updates, err := mod.EndBlock(ctx, blockCtx)
		require.NoError(t, err)
		assert.Empty(t, effects)
		assert.Empty(t, updates)
	})
}

func TestMsgSubmitProposal_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgSubmitProposal
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid message",
			msg: &MsgSubmitProposal{
				Proposer:     ptypes.AccountName("alice"),
				Title:        "Test",
				Description:  "Description",
				ProposalType: ProposalTypeText,
				InitialDeposit: ptypes.Coin{
					Denom:  "stake",
					Amount: 100,
				},
			},
			wantErr: false,
		},
		{
			name: "empty proposer",
			msg: &MsgSubmitProposal{
				Proposer:     ptypes.AccountName(""),
				Title:        "Test",
				Description:  "Description",
				ProposalType: ProposalTypeText,
			},
			wantErr: true,
			errMsg:  "invalid proposer",
		},
		{
			name: "empty title",
			msg: &MsgSubmitProposal{
				Proposer:     ptypes.AccountName("alice"),
				Title:        "",
				Description:  "Description",
				ProposalType: ProposalTypeText,
			},
			wantErr: true,
			errMsg:  "title cannot be empty",
		},
		{
			name: "empty description",
			msg: &MsgSubmitProposal{
				Proposer:     ptypes.AccountName("alice"),
				Title:        "Test",
				Description:  "",
				ProposalType: ProposalTypeText,
			},
			wantErr: true,
			errMsg:  "description cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMsgVote_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgVote
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid message",
			msg: &MsgVote{
				Voter:      ptypes.AccountName("alice"),
				ProposalID: 1,
				Option:     VoteYes,
			},
			wantErr: false,
		},
		{
			name: "zero proposal ID",
			msg: &MsgVote{
				Voter:      ptypes.AccountName("alice"),
				ProposalID: 0,
				Option:     VoteYes,
			},
			wantErr: true,
			errMsg:  "proposal ID cannot be zero",
		},
		{
			name: "invalid vote option",
			msg: &MsgVote{
				Voter:      ptypes.AccountName("alice"),
				ProposalID: 1,
				Option:     VoteOption("invalid"),
			},
			wantErr: true,
			errMsg:  "invalid vote option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMsgDeposit_ValidateBasic(t *testing.T) {
	tests := []struct {
		name    string
		msg     *MsgDeposit
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid message",
			msg: &MsgDeposit{
				Depositor:  ptypes.AccountName("alice"),
				ProposalID: 1,
				Amount:     ptypes.Coin{Denom: "stake", Amount: 100},
			},
			wantErr: false,
		},
		{
			name: "zero proposal ID",
			msg: &MsgDeposit{
				Depositor:  ptypes.AccountName("alice"),
				ProposalID: 0,
				Amount:     ptypes.Coin{Denom: "stake", Amount: 100},
			},
			wantErr: true,
			errMsg:  "proposal ID cannot be zero",
		},
		{
			name: "zero amount",
			msg: &MsgDeposit{
				Depositor:  ptypes.AccountName("alice"),
				ProposalID: 1,
				Amount:     ptypes.Coin{Denom: "stake", Amount: 0},
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.ValidateBasic()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
