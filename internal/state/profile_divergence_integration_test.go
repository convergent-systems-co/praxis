package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type packageProfileEvaluator struct {
	id    string
	score float64
}

func (e packageProfileEvaluator) ID() string      { return e.id }
func (e packageProfileEvaluator) Version() string { return "2" }
func (e packageProfileEvaluator) EvaluateProfile(context.Context, []adaptation.Observation) (adaptation.Score, adaptation.Score, error) {
	range01 := adaptation.NumericRange{Minimum: 0, Maximum: 1}
	return adaptation.Score{Value: e.score, Range: range01}, adaptation.Score{Value: 0.85, Range: range01}, nil
}

func TestBehavioralProfileDerivationAndDivergenceSurviveRestartAcrossDomains(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	domains := []struct {
		name, agent, dimension, context, rawName, rawUnit, evaluator string
		observed, measured                                           float64
	}{
		{"software-delivery", "agent-delivery-profile", "validation-depth", "workspace:delivery", "test_failures", "count", "delivery.profile-depth", 0.82, 0.88},
		{"research", "agent-research-profile", "source-breadth", "topic:research", "evidence_items", "sources", "research.profile-breadth", 0.76, 0.81},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "praxis.db")
			db, err := OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := adaptation.NewLedger(NewSQLiteEventStore(db), allowEvidenceConfirmation{})
			if err != nil {
				t.Fatal(err)
			}
			observation, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: domain.agent, RunID: domain.name + "-profile-run", GoalClass: "profile-evaluation", Domain: domain.name, BehaviorKey: domain.dimension, Context: domain.context, CausationRoot: domain.name + "-profile-root", Trust: contracts.TrustObserved, Outcome: "complete", PathID: "package-profile-path", RawMeasures: []adaptation.Measure{{Name: domain.rawName, Value: 7, Kind: adaptation.RawMeasure, Unit: domain.rawUnit, Provenance: "runtime:" + domain.name, MeasuredAt: base}}, ObservedAt: base})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Record(ctx, observation); err != nil {
				t.Fatal(err)
			}
			range01 := adaptation.NumericRange{Minimum: 0, Maximum: 1}
			confidence := adaptation.Score{Value: 0.8, Range: range01}
			declared := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agent, Dimension: domain.dimension, Score: adaptation.Score{Value: 0.25, Range: range01}, EvidenceClass: adaptation.Declared, Confidence: confidence, Provenance: "publisher:" + domain.name, Context: domain.context, RecordedAt: base})
			inherited := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agent, Dimension: domain.dimension, Score: adaptation.Score{Value: 0.3, Range: range01}, EvidenceClass: adaptation.Inherited, Confidence: confidence, Provenance: "ancestor:" + domain.name, Context: domain.context, RecordedAt: base.Add(time.Minute)})
			observed, derivation, err := adaptation.DeriveObservedProfile(ctx, []adaptation.Observation{observation}, domain.dimension, "package-profile-transform/v2", packageProfileEvaluator{id: domain.evaluator, score: domain.observed}, base.Add(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			normalized, err := adaptation.FreezeMeasurement(adaptation.Measurement{SubjectAgentID: domain.agent, GoalClass: observation.GoalClass, Domain: domain.name, BehaviorKey: domain.dimension, Context: domain.context, Measure: adaptation.Measure{Name: domain.dimension + "_score", Value: domain.measured, Kind: adaptation.NormalizedScore, NormalizedRange: &range01, Evaluator: &adaptation.EvaluatorRef{ID: domain.evaluator + ".measurement", Version: "3"}, TransformID: "profile-score/v3", Provenance: "package:" + domain.name, SourceObservationIDs: []string{observation.ID}, MeasuredAt: base.Add(3 * time.Minute)}})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.RecordMeasurement(ctx, normalized); err != nil {
				t.Fatal(err)
			}
			measured, err := adaptation.ProfileFactFromNormalizedMeasurement(normalized, domain.dimension, confidence, "measurement:"+domain.name, base.Add(3*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			confirmed := mustProfileFact(t, adaptation.ProfileFact{SubjectAgentID: domain.agent, Dimension: domain.dimension, Score: adaptation.Score{Value: 0.9, Range: range01}, EvidenceClass: adaptation.Confirmed, Confidence: adaptation.Score{Value: 1, Range: range01}, Provenance: "human-confirmation", ConfirmationAuthorityID: "owner-" + domain.name, ConfirmationEvidenceRef: "approval:" + domain.name, Context: domain.context, RecordedAt: base.Add(4 * time.Minute)})
			for _, fact := range []adaptation.ProfileFact{declared, inherited, measured, confirmed} {
				if err := ledger.RecordProfileFact(ctx, fact); err != nil {
					t.Fatal(err)
				}
			}
			if err := ledger.RecordProfileFact(ctx, observed); err == nil {
				t.Fatal("caller-authored observed profile bypassed evaluator derivation")
			}
			if err := ledger.RecordObservedProfile(ctx, observed, derivation); err != nil {
				t.Fatal(err)
			}
			policy, err := adaptation.FreezeProfileDivergencePolicy(adaptation.ProfileDivergencePolicy{PackageID: "package." + domain.name, Dimension: domain.dimension, Context: domain.context, ReferenceClasses: []adaptation.EvidenceClass{adaptation.Declared, adaptation.Inherited}, CurrentClasses: []adaptation.EvidenceClass{adaptation.Observed, adaptation.Measured}, MinimumDelta: 0.4})
			if err != nil {
				t.Fatal(err)
			}
			report, err := adaptation.EvaluateProfileDivergence([]adaptation.ProfileFact{declared, inherited, observed, measured, confirmed}, policy, base.Add(5*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if !report.Diverged || report.ReferenceFactID != inherited.ID || report.CurrentFactID != measured.ID {
				t.Fatalf("package policy did not expose semantic profile drift: %#v", report)
			}
			if err := ledger.RecordProfileDivergence(ctx, report); err != nil {
				t.Fatal(err)
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
			history, err := restarted.ProfileHistory(ctx, domain.agent)
			if err != nil {
				t.Fatal(err)
			}
			byClass := map[adaptation.EvidenceClass]adaptation.ProfileFact{}
			for _, fact := range history {
				byClass[fact.EvidenceClass] = fact
			}
			for class, expectedID := range map[adaptation.EvidenceClass]string{adaptation.Declared: declared.ID, adaptation.Inherited: inherited.ID, adaptation.Observed: observed.ID, adaptation.Measured: measured.ID, adaptation.Confirmed: confirmed.ID} {
				if byClass[class].ID != expectedID {
					t.Fatalf("restart lost %s evidence identity", class)
				}
			}
			derivations, err := restarted.ProfileDerivations(ctx, domain.agent)
			if err != nil {
				t.Fatal(err)
			}
			derivationFound := false
			for _, item := range derivations {
				if item.ID == derivation.ID && item.Evaluator.ID == domain.evaluator && item.SourceObservationIDs[0] == observation.ID {
					derivationFound = true
				}
			}
			if !derivationFound {
				t.Fatal("restart lost evaluator-bound observed-profile derivation")
			}
			divergences, err := restarted.ProfileDivergences(ctx, domain.agent)
			if err != nil {
				t.Fatal(err)
			}
			divergenceFound := false
			for _, item := range divergences {
				if item.ID == report.ID && item.Policy.ID == policy.ID && item.ReferenceFactID == inherited.ID && item.CurrentFactID == measured.ID {
					divergenceFound = true
				}
			}
			if !divergenceFound {
				t.Fatal("restart lost profile divergence and package policy")
			}
		})
	}
}
