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
	evidence map[string]EligibilityEvidence
}

func (a *surfaceAuthority) EvaluateRouteEligibility(_ context.Context, _ RouteRequest, candidate RouteCandidate) (EligibilityEvidence, error) {
	evidence, ok := a.evidence[candidate.SurfaceID]
	if !ok {
		return EligibilityEvidence{}, errors.New("missing authoritative surface evidence")
	}
	return evidence, nil
}

func surfaceComposer(t *testing.T, authority EligibilityAuthority) *SurfaceEligibilityComposer {
	t.Helper()
	composer, err := NewSurfaceEligibilityComposer(authority)
	if err != nil {
		t.Fatal(err)
	}
	return composer
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

func authorizeSurfaces(t *testing.T, request SurfaceRouteRequest, surfaces []ExecutorSurface) *surfaceAuthority {
	t.Helper()
	authority := &surfaceAuthority{evidence: map[string]EligibilityEvidence{}}
	for _, surface := range surfaces {
		canonical, digest, err := freezeExecutorSurface(surface)
		if err != nil {
			t.Fatal(err)
		}
		capabilityRef, policyRef := "capability:"+canonical.ID, "policy:"+canonical.ID
		securityRef, availabilityRef := "security:"+canonical.ID, "health:"+canonical.ID
		budgetRef, quotaRef := "budget:"+canonical.ID, "quota:"+canonical.ID
		evidence, err := FreezeEligibility(EligibilityEvidence{
			RequestID: request.RouteRequest.ID, ExecutorID: canonical.ExecutorID, ProviderID: canonical.ProviderID, Tier: request.RouteRequest.Tier,
			AuthorityID: surfaceEvaluator.ID, CapabilityEvidenceRef: capabilityRef, CapabilityEvidenceDigest: inferenceDigest(capabilityRef), PolicyEvidenceRef: policyRef, PolicyEvidenceDigest: inferenceDigest(policyRef),
			Authorized: true, CapabilityGranted: true, PolicyAllowed: true, Available: true,
			SurfaceRequestID: request.ID, SurfaceID: canonical.ID, SurfaceDigest: digest,
			AuthorityGenerationRef: "authority-generation:runtime-governor", AuthorityGenerationVersion: "7", AuthorityGenerationDigest: inferenceDigest("authority-generation:runtime-governor@7"),
			AuthorityScope: request.ID, SecurityAllowed: true, SecurityEvidenceRef: securityRef, SecurityEvidenceDigest: inferenceDigest(securityRef), AvailabilityEvidenceRef: availabilityRef, AvailabilityEvidenceDigest: inferenceDigest(availabilityRef),
			BudgetAllowed: true, BudgetEvidenceRef: budgetRef, BudgetEvidenceDigest: inferenceDigest(budgetRef), QuotaAllowed: true, QuotaEvidenceRef: quotaRef, QuotaEvidenceDigest: inferenceDigest(quotaRef),
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
	decision, err := SelectExecutorSurface(ctx, request, surfaces, surfaceComposer(t, authority), surfaceDecisionTime)
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
	again, err := SelectExecutorSurface(ctx, request, reversed, surfaceComposer(t, authority), surfaceDecisionTime)
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
		decision, err := SelectExecutorSurface(context.Background(), request, surfaces, surfaceComposer(t, authorizeSurfaces(t, request, surfaces)), surfaceDecisionTime)
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
		decision, err := SelectExecutorSurface(context.Background(), request, surfaces, surfaceComposer(t, authorizeSurfaces(t, request, surfaces)), surfaceDecisionTime)
		if err != nil || !decision.Fallback || decision.SelectedProfile != "z-local" || decision.SelectedSurfaceID != "surface-z" {
			t.Fatalf("semantic fallback order was not preserved: %#v %v", decision, err)
		}
	})
}

func TestSurfaceV2RejectsForgedAndMismatchedEligibilityEvidence(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription metadata", contracts.TransportSubscriptionCLI, "interactive")
	tests := []struct {
		name         string
		mutate       func(*EligibilityEvidence)
		refreeze     bool
		freezeReject bool
	}{
		{name: "tampered boolean", mutate: func(e *EligibilityEvidence) { e.SecurityAllowed = false }},
		{name: "tampered ref", mutate: func(e *EligibilityEvidence) { e.SecurityEvidenceRef = "forged" }},
		{name: "wrong request", mutate: func(e *EligibilityEvidence) { e.RequestID = "sha256:other" }, refreeze: true},
		{name: "wrong surface request", mutate: func(e *EligibilityEvidence) { e.SurfaceRequestID = "sha256:other"; e.AuthorityScope = "sha256:other" }, refreeze: true},
		{name: "wrong surface", mutate: func(e *EligibilityEvidence) { e.SurfaceID = "surface-b" }, refreeze: true},
		{name: "wrong metadata digest", mutate: func(e *EligibilityEvidence) { e.SurfaceDigest = inferenceDigest("other surface") }, refreeze: true},
		{name: "wrong scope", mutate: func(e *EligibilityEvidence) { e.AuthorityScope = "sha256:other" }, refreeze: true, freezeReject: true},
		{name: "opaque capability reference", mutate: func(e *EligibilityEvidence) { e.CapabilityEvidenceDigest = "" }, refreeze: true, freezeReject: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority := authorizeSurfaces(t, request, []ExecutorSurface{surface})
			evidence := authority.evidence[surface.ID]
			test.mutate(&evidence)
			if test.refreeze {
				var err error
				evidence, err = FreezeEligibility(evidence)
				if err != nil {
					if test.freezeReject {
						return
					}
					t.Fatalf("FreezeEligibility() error = %v", err)
				}
				if test.freezeReject {
					t.Fatal("FreezeEligibility() accepted incomplete governed surface lineage")
				}
			}
			authority.evidence[surface.ID] = evidence
			if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authority), surfaceDecisionTime); err == nil {
				t.Fatal("forged or mismatched eligibility evidence was accepted")
			}
		})
	}
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authorizeSurfaces(t, request, []ExecutorSurface{surface})), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	decision.Evaluator = contracts.PrincipalRef{ID: "caller-selected", Kind: "authority"}
	if err := VerifySurfaceRoutingDecision(decision); err == nil {
		t.Fatal("caller-selected evaluator principal was accepted on replay")
	}
}

func TestSurfaceEligibilityRejectsWellFormedMismatchedDigestsAtFreezeAndReplay(t *testing.T) {
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.TelemetryRequirements = []string{"quota_state"}
	}))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	validEvidence := func(t *testing.T) EligibilityEvidence {
		t.Helper()
		authority := authorizeSurfaces(t, request, []ExecutorSurface{surface})
		evidence := authority.evidence[surface.ID]
		evidence.Telemetry = map[string]SurfaceTelemetryEvidence{
			"quota_state": {Known: true, Value: "available", ProvenanceRef: "quota:probe", ProvenanceDigest: inferenceDigest("quota:probe"), ObservedAt: surfaceDecisionTime.Add(-time.Minute), ValidUntil: surfaceDecisionTime.Add(time.Minute)},
		}
		frozen, err := FreezeEligibility(evidence)
		if err != nil {
			t.Fatal(err)
		}
		return frozen
	}
	tests := []struct {
		name   string
		mutate func(*EligibilityEvidence)
	}{
		{name: "capability", mutate: func(e *EligibilityEvidence) { e.CapabilityEvidenceDigest = inferenceDigest("other capability") }},
		{name: "policy", mutate: func(e *EligibilityEvidence) { e.PolicyEvidenceDigest = inferenceDigest("other policy") }},
		{name: "security", mutate: func(e *EligibilityEvidence) { e.SecurityEvidenceDigest = inferenceDigest("other security") }},
		{name: "availability", mutate: func(e *EligibilityEvidence) { e.AvailabilityEvidenceDigest = inferenceDigest("other availability") }},
		{name: "budget", mutate: func(e *EligibilityEvidence) { e.BudgetEvidenceDigest = inferenceDigest("other budget") }},
		{name: "quota", mutate: func(e *EligibilityEvidence) { e.QuotaEvidenceDigest = inferenceDigest("other quota") }},
		{name: "telemetry", mutate: func(e *EligibilityEvidence) {
			telemetry := e.Telemetry["quota_state"]
			telemetry.ProvenanceDigest = inferenceDigest("other telemetry")
			e.Telemetry["quota_state"] = telemetry
		}},
		{name: "authority generation", mutate: func(e *EligibilityEvidence) { e.AuthorityGenerationDigest = inferenceDigest("other generation") }},
	}
	for _, test := range tests {
		t.Run(test.name+" freeze", func(t *testing.T) {
			evidence := validEvidence(t)
			evidence.Telemetry = cloneSurfaceTelemetry(evidence.Telemetry)
			test.mutate(&evidence)
			if _, err := FreezeEligibility(evidence); err == nil {
				t.Fatal("freeze accepted a well-formed digest that did not bind its reference")
			}
		})
		t.Run(test.name+" replay", func(t *testing.T) {
			authority := authorizeSurfaces(t, request, []ExecutorSurface{surface})
			authority.evidence[surface.ID] = validEvidence(t)
			decision, err := SelectExecutorSurface(context.Background(), request, []ExecutorSurface{surface}, surfaceComposer(t, authority), surfaceDecisionTime)
			if err != nil {
				t.Fatal(err)
			}
			evidence := &decision.Evaluations[0].Evidence.RouteEligibility
			evidence.Telemetry = cloneSurfaceTelemetry(evidence.Telemetry)
			test.mutate(evidence)
			evidence.ID = ""
			evidence.ID = inferenceDigest(*evidence)
			decision.Evaluations[0].Evidence.ID = ""
			decision.Evaluations[0].Evidence.ID = inferenceDigest(decision.Evaluations[0].Evidence)
			decision.ID = ""
			decision.ID = inferenceDigest(decision)
			if err := VerifySurfaceRoutingDecision(decision); err == nil {
				t.Fatal("replay accepted a well-formed digest that did not bind its reference")
			}
		})
	}
}

func TestSurfaceCompositionDerivesOneGovernedEvaluatorLineage(t *testing.T) {
	request := surfaceRequest(t, surfaceTarget(nil))
	surfaces := []ExecutorSurface{
		executorSurface("surface-a", "one", contracts.TransportSubscriptionCLI, "interactive"),
		executorSurface("surface-b", "two", contracts.TransportSubscriptionCLI, "interactive"),
	}
	authority := authorizeSurfaces(t, request, surfaces)
	second := authority.evidence["surface-b"]
	second.AuthorityID = "other-governor"
	second.AuthorityGenerationRef = "authority-generation:other"
	second.AuthorityGenerationDigest = inferenceDigest("authority-generation:other@7")
	second, err := FreezeEligibility(second)
	if err != nil {
		t.Fatal(err)
	}
	authority.evidence["surface-b"] = second
	if _, err := SelectExecutorSurface(context.Background(), request, surfaces, surfaceComposer(t, authority), surfaceDecisionTime); err == nil {
		t.Fatal("surface composition accepted mixed evaluator authority lineages")
	}
	if _, err := NewSurfaceEligibilityComposer(nil); err == nil {
		t.Fatal("surface composer accepted a missing governed eligibility authority")
	}
}

func TestSurfaceCompositionEnforcesGovernedBudgetAndQuota(t *testing.T) {
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	for _, test := range []struct {
		name   string
		reason string
		deny   func(*EligibilityEvidence)
	}{
		{name: "budget", reason: ReasonBudgetExhausted, deny: func(e *EligibilityEvidence) { e.BudgetAllowed = false }},
		{name: "quota", reason: ReasonQuotaUnavailable, deny: func(e *EligibilityEvidence) { e.QuotaAllowed = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			authority := authorizeSurfaces(t, request, []ExecutorSurface{surface})
			evidence := authority.evidence[surface.ID]
			test.deny(&evidence)
			var err error
			evidence, err = FreezeEligibility(evidence)
			if err != nil {
				t.Fatal(err)
			}
			authority.evidence[surface.ID] = evidence
			decision, err := SelectExecutorSurface(context.Background(), request, []ExecutorSurface{surface}, surfaceComposer(t, authority), surfaceDecisionTime)
			if err != nil || decision.Outcome != SurfaceNoEligible || !contains(decision.Evaluations[0].ReasonCodes, test.reason) {
				t.Fatalf("governed %s denial was not enforced: %#v %v", test.name, decision, err)
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
	denied.SecurityAllowed = false
	denied, _ = FreezeEligibility(denied)
	authority.evidence[preferred.ID] = denied
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{preferred, fallback}, surfaceComposer(t, authority), surfaceDecisionTime)
	if err != nil || decision.SelectedSurfaceID != fallback.ID || !decision.Fallback || decision.ReasonCodes[0] != ReasonAllowedFallback {
		t.Fatalf("denied preferred surface won or fallback was not explicit: %#v %v", decision, err)
	}

	stale := authority.evidence[fallback.ID]
	stale.EvaluatedAt, stale.ValidUntil = surfaceDecisionTime.Add(-2*time.Hour), surfaceDecisionTime.Add(-time.Hour)
	stale, _ = FreezeEligibility(stale)
	authority.evidence[fallback.ID] = stale
	if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{preferred, fallback}, surfaceComposer(t, authority), surfaceDecisionTime); err == nil {
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
	denied.Authorized = false
	denied, _ = FreezeEligibility(denied)
	authority.evidence[required.ID] = denied
	decision, err := SelectExecutorSurface(ctx, requiredRequest, []ExecutorSurface{required, fallback}, surfaceComposer(t, authority), surfaceDecisionTime)
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
	decision, err = SelectExecutorSurface(ctx, apiRequest, []ExecutorSurface{api, cli}, surfaceComposer(t, authority), surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceAPIUseProhibited || decision.SelectedSurfaceID != "" || !contains(decision.Evaluations[0].ReasonCodes, ReasonAPIForbidden) {
		t.Fatalf("API prohibition did not fail closed: %#v %v", decision, err)
	}
}

func TestSurfaceV2TelemetryRequiresProvenanceFreshnessAndExplicitUnknown(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) { target.TelemetryRequirements = []string{"quota_state"} }))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	authority := authorizeSurfaces(t, request, []ExecutorSurface{surface})
	if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authority), surfaceDecisionTime); err == nil {
		t.Fatal("authority omission of required telemetry was accepted")
	}
	evidence := authority.evidence[surface.ID]
	evidence.Telemetry = map[string]SurfaceTelemetryEvidence{"quota_state": {Known: false, ProvenanceRef: "quota:probe", ProvenanceDigest: inferenceDigest("quota:probe"), ObservedAt: surfaceDecisionTime.Add(-time.Minute), ValidUntil: surfaceDecisionTime.Add(time.Minute)}}
	evidence, _ = FreezeEligibility(evidence)
	authority.evidence[surface.ID] = evidence
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authority), surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceTelemetryUnsatisfied || !contains(decision.Evaluations[0].ReasonCodes, ReasonTelemetryUnknown+":quota_state") {
		t.Fatalf("explicit unknown telemetry was not preserved: %#v %v", decision, err)
	}
	evidence = authority.evidence[surface.ID]
	evidence.Telemetry["quota_state"] = SurfaceTelemetryEvidence{Known: true, Value: "available", ProvenanceRef: "quota:probe", ProvenanceDigest: inferenceDigest("quota:probe"), ObservedAt: surfaceDecisionTime.Add(-time.Hour), ValidUntil: surfaceDecisionTime.Add(-time.Minute)}
	evidence, _ = FreezeEligibility(evidence)
	authority.evidence[surface.ID] = evidence
	if _, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authority), surfaceDecisionTime); err == nil {
		t.Fatal("stale telemetry evidence was accepted")
	}
}

func TestSurfaceDecisionV2PersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(func(target *contracts.ExecutionTarget) { target.PreferredProfiles = []string{"interactive"} }))
	surface := executorSurface("surface-a", "Codex subscription CLI", contracts.TransportSubscriptionCLI, "interactive")
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authorizeSurfaces(t, request, []ExecutorSurface{surface})), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	record, err := FreezeUnifiedRouteRecord(UnifiedRouteRecord{Decision: decision})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewRouteLedger(state.NewSQLiteEventStore(db))
	if err := ledger.RecordUnifiedRoute(ctx, record); err != nil {
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
	replayedRecord, err := restarted.UnifiedRoute(ctx, "agent:durable", request.ID)
	var replayed *SurfaceRoutingDecision
	if replayedRecord != nil {
		replayed = &replayedRecord.Decision
	}
	if err != nil || replayedRecord == nil || replayedRecord.ID != record.ID || replayed.ID != decision.ID || replayed.Evaluator != surfaceEvaluator || replayed.EvaluatorGenerationDigest != decision.EvaluatorGenerationDigest || replayed.EvaluatorScope != request.ID || replayed.Evaluations[0].Evidence.RouteEligibility.ID == "" || !reflect.DeepEqual(replayed.ReasonCodes, decision.ReasonCodes) {
		t.Fatalf("v2 authoritative decision did not survive restart: %#v %v", replayed, err)
	}
}

func TestConcurrentSurfaceDecisionWritesConvergeOrConflictExplicitly(t *testing.T) {
	ctx := context.Background()
	request := surfaceRequest(t, surfaceTarget(nil))
	surface := executorSurface("surface-a", "subscription", contracts.TransportSubscriptionCLI, "interactive")
	decision, err := SelectExecutorSurface(ctx, request, []ExecutorSurface{surface}, surfaceComposer(t, authorizeSurfaces(t, request, []ExecutorSurface{surface})), surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	record, err := FreezeUnifiedRouteRecord(UnifiedRouteRecord{Decision: decision})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewRouteLedger(eventstore.NewMemoryStore())
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() { ready.Done(); <-start; errs <- ledger.RecordUnifiedRoute(ctx, record) }()
	}
	ready.Wait()
	close(start)
	first, second := <-errs, <-errs
	for _, err := range []error{first, second} {
		if err != nil && !errors.Is(err, eventstore.ErrVersionConflict) {
			t.Fatalf("concurrent write returned ambiguous error: %v", err)
		}
	}
	replayed, err := ledger.UnifiedRoutes(ctx, request.RouteRequest.SubjectAgentID)
	if err != nil || len(replayed) != 1 || replayed[0].ID != record.ID {
		t.Fatalf("concurrent writes did not converge on one decision: %#v %v", replayed, err)
	}
	if err := ledger.RecordUnifiedRoute(ctx, record); err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
}
