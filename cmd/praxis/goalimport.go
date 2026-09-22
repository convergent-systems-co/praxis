package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
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
	if err := contracts.UnmarshalExactJSON(body, &doc, false); err != nil {
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
