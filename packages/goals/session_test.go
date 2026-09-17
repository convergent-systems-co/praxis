package goals

import (
	"errors"
	"testing"
)

func advanceToCandidate(t *testing.T, s *Session) {
	t.Helper()
	for _, outcome := range []string{"captured", "rigorous", "ready", "calibrate", "review_all", "ready", "ready", "ready", "ready"} {
		if err := s.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	if s.Stage != StageCandidateBaseline {
		t.Fatalf("expected candidate stage, got %s", s.Stage)
	}
}

func attachCandidate(t *testing.T, s *Session, version, predecessor string) {
	t.Helper()
	b := s.Baseline
	b.Version = version
	b.PredecessorDigest = predecessor
	b.RefinedOutcome = "Produce a decision-ready research brief with explicit evidence criteria"
	b.Rigor = RigorRigorous
	b.RecommendationMode = RecommendationReviewAll
	b.PlanRef = "plan:research-brief"
	if err := s.SetBaseline(b); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance("digested"); err != nil {
		t.Fatal(err)
	}
}

func attachReview(t *testing.T, s *Session, requiresResolution bool) {
	t.Helper()
	if err := s.ReviewArchitecture(reviewRequestFixture(s.Baseline, requiresResolution)); err != nil {
		t.Fatal(err)
	}
}

func TestGoalSessionFirstRunOrdersSpecifyPlanCandidateThenReview(t *testing.T) {
	s, err := NewSession("session-first", "Help me make a reliable research plan")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReviewArchitecture(reviewRequestFixture(GoalBaseline{}, false)); !errors.Is(err, ErrInvalidSessionTransition) {
		t.Fatalf("pre-Specify review was accepted: %v", err)
	}
	advanceToCandidate(t, &s)
	if s.StageOutcomes[StageSpecify] != "ready" || s.StageOutcomes[StagePlan] != "ready" {
		t.Fatalf("Specify/Plan were not completed before candidate: %#v", s.StageOutcomes)
	}
	attachCandidate(t, &s, "1", "")
	if s.Stage != StageArchitectureReview || s.Baseline.Digest == "" {
		t.Fatalf("candidate was not digested before review: %+v", s)
	}
	attachReview(t, &s, false)
	if err := s.Advance("ready"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageBaseline {
		t.Fatalf("review did not lead to persistence boundary: %s", s.Stage)
	}
	if err := s.Advance("stored"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageComplete {
		t.Fatalf("session did not complete: %s", s.Stage)
	}
}

func TestGoalSessionSuccessorReviewSurvivesRestart(t *testing.T) {
	predecessor := reviewedBaselineFixture(t, "session-successor", "1", "")
	s, _ := NewSession("session-successor", "original")
	advanceToCandidate(t, &s)
	attachCandidate(t, &s, "2", predecessor.Digest)
	attachReview(t, &s, false)
	payload, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(payload)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Stage != StageArchitectureReview || restored.Baseline.PredecessorDigest != predecessor.Digest || restored.ReviewReceipt.ID == "" {
		t.Fatalf("restart lost successor review state: %+v", restored)
	}
	if err := restored.Advance("ready"); err != nil {
		t.Fatal(err)
	}
}

func TestGoalSessionReviewRequiredResolutionSurvivesRestart(t *testing.T) {
	s, _ := NewSession("session-review", "original")
	advanceToCandidate(t, &s)
	attachCandidate(t, &s, "1", "")
	attachReview(t, &s, true)
	if err := s.Advance("ready"); !errors.Is(err, ErrUnresolvedBaselineReview) {
		t.Fatalf("unresolved receipt bypassed decision: %v", err)
	}
	if err := s.Advance("review_required"); err != nil {
		t.Fatal(err)
	}
	payload, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := restored.Advance("resolved"); !errors.Is(err, ErrUnresolvedBaselineReview) {
		t.Fatalf("decision advanced without resolution evidence: %v", err)
	}
	if err := restored.ResolveArchitectureReview("decision:review/approved"); err != nil {
		t.Fatal(err)
	}
	if err := restored.Advance("resolved"); err != nil {
		t.Fatal(err)
	}
	if err := restored.Advance("ephemeral"); err != nil {
		t.Fatal(err)
	}
}

func TestGoalSessionRejectsAbsentStaleAndPostReviewMutation(t *testing.T) {
	s, _ := NewSession("session-reject", "original")
	advanceToCandidate(t, &s)
	attachCandidate(t, &s, "1", "")
	if err := s.Advance("ready"); err == nil {
		t.Fatal("absent receipt was accepted")
	}
	attachReview(t, &s, false)
	stale := s.ReviewReceipt
	stale.BaselineDigest = "sha256:stale"
	if err := s.SetArchitectureReview(stale); !errors.Is(err, ErrBaselineReviewEvidence) {
		t.Fatalf("stale receipt was accepted: %v", err)
	}
	if err := s.Advance("ready"); err != nil {
		t.Fatal(err)
	}
	mutated := s.Baseline
	mutated.RefinedOutcome = "mutated after review"
	if err := s.SetBaseline(mutated); !errors.Is(err, ErrInvalidSessionTransition) {
		t.Fatalf("post-review SetBaseline was accepted: %v", err)
	}
	s.Baseline.RefinedOutcome = "mutated after review"
	if err := s.Advance("stored"); !errors.Is(err, ErrBaselineDigestMismatch) {
		t.Fatalf("post-review mutation completed: %v", err)
	}
}

func TestGoalSessionDirectPathStillRequiresSpecifyPlanAndSingleReview(t *testing.T) {
	s, _ := NewSession("session-direct", "Rename a local note")
	if err := s.Advance("captured"); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance("direct"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageSpecify {
		t.Fatalf("direct path bypassed Specify: %s", s.Stage)
	}
	if err := s.Advance("ready"); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance("no_execution"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageCandidateBaseline {
		t.Fatalf("direct path bypassed candidate construction: %s", s.Stage)
	}
}

func TestGoalSessionPreservesBlockAndRejectsPostCompletionMutation(t *testing.T) {
	s, _ := NewSession("session-blocked", "Decide whether to publish")
	if err := s.Advance("blocked"); err != nil || s.Stage != StageFailed {
		t.Fatalf("blocked session was not failed: %s, %v", s.Stage, err)
	}
	s.Stage = StageComplete
	if err := s.Advance("anything"); !errors.Is(err, ErrSessionComplete) {
		t.Fatalf("got %v", err)
	}
}
