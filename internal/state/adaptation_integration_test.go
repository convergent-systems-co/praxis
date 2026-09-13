package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestAdaptiveEvidenceLifecycleIsDomainNeutralAndSurvivesSQLiteRestart(t *testing.T) {
	domains := []struct {
		name        string
		agentID     string
		goalClass   string
		behaviorKey string
		measure     string
		dimension   string
		paths       []string
		qualities   []float64
		providers   []string
		successes   []bool
	}{
		{name: "software-delivery", agentID: "agent-delivery", goalClass: "change-validation", behaviorKey: "validate-change", measure: "validation_depth", dimension: "evidence-depth", paths: []string{"test-review", "test-review", "repair-review"}, qualities: []float64{0.96, 0.94, 0.62}, providers: []string{"provider-a", "provider-a", "provider-b"}, successes: []bool{true, true, false}},
		{name: "research", agentID: "agent-research", goalClass: "source-synthesis", behaviorKey: "triangulate-claim", measure: "source_coverage", dimension: "evidence-breadth", paths: []string{"primary-secondary", "primary-secondary", "primary-secondary"}, qualities: []float64{0.91, 0.90, 0.89}, providers: []string{"provider-x", "provider-x", "provider-y"}, successes: []bool{true, true, true}},
	}

	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "praxis.db")
			db, err := OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := adaptation.NewLedger(NewSQLiteEventStore(db))
			if err != nil {
				t.Fatal(err)
			}
			base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
			for index := range domain.paths {
				observation, freezeErr := adaptation.FreezeObservation(adaptation.Observation{
					SubjectAgentID: domain.agentID, RunID: domain.name + "-run-" + string(rune('a'+index)), GoalClass: domain.goalClass,
					Domain: domain.name, BehaviorKey: domain.behaviorKey, Context: domain.name + "-context", CausationRoot: domain.name + "-root-" + string(rune('a'+index)),
					Trust: contracts.TrustObserved, ReasoningTier: "D1", ProviderID: domain.providers[index], Successful: domain.successes[index],
					Quality: domain.qualities[index], InferenceTokens: 20 + index, PathID: domain.paths[index], Measures: map[string]float64{domain.measure: domain.qualities[index]}, ObservedAt: base.Add(time.Duration(index) * time.Minute),
				})
				if freezeErr != nil {
					t.Fatal(freezeErr)
				}
				if err := ledger.Record(ctx, observation); err != nil {
					t.Fatal(err)
				}
				if err := ledger.Record(ctx, observation); err != nil {
					t.Fatal("content-identical replay must be idempotent:", err)
				}
			}

			observations, err := ledger.Observations(ctx, domain.agentID)
			if err != nil || len(observations) != 3 {
				t.Fatalf("observations=%d err=%v", len(observations), err)
			}
			facts, err := adaptation.DeriveProfileFacts(observations, []adaptation.ProfileRule{
				{Dimension: domain.dimension + "-observed", Measure: domain.measure, EvidenceClass: adaptation.Observed, EvaluatorVersion: "fixture-observer/v1"},
				{Dimension: domain.dimension, Measure: domain.measure, EvidenceClass: adaptation.Measured, EvaluatorVersion: "fixture-evaluator/v1"},
			}, base.Add(5*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			declared, err := adaptation.FreezeProfileFact(adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: domain.dimension, Value: 0.8, EvidenceClass: adaptation.Declared, Confidence: 0.5, Provenance: "package:" + domain.name, Context: domain.name + "-context", RecordedAt: base.Add(-time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			inherited, err := adaptation.FreezeProfileFact(adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: domain.dimension, Value: 0.82, EvidenceClass: adaptation.Inherited, Confidence: 0.5, Provenance: "ancestor:" + domain.name, Context: domain.name + "-context", RecordedAt: base.Add(-30 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			confirmed, err := adaptation.FreezeProfileFact(adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: domain.dimension, Value: 0.9, EvidenceClass: adaptation.Confirmed, Confidence: 1, Provenance: "user:owner", Context: domain.name + "-context", RecordedAt: base.Add(6 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			for _, fact := range []adaptation.ProfileFact{declared, inherited, facts[0], facts[1], confirmed} {
				if err := ledger.RecordProfileFact(ctx, fact); err != nil {
					t.Fatal(err)
				}
			}

			policy := adaptation.AnalysisPolicy{ID: "policy:" + domain.name + "/v1", MinimumIndependentRoots: 2, RegressionQualityDrop: 0.2, MaximumPathVariants: 1, PortabilityMinimumSamples: 1}
			report, err := adaptation.Analyze(observations, policy)
			if err != nil {
				t.Fatal(err)
			}
			if err := adaptation.VerifyLongitudinalReport(report); err != nil {
				t.Fatal(err)
			}
			if len(report.RepeatedInference) != 1 || report.PolicyViolations != 0 || report.SecurityViolations != 0 {
				t.Fatalf("unexpected report: %#v", report)
			}
			if domain.name == "software-delivery" {
				if len(report.Regressions) != 1 || len(report.PathVariance) != 1 || len(report.PortabilityFailures) != 1 {
					t.Fatalf("delivery signals missing: %#v", report)
				}
			} else if len(report.Regressions) != 0 || len(report.PathVariance) != 0 || len(report.PortabilityFailures) != 0 {
				t.Fatalf("research policy produced false diagnoses: %#v", report)
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restarted, err := adaptation.NewLedger(NewSQLiteEventStore(db))
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := restarted.Observations(ctx, domain.agentID)
			if err != nil || len(replayed) != len(observations) {
				t.Fatalf("restart observations=%d err=%v", len(replayed), err)
			}
			history, err := restarted.ProfileHistory(ctx, domain.agentID)
			if err != nil || len(history) != 5 {
				t.Fatalf("restart profile history=%d err=%v", len(history), err)
			}
			classes := map[adaptation.EvidenceClass]bool{}
			for _, fact := range history {
				classes[fact.EvidenceClass] = true
			}
			for _, class := range []adaptation.EvidenceClass{adaptation.Declared, adaptation.Inherited, adaptation.Observed, adaptation.Measured, adaptation.Confirmed} {
				if !classes[class] {
					t.Fatalf("profile history lost evidence class %s", class)
				}
			}
			if replayed[0].RunID == "" || replayed[0].ProviderID == "" || replayed[0].CausationRoot == "" || replayed[0].Measures[domain.measure] == 0 {
				t.Fatal("restart lost execution identity or package-defined measure")
			}
		})
	}
}

func TestAdaptiveEvidenceRejectsAuthorityAndMeasurementRelabeling(t *testing.T) {
	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if _, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: "agent", RunID: "run", GoalClass: "goal", Domain: "generic", BehaviorKey: "behavior", Context: "context", CausationRoot: "root", Trust: contracts.TrustPolicy, ReasoningTier: "D1", Successful: true, Quality: 1, PathID: "path", ObservedAt: base}); err == nil {
		t.Fatal("execution observation must not claim policy authority")
	}
	if _, err := adaptation.FreezeProfileFact(adaptation.ProfileFact{SubjectAgentID: "agent", Dimension: "autonomy", Value: 0.5, EvidenceClass: adaptation.Declared, Confidence: 0.5, SampleSize: 10, EvaluatorVersion: "fake", Provenance: "publisher", Context: "generic", SourceObservationIDs: []string{"observation"}, RecordedAt: base}); err == nil {
		t.Fatal("declared profile fact masqueraded as measured evidence")
	}
}
