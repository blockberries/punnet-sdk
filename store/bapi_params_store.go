package store

import (
	"context"
	"fmt"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
)

// BAPIParamsStore provides typed access to consensus parameters.
type BAPIParamsStore struct {
	*TypedStore[*types.ConsensusParams]
}

// NewBAPIParamsStore creates a new params store backed by blockberry's StateStore.
func NewBAPIParamsStore(store statestore.StateStore) *BAPIParamsStore {
	return &BAPIParamsStore{
		TypedStore: NewTypedStore[*types.ConsensusParams](store, "params/"),
	}
}

const currentParamsKey = "current"

// Get retrieves the current consensus parameters.
func (s *BAPIParamsStore) Get(ctx context.Context) (*types.ConsensusParams, error) {
	return s.TypedStore.Get(ctx, currentParamsKey)
}

// Set stores the consensus parameters.
func (s *BAPIParamsStore) Set(ctx context.Context, params *types.ConsensusParams) error {
	if params == nil {
		return ErrInvalidValue
	}

	// Validate params
	if params.MaxBlockBytes == 0 {
		return fmt.Errorf("MaxBlockBytes cannot be zero")
	}
	if params.MaxTxBytes == 0 {
		return fmt.Errorf("MaxTxBytes cannot be zero")
	}
	if params.MaxTxBytes > params.MaxBlockBytes {
		return fmt.Errorf("MaxTxBytes cannot exceed MaxBlockBytes")
	}

	return s.TypedStore.Set(ctx, currentParamsKey, params)
}

// GetWithProof retrieves the consensus parameters with a Merkle proof.
func (s *BAPIParamsStore) GetWithProof(ctx context.Context) (*types.ConsensusParams, *statestore.Proof, error) {
	return s.TypedStore.GetWithProof(ctx, currentParamsKey)
}

// GetAtHeight retrieves the consensus parameters at a specific block height.
func (s *BAPIParamsStore) GetAtHeight(ctx context.Context, height int64) (*types.ConsensusParams, error) {
	return s.TypedStore.GetAtHeight(ctx, currentParamsKey, height)
}

// UpdateMaxBlockBytes updates only the MaxBlockBytes parameter.
func (s *BAPIParamsStore) UpdateMaxBlockBytes(ctx context.Context, maxBlockBytes uint64) error {
	params, err := s.Get(ctx)
	if err != nil {
		return fmt.Errorf("get params: %w", err)
	}

	if maxBlockBytes == 0 {
		return fmt.Errorf("MaxBlockBytes cannot be zero")
	}
	if params.MaxTxBytes > maxBlockBytes {
		return fmt.Errorf("MaxBlockBytes cannot be less than MaxTxBytes")
	}

	params.MaxBlockBytes = maxBlockBytes
	return s.Set(ctx, params)
}

// UpdateMaxTxBytes updates only the MaxTxBytes parameter.
func (s *BAPIParamsStore) UpdateMaxTxBytes(ctx context.Context, maxTxBytes uint64) error {
	params, err := s.Get(ctx)
	if err != nil {
		return fmt.Errorf("get params: %w", err)
	}

	if maxTxBytes == 0 {
		return fmt.Errorf("MaxTxBytes cannot be zero")
	}
	if maxTxBytes > params.MaxBlockBytes {
		return fmt.Errorf("MaxTxBytes cannot exceed MaxBlockBytes")
	}

	params.MaxTxBytes = maxTxBytes
	return s.Set(ctx, params)
}

// UpdateMaxEvidenceAge updates only the MaxEvidenceAge parameter.
func (s *BAPIParamsStore) UpdateMaxEvidenceAge(ctx context.Context, maxEvidenceAge types.Duration) error {
	params, err := s.Get(ctx)
	if err != nil {
		return fmt.Errorf("get params: %w", err)
	}

	params.MaxEvidenceAge = maxEvidenceAge
	return s.Set(ctx, params)
}

// DefaultConsensusParams returns default consensus parameters.
func DefaultConsensusParams() *types.ConsensusParams {
	return &types.ConsensusParams{
		MaxBlockBytes:  22020096,                               // 21 MB
		MaxTxBytes:     1048576,                                // 1 MB
		MaxEvidenceAge: types.Duration{Nanos: 172800000000000}, // 48 hours in nanoseconds
	}
}
