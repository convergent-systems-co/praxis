package learning_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/learning"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestDurableContradictionEvidenceForksBehaviorTowardInferenceAcrossDomains(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	domains := []struct{ name, behavior, rawName, rawUnit, evaluator string }{
		{"software-delivery", "normalize-artifact-id", "test_failures", "count", "delivery.equivalence"},
		{"research", "normalize-source-id", "citation_mismatches", "references", "research.equivalence"},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			seed, err := learning.NewBehaviorGeneration("", []learning.AdvisoryInstruction{{ID: "normalize-code", Text: "Normalize the package-defined identifier.", Tier: learning.TierD2, Learnable: true, Active: true}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			compiledEvidence := []learning.ExecutionObservation{{ID: "success-one", InstructionID: "normalize-code", GoalClass: "normalization", Context: domain.name, Input: " ab-1 ", Output: "AB-1", Successful: true, TrustClass: "runtime_verified", CausationRoot: "success-root-one", InferenceTokens: 20}, {ID: "success-two", InstructionID: "normalize-code", GoalClass: "normalization", Context: domain.name, Input: " cd-2 ", Output: "CD-2", Successful: true, TrustClass: "runtime_verified", CausationRoot: "success-root-two", InferenceTokens: 20}}
			compiled, err := learning.LearnDeterministicCandidate(seed, "normalize-code", compiledEvidence, 2)
			if err != nil {
				t.Fatal(err)
			}
			compiledSources := map[string]bool{}
			for _, sourceID := range compiled.SourceEvidenceIDs {
				if compiledSources[sourceID] {
					t.Fatalf("compiled generation duplicated causal evidence %s", sourceID)
				}
				compiledSources[sourceID] = true
			}
			for _, expected := range []string{"success-one", "success-two"} {
				if !compiledSources[expected] {
					t.Fatalf("compiled generation lost required source evidence %s", expected)
				}
			}
			promotion, err := learning.EvaluateBehaviorCandidate(seed, compiled, "normalize-code", compiledEvidence, []learning.BehaviorReplayCase{{ID: "original", Input: " ef-3 ", RequiredOutput: "EF-3", ActiveTokens: 20}, {ID: "regression", Input: " gh-4 ", RequiredOutput: "GH-4", ActiveTokens: 20}}, 2)
			if err != nil || !promotion.Passed {
				t.Fatalf("fixture promotion failed: %#v %v", promotion, err)
			}
			registryPath := filepath.Join(t.TempDir(), "behavior.json")
			registry, err := learning.OpenBehaviorRegistry(registryPath, seed)
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.Promote(compiled, promotion, learning.GovernanceDecision{AuthorityID: "promotion-governor", Approved: true, EvaluationID: promotion.ID, DecidedAt: base}, "learning-plane"); err != nil {
				t.Fatal(err)
			}

			databasePath := filepath.Join(t.TempDir(), "praxis.db")
			db, err := state.OpenSQLite(ctx, databasePath)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := adaptation.NewLedger(state.NewSQLiteEventStore(db))
			if err != nil {
				t.Fatal(err)
			}
			observations := make([]adaptation.Observation, 0, 2)
			for index, run := range []string{"contradiction-run-one", "contradiction-run-two"} {
				observedAt := base.Add(time.Duration(index+1) * time.Minute)
				observation, freezeErr := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: "agent-" + domain.name, RunID: run, GoalClass: "normalization", Domain: domain.name, BehaviorKey: domain.behavior, Context: "context-" + domain.name, CausationRoot: run, Trust: contracts.TrustObserved, ReasoningTier: "D0", ProviderID: "compiled-generation", Outcome: "contradicted", PathID: compiled.ID, RawMeasures: []adaptation.Measure{{Name: domain.rawName, Value: float64(index + 1), Kind: adaptation.RawMeasure, Unit: domain.rawUnit, Provenance: "runtime:" + domain.name, MeasuredAt: observedAt}}, Invariants: []adaptation.InvariantResult{{Class: adaptation.SecurityInvariant, ControlID: "adaptation-safe", Passed: true, EvidenceRef: "security:" + run}, {Class: adaptation.PolicyInvariant, ControlID: "scope-allowed", Passed: true, EvidenceRef: "policy:" + run}}, ObservedAt: observedAt})
				if freezeErr != nil {
					t.Fatal(freezeErr)
				}
				if err := ledger.Record(ctx, observation); err != nil {
					t.Fatal(err)
				}
				observations = append(observations, observation)
			}
			sourceIDs := []string{observations[0].ID, observations[1].ID}
			measurement, err := adaptation.FreezeMeasurement(adaptation.Measurement{SubjectAgentID: observations[0].SubjectAgentID, GoalClass: observations[0].GoalClass, Domain: domain.name, BehaviorKey: domain.behavior, Context: observations[0].Context, Measure: adaptation.Measure{Name: "deterministic_equivalence_score", Value: 0.2, Kind: adaptation.NormalizedScore, NormalizedRange: &adaptation.NumericRange{Minimum: 0, Maximum: 1}, Evaluator: &adaptation.EvaluatorRef{ID: domain.evaluator, Version: "2"}, TransformID: "behavioral-replay/v2", Provenance: "package:" + domain.name, SourceObservationIDs: sourceIDs, MeasuredAt: base.Add(3 * time.Minute)}})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.RecordMeasurement(ctx, measurement); err != nil {
				t.Fatal(err)
			}
			analysisPolicy, err := adaptation.FreezeAnalysisPolicy(adaptation.AnalysisPolicy{Rules: []adaptation.ThresholdRule{{ID: "equivalence-floor", Diagnosis: "compiled-behavior-contradicted", MeasureName: "deterministic_equivalence_score", MeasureKind: adaptation.NormalizedScore, Operator: adaptation.LessThan, Threshold: 0.8, MinimumIndependentRoots: 2}}})
			if err != nil {
				t.Fatal(err)
			}
			report, err := adaptation.EvaluateLongitudinal(observations, []adaptation.Measurement{measurement}, analysisPolicy, base.Add(4*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			analysis := adaptation.AnalysisRecord{Policy: analysisPolicy, Report: report}
			if err := ledger.RecordAnalysis(ctx, analysis); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			db, err = state.OpenSQLite(ctx, databasePath)
			if err != nil {
				t.Fatal(err)
			}
			ledger, _ = adaptation.NewLedger(state.NewSQLiteEventStore(db))
			replayedObservations, err := ledger.Observations(ctx, observations[0].SubjectAgentID)
			if err != nil {
				t.Fatal(err)
			}
			replayedMeasurements, err := ledger.Measurements(ctx, observations[0].SubjectAgentID)
			if err != nil {
				t.Fatal(err)
			}
			replayedAnalyses, err := ledger.AnalysisHistory(ctx, observations[0].SubjectAgentID)
			if err != nil {
				t.Fatal(err)
			}
			byObservationID := map[string]adaptation.Observation{}
			for _, item := range replayedObservations {
				byObservationID[item.ID] = item
			}
			if byObservationID[observations[0].ID].RawMeasures[0].Name != domain.rawName || byObservationID[observations[1].ID].RawMeasures[0].Unit != domain.rawUnit {
				t.Fatalf("restart lost domain-native contradiction facts: %#v", replayedObservations)
			}
			byMeasurementID := map[string]adaptation.Measurement{}
			for _, item := range replayedMeasurements {
				byMeasurementID[item.ID] = item
			}
			byReportID := map[string]adaptation.AnalysisRecord{}
			for _, item := range replayedAnalyses {
				byReportID[item.Report.ID] = item
			}
			replayedMeasurement, measurementFound := byMeasurementID[measurement.ID]
			replayedAnalysis, analysisFound := byReportID[report.ID]
			if !measurementFound || replayedMeasurement.Measure.Evaluator == nil || replayedMeasurement.Measure.Evaluator.ID != domain.evaluator || !analysisFound || replayedAnalysis.Policy.ID != analysisPolicy.ID {
				t.Fatalf("restart lost derived contradiction chain: %#v %#v", replayedMeasurements, replayedAnalyses)
			}

			demotionPolicy, err := learning.FreezeDemotionPolicy(learning.DemotionPolicy{TriggerDiagnoses: []string{"compiled-behavior-contradicted"}, MinimumIndependentContradictions: 2})
			if err != nil {
				t.Fatal(err)
			}
			candidate, record, err := learning.ProposeInferenceDemotion(compiled, "normalize-code", domain.behavior, replayedObservations, replayedMeasurements, byReportID[report.ID], demotionPolicy)
			if err != nil {
				t.Fatal(err)
			}
			if registry.Active().ID != compiled.ID {
				t.Fatal("proposal modified active deterministic generation")
			}
			prompt, err := learning.ActivePrompt(candidate)
			if err != nil {
				t.Fatal(err)
			}
			restored := false
			for _, instruction := range prompt {
				if instruction.ID == "normalize-code" && instruction.Text == "Normalize the package-defined identifier." && instruction.Tier == learning.TierD2 {
					restored = true
				}
			}
			if !restored {
				t.Fatalf("candidate did not restore exact bounded inference: %#v", prompt)
			}
			if err := registry.DemoteTo(candidate, record, learning.GovernanceDecision{AuthorityID: "demotion-governor", Approved: true, EvaluationID: record.Evaluation.ID, DecidedAt: base.Add(5 * time.Minute)}, "learning-plane"); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			restartedRegistry, err := learning.OpenBehaviorRegistry(registryPath, seed)
			if err != nil {
				t.Fatal(err)
			}
			if restartedRegistry.Active().ID != candidate.ID || restartedRegistry.State(compiled.ID) != learning.CandidateDemoted || !restartedRegistry.HasGeneration(compiled.ID) {
				t.Fatalf("restart lost governed fork lineage: active=%s", restartedRegistry.Active().ID)
			}
			if err := restartedRegistry.Rollback(); err != nil {
				t.Fatal(err)
			}
			rolledBack, err := learning.OpenBehaviorRegistry(registryPath, seed)
			if err != nil {
				t.Fatal(err)
			}
			if rolledBack.Active().ID != compiled.ID || !rolledBack.HasGeneration(candidate.ID) {
				t.Fatal("rollback lost deterministic or inference generation")
			}
			if output, err := learning.ExecuteDeterministic(rolledBack.Active(), "normalize-code", " ij-5 "); err != nil || output != "IJ-5" {
				t.Fatalf("rollback generation is not executable: %q %v", output, err)
			}
		})
	}
}
