package goals

import (
	"errors"
	"testing"
)

func TestGoalSessionSnapshotsAndResumesInteractiveProgress(t *testing.T) {
	s, err := NewSession("session-1", "Help me make a reliable research plan")
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"captured", "rigorous", "ready", "calibrate", "review_all"} {
		if err := s.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	checkpoint, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Stage != StageDecide || restored.Baseline.OriginalIntent != "Help me make a reliable research plan" {
		t.Fatalf("resume lost interactive state: %+v", restored)
	}
	if restored.StageOutcomes[StageCalibrate] != "review_all" {
		t.Fatalf("resume lost responsibility evidence: %#v", restored.StageOutcomes)
	}
	for _, outcome := range []string{"ready", "ready", "ready", "ready"} {
		if err := restored.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	if restored.Stage != StageBaseline {
		t.Fatalf("expected baseline checkpoint, got %s", restored.Stage)
	}
}

func TestGoalSessionCompletionRequiresDigestBoundBaseline(t *testing.T) {
	s, err := NewSession("session-complete", "Produce a decision-ready research brief")
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"captured", "rigorous", "ready", "calibrate", "review_all", "ready", "ready", "ready", "ready"} {
		if err := s.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	if s.Stage != StageBaseline {
		t.Fatalf("expected baseline stage, got %s", s.Stage)
	}
	if err := s.Advance("stored"); err == nil {
		t.Fatal("incomplete baseline must not complete the session")
	}
	if s.Stage != StageBaseline {
		t.Fatalf("failed baseline finalization changed stage: %s", s.Stage)
	}
	if _, recorded := s.StageOutcomes[StageBaseline]; recorded {
		t.Fatal("failed baseline finalization recorded an uncommitted outcome")
	}

	s.Baseline.RefinedOutcome = "Produce a decision-ready research brief with explicit evidence criteria"
	s.Baseline.Rigor = RigorRigorous
	s.Baseline.RecommendationMode = RecommendationReviewAll
	if err := s.Advance("stored"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageComplete || s.Baseline.Digest == "" {
		t.Fatalf("completion did not bind baseline digest: %+v", s)
	}
	if err := s.Baseline.VerifyDigest(); err != nil {
		t.Fatal(err)
	}
}

func TestGoalSessionSetBaselineRejectsStaleDigest(t *testing.T) {
	s, _ := NewSession("session-digest", "original")
	s.Stage = StageBaseline
	b := s.Baseline
	b.RefinedOutcome = "outcome"
	b.Rigor = RigorStructured
	b.RecommendationMode = RecommendationReviewAll
	digest, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	b.Digest = digest
	b.RefinedOutcome = "mutated after digest"
	if err := s.SetBaseline(b); err != ErrBaselineDigestMismatch {
		t.Fatalf("expected stale digest rejection, got %v", err)
	}
}

func TestGoalSessionRestoreRejectsMutatedCompletedBaseline(t *testing.T) {
	s, _ := NewSession("session-restore", "original")
	s.Stage = StageComplete
	s.Baseline.RefinedOutcome = "outcome"
	s.Baseline.Rigor = RigorStructured
	s.Baseline.RecommendationMode = RecommendationReviewAll
	digest, err := s.Baseline.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	s.Baseline.Digest = digest
	payload, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload[:len(payload)-2], []byte(`"x"}`)...)
	if _, err := RestoreSession(payload); err == nil {
		t.Fatal("mutated completed baseline must fail closed")
	}
}

func TestGoalSessionDirectPathSkipsHeavyweightStages(t *testing.T) {
	s, err := NewSession("session-direct", "Rename a local note")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Advance("captured"); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance("direct"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageBaseline {
		t.Fatalf("direct path should reach baseline, got %s", s.Stage)
	}
}

func TestGoalSessionPreservesBlockAndRejectsPostCompletionMutation(t *testing.T) {
	s, err := NewSession("session-blocked", "Decide whether to publish")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Advance("blocked"); err != nil {
		t.Fatal(err)
	}
	if s.Stage != StageFailed {
		t.Fatalf("blocked session must remain failed, got %s", s.Stage)
	}
	s.Stage = StageComplete
	if err := s.Advance("anything"); !errors.Is(err, ErrSessionComplete) {
		t.Fatalf("got %v", err)
	}
}

func TestGoalSessionBaselineCannotChangeOriginalIntent(t *testing.T) {
	s, _ := NewSession("session-baseline", "original")
	s.Stage = StageBaseline
	b := s.Baseline
	b.RefinedOutcome = "outcome"
	b.Rigor = RigorStructured
	b.RecommendationMode = RecommendationReviewAll
	b.OriginalIntent = "mutated"
	if err := s.SetBaseline(b); !errors.Is(err, ErrInvalidSessionTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestGoalSessionCompilesFourMateriallyDifferentDomainBaselines(t *testing.T) {
	fixtures := []struct {
		name         string
		intent       string
		outcome      string
		artifactID   string
		artifactRole string
	}{
		{name: "software architecture", intent: "Make a service reliable", outcome: "A bounded service architecture", artifactID: "adr-auth", artifactRole: "decision-record"},
		{name: "structured research", intent: "Understand a changing ecosystem", outcome: "A reproducible research program", artifactID: "evidence-map", artifactRole: "evidence-map"},
		{name: "operational planning", intent: "Make weekday mornings predictable", outcome: "A low-conflict household routine", artifactID: "morning-routine", artifactRole: "routine-map"},
		{name: "writing production", intent: "Explain this idea clearly", outcome: "A publishable explanatory brief", artifactID: "brief-outline", artifactRole: "content-model"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			session, err := NewSession(fixture.name, fixture.intent)
			if err != nil {
				t.Fatal(err)
			}
			for _, outcome := range []string{"captured", "rigorous", "ready", "calibrate", "review_all", "ready", "ready", "ready", "ready"} {
				if err := session.Advance(outcome); err != nil {
					t.Fatal(err)
				}
			}
			if session.Stage != StageBaseline {
				t.Fatalf("expected baseline stage, got %s", session.Stage)
			}
			session.Baseline.RefinedOutcome = fixture.outcome
			session.Baseline.Rigor = RigorRigorous
			session.Baseline.RecommendationMode = RecommendationReviewAll
			session.Baseline.Artifacts = []ArtifactRef{{ID: fixture.artifactID, Role: fixture.artifactRole, Digest: "sha256:domain-fixture"}}
			session.Baseline.PlanRef = fixture.name + "-plan"
			if err := session.Advance("stored"); err != nil {
				t.Fatal(err)
			}
			if session.Stage != StageComplete || session.Baseline.Digest == "" {
				t.Fatalf("domain baseline was not completed with a digest: %+v", session)
			}
			if session.Baseline.OriginalIntent != fixture.intent || len(session.Baseline.Artifacts) != 1 || session.Baseline.Artifacts[0].Role != fixture.artifactRole {
				t.Fatalf("domain materialization lost intent or artifact semantics: %+v", session.Baseline)
			}
			if err := session.Baseline.VerifyDigest(); err != nil {
				t.Fatal(err)
			}
			if err := session.Advance("anything"); !errors.Is(err, ErrSessionComplete) {
				t.Fatalf("completed baseline unexpectedly accepted mutation: %v", err)
			}
		})
	}
}
