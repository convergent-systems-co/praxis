package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type packageSeriesEvaluator struct {
	identity                          adaptation.EvaluatorRef
	name, unit, transform, provenance string
	calculate                         func([]adaptation.Observation) float64
}

func (e packageSeriesEvaluator) Identity() adaptation.EvaluatorRef { return e.identity }
func (e packageSeriesEvaluator) EvaluateSeries(observations []adaptation.Observation, at time.Time) (adaptation.Measure, error) {
	sources := make([]string, 0, len(observations))
	for _, observation := range observations {
		sources = append(sources, observation.ID)
	}
	identity := e.identity
	return adaptation.Measure{Name: e.name, Value: e.calculate(observations), Kind: adaptation.DerivedMeasure, Unit: e.unit, Evaluator: &identity, TransformID: e.transform, Provenance: e.provenance, SourceObservationIDs: sources, MeasuredAt: at}, nil
}

func TestVersionedPackageEvaluatorsDiagnoseLongitudinalFailuresAcrossDomainsAndRestart(t *testing.T) {
	domains := []struct {
		name, failureOutcome, regressionMetric, repeatedMetric, varianceMetric, portabilityMetric, nativeName, nativeUnit string
	}{
		{name: "software-delivery", failureOutcome: "regressed", regressionMetric: "regression_count", repeatedMetric: "inference_repetition_count", varianceMetric: "delivery_path_variance", portabilityMetric: "provider_failure_count", nativeName: "token_count", nativeUnit: "tokens"},
		{name: "research", failureOutcome: "citation_regressed", regressionMetric: "citation_regression_count", repeatedMetric: "synthesis_reasoning_repetition", varianceMetric: "strategy_variance", portabilityMetric: "engine_portability_failures", nativeName: "search_duration", nativeUnit: "seconds"},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC)
			path := filepath.Join(t.TempDir(), "praxis.db")
			db, err := OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			ledger, _ := adaptation.NewLedger(NewSQLiteEventStore(db))
			subject := "agent-" + domain.name
			for index := 0; index < 4; index++ {
				provider := "provider-alpha"
				outcome := "complete"
				if index >= 2 {
					provider = "provider-beta"
				}
				if index == 3 {
					outcome = domain.failureOutcome
				}
				observation, freezeErr := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: subject, RunID: "run-" + string(rune('a'+index)), GoalClass: "repeatable-work", Domain: domain.name, BehaviorKey: "execute", Context: "scope", CausationRoot: "root-" + string(rune('a'+index)), Trust: contracts.TrustObserved, ReasoningTier: "D1", ProviderID: provider, Outcome: outcome, PathID: "path-" + string(rune('a'+index%3)), RawMeasures: []adaptation.Measure{{Name: domain.nativeName, Value: float64(10 + index), Kind: adaptation.RawMeasure, Unit: domain.nativeUnit, Provenance: "instrumentation:" + domain.name, MeasuredAt: base.Add(time.Duration(index) * time.Minute)}}, ObservedAt: base.Add(time.Duration(index) * time.Minute)})
				if freezeErr != nil {
					t.Fatal(freezeErr)
				}
				if err := ledger.Record(ctx, observation); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			ledger, _ = adaptation.NewLedger(NewSQLiteEventStore(db))
			identities := []adaptation.EvaluatorRef{{ID: domain.name + "/regression", Version: "1"}, {ID: domain.name + "/repeated", Version: "1"}, {ID: domain.name + "/variance", Version: "1"}, {ID: domain.name + "/portability", Version: "1"}}
			evaluators := []adaptation.SeriesEvaluator{
				packageSeriesEvaluator{identity: identities[0], name: domain.regressionMetric, unit: "count", transform: "package-regression/v1", provenance: "package:" + domain.name, calculate: func(observations []adaptation.Observation) float64 {
					return countObservations(observations, func(o adaptation.Observation) bool { return o.Outcome == domain.failureOutcome })
				}},
				packageSeriesEvaluator{identity: identities[1], name: domain.repeatedMetric, unit: "executions", transform: "package-repetition/v1", provenance: "package:" + domain.name, calculate: func(observations []adaptation.Observation) float64 {
					return countObservations(observations, func(o adaptation.Observation) bool { return o.ReasoningTier == "D1" || o.ReasoningTier == "D2" })
				}},
				packageSeriesEvaluator{identity: identities[2], name: domain.varianceMetric, unit: "paths", transform: "package-path-variance/v1", provenance: "package:" + domain.name, calculate: func(observations []adaptation.Observation) float64 {
					paths := map[string]bool{}
					for _, o := range observations {
						paths[o.PathID] = true
					}
					return float64(len(paths) - 1)
				}},
				packageSeriesEvaluator{identity: identities[3], name: domain.portabilityMetric, unit: "failures", transform: "package-portability/v1", provenance: "package:" + domain.name, calculate: func(observations []adaptation.Observation) float64 {
					return countObservations(observations, func(o adaptation.Observation) bool {
						return o.ProviderID == "provider-beta" && o.Outcome == domain.failureOutcome
					})
				}},
			}
			rules := []adaptation.ThresholdRule{}
			for index, metric := range []string{domain.regressionMetric, domain.repeatedMetric, domain.varianceMetric, domain.portabilityMetric} {
				unit, threshold := "count", float64(1)
				if index == 1 {
					unit, threshold = "executions", 3
				}
				if index == 2 {
					unit = "paths"
				}
				if index == 3 {
					unit = "failures"
				}
				rules = append(rules, adaptation.ThresholdRule{ID: "rule-" + metric, Diagnosis: "diagnose-" + metric, MeasureName: metric, MeasureKind: adaptation.DerivedMeasure, Unit: unit, Operator: adaptation.GreaterThanOrEqual, Threshold: threshold, MinimumIndependentRoots: 4})
			}
			policy, err := adaptation.FreezeAnalysisPolicy(adaptation.AnalysisPolicy{Rules: rules})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := adaptation.FreezeEvaluationPlan(adaptation.EvaluationPlan{SubjectAgentID: subject, GoalClass: "repeatable-work", Domain: domain.name, BehaviorKey: "execute", Context: "scope", Evaluators: identities, Policy: policy})
			if err != nil {
				t.Fatal(err)
			}
			record, measurements, err := adaptation.RunEvaluationPlan(ctx, ledger, plan, evaluators, base.Add(10*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			diagnoses := map[string]adaptation.Diagnosis{}
			for _, diagnosis := range record.Report.Diagnoses {
				diagnoses[diagnosis.Code] = diagnosis
			}
			for _, metric := range []string{domain.regressionMetric, domain.repeatedMetric, domain.varianceMetric, domain.portabilityMetric} {
				diagnosis, ok := diagnoses["diagnose-"+metric]
				if !ok {
					t.Fatalf("package diagnosis missing for %s: %#v", metric, diagnoses)
				}
				_, sources, traceErr := adaptation.TraceDiagnosis(record.Report, diagnosis.ID)
				if traceErr != nil || !sameObservationIdentitySet(sources, record.Report.ObservationIDs) {
					t.Fatalf("diagnosis %s lost durable source trace: %v %v", metric, sources, traceErr)
				}
			}
			measurementsByEvaluator := map[adaptation.EvaluatorRef]adaptation.Measurement{}
			for _, measurement := range measurements {
				if measurement.Measure.Evaluator != nil {
					measurementsByEvaluator[*measurement.Measure.Evaluator] = measurement
				}
			}
			for index, identity := range identities {
				measurement, ok := measurementsByEvaluator[identity]
				if !ok || measurement.Measure.Name != []string{domain.regressionMetric, domain.repeatedMetric, domain.varianceMetric, domain.portabilityMetric}[index] || !sameObservationIdentitySet(measurement.Measure.SourceObservationIDs, record.Report.ObservationIDs) {
					t.Fatalf("declared evaluator did not produce its traceable semantic measurement: identity=%#v measurement=%#v", identity, measurement)
				}
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
			history, err := restarted.AnalysisHistory(ctx, subject)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, persisted := range history {
				if persisted.Report.ID == record.Report.ID && persisted.Policy.ID == policy.ID {
					found = true
				}
			}
			if !found {
				t.Fatalf("content-addressed longitudinal diagnosis did not survive restart: %#v", history)
			}
		})
	}
}

func countObservations(observations []adaptation.Observation, matches func(adaptation.Observation) bool) float64 {
	count := 0
	for _, observation := range observations {
		if matches(observation) {
			count++
		}
	}
	return float64(count)
}

func sameObservationIdentitySet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, id := range left {
		seen[id]++
	}
	for _, id := range right {
		seen[id]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}
