package inference

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type routeAuthority struct {
	evidence map[string]EligibilityEvidence
}

func bridgeRouteRecord(t *testing.T, request RouteRequest, surface ExecutorSurface, decidedAt time.Time) RouteRecord {
	t.Helper()
	policy, err := FreezeRoutingPolicy(RoutingPolicy{Tier: request.Tier, RequiredMetrics: []MetricRule{{ID: "quality", Name: "quality", Kind: adaptation.DerivedMeasure, Operator: adaptation.GreaterThanOrEqual, Threshold: 1}}, ObjectiveMetric: "quality", MinimumIndependentRoots: 1})
	if err != nil {
		t.Fatal(err)
	}
	eligibility, err := FreezeEligibility(EligibilityEvidence{RequestID: request.ID, ExecutorID: surface.ExecutorID, ProviderID: surface.ProviderID, Tier: request.Tier, AuthorityID: surfaceEvaluator.ID, CapabilityEvidenceRef: "capability:" + surface.ID, PolicyEvidenceRef: "policy:" + surface.ID, Authorized: true, CapabilityGranted: true, PolicyAllowed: true, Available: true, EvaluatedAt: decidedAt.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	decision := EvidenceDecision{RequestID: request.ID, PolicyID: policy.ID, ExecutorID: surface.ExecutorID, ProviderID: surface.ProviderID, Tier: request.Tier, EligibilityID: eligibility.ID, MeasurementIDs: []string{"measurement"}, ObservationIDs: []string{"observation"}, IndependentRoots: []string{"root"}, ObjectiveMeasureID: "measurement"}
	decision.ID = inferenceDigest(decision)
	record, err := FreezeRouteRecord(RouteRecord{Request: request, Policy: policy, Eligibility: eligibility, Decision: decision, DecidedAt: decidedAt})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestSurfaceV2AndEvidenceRouteMustShareSelectionLineage(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	surfaceDecision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authorizeSurfaces(t, request, []ExecutorSurface{surface})), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	matching := bridgeRouteRecord(t, request.RouteRequest, surface, surfaceDecisionTime)
	other := surface
	other.ExecutorID, other.ProviderID = "executor:other", "provider:other"
	disagreeing := bridgeRouteRecord(t, request.RouteRequest, other, surfaceDecisionTime)

	ledgers := []*RouteLedger{}
	for range 3 {
		ledger, _ := NewRouteLedger(eventstore.NewMemoryStore())
		ledgers = append(ledgers, ledger)
	}
	if err := ledgers[0].RecordSurfaceDecision(ctx, surfaceDecision); err != nil {
		t.Fatal(err)
	}
	if err := ledgers[0].Record(ctx, disagreeing); err == nil {
		t.Fatal("legacy route disagreed with an already persisted surface selection")
	}
	if err := ledgers[1].Record(ctx, disagreeing); err != nil {
		t.Fatal(err)
	}
	if err := ledgers[1].RecordSurfaceDecision(ctx, surfaceDecision); err == nil {
		t.Fatal("surface selection disagreed with an already persisted legacy route")
	}
	if err := ledgers[2].Record(ctx, matching); err != nil {
		t.Fatal(err)
	}
	if err := ledgers[2].RecordSurfaceDecision(ctx, surfaceDecision); err != nil {
		t.Fatalf("matching route lineage was rejected: %v", err)
	}
	if replayed, err := ledgers[2].SurfaceDecisions(ctx, request.RouteRequest.SubjectAgentID); err != nil || len(replayed) != 1 {
		t.Fatalf("bound route lineage did not replay: %#v %v", replayed, err)
	}
}

func TestSurfaceRouteV1AndTamperedV2EventMetadataFailClosed(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authorizeSurfaces(t, request, []ExecutorSurface{surface})), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(decision)
	t.Run("unsupported v1", func(t *testing.T) {
		v1Decision := decision
		v1Decision.Version = "v1"
		if err := VerifySurfaceRoutingDecision(v1Decision); !errors.Is(err, contracts.ErrUnsupportedPreReleaseContractVersion) {
			t.Fatalf("v1 decision did not fail as unsupported pre-release: %v", err)
		}
		store := eventstore.NewMemoryStore()
		_, err := store.Append(ctx, routeAggregate(request.RouteRequest.SubjectAgentID), 0, []eventstore.Event{{ID: "event:v1", AggregateType: "inference_routes", Type: surfaceRouteDecisionEvent, Version: "v1", Actor: surfaceEvaluator, CommandID: "surface-route:v1", CorrelationID: request.RouteRequest.RunID, Trust: contracts.TrustPolicy, Payload: []byte(`{}`), CreatedAt: surfaceDecisionTime}})
		if err != nil {
			t.Fatal(err)
		}
		ledger, _ := NewRouteLedger(store)
		if _, err := ledger.SurfaceDecisions(ctx, request.RouteRequest.SubjectAgentID); !errors.Is(err, contracts.ErrUnsupportedPreReleaseContractVersion) {
			t.Fatalf("v1 replay did not fail as unsupported pre-release: %v", err)
		}
	})
	for _, test := range []struct {
		name   string
		mutate func(*eventstore.Event)
	}{
		{name: "actor", mutate: func(event *eventstore.Event) { event.Actor = contracts.PrincipalRef{ID: "other", Kind: "authority"} }},
		{name: "command", mutate: func(event *eventstore.Event) { event.CommandID = "surface-route:other" }},
		{name: "causation", mutate: func(event *eventstore.Event) { event.CausationID = "request:other" }},
		{name: "created time", mutate: func(event *eventstore.Event) { event.CreatedAt = surfaceDecisionTime.Add(time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := eventstore.NewMemoryStore()
			event := eventstore.Event{ID: "event:" + decision.ID, AggregateType: "inference_routes", Type: surfaceRouteDecisionEvent, Version: "v2", Actor: surfaceEvaluator, CommandID: "surface-route:" + decision.ID, CorrelationID: request.RouteRequest.RunID, CausationID: request.RouteRequest.ID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: surfaceDecisionTime}
			test.mutate(&event)
			if _, err := store.Append(ctx, routeAggregate(request.RouteRequest.SubjectAgentID), 0, []eventstore.Event{event}); err != nil {
				t.Fatal(err)
			}
			ledger, _ := NewRouteLedger(store)
			if _, err := ledger.SurfaceDecisions(ctx, request.RouteRequest.SubjectAgentID); err == nil {
				t.Fatal("tampered surface event metadata replayed")
			}
		})
	}
}

func TestRouteReplayRejectsAuthorityMetadataMismatch(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 13, 23, 30, 0, 0, time.UTC)
	request, err := FreezeRouteRequest(RouteRequest{SubjectAgentID: "agent-research", RunID: "run-research", GoalClass: "research", Domain: "research", BehaviorKey: "search", Context: "literature", Tier: D1})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := FreezeRoutingPolicy(RoutingPolicy{Tier: D1, RequiredMetrics: []MetricRule{{ID: "sources", Name: "verified_sources", Kind: adaptation.DerivedMeasure, Unit: "sources", Operator: adaptation.GreaterThanOrEqual, Threshold: 5}}, ObjectiveMetric: "verified_sources", MinimumIndependentRoots: 1})
	if err != nil {
		t.Fatal(err)
	}
	eligibility, err := FreezeEligibility(EligibilityEvidence{RequestID: request.ID, ExecutorID: "research-executor", ProviderID: "research-provider", Tier: D1, AuthorityID: "runtime-governor", CapabilityEvidenceRef: "capability:research", PolicyEvidenceRef: "policy:research", Authorized: true, CapabilityGranted: true, PolicyAllowed: true, Available: true, EvaluatedAt: base})
	if err != nil {
		t.Fatal(err)
	}
	decision := EvidenceDecision{RequestID: request.ID, PolicyID: policy.ID, ExecutorID: eligibility.ExecutorID, ProviderID: eligibility.ProviderID, Tier: D1, EligibilityID: eligibility.ID, MeasurementIDs: []string{"measurement"}, ObservationIDs: []string{"observation"}, IndependentRoots: []string{"root"}, ObjectiveMeasureID: "measurement"}
	decision.ID = inferenceDigest(decision)
	record, err := FreezeRouteRecord(RouteRecord{Request: request, Policy: policy, Eligibility: eligibility, Decision: decision, DecidedAt: base.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := state.NewSQLiteEventStore(db)
	payload, _ := json.Marshal(record)
	_, err = store.Append(ctx, routeAggregate(request.SubjectAgentID), 0, []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "inference_routes", Type: routeDecisionEvent, Version: routeEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: "different-authority", Kind: "authority"}, CommandID: "route:" + record.ID, CorrelationID: request.RunID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: record.DecidedAt}})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewRouteLedger(store)
	if _, err := ledger.Records(ctx, request.SubjectAgentID); err == nil {
		t.Fatal("replay accepted route event whose actor did not match eligibility authority")
	}
}

func (a routeAuthority) EvaluateRouteEligibility(_ context.Context, _ RouteRequest, candidate RouteCandidate) (EligibilityEvidence, error) {
	return a.evidence[candidate.ExecutorID], nil
}

func TestEvidenceRoutingIsPolicyDrivenGovernedAndDurableAcrossDomains(t *testing.T) {
	domains := []struct {
		name, metric, unit string
		threshold          float64
		preferLower        bool
		values             map[string]float64
		want               string
	}{
		{name: "software-delivery", metric: "average_latency", unit: "ms", threshold: 1000, preferLower: true, values: map[string]float64{"executor-fast": 420, "executor-steady": 700, "executor-denied": 100}, want: "executor-fast"},
		{name: "research", metric: "verified_source_count", unit: "sources", threshold: 5, preferLower: false, values: map[string]float64{"executor-fast": 7, "executor-steady": 12, "executor-denied": 30}, want: "executor-steady"},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
			request, err := FreezeRouteRequest(RouteRequest{SubjectAgentID: "agent-" + domain.name, RunID: "selected-run", GoalClass: "routed-work", Domain: domain.name, BehaviorKey: "execute", Context: "scope", Tier: D1})
			if err != nil {
				t.Fatal(err)
			}
			observations := []adaptation.Observation{}
			measurements := []adaptation.Measurement{}
			candidates := []RouteCandidate{}
			eligibility := map[string]EligibilityEvidence{}
			index := 0
			for executor, value := range domain.values {
				provider := "provider:" + executor
				sourceIDs := []string{}
				for root := 0; root < 2; root++ {
					observation, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: "agent-" + domain.name, RunID: executor + "-run-" + string(rune('a'+root)), GoalClass: "routed-work", Domain: domain.name, BehaviorKey: "execute", Context: "scope", CausationRoot: executor + "-root-" + string(rune('a'+root)), Trust: contracts.TrustObserved, ReasoningTier: "D1", ProviderID: provider, Outcome: "complete", PathID: "inference", ObservedAt: base.Add(time.Duration(index+root) * time.Minute)})
					if err != nil {
						t.Fatal(err)
					}
					observations, sourceIDs = append(observations, observation), append(sourceIDs, observation.ID)
				}
				measurement, err := adaptation.FreezeMeasurement(adaptation.Measurement{SubjectAgentID: "agent-" + domain.name, GoalClass: "routed-work", Domain: domain.name, BehaviorKey: "execute", Context: "scope", Measure: adaptation.Measure{Name: domain.metric, Value: value, Kind: adaptation.DerivedMeasure, Unit: domain.unit, Evaluator: &adaptation.EvaluatorRef{ID: "package-evaluator", Version: "1"}, TransformID: domain.metric + "/v1", Provenance: "package:" + domain.name, SourceObservationIDs: sourceIDs, MeasuredAt: base.Add(10 * time.Minute)}})
				if err != nil {
					t.Fatal(err)
				}
				measurements = append(measurements, measurement)
				candidates = append(candidates, RouteCandidate{ExecutorID: executor, ProviderID: provider, MeasurementIDs: []string{measurement.ID}})
				allowed := executor != "executor-denied"
				entry, err := FreezeEligibility(EligibilityEvidence{RequestID: request.ID, ExecutorID: executor, ProviderID: provider, Tier: D1, AuthorityID: "runtime-governor", CapabilityEvidenceRef: "capability:" + executor, PolicyEvidenceRef: "policy:" + domain.name, Authorized: allowed, CapabilityGranted: allowed, PolicyAllowed: allowed, Available: true, EvaluatedAt: base.Add(11 * time.Minute)})
				if err != nil {
					t.Fatal(err)
				}
				eligibility[executor] = entry
				index += 2
			}
			operator := adaptation.GreaterThanOrEqual
			if domain.preferLower {
				operator = adaptation.LessThanOrEqual
			}
			policy, err := FreezeRoutingPolicy(RoutingPolicy{Tier: D1, RequiredMetrics: []MetricRule{{ID: "package-objective", Name: domain.metric, Kind: adaptation.DerivedMeasure, Unit: domain.unit, Operator: operator, Threshold: domain.threshold}}, ObjectiveMetric: domain.metric, PreferLowerObjective: domain.preferLower, MinimumIndependentRoots: 2})
			if err != nil {
				t.Fatal(err)
			}
			mismatched := make(map[string]EligibilityEvidence, len(eligibility))
			for executor, entry := range eligibility {
				entry.RequestID = "sha256:another-request"
				entry.ID = ""
				entry, err = FreezeEligibility(entry)
				if err != nil {
					t.Fatal(err)
				}
				mismatched[executor] = entry
			}
			if _, err := RouteFromEvidence(ctx, request, policy, candidates, observations, measurements, routeAuthority{evidence: mismatched}); err == nil {
				t.Fatal("routing accepted eligibility evidence issued for another request")
			}
			decision, err := RouteFromEvidence(ctx, request, policy, candidates, observations, measurements, routeAuthority{evidence: eligibility})
			if err != nil || decision.ExecutorID != domain.want || decision.ExecutorID == "executor-denied" || decision.RequestID != request.ID {
				t.Fatalf("routing ignored package objective or hard eligibility: %#v %v", decision, err)
			}

			path := filepath.Join(t.TempDir(), "praxis.db")
			db, err := state.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			ledger, _ := NewRouteLedger(state.NewSQLiteEventStore(db))
			record, err := FreezeRouteRecord(RouteRecord{Request: request, Policy: policy, Eligibility: eligibility[domain.want], Decision: decision, DecidedAt: base.Add(12 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Record(ctx, record); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = state.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restarted, _ := NewRouteLedger(state.NewSQLiteEventStore(db))
			replayed, err := restarted.Records(ctx, record.Request.SubjectAgentID)
			if err != nil || len(replayed) != 1 || replayed[0].ID != record.ID || replayed[0].Decision.ProviderID != "provider:"+domain.want {
				t.Fatalf("route authority/provider evidence did not survive restart: %#v %v", replayed, err)
			}
		})
	}
}
