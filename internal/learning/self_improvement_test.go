package learning

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/conformance"
)

func TestPraxisPlanningSelfImprovementConsumesFrozenBlindReport(t *testing.T) {
	b, err := os.ReadFile("../../docs/research/conformance/blind-source-qualified.json")
	if err != nil {
		t.Fatal(err)
	}
	var frozen conformance.Result
	if err := json.Unmarshal(b, &frozen); err != nil {
		t.Fatal(err)
	}
	if err := conformance.VerifyFrozen(frozen); err != nil {
		t.Fatal(err)
	}
	active, _ := NewPlanningGeneration("", []string{"derive-plan", "implement-plan", "verify-plan"}, false, "")
	record, err := CandidateFromConformance("praxis-planning-candidate", active.ID, "original-plus-regression-replay", active.ID, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.SourceObservations) < 20 {
		t.Fatalf("candidate did not consume the whole frozen finding set: %d observations", len(record.SourceObservations))
	}
	candidate, err := ProposePlanningGeneration(record, active, frozen)
	if err != nil {
		t.Fatal(err)
	}
	var original, plan, supported []string
	for _, finding := range frozen.Findings {
		original = append(original, finding.ClaimID)
		if finding.Status == conformance.Satisfied {
			plan = append(plan, finding.ClaimID)
			supported = append(supported, finding.ClaimID)
		}
	}
	evaluation, err := EvaluatePlanningCandidate(active, candidate, frozen, []PlanningReplayScenario{
		{ID: "praxis-original-planning-replay", OriginalClaimIDs: original, PlanClaimIDs: plan, SupportedClaimIDs: supported},
		{ID: "independent-complete-regression", OriginalClaimIDs: []string{"unrelated-one", "unrelated-two"}, PlanClaimIDs: []string{"unrelated-one", "unrelated-two"}, SupportedClaimIDs: []string{"unrelated-one", "unrelated-two"}},
	}, []Evidence{{ID: "blind-run", SourceID: frozen.Digest, CausationRoot: frozen.GoalDigest}, {ID: "regression-run", SourceID: "independent-corpus", CausationRoot: "regression-corpus"}})
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Passed || len(evaluation.CandidateResults[0].DetectedGapIDs) != len(record.SourceObservations) {
		t.Fatalf("actual frozen findings were not improved: %#v", evaluation)
	}
}

func frozenPlanningFailure(t *testing.T) conformance.Result {
	t.Helper()
	r, err := conformance.Evaluate("sha256:original-goal", []conformance.Claim{
		{ID: "opaque-alpha", Statement: "first independently derived property", Behavioral: true, Critical: true, SourceRef: "original:1"},
		{ID: "opaque-beta", Statement: "second independently derived property", Behavioral: true, Critical: true, SourceRef: "original:2"},
	}, nil, time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPlanningProcessLearnsFromFrozenConformanceWithoutKnownAnswer(t *testing.T) {
	frozen := frozenPlanningFailure(t)
	active, err := NewPlanningGeneration("", []string{"derive-plan", "implement-plan", "verify-plan"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	candidateRecord, err := CandidateFromConformance("candidate", active.ID, "planning-replay-v1", active.ID, frozen)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ProposePlanningGeneration(candidateRecord, active, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.ID == active.ID || candidate.ParentID != active.ID {
		t.Fatal("active generation was mutated instead of creating a child")
	}

	scenarios := []PlanningReplayScenario{
		{ID: "original-plan-satisfied-itself", OriginalClaimIDs: []string{"opaque-alpha", "opaque-beta"}, PlanClaimIDs: []string{"opaque-alpha"}, SupportedClaimIDs: []string{"opaque-alpha"}},
		{ID: "complete-regression", OriginalClaimIDs: []string{"regression-x"}, PlanClaimIDs: []string{"regression-x"}, SupportedClaimIDs: []string{"regression-x"}},
		{ID: "unrelated-omission-regression", OriginalClaimIDs: []string{"unrelated-gamma", "unrelated-delta"}, PlanClaimIDs: []string{"unrelated-gamma"}, SupportedClaimIDs: []string{"unrelated-gamma"}},
	}
	evaluation, err := EvaluatePlanningCandidate(active, candidate, frozen, scenarios, []Evidence{
		{ID: "run-a", SourceID: "original-replay", CausationRoot: "goal-baseline"},
		{ID: "run-b", SourceID: "regression-replay", CausationRoot: "regression-corpus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Passed || !evaluation.ImprovedCoverage || evaluation.RegressionCount != 0 {
		t.Fatalf("candidate did not improve general original-goal coverage: %#v", evaluation)
	}
	if got := evaluation.CandidateResults[0].DetectedGapIDs; len(got) != 1 || got[0] != "opaque-beta" {
		t.Fatalf("candidate failed generic omission discovery: %#v", got)
	}
	if got := evaluation.CandidateResults[2].DetectedGapIDs; len(got) != 1 || got[0] != "unrelated-delta" {
		t.Fatalf("candidate overfit the first scenario: %#v", got)
	}

	registry := NewGenerationRegistry(active)
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	selfDecision := GovernanceDecision{AuthorityID: "learner", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, selfDecision, "learner"); err == nil {
		t.Fatal("candidate promoted itself")
	}
	decision := GovernanceDecision{AuthorityID: "governance-policy", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, decision, "learner"); err != nil {
		t.Fatal(err)
	}
	if registry.Active().ID != candidate.ID || registry.RollbackIdentity() != active.ID {
		t.Fatal("promotion did not retain rollback identity")
	}
	if err := registry.Rollback(); err != nil {
		t.Fatal(err)
	}
	if registry.Active().ID != active.ID {
		t.Fatal("rollback did not restore the prior generation")
	}
}

func TestFailedCandidateRemainsRegisteredEvidence(t *testing.T) {
	frozen := frozenPlanningFailure(t)
	active, _ := NewPlanningGeneration("", []string{"plan", "complete"}, false, "")
	record, _ := CandidateFromConformance("candidate", active.ID, "replay", active.ID, frozen)
	candidate, _ := ProposePlanningGeneration(record, active, frozen)
	registry := NewGenerationRegistry(active)
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	evaluation, err := EvaluatePlanningCandidate(active, candidate, frozen, []PlanningReplayScenario{
		{ID: "a", OriginalClaimIDs: []string{"a"}, PlanClaimIDs: []string{"a"}, SupportedClaimIDs: []string{"a"}, SecurityViolations: 1},
		{ID: "b", OriginalClaimIDs: []string{"b"}, PlanClaimIDs: []string{"b"}, SupportedClaimIDs: []string{"b"}},
	}, []Evidence{{ID: "a", SourceID: "a", CausationRoot: "a"}, {ID: "b", SourceID: "b", CausationRoot: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Passed {
		t.Fatal("security regression must fail candidate")
	}
	decision := GovernanceDecision{AuthorityID: "governance", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, decision, "learner"); err == nil {
		t.Fatal("failed candidate promoted")
	}
	if !registry.HasGeneration(candidate.ID) {
		t.Fatal("failed candidate was silently discarded")
	}
}

func TestGenerationPromotionAndRollbackSurviveRestart(t *testing.T) {
	frozen := frozenPlanningFailure(t)
	active, _ := NewPlanningGeneration("", []string{"plan", "complete"}, false, "")
	record, _ := CandidateFromConformance("candidate", active.ID, "replay", active.ID, frozen)
	candidate, _ := ProposePlanningGeneration(record, active, frozen)
	scenarios := []PlanningReplayScenario{
		{ID: "omission", OriginalClaimIDs: []string{"a", "b"}, PlanClaimIDs: []string{"a"}, SupportedClaimIDs: []string{"a"}},
		{ID: "regression", OriginalClaimIDs: []string{"x"}, PlanClaimIDs: []string{"x"}, SupportedClaimIDs: []string{"x"}},
	}
	evaluation, err := EvaluatePlanningCandidate(active, candidate, frozen, scenarios, []Evidence{{ID: "a", SourceID: "a", CausationRoot: "a"}, {ID: "b", SourceID: "b", CausationRoot: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/generations.json"
	registry, err := OpenGenerationRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	decision := GovernanceDecision{AuthorityID: "governance", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, decision, "learner"); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenGenerationRegistry(path, PlanningGeneration{})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Active().ID != candidate.ID || restarted.RollbackIdentity() != active.ID {
		t.Fatal("promotion or rollback identity was lost across restart")
	}
	if err := restarted.Rollback(); err != nil {
		t.Fatal(err)
	}
	restartedAgain, err := OpenGenerationRegistry(path, PlanningGeneration{})
	if err != nil {
		t.Fatal(err)
	}
	if restartedAgain.Active().ID != active.ID || !restartedAgain.HasGeneration(candidate.ID) {
		t.Fatal("rollback or candidate evidence was lost across restart")
	}
}
