package goalstore

import (
	"context"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

type FinalizeRequest struct {
	Baseline  goals.GoalBaseline
	Persist   bool
	CreatedAt time.Time
	ExpiresAt *time.Time
}

type FinalizeResult struct {
	Baseline  goals.GoalBaseline
	Persisted bool
}

// Finalize produces an exact digest-addressed Goal Baseline. Ephemeral mode
// never invokes durable storage; it is not implemented as persist-then-delete.
func (r Repository) Finalize(ctx context.Context, req FinalizeRequest) (FinalizeResult, error) {
	if err := req.Baseline.Validate(); err != nil {
		return FinalizeResult{}, err
	}
	digest, err := req.Baseline.ComputeDigest()
	if err != nil {
		return FinalizeResult{}, err
	}
	if req.Baseline.Digest != "" && req.Baseline.Digest != digest {
		return FinalizeResult{}, goals.ErrBaselineDigestMismatch
	}
	baseline := req.Baseline
	baseline.Digest = digest
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
