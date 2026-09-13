package agent_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/stateprovider"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fixedInferencePlans map[string]agent.InferenceRoutingPlan

func (p fixedInferencePlans) ResolveInferencePlan(_ context.Context, _ kernel.GraphDef, _ kernel.NodeDef, run *kernel.RunExecution, _ agent.ExecutionContext) (agent.InferenceRoutingPlan, error) {
	return p[run.RunID], nil
}

func TestRoutedInferenceDoesNotRedispatchPersistedPendingSelection(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)
	store := eventstore.NewMemoryStore()
	evidence, _ := adaptation.NewLedger(store)
	routes, _ := inference.NewRouteLedger(store)
	measurements := seedCapabilityEvidence(t, ctx, evidence, "agent-research", "research", "verified_source_count", "sources", map[string]float64{"research": 9}, base)
	request := freezeAgentRouteRequest(t, "agent-research", "run-pending", "research")
	policy, err := inference.FreezeRoutingPolicy(inference.RoutingPolicy{Tier: inference.D1, RequiredMetrics: []inference.MetricRule{{ID: "sources", Name: "verified_source_count", Kind: adaptation.DerivedMeasure, Unit: "sources", Operator: adaptation.GreaterThanOrEqual, Threshold: 5}}, ObjectiveMetric: "verified_source_count", MinimumIndependentRoots: 2})
	if err != nil {
		t.Fatal(err)
	}
	candidate := inference.RouteCandidate{ExecutorID: "research", ProviderID: "provider-research", MeasurementIDs: []string{measurements["research"].ID}}
	eligibility, err := inference.FreezeEligibility(inference.EligibilityEvidence{RequestID: request.ID, ExecutorID: candidate.ExecutorID, ProviderID: candidate.ProviderID, Tier: request.Tier, AuthorityID: "runtime-governor", CapabilityEvidenceRef: "capability:research", PolicyEvidenceRef: "policy:research", Authorized: true, CapabilityGranted: true, PolicyAllowed: true, Available: true, EvaluatedAt: base.Add(20 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	authority := fixedRouteAuthority{request.ID + "\x00" + candidate.ExecutorID: eligibility}
	observations, _ := evidence.Observations(ctx, request.SubjectAgentID)
	derived, _ := evidence.Measurements(ctx, request.SubjectAgentID)
	decision, err := inference.RouteFromEvidence(ctx, request, policy, []inference.RouteCandidate{candidate}, observations, derived, authority)
	if err != nil {
		t.Fatal(err)
	}
	record, err := inference.FreezeRouteRecord(inference.RouteRecord{Request: request, Policy: policy, Eligibility: eligibility, Decision: decision, DecidedAt: base.Add(21 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := routes.Record(ctx, record); err != nil {
		t.Fatal(err)
	}
	calls := 0
	executor := agent.RoutedAgentExecutor{Delegate: ordinaryAgentExecutor{}, Plans: fixedInferencePlans{request.RunID: {Request: request, Policy: policy, Candidates: []inference.RouteCandidate{candidate}}}, Authority: authority, Routes: routes, Evidence: evidence, Executors: map[string]agent.RoutedInferenceExecutor{"research": measuredInferenceExecutor{id: "research", provider: "provider-research", measure: "evidence_items", unit: "sources", value: 10, calls: &calls, observedAt: base.Add(22 * time.Minute)}}}
	graph := operationalFixture().Definition
	var inferenceNode kernel.NodeDef
	for _, node := range graph.Nodes {
		if node.Class == kernel.NodeInference {
			inferenceNode = node
			break
		}
	}
	_, err = executor.ExecuteAgentNode(ctx, graph, inferenceNode, &kernel.RunExecution{RunID: request.RunID}, agent.ExecutionContext{AgentID: request.SubjectAgentID, GoalRef: "goal", Scope: "scope"})
	if !errors.Is(err, agent.ErrRoutedInferenceOutcomeUnknown) || calls != 0 {
		t.Fatalf("pending selection was redispatched: calls=%d err=%v", calls, err)
	}
}

type fixedRouteAuthority map[string]inference.EligibilityEvidence

func (a fixedRouteAuthority) EvaluateRouteEligibility(_ context.Context, request inference.RouteRequest, candidate inference.RouteCandidate) (inference.EligibilityEvidence, error) {
	return a[request.ID+"\x00"+candidate.ExecutorID], nil
}

type ordinaryAgentExecutor struct{}

func (ordinaryAgentExecutor) ID() string { return "ordinary-graph-runtime" }
func (ordinaryAgentExecutor) ExecuteAgentNode(_ context.Context, _ kernel.GraphDef, _ kernel.NodeDef, _ *kernel.RunExecution, _ agent.ExecutionContext) (kernel.NodeResult, error) {
	return kernel.NodeResult{Outcome: "next"}, nil
}

type measuredInferenceExecutor struct {
	id, provider, measure, unit string
	value                       float64
	calls                       *int
	observedAt                  time.Time
}

func (e measuredInferenceExecutor) ID() string         { return e.id }
func (e measuredInferenceExecutor) ProviderID() string { return e.provider }
func (e measuredInferenceExecutor) ExecuteRoutedInference(_ context.Context, _ kernel.GraphDef, _ kernel.NodeDef, _ *kernel.RunExecution, _ agent.ExecutionContext) (agent.InferenceExecution, error) {
	*e.calls++
	return agent.InferenceExecution{Result: kernel.NodeResult{Outcome: "next", Evidence: []string{"executor-evidence:" + e.id}}, RawMeasures: []adaptation.Measure{{Name: e.measure, Value: e.value, Kind: adaptation.RawMeasure, Unit: e.unit, Provenance: "instrumentation:" + e.provider, MeasuredAt: e.observedAt}}, Invariants: []adaptation.InvariantResult{{Class: adaptation.SecurityInvariant, ControlID: "dispatch-authorized", Passed: true, EvidenceRef: "security:" + e.id}, {Class: adaptation.PolicyInvariant, ControlID: "package-policy", Passed: true, EvidenceRef: "policy:" + e.id}}}, nil
}

func TestOperationalGraphRoutesDispatchesObservesAndReplaysAcrossDomains(t *testing.T) {
	domains := []struct {
		name, unit, objective, rawName string
		alpha, beta, output            float64
		preferLower                    bool
	}{
		{name: "software-delivery", unit: "ms", objective: "average_latency", rawName: "latency_ms", alpha: 420, beta: 760, output: 510, preferLower: true},
		{name: "research", unit: "sources", objective: "verified_source_count", rawName: "evidence_items", alpha: 12, beta: 8, output: 14, preferLower: false},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
			path := filepath.Join(t.TempDir(), "praxis.db")
			provider, err := stateprovider.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			identity := agent.Agent{ID: "agent-" + domain.name, OwnerScope: "human:1", CurrentGeneration: "generation-1", Lifecycle: agent.AgentActive, CreatedAt: base}
			generation := agent.Generation{ID: "generation-1", AgentID: identity.ID, Number: 1, GraphRefs: []string{"agent.operations@2"}, CreationReason: "fixture", GovernanceRef: "governance:create", CreatedAt: base}
			runtimeA := agent.Runtime{Events: provider.Events(), Graphs: fixedGraphs{operationalFixture()}, MaxMemory: 2, Now: func() time.Time { return base }}
			runtimeA.Memory = runtimeA
			if err := runtimeA.Create(ctx, identity, generation, contracts.PrincipalRef{ID: "human-1", Kind: "user"}, "create-"+identity.ID); err != nil {
				t.Fatal(err)
			}
			evidenceA, _ := adaptation.NewLedger(provider.Events())
			routesA, _ := inference.NewRouteLedger(provider.Events())
			deniedValue := float64(100)
			if domain.preferLower {
				deniedValue = 1
			}
			measurements := seedCapabilityEvidence(t, ctx, evidenceA, identity.ID, domain.name, domain.objective, domain.unit, map[string]float64{"alpha": domain.alpha, "beta": domain.beta, "denied": deniedValue}, base)
			operator := adaptation.GreaterThanOrEqual
			threshold := float64(5)
			if domain.preferLower {
				operator, threshold = adaptation.LessThanOrEqual, 1000
			}
			policy, err := inference.FreezeRoutingPolicy(inference.RoutingPolicy{Tier: inference.D1, RequiredMetrics: []inference.MetricRule{{ID: "objective", Name: domain.objective, Kind: adaptation.DerivedMeasure, Unit: domain.unit, Operator: operator, Threshold: threshold}}, ObjectiveMetric: domain.objective, PreferLowerObjective: domain.preferLower, MinimumIndependentRoots: 2})
			if err != nil {
				t.Fatal(err)
			}
			firstRequest := freezeAgentRouteRequest(t, identity.ID, "run-first", domain.name)
			secondRequest := freezeAgentRouteRequest(t, identity.ID, "run-replacement", domain.name)
			plans := fixedInferencePlans{
				firstRequest.RunID: {
					Request: firstRequest,
					Policy:  policy,
					Candidates: []inference.RouteCandidate{
						{ExecutorID: "denied", ProviderID: "provider-denied", MeasurementIDs: []string{measurements["denied"].ID}},
						{ExecutorID: "alpha", ProviderID: "provider-alpha", MeasurementIDs: []string{measurements["alpha"].ID}},
						{ExecutorID: "beta", ProviderID: "provider-beta", MeasurementIDs: []string{measurements["beta"].ID}},
					},
				},
				secondRequest.RunID: {
					Request:    secondRequest,
					Policy:     policy,
					Candidates: []inference.RouteCandidate{{ExecutorID: "beta", ProviderID: "provider-beta", MeasurementIDs: []string{measurements["beta"].ID}}},
				},
			}
			authority := fixedRouteAuthority{}
			for _, request := range []inference.RouteRequest{firstRequest, secondRequest} {
				for _, candidate := range plans[request.RunID].Candidates {
					allowed := candidate.ExecutorID != "denied"
					entry, freezeErr := inference.FreezeEligibility(inference.EligibilityEvidence{RequestID: request.ID, ExecutorID: candidate.ExecutorID, ProviderID: candidate.ProviderID, Tier: request.Tier, AuthorityID: "runtime-governor", CapabilityEvidenceRef: "capability:" + candidate.ExecutorID, PolicyEvidenceRef: "policy:" + domain.name, Authorized: allowed, CapabilityGranted: allowed, PolicyAllowed: allowed, Available: true, EvaluatedAt: base.Add(20 * time.Minute)})
					if freezeErr != nil {
						t.Fatal(freezeErr)
					}
					authority[request.ID+"\x00"+candidate.ExecutorID] = entry
				}
			}
			alphaCalls, betaCalls, deniedCalls := 0, 0, 0
			routedA := agent.RoutedAgentExecutor{Delegate: ordinaryAgentExecutor{}, Plans: plans, Authority: authority, Routes: routesA, Evidence: evidenceA, Executors: map[string]agent.RoutedInferenceExecutor{"alpha": measuredInferenceExecutor{id: "alpha", provider: "provider-alpha", measure: domain.rawName, unit: domain.unit, value: domain.output, calls: &alphaCalls, observedAt: base.Add(21 * time.Minute)}, "beta": measuredInferenceExecutor{id: "beta", provider: "provider-beta", measure: domain.rawName, unit: domain.unit, value: domain.output, calls: &betaCalls, observedAt: base.Add(21 * time.Minute)}, "denied": measuredInferenceExecutor{id: "denied", provider: "provider-denied", measure: domain.rawName, unit: domain.unit, value: domain.output, calls: &deniedCalls, observedAt: base.Add(21 * time.Minute)}}, Now: func() time.Time { return base.Add(22 * time.Minute) }}
			first, err := runtimeA.Execute(ctx, agent.ExecuteRequest{AgentID: identity.ID, RunID: firstRequest.RunID, GoalRef: "goal", Scope: "scope", Executor: routedA})
			if err != nil || first.Run.State != kernel.RunSucceeded || alphaCalls != 1 || betaCalls != 0 || deniedCalls != 0 {
				t.Fatalf("first routed graph execution failed: result=%#v calls=%d/%d/denied:%d err=%v", first, alphaCalls, betaCalls, deniedCalls, err)
			}
			if err := provider.Close(); err != nil {
				t.Fatal(err)
			}

			restarted, err := stateprovider.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			evidenceB, _ := adaptation.NewLedger(restarted.Events())
			routesB, _ := inference.NewRouteLedger(restarted.Events())
			runtimeB := agent.Runtime{Events: restarted.Events(), Graphs: fixedGraphs{operationalFixture()}, MaxMemory: 2, Now: func() time.Time { return base.Add(30 * time.Minute) }}
			runtimeB.Memory = runtimeB
			routedB := agent.RoutedAgentExecutor{Delegate: ordinaryAgentExecutor{}, Plans: plans, Authority: authority, Routes: routesB, Evidence: evidenceB, Executors: map[string]agent.RoutedInferenceExecutor{"beta": measuredInferenceExecutor{id: "beta", provider: "provider-beta", measure: domain.rawName, unit: domain.unit, value: domain.output, calls: &betaCalls, observedAt: base.Add(31 * time.Minute)}}, Now: func() time.Time { return base.Add(32 * time.Minute) }}
			second, err := runtimeB.Execute(ctx, agent.ExecuteRequest{AgentID: identity.ID, RunID: secondRequest.RunID, GoalRef: "goal", Scope: "scope", Executor: routedB})
			if err != nil || second.Run.State != kernel.RunSucceeded || alphaCalls != 1 || betaCalls != 1 || deniedCalls != 0 || second.AgentID != first.AgentID || second.GenerationID != first.GenerationID {
				t.Fatalf("replacement routed graph execution failed: result=%#v calls=%d/%d err=%v", second, alphaCalls, betaCalls, err)
			}
			records, err := routesB.Records(ctx, identity.ID)
			if err != nil {
				t.Fatalf("route history lost provider replacement: %#v %v", records, err)
			}
			recordsByRequest := map[string]inference.RouteRecord{}
			for _, record := range records {
				recordsByRequest[record.Request.ID] = record
			}
			if len(recordsByRequest) != len(records) || recordsByRequest[firstRequest.ID].Decision.ProviderID != "provider-alpha" || recordsByRequest[secondRequest.ID].Decision.ProviderID != "provider-beta" || recordsByRequest[firstRequest.ID].Decision.ExecutorID == "denied" {
				t.Fatalf("route history does not contain the governed request/provider decisions: %#v", recordsByRequest)
			}
			outcomes, err := routesB.Outcomes(ctx, identity.ID)
			if err != nil {
				t.Fatalf("route outcomes lost native evidence or lineage: %#v %v", outcomes, err)
			}
			outcomesByRequest := map[string]inference.RouteOutcome{}
			for _, outcome := range outcomes {
				outcomesByRequest[outcome.RequestID] = outcome
			}
			if len(outcomesByRequest) != len(outcomes) {
				t.Fatalf("multiple outcomes survived for one request: %#v", outcomes)
			}
			for _, request := range []inference.RouteRequest{firstRequest, secondRequest} {
				outcome, ok := outcomesByRequest[request.ID]
				record := recordsByRequest[request.ID]
				if !ok || outcome.RouteRecordID != record.ID || outcome.Observation.PathID != record.ID || outcome.Observation.RunID != request.RunID || outcome.Observation.ProviderID != record.Decision.ProviderID || !hasRawMeasure(outcome.Observation.RawMeasures, domain.rawName, domain.unit) || !hasPassedInvariant(outcome.Observation.Invariants, adaptation.SecurityInvariant, "dispatch-authorized") || !hasPassedInvariant(outcome.Observation.Invariants, adaptation.PolicyInvariant, "package-policy") {
					t.Fatalf("required routed outcome evidence is missing or corrupted: request=%#v outcome=%#v", request, outcome)
				}
			}
			observations, err := evidenceB.Observations(ctx, identity.ID)
			if err != nil {
				t.Fatal(err)
			}
			observationsByID := map[string]adaptation.Observation{}
			for _, observation := range observations {
				observationsByID[observation.ID] = observation
			}
			for executor, measurement := range measurements {
				for _, sourceID := range measurement.Measure.SourceObservationIDs {
					if observation, ok := observationsByID[sourceID]; !ok || observation.ProviderID != "provider-"+executor || observation.Domain != domain.name || observation.CausationRoot == "" {
						t.Fatalf("required demonstrated-capability source is absent after restart: executor=%s source=%s", executor, sourceID)
					}
				}
			}
			for _, outcome := range outcomesByRequest {
				if observation, ok := observationsByID[outcome.Observation.ID]; !ok || !reflect.DeepEqual(observation, outcome.Observation) {
					t.Fatalf("exact routed outcome observation is absent from adaptive replay: %#v", outcome.Observation)
				}
			}
		})
	}
}

func hasRawMeasure(measures []adaptation.Measure, name, unit string) bool {
	for _, measure := range measures {
		if measure.Name == name && measure.Unit == unit && measure.Kind == adaptation.RawMeasure {
			return true
		}
	}
	return false
}

func hasPassedInvariant(invariants []adaptation.InvariantResult, class adaptation.InvariantClass, controlID string) bool {
	for _, invariant := range invariants {
		if invariant.Class == class && invariant.ControlID == controlID && invariant.Passed && invariant.EvidenceRef != "" {
			return true
		}
	}
	return false
}

func freezeAgentRouteRequest(t *testing.T, agentID, runID, domain string) inference.RouteRequest {
	t.Helper()
	request, err := inference.FreezeRouteRequest(inference.RouteRequest{SubjectAgentID: agentID, RunID: runID, GoalClass: "work", Domain: domain, BehaviorKey: "reflect", Context: "scope", Tier: inference.D1})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func seedCapabilityEvidence(t *testing.T, ctx context.Context, ledger *adaptation.Ledger, subject, domain, metric, unit string, values map[string]float64, base time.Time) map[string]adaptation.Measurement {
	t.Helper()
	out := map[string]adaptation.Measurement{}
	index := 0
	for executor, value := range values {
		sources := []string{}
		for root := 0; root < 2; root++ {
			observation, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: subject, RunID: "history-" + executor + string(rune('a'+root)), GoalClass: "work", Domain: domain, BehaviorKey: "reflect", Context: "scope", CausationRoot: "root-" + executor + string(rune('a'+root)), Trust: contracts.TrustObserved, ReasoningTier: "D1", ProviderID: "provider-" + executor, Outcome: "next", PathID: "historical", ObservedAt: base.Add(time.Duration(index+root) * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Record(ctx, observation); err != nil {
				t.Fatal(err)
			}
			sources = append(sources, observation.ID)
		}
		measurement, err := adaptation.FreezeMeasurement(adaptation.Measurement{SubjectAgentID: subject, GoalClass: "work", Domain: domain, BehaviorKey: "reflect", Context: "scope", Measure: adaptation.Measure{Name: metric, Value: value, Kind: adaptation.DerivedMeasure, Unit: unit, Evaluator: &adaptation.EvaluatorRef{ID: "package-evaluator", Version: "1"}, TransformID: metric + "/v1", Provenance: "package:" + domain, SourceObservationIDs: sources, MeasuredAt: base.Add(10 * time.Minute)}})
		if err != nil {
			t.Fatal(err)
		}
		if err := ledger.RecordMeasurement(ctx, measurement); err != nil {
			t.Fatal(err)
		}
		out[executor] = measurement
		index += 2
	}
	return out
}
