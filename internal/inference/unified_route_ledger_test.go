package inference

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func unifiedRecordFor(t *testing.T, request SurfaceRouteRequest, surfaces []ExecutorSurface, authority EligibilityAuthority) UnifiedRouteRecord {
	t.Helper()
	decision, err := SelectExecutorSurface(context.Background(), request, surfaces, surfaceComposer(t, authority), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	record, err := FreezeUnifiedRouteRecord(UnifiedRouteRecord{Decision: decision})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestUnifiedRouteRecordPersistsSelectionAtomically(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	record := unifiedRecordFor(t, request, []ExecutorSurface{surface}, authorizeSurfaces(t, request, []ExecutorSurface{surface}))
	store := eventstore.NewMemoryStore()
	ledger, _ := NewRouteLedger(store)
	if err := ledger.RecordUnifiedRoute(ctx, record); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadAggregate(ctx, routeAggregate(request.RouteRequest.SubjectAgentID), 0)
	if err != nil || len(events) != 1 || events[0].Type != unifiedRouteRecordEvent {
		t.Fatalf("unified selection was not one atomic event: %#v %v", events, err)
	}
	if events[0].Type == routeDecisionEvent || events[0].Type == surfaceRouteDecisionEvent {
		t.Fatal("unified selection emitted a paired authoritative event")
	}
}

func TestUnifiedRouteRecordPersistsEveryStableTerminalFailure(t *testing.T) {
	tests := []struct {
		name    string
		outcome SurfaceDecisionOutcome
		build   func(*testing.T) (SurfaceRouteRequest, []ExecutorSurface, EligibilityAuthority)
	}{
		{name: "no eligible", outcome: SurfaceNoEligible, build: func(t *testing.T) (SurfaceRouteRequest, []ExecutorSurface, EligibilityAuthority) {
			request := surfaceRequest(t, surfaceTarget(nil))
			surfaces := []ExecutorSurface{executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")}
			authority := authorizeSurfaces(t, request, surfaces)
			evidence := authority.evidence[surfaces[0].ID]
			evidence.Available = false
			evidence, _ = FreezeEligibility(evidence)
			authority.evidence[surfaces[0].ID] = evidence
			return request, surfaces, authority
		}},
		{name: "required unavailable", outcome: SurfaceRequiredUnavailable, build: func(t *testing.T) (SurfaceRouteRequest, []ExecutorSurface, EligibilityAuthority) {
			request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) { target.RequiredProfiles = []string{"regulated"} }))
			surfaces := []ExecutorSurface{executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")}
			return request, surfaces, authorizeSurfaces(t, request, surfaces)
		}},
		{name: "fallback prohibited", outcome: SurfaceFallbackProhibited, build: func(t *testing.T) (SurfaceRouteRequest, []ExecutorSurface, EligibilityAuthority) {
			request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
				target.PreferredProfiles, target.AllowedFallbackProfiles = []string{"missing"}, nil
			}))
			surfaces := []ExecutorSurface{executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")}
			return request, surfaces, authorizeSurfaces(t, request, surfaces)
		}},
		{name: "api prohibited", outcome: SurfaceAPIUseProhibited, build: func(t *testing.T) (SurfaceRouteRequest, []ExecutorSurface, EligibilityAuthority) {
			request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
				target.APIPolicy = contracts.APIPolicyForbid
				target.TransportPolicy = []contracts.TransportClass{contracts.TransportSubscriptionCLI}
			}))
			surfaces := []ExecutorSurface{executorSurface("surface-api", "metered", contracts.TransportMeteredAPI, "interactive")}
			return request, surfaces, authorizeSurfaces(t, request, surfaces)
		}},
		{name: "telemetry unsatisfied", outcome: SurfaceTelemetryUnsatisfied, build: func(t *testing.T) (SurfaceRouteRequest, []ExecutorSurface, EligibilityAuthority) {
			request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) { target.TelemetryRequirements = []string{"quota_state"} }))
			surfaces := []ExecutorSurface{executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")}
			authority := authorizeSurfaces(t, request, surfaces)
			evidence := authority.evidence[surfaces[0].ID]
			evidence.Telemetry = map[string]SurfaceTelemetryEvidence{"quota_state": {Known: false, ProvenanceRef: "quota:probe", ProvenanceDigest: inferenceDigest("quota:probe"), ObservedAt: surfaceDecisionTime.Add(-time.Minute), ValidUntil: surfaceDecisionTime.Add(time.Minute)}}
			evidence, _ = FreezeEligibility(evidence)
			authority.evidence[surfaces[0].ID] = evidence
			return request, surfaces, authority
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, surfaces, authority := test.build(t)
			record := unifiedRecordFor(t, request, surfaces, authority)
			if record.Decision.Outcome != test.outcome {
				t.Fatalf("outcome = %s, want %s", record.Decision.Outcome, test.outcome)
			}
			store := eventstore.NewMemoryStore()
			ledger, _ := NewRouteLedger(store)
			if err := ledger.RecordUnifiedRoute(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			replayed, err := ledger.UnifiedRoutes(context.Background(), request.RouteRequest.SubjectAgentID)
			if err != nil || len(replayed) != 1 || replayed[0].ID != record.ID || replayed[0].Decision.Outcome != test.outcome {
				t.Fatalf("stable failure did not replay through unified contract: %#v %v", replayed, err)
			}
			events, _ := store.LoadAggregate(context.Background(), routeAggregate(request.RouteRequest.SubjectAgentID), 0)
			if len(events) != 1 || events[0].Type != unifiedRouteRecordEvent {
				t.Fatalf("stable failure was not one atomic unified event: %#v", events)
			}
		})
	}
}

func TestUnifiedRouteReplayRejectsPayloadAndMetadataTampering(t *testing.T) {
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	record := unifiedRecordFor(t, request, []ExecutorSurface{surface}, authorizeSurfaces(t, request, []ExecutorSurface{surface}))
	payload, _ := json.Marshal(record)
	tests := []struct {
		name   string
		mutate func(*eventstore.Event)
	}{
		{name: "actor", mutate: func(event *eventstore.Event) { event.Actor.ID = "other" }},
		{name: "causation", mutate: func(event *eventstore.Event) { event.CausationID = "sha256:other" }},
		{name: "payload", mutate: func(event *eventstore.Event) {
			var tampered UnifiedRouteRecord
			_ = json.Unmarshal(event.Payload, &tampered)
			tampered.Decision.Request.AgentGeneration = "forged"
			event.Payload, _ = json.Marshal(tampered)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := eventstore.NewMemoryStore()
			event := eventstore.Event{ID: "event:" + record.ID, AggregateType: "inference_routes", Type: unifiedRouteRecordEvent, Version: "v2", Actor: record.Decision.Evaluator, CommandID: "unified-route:" + record.ID, CorrelationID: request.RouteRequest.RunID, CausationID: request.ID, Trust: contracts.TrustPolicy, Payload: append([]byte(nil), payload...), CreatedAt: record.Decision.DecidedAt}
			test.mutate(&event)
			_, _ = store.Append(context.Background(), routeAggregate(request.RouteRequest.SubjectAgentID), 0, []eventstore.Event{event})
			ledger, _ := NewRouteLedger(store)
			if _, err := ledger.UnifiedRoutes(context.Background(), request.RouteRequest.SubjectAgentID); err == nil {
				t.Fatal("tampered unified event replayed")
			}
		})
	}
}

func TestUnifiedRouteOutcomeBindsRecordIdentityAndReplayMetadata(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	record := unifiedRecordFor(t, request, []ExecutorSurface{surface}, authorizeSurfaces(t, request, []ExecutorSurface{surface}))
	store := eventstore.NewMemoryStore()
	ledger, _ := NewRouteLedger(store)
	if err := ledger.RecordUnifiedRoute(ctx, record); err != nil {
		t.Fatal(err)
	}
	observedAt := surfaceDecisionTime.Add(time.Minute)
	observation, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: request.RouteRequest.SubjectAgentID, RunID: request.RouteRequest.RunID, GoalClass: request.RouteRequest.GoalClass, Domain: request.RouteRequest.Domain, BehaviorKey: request.RouteRequest.BehaviorKey, Context: request.RouteRequest.Context, CausationRoot: record.ID, Trust: contracts.TrustObserved, ReasoningTier: string(request.RouteRequest.Tier), ProviderID: surface.ProviderID, Outcome: "complete", PathID: record.ID, ObservedAt: observedAt})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := FreezeRouteOutcome(RouteOutcome{RouteRecordID: record.ID, RequestID: request.ID, ExecutorID: surface.ExecutorID, ProviderID: surface.ProviderID, ResultOutcome: "complete", Observation: observation, ObservedAt: observedAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordOutcome(ctx, request.RouteRequest.SubjectAgentID, outcome); err != nil {
		t.Fatal(err)
	}
	replayedRecord, replayedOutcome, err := ledger.UnifiedExecution(ctx, request.RouteRequest.SubjectAgentID, request.ID)
	if err != nil || replayedRecord == nil || replayedOutcome == nil || replayedOutcome.RouteRecordID != replayedRecord.ID {
		t.Fatalf("execution outcome did not replay against unified identity: %#v %#v %v", replayedRecord, replayedOutcome, err)
	}

	badStore := eventstore.NewMemoryStore()
	badLedger, _ := NewRouteLedger(badStore)
	if err := badLedger.RecordUnifiedRoute(ctx, record); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(outcome)
	_, err = badStore.Append(ctx, routeAggregate(request.RouteRequest.SubjectAgentID), 1, []eventstore.Event{{ID: "event:" + outcome.ID, AggregateType: "inference_routes", Type: routeOutcomeEvent, Version: "v1", Actor: contracts.PrincipalRef{ID: request.RouteRequest.SubjectAgentID, Kind: "agent"}, CommandID: "route-outcome:" + outcome.ID, CorrelationID: request.RouteRequest.RunID, CausationID: "sha256:other-route", Trust: contracts.TrustObserved, Payload: payload, CreatedAt: observedAt}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := badLedger.Outcomes(ctx, request.RouteRequest.SubjectAgentID); err == nil {
		t.Fatal("outcome with tampered unified-record causation replayed")
	}
}

func TestUnifiedTerminalFailureCannotAcceptExecutionOutcome(t *testing.T) {
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles = []string{"missing"}
		target.AllowedFallbackProfiles = nil
	}))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	record := unifiedRecordFor(t, request, []ExecutorSurface{surface}, authorizeSurfaces(t, request, []ExecutorSurface{surface}))
	ledger, _ := NewRouteLedger(eventstore.NewMemoryStore())
	if err := ledger.RecordUnifiedRoute(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	observedAt := surfaceDecisionTime.Add(time.Minute)
	observation, _ := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: request.RouteRequest.SubjectAgentID, RunID: request.RouteRequest.RunID, GoalClass: request.RouteRequest.GoalClass, Domain: request.RouteRequest.Domain, BehaviorKey: request.RouteRequest.BehaviorKey, Context: request.RouteRequest.Context, CausationRoot: record.ID, Trust: contracts.TrustObserved, ReasoningTier: string(request.RouteRequest.Tier), ProviderID: surface.ProviderID, Outcome: "complete", PathID: record.ID, ObservedAt: observedAt})
	outcome, _ := FreezeRouteOutcome(RouteOutcome{RouteRecordID: record.ID, RequestID: request.ID, ExecutorID: surface.ExecutorID, ProviderID: surface.ProviderID, ResultOutcome: "complete", Observation: observation, ObservedAt: observedAt})
	if err := ledger.RecordOutcome(context.Background(), request.RouteRequest.SubjectAgentID, outcome); err == nil {
		t.Fatal("terminal routing failure accepted an execution outcome")
	}
}

func TestLegacySurfaceWriteIsUnsupportedButValidMigrationReadRemains(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authorizeSurfaces(t, request, []ExecutorSurface{surface})), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	store := eventstore.NewMemoryStore()
	ledger, _ := NewRouteLedger(store)
	if err := ledger.RecordSurfaceDecision(ctx, decision); !errors.Is(err, contracts.ErrUnsupportedPreReleaseContractVersion) {
		t.Fatalf("legacy surface write error = %v", err)
	}
	payload, _ := json.Marshal(decision)
	_, err = store.Append(ctx, routeAggregate(request.RouteRequest.SubjectAgentID), 0, []eventstore.Event{{ID: "event:" + decision.ID, AggregateType: "inference_routes", Type: surfaceRouteDecisionEvent, Version: "v2", Actor: decision.Evaluator, CommandID: "surface-route:" + decision.ID, CorrelationID: request.RouteRequest.RunID, CausationID: request.RouteRequest.ID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: decision.DecidedAt}})
	if err != nil {
		t.Fatal(err)
	}
	read, err := ledger.SurfaceDecisions(ctx, request.RouteRequest.SubjectAgentID)
	if err != nil || len(read) != 1 || read[0].ID != decision.ID {
		t.Fatalf("valid legacy migration read failed: %#v %v", read, err)
	}
}
