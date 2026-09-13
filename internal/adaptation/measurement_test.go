package adaptation

import (
	"context"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestMeasureKindsDoNotImplyEachOthersSemantics(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	raw := Measure{Name: "latency", Value: 842, Kind: RawMeasure, Unit: "ms", Provenance: "runtime-clock", MeasuredAt: now}
	if err := raw.Validate(); err != nil {
		t.Fatal(err)
	}
	raw.Value = -42
	if err := raw.Validate(); err != nil {
		t.Fatal("raw values must not be silently normalized or forced positive:", err)
	}

	derived := Measure{Name: "average_latency", Value: 900, Kind: DerivedMeasure, Unit: "ms", Evaluator: &EvaluatorRef{ID: "mean", Version: "1"}, TransformID: "arithmetic-mean/v1", Provenance: "package-evaluator", SourceObservationIDs: []string{"one"}, MeasuredAt: now}
	if err := derived.Validate(); err != nil {
		t.Fatal(err)
	}
	derived.Evaluator = nil
	if err := derived.Validate(); err == nil {
		t.Fatal("derived measure omitted evaluator identity")
	}

	normalized := Measure{Name: "quality", Value: 1.1, Kind: NormalizedScore, NormalizedRange: &NumericRange{Minimum: 0, Maximum: 1}, Evaluator: &EvaluatorRef{ID: "quality", Version: "2"}, TransformID: "quality-projection/v2", Provenance: "package-evaluator", SourceObservationIDs: []string{"one"}, MeasuredAt: now}
	if err := normalized.Validate(); err == nil {
		t.Fatal("normalized score escaped its explicit range")
	}

	observation, err := FreezeObservation(Observation{SubjectAgentID: "agent", RunID: "run", GoalClass: "goal", Domain: "generic", BehaviorKey: "behavior", Context: "scope", CausationRoot: "root", Trust: contracts.TrustDerived, Outcome: "complete", PathID: "path", ObservedAt: now})
	if err == nil || observation.ID != "" {
		t.Fatal("raw observation accepted derived trust")
	}
}

func TestConfirmedEvidenceCannotBypassAuthorityBoundary(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	fact, err := FreezeProfileFact(ProfileFact{SubjectAgentID: "agent", Dimension: "autonomy", Score: Score{Value: 0.8, Range: NumericRange{Minimum: 0, Maximum: 1}}, EvidenceClass: Confirmed, Confidence: Score{Value: 1, Range: NumericRange{Minimum: 0, Maximum: 1}}, Provenance: "human-statement", ConfirmationAuthorityID: "owner", ConfirmationEvidenceRef: "approval:one", Context: "scope", RecordedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := NewLedger(eventstore.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordProfileFact(context.Background(), fact); err == nil {
		t.Fatal("confirmed evidence appended without deterministic authority")
	}
}

func TestPolicyDiagnosisRequiresIndependentCausationRoots(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	freeze := func(run string) Observation {
		observation, err := FreezeObservation(Observation{SubjectAgentID: "agent", RunID: run, GoalClass: "goal", Domain: "generic", BehaviorKey: "behavior", Context: "scope", CausationRoot: "shared-root", Trust: contracts.TrustObserved, Outcome: "complete", PathID: "path", RawMeasures: []Measure{{Name: "duration", Value: 12, Kind: RawMeasure, Unit: "seconds", Provenance: "clock", MeasuredAt: now}}, ObservedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		return observation
	}
	observations := []Observation{freeze("run-a"), freeze("run-copy")}
	measurement, err := FreezeMeasurement(Measurement{SubjectAgentID: "agent", GoalClass: "goal", Domain: "generic", BehaviorKey: "behavior", Context: "scope", Measure: Measure{Name: "average_duration", Value: 12, Kind: DerivedMeasure, Unit: "seconds", Evaluator: &EvaluatorRef{ID: "mean", Version: "1"}, TransformID: "arithmetic-mean/v1", Provenance: "package", SourceObservationIDs: []string{observations[0].ID, observations[1].ID}, MeasuredAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := FreezeAnalysisPolicy(AnalysisPolicy{Rules: []ThresholdRule{{ID: "duration", Diagnosis: "slow", MeasureName: "average_duration", MeasureKind: DerivedMeasure, Unit: "seconds", Operator: GreaterThan, Threshold: 10, MinimumIndependentRoots: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateLongitudinal(observations, []Measurement{measurement}, policy, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Diagnoses) != 0 {
		t.Fatal("correlated observation copies satisfied independent evidence policy")
	}
}

func TestLedgerRejectsDigestValidAnalysisNotDerivedFromCitedEvidence(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	observation, err := FreezeObservation(Observation{SubjectAgentID: "agent", RunID: "run", GoalClass: "goal", Domain: "research", BehaviorKey: "search", Context: "scope", CausationRoot: "root", Trust: contracts.TrustObserved, Outcome: "complete", PathID: "path", RawMeasures: []Measure{{Name: "duration", Value: 90, Kind: RawMeasure, Unit: "seconds", Provenance: "clock", MeasuredAt: now}}, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := FreezeMeasurement(Measurement{SubjectAgentID: "agent", GoalClass: "goal", Domain: "research", BehaviorKey: "search", Context: "scope", Measure: Measure{Name: "average_duration", Value: 90, Kind: DerivedMeasure, Unit: "seconds", Evaluator: &EvaluatorRef{ID: "mean", Version: "1"}, TransformID: "mean/v1", Provenance: "research-package", SourceObservationIDs: []string{observation.ID}, MeasuredAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := FreezeAnalysisPolicy(AnalysisPolicy{Rules: []ThresholdRule{{ID: "duration", Diagnosis: "slow", MeasureName: "average_duration", MeasureKind: DerivedMeasure, Unit: "seconds", Operator: GreaterThan, Threshold: 60, MinimumIndependentRoots: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateLongitudinal([]Observation{observation}, []Measurement{measurement}, policy, now.Add(time.Minute))
	if err != nil || len(report.Diagnoses) != 1 {
		t.Fatal("expected policy diagnosis", err)
	}

	// Preserve all identity-bearing fields while replacing the actual policy result.
	// A content digest alone cannot make this a valid derivation.
	report.Diagnoses = nil
	report.ID = ""
	digest, err := digestValue(report)
	if err != nil {
		t.Fatal(err)
	}
	report.ID = "sha256:" + digest
	record := AnalysisRecord{Policy: policy, Report: report}
	if err := VerifyAnalysisRecord(record); err != nil {
		t.Fatal("fixture should remain intrinsically digest-valid:", err)
	}

	ledger, err := NewLedger(eventstore.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Record(ctx, observation); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordMeasurement(ctx, measurement); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordAnalysis(ctx, record); err == nil {
		t.Fatal("ledger accepted a digest-valid report that was not produced by its cited evidence and policy")
	}
}
