package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// goalEstablishmentDocument is the supported external Goal admission
// contract. It deliberately excludes digest, predecessor, import provenance,
// and WorkPlan fields: Praxis derives identity evidence and only later
// lifecycle transitions may attach authority-bearing executable work.
type goalEstablishmentDocument struct {
	SchemaVersion      string                   `json:"schema_version"`
	GoalID             string                   `json:"goal_id"`
	OriginalIntent     string                   `json:"original_intent"`
	RefinedOutcome     string                   `json:"refined_outcome"`
	Scope              string                   `json:"scope,omitempty"`
	NonGoals           []string                 `json:"non_goals,omitempty"`
	Constraints        []string                 `json:"constraints,omitempty"`
	SuccessCriteria    []string                 `json:"success_criteria,omitempty"`
	EvidenceRefs       []string                 `json:"evidence_refs,omitempty"`
	Assumptions        []string                 `json:"assumptions,omitempty"`
	Artifacts          []goals.ArtifactRef      `json:"artifacts,omitempty"`
	PlanRef            string                   `json:"plan_ref,omitempty"`
	ValidityPredicates []string                 `json:"validity_predicates,omitempty"`
	Rigor              goals.Rigor              `json:"rigor"`
	RecommendationMode goals.RecommendationMode `json:"recommendation_mode"`
}

func decodeGoalEstablishment(body []byte) (goalEstablishmentDocument, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var document goalEstablishmentDocument
	if err := decoder.Decode(&document); err != nil {
		return goalEstablishmentDocument{}, fmt.Errorf("decode Goal establishment document: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return goalEstablishmentDocument{}, fmt.Errorf("decode Goal establishment document: %w", err)
	}
	if document.SchemaVersion != "1" {
		return goalEstablishmentDocument{}, errors.New("Goal establishment requires schema_version 1")
	}
	if document.GoalID == "" || strings.TrimSpace(document.GoalID) != document.GoalID {
		return goalEstablishmentDocument{}, errors.New("Goal establishment requires a stable goal_id")
	}
	if len(document.SuccessCriteria) == 0 {
		return goalEstablishmentDocument{}, errors.New("Goal establishment requires at least one success criterion")
	}
	for _, criterion := range document.SuccessCriteria {
		if strings.TrimSpace(criterion) == "" {
			return goalEstablishmentDocument{}, errors.New("Goal establishment success criteria must not be empty")
		}
	}
	return document, nil
}

func (d goalEstablishmentDocument) baseline(sourceRef string, source []byte) goals.GoalBaseline {
	sum := sha256.Sum256(source)
	return goals.GoalBaseline{
		ID: d.GoalID, Version: "1", OriginalIntent: d.OriginalIntent, RefinedOutcome: d.RefinedOutcome,
		Scope: d.Scope, NonGoals: append([]string(nil), d.NonGoals...), Constraints: append([]string(nil), d.Constraints...),
		SuccessCriteria: append([]string(nil), d.SuccessCriteria...), EvidenceRefs: append([]string(nil), d.EvidenceRefs...),
		Assumptions: append([]string(nil), d.Assumptions...), Artifacts: append([]goals.ArtifactRef(nil), d.Artifacts...), PlanRef: d.PlanRef,
		ValidityPredicates: append([]string(nil), d.ValidityPredicates...), Rigor: d.Rigor, RecommendationMode: d.RecommendationMode,
		ImportSourceRef: sourceRef, ImportSourceDigest: "sha256:" + hex.EncodeToString(sum[:]),
	}
}

// establishGoalBaseline admits generation 1 from an external outcome without
// asking the caller to manufacture canonical bytes, a digest, or persistence
// state. Replays are exact; changed content under the same identity is a
// conflict and must use a future explicit successor transition.
func establishGoalBaseline(ctx context.Context, repo goalstore.Repository, sourceRef string, body []byte, now time.Time) (goals.GoalBaseline, bool, error) {
	document, err := decodeGoalEstablishment(body)
	if err != nil {
		return goals.GoalBaseline{}, false, err
	}
	baseline := document.baseline(sourceRef, body)
	if err := baseline.Validate(); err != nil {
		return goals.GoalBaseline{}, false, fmt.Errorf("validate Goal establishment: %w", err)
	}
	digest, err := baseline.ComputeDigest()
	if err != nil {
		return goals.GoalBaseline{}, false, fmt.Errorf("digest Goal establishment: %w", err)
	}
	baseline.Digest = digest
	if existing, loadErr := repo.Load(ctx, baseline.ID, baseline.Version, now); loadErr == nil {
		if sameGoalEstablishment(existing, baseline) {
			return existing, true, nil
		}
		return goals.GoalBaseline{}, false, goalstore.ErrBaselineImportConflict
	} else if !errors.Is(loadErr, state.ErrSecureBlobNotFound) {
		return goals.GoalBaseline{}, false, fmt.Errorf("inspect existing Goal generation: %w", loadErr)
	}
	saved, err := repo.Save(ctx, baseline, now, nil)
	if err == nil {
		return saved, false, nil
	}
	// A concurrent exact establishment is an idempotent replay. A concurrent
	// different write remains a deterministic conflict.
	if existing, loadErr := repo.Load(ctx, baseline.ID, baseline.Version, now); loadErr == nil {
		if sameGoalEstablishment(existing, baseline) {
			return existing, true, nil
		}
		return goals.GoalBaseline{}, false, goalstore.ErrBaselineImportConflict
	}
	return goals.GoalBaseline{}, false, fmt.Errorf("persist established Goal: %w", err)
}

func sameGoalEstablishment(left, right goals.GoalBaseline) bool {
	return left.Digest == right.Digest && left.ImportSourceDigest == right.ImportSourceDigest
}
