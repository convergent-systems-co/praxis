package inference

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var surfaceDecisionTime = time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC)
var surfaceEvaluator = contracts.PrincipalRef{ID: "runtime-governor", Kind: "authority"}

type surfaceAuthority struct {
	principal contracts.PrincipalRef
	evidence  map[string]SurfaceEligibilityEvidence
}

func (a surfaceAuthority) Evaluator() contracts.PrincipalRef { return a.principal }
func (a surfaceAuthority) EvaluateSurfaceEligibility(_ context.Context, _ SurfaceRouteRequest, surface ExecutorSurface) (SurfaceEligibilityEvidence, error) {
	evidence, ok := a.evidence[surface.ID]
	if !ok {
		return SurfaceEligibilityEvidence{}, errors.New("missing authoritative surface evidence")
	}
	return evidence, nil
}

func surfaceTarget(mut func(*contracts.ExecutionTarget)) contracts.EffectiveExecutionTarget {
	target := contracts.ExecutionTarget{Version: "1", RequiredCapabilities: []string{"reasoning"}, ReasoningTier: "D1", APIPolicy: contracts.APIPolicyAllow, SourceAuthority: "node:deliver", Scope: "node:deliver", TransportPolicy: []contracts.TransportClass{contracts.TransportSubscriptionCLI, contracts.TransportMeteredAPI}}
	if mut != nil {
		mut(&target)
	}
	effective, err := contracts.MergeExecutionTargets([]contracts.ExecutionTargetContribution{{Authority: contracts.AuthorityNode, SourceRef: "node:deliver", SourceDigest: "sha256:node-deliver", Target: target}})
	if err != nil {
		panic(err)
	}
	return effective
}

func surfaceRequest(t *testing.T, target contracts.EffectiveExecutionTarget) SurfaceRouteRequest {
	t.Helper()
	route, err := FreezeRouteRequest(RouteRequest{SubjectAgentID: "agent:durable", RunID: "run:surface-routing", GoalClass: "delivery", Domain: "software", BehaviorKey: "implement", Context: "node:deliver", Tier: D1})
	if err != nil {
		t.Fatal(err)
	}
	request, err := FreezeSurfaceRouteRequest(SurfaceRouteRequest{RouteRequest: route, AgentGeneration: "generation:7", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func executorSurface(id, provider string, transport contracts.TransportClass, profiles ...string) ExecutorSurface {
	return ExecutorSurface{ID: id, ExecutorID: "executor:" + id, ProviderID: "provider:" + id, ProviderMetadata: provider, ModelMetadata: provider + " model", Capabilities: []string{"reasoning"}, Profiles: profiles, Transport: transport}
}

func authorizeSurfaces(t *testing.T, request SurfaceRouteRequest, surfaces []ExecutorSurface) surfaceAuthority {
	t.Helper()
	authority := surfaceAuthority{principal: surfaceEvaluator, evidence: map[string]SurfaceEligibilityEvidence{}}
	for _, surface := range surfaces {
		canonical, digest, err := freezeExecutorSurface(surface)
		if err != nil {
			t.Fatal(err)
		}
		evidence, err := FreezeSurfaceEligibilityEvidence(SurfaceEligibilityEvidence{
			RequestID: request.ID, SurfaceID: canonical.ID, SurfaceDigest: digest, Evaluator: surfaceEvaluator,
			Available: true, CapabilityGranted: true, SecurityAllowed: true, Authorized: true, PolicyAllowed: true,
			CapabilityEvidenceRef: "capability:" + canonical.ID, SecurityEvidenceRef: "security:" + canonical.ID,
			AuthorityEvidenceRef: "authority:" + canonical.ID, PolicyEvidenceRef: "policy:" + canonical.ID, AvailabilityRef: "health:" + canonical.ID,
			EvaluatedAt: surfaceDecisionTime.Add(-time.Minute), ValidUntil: surfaceDecisionTime.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		authority.evidence[canonical.ID] = evidence
	}
	return authority
}

func TestExecutorSurfaceV2UsesAuthoritativeEvidenceAndNeutralMetadata(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles, target.AllowedFallbackProfiles = []string{"interactive"}, []string{"batch"}
	}))
	surfaces := []ExecutorSurface{
		executorSurface("surface-d", "Anthropic metered API", contracts.TransportMeteredAPI, "batch"),
		executorSurface("surface-c", "Claude subscription CLI", contracts.TransportSubscriptionCLI, "interactive"),
		executorSurface("surface-b", "OpenAI metered API", contracts.TransportMeteredAPI, "batch"),
		executorSurface("surface-a", "Codex subscription CLI", contracts.TransportSubscriptionCLI, "interactive"),
	}
	authority := authorizeSurfaces(t, request, surfaces)
	decision, err := SelectExecutorSurface(ctx, request, surfaces, authority, surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Version != "v2" || decision.Evaluator != surfaceEvaluator || decision.SelectedSurfaceID != "surface-a" || decision.Fallback || decision.Request.RouteRequest.SubjectAgentID != "agent:durable" {
		t.Fatalf("v2 selection lost authority, determinism, or agent lineage: %#v", decision)
	}
	reversed := append([]ExecutorSurface(nil), surfaces...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	again, err := SelectExecutorSurface(ctx, request, reversed, authority, surfaceDecisionTime)
	if err != nil || again.ID != decision.ID {
		t.Fatalf("input order changed v2 decision: %#v %v", again, err)
	}
}

func TestSurfaceRequestRejectsUnverifiedEffectiveTarget(t *testing.T) {
	base := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles = []string{"z-quality", "a-economy"}
	}))
	tests := []struct {
		name   string
		mutate func(*contracts.EffectiveExecutionTarget)
	}{
		{name: "manual without contributions", mutate: func(target *contracts.EffectiveExecutionTarget) {
			target.Contributions = nil
		}},
		{name: "post-merge target mutation", mutate: func(target *contracts.EffectiveExecutionTarget) {
			target.Target.PreferredProfiles[0], target.Target.PreferredProfiles[1] = target.Target.PreferredProfiles[1], target.Target.PreferredProfiles[0]
		}},
		{name: "post-merge provenance mutation", mutate: func(target *contracts.EffectiveExecutionTarget) {
			target.Contributions[0].SourceDigest = "sha256:forged"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			request.ID = ""
			test.mutate(&request.Target)
			if _, err := FreezeSurfaceRouteRequest(request); err == nil {
				t.Fatal("surface boundary accepted an unverified effective target")
			}
		})
	}
}

func TestSurfaceV2PreservesAuthorityOrderedPreferenceAndFallback(t *testing.T) {
	merge := func(t *testing.T, preferred, fallback []string) contracts.EffectiveExecutionTarget {
		t.Helper()
		platform := contracts.ExecutionTarget{
			Version: "1", RequiredCapabilities: []string{"reasoning"}, ReasoningTier: "D1",
			PreferredProfiles: preferred, AllowedFallbackProfiles: fallback,
			APIPolicy: contracts.APIPolicyAllow, SourceAuthority: "platform:security", Scope: "node:deliver",
			TransportPolicy: []contracts.TransportClass{contracts.TransportSubscriptionCLI},
		}
		node := platform
		node.SourceAuthority = "node:deliver"
		node.PreferredProfiles = []string{"b-node"}
		node.AllowedFallbackProfiles = append([]string(nil), fallback...)
		for left, right := 0, len(node.AllowedFallbackProfiles)-1; left < right; left, right = left+1, right-1 {
			node.AllowedFallbackProfiles[left], node.AllowedFallbackProfiles[right] = node.AllowedFallbackProfiles[right], node.AllowedFallbackProfiles[left]
		}
		effective, err := contracts.MergeExecutionTargets([]contracts.ExecutionTargetContribution{
			{Authority: contracts.AuthorityNode, SourceRef: "node:deliver", SourceDigest: "sha256:node", Target: node},
			{Authority: contracts.AuthorityPlatformSecurity, SourceRef: "platform:security", SourceDigest: "sha256:platform", Target: platform},
		})
		if err != nil {
			t.Fatal(err)
		}
		return effective
	}

	t.Run("preferred", func(t *testing.T) {
		request := surfaceRequest(t, merge(t, []string{"z-quality", "a-economy"}, []string{"z-local", "a-local"}))
		if got, want := request.Target.Target.PreferredProfiles, []string{"z-quality", "a-economy", "b-node"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("frozen preferred profiles = %v, want %v", got, want)
		}
		surfaces := []ExecutorSurface{
			executorSurface("surface-a", "economy", contracts.TransportSubscriptionCLI, "a-economy"),
			executorSurface("surface-z", "quality", contracts.TransportSubscriptionCLI, "z-quality"),
		}
		decision, err := SelectExecutorSurface(context.Background(), request, surfaces, authorizeSurfaces(t, request, surfaces), surfaceDecisionTime)
		if err != nil || decision.SelectedProfile != "z-quality" || decision.SelectedSurfaceID != "surface-z" {
			t.Fatalf("semantic preferred order was not preserved: %#v %v", decision, err)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		request := surfaceRequest(t, merge(t, []string{"missing"}, []string{"z-local", "a-local"}))
		if got, want := request.Target.Target.AllowedFallbackProfiles, []string{"z-local", "a-local"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("frozen fallback profiles = %v, want %v", got, want)
		}
		surfaces := []ExecutorSurface{
			executorSurface("surface-a", "local-a", contracts.TransportSubscriptionCLI, "a-local"),
			executorSurface("surface-z", "local-z", contracts.TransportSubscriptionCLI, "z-local"),
		}
		decision, err := SelectExecutorSurface(context.Background(), request, surfaces, authorizeSurfaces(t, request, surfaces), surfaceDecisionTime)
		if err != nil || !decision.Fallback || decision.SelectedProfile != "z-local" || decision.SelectedSurfaceID != "surface-z" {
			t.Fatalf("semantic fallback order was not preserved: %#v %v", decision, err)
		}
	})
}

func TestSurfaceV2RejectsForgedAndMismatchedEligibilityEvidence(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription metadata", contracts.TransportSubscriptionCLI, "interactive")
	base := authorizeSurfaces(t, request, []ExecutorSurface{surface})
	tests := []struct {
		name     string
		mutate   func(*SurfaceEligibilityEvidence)
		refreeze bool
	}{
		{name: "tampered boolean", mutate: func(e *SurfaceEligibilityEvidence) { e.SecurityAllowed = false }},
		{name: "tampered ref", mutate: func(e *SurfaceEligibilityEvidence) { e.SecurityEvidenceRef = "forged" }},
		{name: "wrong request", mutate: func(e *SurfaceEligibilityEvidence) { e.RequestID = "sha256:other" }, refreeze: true},
		{name: "wrong surface", mutate: func(e *SurfaceEligibilityEvidence) { e.SurfaceID = "surface-b" }, refreeze: true},
		{name: "wrong metadata digest", mutate: func(e *SurfaceEligibilityEvidence) { e.SurfaceDigest = "sha256:other" }, refreeze: true},
		{name: "wrong evaluator", mutate: func(e *SurfaceEligibilityEvidence) {
			e.Evaluator = contracts.PrincipalRef{ID: "other", Kind: "authority"}
		}, refreeze: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority := base
			authority.evidence = map[string]SurfaceEligibilityEvidence{}
			evidence := base.evidence[surface.ID]
			test.mutate(&evidence)
			if test.refreeze {
				var err error
				evidence, err = FreezeSurfaceEligibilityEvidence(evidence)
				if err != nil {
					t.Fatal(err)
				}
			}
			authority.evidence[surface.ID] = evidence
			if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, authority, surfaceDecisionTime); err == nil {
				t.Fatal("forged or mismatched eligibility evidence was accepted")
			}
		})
	}
}

func TestSurfaceV2RejectsStaleEvidenceAndSelectsOnlyAuthorizedFallback(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles, target.AllowedFallbackProfiles = []string{"high-depth"}, []string{"balanced"}
	}))
	preferred := executorSurface("surface-a", "preferred", contracts.TransportSubscriptionCLI, "high-depth")
	fallback := executorSurface("surface-b", "fallback", contracts.TransportSubscriptionCLI, "balanced")
	authority := authorizeSurfaces(t, request, []ExecutorSurface{preferred, fallback})
	denied := authority.evidence[preferred.ID]
	denied.SecurityAllowed, denied.ReasonCodes = false, []string{"security:denied"}
	denied, _ = FreezeSurfaceEligibilityEvidence(denied)
	authority.evidence[preferred.ID] = denied
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{preferred, fallback}, authority, surfaceDecisionTime)
	if err != nil || decision.SelectedSurfaceID != fallback.ID || !decision.Fallback || decision.ReasonCodes[0] != ReasonAllowedFallback {
		t.Fatalf("denied preferred surface won or fallback was not explicit: %#v %v", decision, err)
	}

	stale := authority.evidence[fallback.ID]
	stale.EvaluatedAt, stale.ValidUntil = surfaceDecisionTime.Add(-2*time.Hour), surfaceDecisionTime.Add(-time.Hour)
	stale, _ = FreezeSurfaceEligibilityEvidence(stale)
	authority.evidence[fallback.ID] = stale
	if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{preferred, fallback}, authority, surfaceDecisionTime); err == nil {
		t.Fatal("stale eligibility evidence was accepted")
	}
}

func TestSurfaceV2PreservesRequiredAndAPIFailClosedSemantics(t *testing.T) {
	ctx := context.Background()
	requiredRequest := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.RequiredProfiles = []string{"regulated"}
		target.AllowedFallbackProfiles = []string{"balanced"}
	}))
	required := executorSurface("surface-a", "required", contracts.TransportSubscriptionCLI, "regulated")
	fallback := executorSurface("surface-b", "fallback", contracts.TransportSubscriptionCLI, "balanced")
	authority := authorizeSurfaces(t, requiredRequest, []ExecutorSurface{required, fallback})
	denied := authority.evidence[required.ID]
	denied.Authorized, denied.ReasonCodes = false, []string{"authority:denied"}
	denied, _ = FreezeSurfaceEligibilityEvidence(denied)
	authority.evidence[required.ID] = denied
	decision, err := SelectExecutorSurface(ctx, requiredRequest, []ExecutorSurface{required, fallback}, authority, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceRequiredUnavailable || decision.SelectedSurfaceID != "" {
		t.Fatalf("required target did not fail closed: %#v %v", decision, err)
	}

	apiRequest := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.APIPolicy = contracts.APIPolicyForbid
		target.TransportPolicy = []contracts.TransportClass{contracts.TransportSubscriptionCLI}
		target.PreferredProfiles = []string{"high-depth"}
		target.AllowedFallbackProfiles = []string{"balanced"}
	}))
	api := executorSurface("surface-api", "metered", contracts.TransportMeteredAPI, "balanced")
	cli := executorSurface("surface-cli", "subscription", contracts.TransportSubscriptionCLI, "economy")
	authority = authorizeSurfaces(t, apiRequest, []ExecutorSurface{api, cli})
	decision, err = SelectExecutorSurface(ctx, apiRequest, []ExecutorSurface{api, cli}, authority, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceAPIUseProhibited || decision.SelectedSurfaceID != "" || !contains(decision.Evaluations[0].ReasonCodes, ReasonAPIForbidden) {
		t.Fatalf("API prohibition did not fail closed: %#v %v", decision, err)
	}
}

func TestSurfaceV2TelemetryRequiresProvenanceFreshnessAndExplicitUnknown(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) { target.TelemetryRequirements = []string{"quota_state"} }))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	authority := authorizeSurfaces(t, request, []ExecutorSurface{surface})
	if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, authority, surfaceDecisionTime); err == nil {
		t.Fatal("authority omission of required telemetry was accepted")
	}
	evidence := authority.evidence[surface.ID]
	evidence.Telemetry = map[string]SurfaceTelemetryEvidence{"quota_state": {Known: false, ProvenanceRef: "quota:probe", ObservedAt: surfaceDecisionTime.Add(-time.Minute), ValidUntil: surfaceDecisionTime.Add(time.Minute)}}
	evidence, _ = FreezeSurfaceEligibilityEvidence(evidence)
	authority.evidence[surface.ID] = evidence
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, authority, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceTelemetryUnsatisfied || !contains(decision.Evaluations[0].ReasonCodes, ReasonTelemetryUnknown+":quota_state") {
		t.Fatalf("explicit unknown telemetry was not preserved: %#v %v", decision, err)
	}
	evidence = authority.evidence[surface.ID]
	evidence.Telemetry["quota_state"] = SurfaceTelemetryEvidence{Known: true, Value: "available", ProvenanceRef: "quota:probe", ObservedAt: surfaceDecisionTime.Add(-time.Hour), ValidUntil: surfaceDecisionTime.Add(-time.Minute)}
	evidence, _ = FreezeSurfaceEligibilityEvidence(evidence)
	authority.evidence[surface.ID] = evidence
	if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, authority, surfaceDecisionTime); err == nil {
		t.Fatal("stale telemetry evidence was accepted")
	}
}

func TestSurfaceDecisionV2PersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) { target.PreferredProfiles = []string{"interactive"} }))
	surface := executorSurface("surface-a", "Codex subscription CLI", contracts.TransportSubscriptionCLI, "interactive")
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, authorizeSurfaces(t, request, []ExecutorSurface{surface}), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewRouteLedger(state.NewSQLiteEventStore(db))
	if err := ledger.RecordSurfaceDecision(ctx, decision); err != nil {
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
	replayed, err := restarted.SurfaceDecision(ctx, "agent:durable", request.ID)
	if err != nil || replayed == nil || replayed.ID != decision.ID || replayed.Evaluator != surfaceEvaluator || !reflect.DeepEqual(replayed.ReasonCodes, decision.ReasonCodes) {
		t.Fatalf("v2 authoritative decision did not survive restart: %#v %v", replayed, err)
	}
}

func TestConcurrentSurfaceDecisionWritesConvergeOrConflictExplicitly(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, authorizeSurfaces(t, request, []ExecutorSurface{surface}), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewRouteLedger(eventstore.NewMemoryStore())
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() { ready.Done(); <-start; errs <- ledger.RecordSurfaceDecision(ctx, decision) }()
	}
	ready.Wait()
	close(start)
	first, second := <-errs, <-errs
	for _, err := range []error{first, second} {
		if err != nil && !errors.Is(err, eventstore.ErrVersionConflict) {
			t.Fatalf("concurrent write returned ambiguous error: %v", err)
		}
	}
	replayed, err := ledger.SurfaceDecisions(ctx, request.RouteRequest.SubjectAgentID)
	if err != nil || len(replayed) != 1 || replayed[0].ID != decision.ID {
		t.Fatalf("concurrent writes did not converge on one decision: %#v %v", replayed, err)
	}
	if err := ledger.RecordSurfaceDecision(ctx, decision); err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
}
