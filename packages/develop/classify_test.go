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

func TestSecuritySensitiveWorkEscalates(t *testing.T) {
	c := ClassifyWork(WorkSignals{FilesLikelyAffected: 1, KnownAcceptanceTests: true, DeterministicFixKnown: true, SecuritySensitive: true})
	if c.Outcome != "plan" || c.Tier != inference.D2 {
		t.Fatalf("security-sensitive work must escalate, got %+v", c)
	}
}

func TestBoundedTestableChangeAvoidsOpenReasoning(t *testing.T) {
	c := ClassifyWork(WorkSignals{FilesLikelyAffected: 5, KnownAcceptanceTests: true})
	if c.Outcome != "fast" || c.Tier != inference.D1 {
		t.Fatalf("bounded testable change should use D1 fast path, got %+v", c)
	}
}
