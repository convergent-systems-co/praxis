package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

type baselineImportDocument struct {
	SchemaVersion string             `json:"schema_version"`
	SourceRef     string             `json:"source_ref"`
	SourceDigest  string             `json:"source_digest"`
	Baseline      goals.GoalBaseline `json:"baseline"`
}

// importGoalBaseline is the ADR-068 / SPEC-031 evidence-to-authority
// boundary. Since the Goals command surface moved into the package, it is
// reached as the goals-lifecycle "import" operation: the document must carry
// schema_version 1, its own canonical absolute path as source_ref, and the
// SHA-256 of the baseline's canonical payload as source_digest. It admits only
// Goal state, never a WorkPlan, authority, or provider selection.
func importGoalBaseline(ctx context.Context, repo goalstore.Repository, path string, body []byte, now time.Time) (goals.GoalBaseline, error) {
	var doc baselineImportDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("decode Goal Baseline import: %w", err)
	}
	if doc.SchemaVersion != "1" || doc.SourceRef == "" || doc.SourceDigest == "" {
		return goals.GoalBaseline{}, errors.New("Goal Baseline import requires schema_version 1 and source provenance")
	}
	if doc.SourceRef != path {
		return goals.GoalBaseline{}, errors.New("Goal Baseline import source_ref must be the canonical absolute path")
	}
	doc.Baseline.ImportSourceRef, doc.Baseline.ImportSourceDigest = doc.SourceRef, doc.SourceDigest
	if err := doc.Baseline.Validate(); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("validate Goal Baseline import: %w", err)
	}
	canonical, err := doc.Baseline.CanonicalBytes()
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("canonicalize Goal Baseline import: %w", err)
	}
	imported, err := repo.ImportBaseline(ctx, goalstore.ImportBaselineRequest{Baseline: doc.Baseline, SourceRef: doc.SourceRef, Source: canonical, CreatedAt: now})
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("import Goal Baseline: %w", err)
	}
	return imported, nil
}

// intakeGoalBaseline is the prose front door to the same import boundary as
// importGoalBaseline (ADR-068, ADR-101). The Baseline is derived
// deterministically from the document, then admitted through
// Repository.ImportBaseline, so an exact repeat is idempotent, a differing
// document under the same generation fails closed, and nothing but Goal state
// is created. The document's own digest is bound into the Baseline as
// evidence; source_ref is the document's canonical absolute path.
func intakeGoalBaseline(ctx context.Context, repo goalstore.Repository, goalID, goalVersion, path string, document []byte, now time.Time) (goals.GoalBaseline, error) {
	baseline, err := goals.BaselineFromProse(goalID, goalVersion, document)
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("derive Goal Baseline from prose: %w", err)
	}
	canonical, err := baseline.CanonicalBytes()
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("canonicalize derived Goal Baseline: %w", err)
	}
	sum := sha256.Sum256(canonical)
	baseline.ImportSourceRef, baseline.ImportSourceDigest = path, "sha256:"+hex.EncodeToString(sum[:])
	if baseline.Digest, err = baseline.ComputeDigest(); err != nil {
		return goals.GoalBaseline{}, err
	}
	admitted, err := repo.ImportBaseline(ctx, goalstore.ImportBaselineRequest{Baseline: baseline, SourceRef: path, Source: canonical, CreatedAt: now})
	if err != nil {
		if errors.Is(err, goalstore.ErrBaselineImportConflict) {
			return goals.GoalBaseline{}, fmt.Errorf("intake Goal Baseline: Goal generation %s/%s already exists with different content; changed prose is a governed successor, never a mutation: %w", goalID, goalVersion, err)
		}
		return goals.GoalBaseline{}, fmt.Errorf("intake Goal Baseline: %w", err)
	}
	return admitted, nil
}
