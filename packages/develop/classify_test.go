package develop

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/inference"
)

func TestKnownNarrowFixUsesDeterministicFastPath(t *testing.T) {
	c := ClassifyWork(WorkSignals{FilesLikelyAffected: 1, KnownAcceptanceTests: true, DeterministicFixKnown: true})
	if c.Outcome != "fast" || c.Tier != inference.D0 {
		t.Fatalf("expected D0 fast path, got %+v", c)
	}
}

func TestSecuritySensitiveWorkWithoutBaselineRoutesToGoals(t *testing.T) {
	c := ClassifyWork(WorkSignals{FilesLikelyAffected: 1, KnownAcceptanceTests: true, DeterministicFixKnown: true, SecuritySensitive: true})
	if c.Outcome != "goals" || c.Tier != inference.D2 {
		t.Fatalf("security-sensitive work without baseline must route through Goals, got %+v", c)
	}
}

func TestMaterialWorkReusesApplicableBaselineWithBoundedPlanning(t *testing.T) {
	c := ClassifyWork(WorkSignals{ArchitectureChange: true, CurrentGoalBaseline: true, BaselineApplicable: true})
	if c.Outcome != "plan" || c.Tier != inference.D1 {
		t.Fatalf("applicable baseline should avoid repeating deep project planning, got %+v", c)
	}
}

func TestStaleBaselineRoutesBackToGoals(t *testing.T) {
	c := ClassifyWork(WorkSignals{ArchitectureChange: true, CurrentGoalBaseline: true, BaselineApplicable: false})
	if c.Outcome != "goals" || c.Tier != inference.D2 {
		t.Fatalf("stale baseline must be reconsidered through Goals, got %+v", c)
	}
}

func TestBoundedTestableChangeAvoidsOpenReasoning(t *testing.T) {
	c := ClassifyWork(WorkSignals{FilesLikelyAffected: 5, KnownAcceptanceTests: true})
	if c.Outcome != "fast" || c.Tier != inference.D1 {
		t.Fatalf("bounded testable change should use D1 fast path, got %+v", c)
	}
}
