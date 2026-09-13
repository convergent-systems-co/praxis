package state

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type adaptiveDomainFixture struct {
	name         string
	agentID      string
	goalClass    string
	behaviorKey  string
	context      string
	raw          [][]adaptation.Measure
	derivedName  string
	derivedValue float64
	derivedUnit  string
	threshold    float64
	diagnosis    string
	scoreName    string
	scoreValue   float64
}

type allowEvidenceConfirmation struct{}

func (allowEvidenceConfirmation) AuthorizeEvidenceConfirmation(context.Context, string, string, string) error {
	return nil
}

func mustProfileFact(t *testing.T, fact adaptation.ProfileFact) adaptation.ProfileFact {
	t.Helper()
	frozen, err := adaptation.FreezeProfileFact(fact)
	if err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestAdaptiveMeasurementProvenanceIsDomainNeutralAndSurvivesRestart(t *testing.T) {
	base := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	domains := []adaptiveDomainFixture{
		{name: "software-delivery", agentID: "agent-delivery", goalClass: "change-validation", behaviorKey: "validate-change", context: "workspace-a",
			raw: [][]adaptation.Measure{
				{{Name: "latency", Value: 842, Kind: adaptation.RawMeasure, Unit: "ms", Provenance: "runtime-clock", MeasuredAt: base}, {Name: "token_count", Value: 12437, Kind: adaptation.RawMeasure, Unit: "tokens", Provenance: "executor-meter", MeasuredAt: base}, {Name: "test_failures", Value: 4, Kind: adaptation.RawMeasure, Unit: "count", Provenance: "test-runner", MeasuredAt: base}},
				{{Name: "latency", Value: 1200, Kind: adaptation.RawMeasure, Unit: "ms", Provenance: "runtime-clock", MeasuredAt: base.Add(time.Minute)}, {Name: "token_count", Value: 14000, Kind: adaptation.RawMeasure, Unit: "tokens", Provenance: "executor-meter", MeasuredAt: base.Add(time.Minute)}, {Name: "test_failures", Value: 0, Kind: adaptation.RawMeasure, Unit: "count", Provenance: "test-runner", MeasuredAt: base.Add(time.Minute)}},
			}, derivedName: "average_latency", derivedValue: 1021, derivedUnit: "ms", threshold: 1000, diagnosis: "latency-outside-package-budget", scoreName: "token_reduction_score", scoreValue: 0.22},
		{name: "research", agentID: "agent-research", goalClass: "source-synthesis", behaviorKey: "triangulate-claim", context: "topic-b",
			raw: [][]adaptation.Measure{
				{{Name: "search_duration", Value: 65, Kind: adaptation.RawMeasure, Unit: "seconds", Provenance: "research-clock", MeasuredAt: base}, {Name: "evidence_items", Value: 9, Kind: adaptation.RawMeasure, Unit: "items", Provenance: "evidence-index", MeasuredAt: base}, {Name: "source_diversity", Value: 5, Kind: adaptation.RawMeasure, Unit: "sources", Provenance: "source-auditor", MeasuredAt: base}},
				{{Name: "search_duration", Value: 80, Kind: adaptation.RawMeasure, Unit: "seconds", Provenance: "research-clock", MeasuredAt: base.Add(time.Minute)}, {Name: "evidence_items", Value: 12, Kind: adaptation.RawMeasure, Unit: "items", Provenance: "evidence-index", MeasuredAt: base.Add(time.Minute)}, {Name: "source_diversity", Value: 7, Kind: adaptation.RawMeasure, Unit: "sources", Provenance: "source-auditor", MeasuredAt: base.Add(time.Minute)}},
			}, derivedName: "average_search_duration", derivedValue: 72.5, derivedUnit: "seconds", threshold: 70, diagnosis: "search-duration-outside-package-budget", scoreName: "source_coverage_score", scoreValue: 0.84},
	}

	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "praxis.db")
			db, err := OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := adaptation.NewLedger(NewSQLiteEventStore(db), allowEvidenceConfirmation{})
			if err != nil {
				t.Fatal(err)
			}
			observations := make([]adaptation.Observation, 0, 2)
			for index, raw := range domain.raw {
				invariants := []adaptation.InvariantResult{{Class: adaptation.PolicyInvariant, ControlID: "scope-authorized", Passed: true, EvidenceRef: "policy-evidence"}, {Class: adaptation.SecurityInvariant, ControlID: "no-secret-release", Passed: true, EvidenceRef: "security-evidence"}}
				if domain.name == "software-delivery" && index == 1 {
					invariants[1].Passed = false
				}
				observation, freezeErr := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: domain.agentID, RunID: domain.name + "-run-" + string(rune('a'+index)), GoalClass: domain.goalClass, Domain: domain.name, BehaviorKey: domain.behaviorKey, Context: domain.context, CausationRoot: domain.name + "-root-" + string(rune('a'+index)), Trust: contracts.TrustObserved, ReasoningTier: "D1", ProviderID: "provider-" + string(rune('a'+index)), Outcome: "completed", PathID: "path-" + string(rune('a'+index)), RawMeasures: raw, Invariants: invariants, ObservedAt: base.Add(time.Duration(index) * time.Minute)})
				if freezeErr != nil {
					t.Fatal(freezeErr)
				}
				if err := ledger.Record(ctx, observation); err != nil {
					t.Fatal(err)
				}
				if err := ledger.Record(ctx, observation); err != nil {
					t.Fatal("content-identical append must be idempotent:", err)
				}
				observations = append(observations, observation)
			}
			sourceIDs := []string{observations[0].ID, observations[1].ID}
			derived, err := adaptation.FreezeMeasurement(adaptation.Measurement{SubjectAgentID: domain.agentID, GoalClass: domain.goalClass, Domain: domain.name, BehaviorKey: domain.behaviorKey, Context: domain.context, Measure: adaptation.Measure{Name: domain.derivedName, Value: domain.derivedValue, Kind: adaptation.DerivedMeasure, Unit: domain.derivedUnit, Evaluator: &adaptation.EvaluatorRef{ID: "package-mean", Version: "1"}, TransformID: "arithmetic-mean/v1", Provenance: "package:" + domain.name, SourceObservationIDs: sourceIDs, MeasuredAt: base.Add(2 * time.Minute)}})
			if err != nil {
				t.Fatal(err)
			}
			score, err := adaptation.FreezeMeasurement(adaptation.Measurement{SubjectAgentID: domain.agentID, GoalClass: domain.goalClass, Domain: domain.name, BehaviorKey: domain.behaviorKey, Context: domain.context, Measure: adaptation.Measure{Name: domain.scoreName, Value: domain.scoreValue, Kind: adaptation.NormalizedScore, NormalizedRange: &adaptation.NumericRange{Minimum: 0, Maximum: 1}, Evaluator: &adaptation.EvaluatorRef{ID: "package-score", Version: "3"}, TransformID: domain.scoreName + "/v3", Provenance: "package:" + domain.name, SourceObservationIDs: sourceIDs, MeasuredAt: base.Add(2 * time.Minute)}})
			if err != nil {
				t.Fatal(err)
			}
			for _, measurement := range []adaptation.Measurement{derived, score} {
				if err := ledger.RecordMeasurement(ctx, measurement); err != nil {
					t.Fatal(err)
				}
			}

			unitRange := adaptation.NumericRange{Minimum: 0, Maximum: 1}
			confidence := adaptation.Score{Value: 0.8, Range: unitRange}
			declared := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: "evidence-style", Score: adaptation.Score{Value: 0.5, Range: unitRange}, EvidenceClass: adaptation.Declared, Confidence: confidence, Provenance: "publisher:" + domain.name, Context: domain.context, RecordedAt: base})
			inherited := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: "evidence-style", Score: adaptation.Score{Value: 0.6, Range: unitRange}, EvidenceClass: adaptation.Inherited, Confidence: confidence, Provenance: "ancestor:" + domain.name, Context: domain.context, RecordedAt: base.Add(time.Second)})
			observed := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: "evidence-style", Score: adaptation.Score{Value: 0.7, Range: unitRange}, EvidenceClass: adaptation.Observed, Confidence: confidence, Provenance: "local-history", Context: domain.context, SourceObservationIDs: sourceIDs, RecordedAt: base.Add(2 * time.Second)})
			measured, err := adaptation.ProfileFactFromNormalizedMeasurement(score, "evidence-style", confidence, "profile-evaluator", base.Add(3*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			confirmed := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agentID, Dimension: "evidence-style", Score: adaptation.Score{Value: 0.9, Range: unitRange}, EvidenceClass: adaptation.Confirmed, Confidence: adaptation.Score{Value: 1, Range: unitRange}, Provenance: "user:owner", ConfirmationAuthorityID: "owner", ConfirmationEvidenceRef: "approval:profile-" + domain.name, Context: domain.context, RecordedAt: base.Add(4 * time.Second)})
			for _, fact := range []adaptation.ProfileFact{declared, inherited, observed, measured, confirmed} {
				if err := ledger.RecordProfileFact(ctx, fact); err != nil {
					t.Fatal(err)
				}
			}

			policy, err := adaptation.FreezeAnalysisPolicy(adaptation.AnalysisPolicy{Rules: []adaptation.ThresholdRule{{ID: "duration-budget", Diagnosis: domain.diagnosis, MeasureName: domain.derivedName, MeasureKind: adaptation.DerivedMeasure, Unit: domain.derivedUnit, Operator: adaptation.GreaterThan, Threshold: domain.threshold, MinimumIndependentRoots: 2}, {ID: "score-use", Diagnosis: "package-score-threshold-met", MeasureName: domain.scoreName, MeasureKind: adaptation.NormalizedScore, Operator: adaptation.GreaterThan, Threshold: 0.2, MinimumIndependentRoots: 2}}})
			if err != nil {
				t.Fatal(err)
			}
			report, err := adaptation.EvaluateLongitudinal(observations, []adaptation.Measurement{derived, score}, policy)
			if err != nil {
				t.Fatal(err)
			}
			if err := adaptation.VerifyLongitudinalReport(report); err != nil {
				t.Fatal(err)
			}
			if len(report.Diagnoses) != 2 {
				t.Fatalf("package policy was not applied: %#v", report)
			}
			measurementIDs, observationIDs, err := adaptation.TraceDiagnosis(report, report.Diagnoses[0].ID)
			if err != nil || len(measurementIDs) == 0 || !reflect.DeepEqual(observationIDs, derived.Measure.SourceObservationIDs) {
				t.Fatalf("diagnosis trace measurements=%v observations=%v err=%v", measurementIDs, observationIDs, err)
			}
			if domain.name == "software-delivery" && (len(report.InvariantFailures) != 1 || report.InvariantFailures[0].Class != adaptation.SecurityInvariant) {
				t.Fatalf("favorable score averaged away security failure: %#v", report)
			}

			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restarted, _ := adaptation.NewLedger(NewSQLiteEventStore(db))
			replayedObservations, err := restarted.Observations(ctx, domain.agentID)
			if err != nil || !reflect.DeepEqual(replayedObservations, observations) {
				t.Fatalf("raw observations changed across restart: %#v err=%v", replayedObservations, err)
			}
			replayedMeasurements, err := restarted.Measurements(ctx, domain.agentID)
			if err != nil || !reflect.DeepEqual(replayedMeasurements, []adaptation.Measurement{derived, score}) {
				t.Fatalf("derived measurements changed across restart: %#v err=%v", replayedMeasurements, err)
			}
			history, err := restarted.ProfileHistory(ctx, domain.agentID)
			if err != nil || len(history) != 5 {
				t.Fatalf("profile evidence classes lost across restart: %d err=%v", len(history), err)
			}
		})
	}
}
