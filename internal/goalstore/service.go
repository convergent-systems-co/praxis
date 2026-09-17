package goalstore

import (
	"context"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

type FinalizeRequest struct {
	Baseline  goals.GoalBaseline
	Review    goals.BaselineReviewReceipt
	Persist   bool
	CreatedAt time.Time
	ExpiresAt *time.Time
}

type FinalizeResult struct {
	Baseline  goals.GoalBaseline
	Persisted bool
}

// Finalize re-verifies an already reviewed candidate at the persistence or
// ephemeral completion boundary. Ephemeral mode never invokes durable storage.
func (r Repository) Finalize(ctx context.Context, req FinalizeRequest) (FinalizeResult, error) {
	if err := req.Baseline.Validate(); err != nil {
		return FinalizeResult{}, err
	}
	if req.Baseline.Digest == "" {
		return FinalizeResult{}, goals.ErrBaselineReviewEvidence
	}
	if err := req.Baseline.VerifyDigest(); err != nil {
		return FinalizeResult{}, err
	}
	if err := goals.VerifyBaselineReviewReceipt(req.Baseline, req.Review, true); err != nil {
		return FinalizeResult{}, err
	}
	baseline := req.Baseline
	if !req.Persist {
		return FinalizeResult{Baseline: baseline, Persisted: false}, nil
	}
	if r.Store == nil {
		return FinalizeResult{}, errors.New("durable Goal Baseline requested but no store is configured")
	}
	saved, err := r.Save(ctx, baseline, req.CreatedAt, req.ExpiresAt)
	if err != nil {
		return FinalizeResult{}, err
	}
	return FinalizeResult{Baseline: saved, Persisted: true}, nil
}
