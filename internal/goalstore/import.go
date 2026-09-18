package goalstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

var (
	ErrBaselineImportConflict = errors.New("imported Goal Baseline conflicts with an existing generation")
	ErrBaselineImportSource   = errors.New("Goal Baseline import source provenance is invalid")
)

// ImportBaselineRequest is the explicit evidence-to-authority boundary. The
// caller supplies a verified canonical GoalBaseline and the exact bytes from
// which it was read; repository location alone is never authority.
type ImportBaselineRequest struct {
	Baseline  goals.GoalBaseline
	SourceRef string
	Source    []byte
	CreatedAt time.Time
}

func (r Repository) ImportBaseline(ctx context.Context, req ImportBaselineRequest) (goals.GoalBaseline, error) {
	if req.SourceRef == "" || len(req.Source) == 0 {
		return goals.GoalBaseline{}, ErrBaselineImportSource
	}
	canonical, err := req.Baseline.CanonicalBytes()
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	if !bytes.Equal(req.Source, canonical) {
		return goals.GoalBaseline{}, fmt.Errorf("%w: source is not the canonical baseline payload", ErrBaselineImportSource)
	}
	sum := sha256.Sum256(canonical)
	sourceDigest := "sha256:" + hex.EncodeToString(sum[:])
	if req.Baseline.ImportSourceRef != req.SourceRef || req.Baseline.ImportSourceDigest != sourceDigest {
		return goals.GoalBaseline{}, fmt.Errorf("%w: baseline must bind the exact source bytes", ErrBaselineImportSource)
	}
	if err := req.Baseline.VerifyDigest(); err != nil {
		return goals.GoalBaseline{}, err
	}
	if req.Baseline.PredecessorDigest != "" {
		present, err := r.Store.HasSecureBlobDigest(ctx, baselineNamespace, req.Baseline.ID, req.Baseline.PredecessorDigest)
		if err != nil {
			return goals.GoalBaseline{}, err
		}
		if !present {
			return goals.GoalBaseline{}, fmt.Errorf("Goal Baseline predecessor %q is not present", req.Baseline.PredecessorDigest)
		}
	}
	if existing, err := r.Load(ctx, req.Baseline.ID, req.Baseline.Version, time.Now().UTC()); err == nil {
		if existing.Digest == req.Baseline.Digest {
			return existing, nil
		}
		return goals.GoalBaseline{}, ErrBaselineImportConflict
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) {
		return goals.GoalBaseline{}, fmt.Errorf("inspect existing Goal Baseline: %w", err)
	}
	return r.Save(ctx, req.Baseline, req.CreatedAt, nil)
}
