package adaptation

import (
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func adaptiveFixture(t *testing.T, id, root string, tokens int) Observation {
	t.Helper()
	observation, err := FreezeObservation(Observation{SubjectAgentID: "agent", RunID: "run-" + id, GoalClass: "goal", Domain: "generic", BehaviorKey: "classify", Context: "scope", CausationRoot: root, Trust: contracts.TrustObserved, ReasoningTier: "D1", ProviderID: "provider", Successful: true, Quality: 0.9, InferenceTokens: tokens, PathID: "path", ObservedAt: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC).Add(time.Duration(tokens) * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestLongitudinalAnalysisDeduplicatesCorrelatedInferenceEvidence(t *testing.T) {
	policy := AnalysisPolicy{ID: "generic/v1", MinimumIndependentRoots: 2, RegressionQualityDrop: 0.2, MaximumPathVariants: 2, PortabilityMinimumSamples: 1}
	correlated := []Observation{adaptiveFixture(t, "a", "same-root", 10), adaptiveFixture(t, "copy", "same-root", 11)}
	report, err := Analyze(correlated, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RepeatedInference) != 0 {
		t.Fatal("correlated copies were counted as repeated independent inference")
	}
	independent := append(correlated, adaptiveFixture(t, "b", "independent-root", 12))
	report, err = Analyze(independent, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RepeatedInference) != 1 {
		t.Fatal("independent repeated inference was not detected")
	}

	tampered := report
	tampered.PolicyViolations++
	if err := VerifyLongitudinalReport(tampered); err == nil {
		t.Fatal("tampered longitudinal report verified")
	}
}
