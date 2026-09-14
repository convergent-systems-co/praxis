package goals

import (
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

var ErrBaselineReviewEvidence = errors.New("architecture review must bind to the persisted Goal Baseline")

// ReviewBaselineOwnership connects the advisory inversion review to a Goal
// Baseline without turning the baseline or the result into authority. The
// baseline must already have an immutable digest, and the request must carry
// that exact digest as goal evidence.
func ReviewBaselineOwnership(b GoalBaseline, req architecturereview.Request) (architecturereview.Result, error) {
	if err := b.Validate(); err != nil {
		return architecturereview.Result{}, err
	}
	if b.Digest == "" {
		return architecturereview.Result{}, ErrBaselineReviewEvidence
	}
	if err := b.VerifyDigest(); err != nil {
		return architecturereview.Result{}, err
	}
	wantID := fmt.Sprintf("baseline:%s@%s", b.ID, b.Version)
	bound := false
	for _, ref := range req.GoalEvidence {
		if ref.ID == wantID && ref.Digest == b.Digest {
			bound = true
			break
		}
	}
	if !bound {
		return architecturereview.Result{}, ErrBaselineReviewEvidence
	}
	return architecturereview.Review(req)
}
