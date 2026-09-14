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
	for _, outcome := range []string{"ready", "ready", "ready", "ready"} {
		if err := restored.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	if restored.Stage != StageBaseline {
		t.Fatalf("expected baseline checkpoint, got %s", restored.Stage)
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
