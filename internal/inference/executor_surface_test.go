package inference

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var surfaceDecisionTime = time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC)

func surfaceTarget(mut func(*contracts.ExecutionTarget)) contracts.EffectiveExecutionTarget {
	target := contracts.ExecutionTarget{
		Version: "1", RequiredCapabilities: []string{"reasoning"}, APIPolicy: contracts.APIPolicyAllow,
		SourceAuthority: "merged", Scope: "node:deliver", TransportPolicy: []contracts.TransportClass{contracts.TransportSubscriptionCLI, contracts.TransportMeteredAPI},
	}
	if mut != nil {
		mut(&target)
	}
	return contracts.EffectiveExecutionTarget{Target: target, Authorities: []contracts.TargetAuthority{contracts.AuthorityNode, contracts.AuthorityPlatformSecurity}}
}

func surfaceRequest(t *testing.T, target contracts.EffectiveExecutionTarget) SurfaceRouteRequest {
	t.Helper()
	request, err := FreezeSurfaceRouteRequest(SurfaceRouteRequest{SubjectAgentID: "agent:durable", AgentGeneration: "generation:7", RunID: "run:surface-routing", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func executorSurface(id, provider string, transport contracts.TransportClass, profiles ...string) ExecutorSurface {
	return ExecutorSurface{
		ID: id, ProviderMetadata: provider, ModelMetadata: provider + " model", Capabilities: []string{"reasoning"}, Profiles: profiles, Transport: transport,
		Available: true, SecurityEligible: true, AuthorityEligible: true, PolicyEligible: true,
		CapabilityEvidenceRef: "capability:" + id, SecurityEvidenceRef: "security:" + id,
		AuthorityEvidenceRef: "authority:" + id, PolicyEvidenceRef: "policy:" + id,
	}
}

func TestExecutorSurfaceFixturesRemainMetadataAndSelectionIsDeterministic(t *testing.T) {
	target := surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles = []string{"interactive"}
		target.AllowedFallbackProfiles = []string{"batch"}
	})
	request := surfaceRequest(t, target)
	fixtures := []ExecutorSurface{
		executorSurface("surface-d", "Anthropic metered API", contracts.TransportMeteredAPI, "batch"),
		executorSurface("surface-c", "Claude subscription CLI", contracts.TransportSubscriptionCLI, "interactive"),
		executorSurface("surface-b", "OpenAI metered API", contracts.TransportMeteredAPI, "batch"),
		executorSurface("surface-a", "Codex subscription CLI", contracts.TransportSubscriptionCLI, "interactive"),
	}
	decision, err := SelectExecutorSurface(request, fixtures, surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	if decision.SelectedSurfaceID != "surface-a" || decision.SelectedProfile != "interactive" || decision.Fallback || decision.ReasonCodes[0] != ReasonPreferredMatch {
		t.Fatalf("provider metadata affected neutral deterministic affinity: %#v", decision)
	}
	if decision.Request.SubjectAgentID == decision.SelectedSurfaceID || decision.Request.SubjectAgentID != "agent:durable" {
		t.Fatalf("surface identity replaced persistent agent identity: %#v", decision.Request)
	}
	reversed := append([]ExecutorSurface(nil), fixtures...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	again, err := SelectExecutorSurface(request, reversed, surfaceDecisionTime)
	if err != nil || again.ID != decision.ID {
		t.Fatalf("input order changed content-addressed decision: %s %s %v", decision.ID, again.ID, err)
	}
}

func TestSurfaceEligibilityPrecedesAffinityAndFallbackIsExplicit(t *testing.T) {
	target := surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles = []string{"high-depth"}
		target.AllowedFallbackProfiles = []string{"balanced"}
	})
	preferred := executorSurface("surface-preferred", "preferred metadata", contracts.TransportSubscriptionCLI, "high-depth")
	preferred.SecurityEligible = false
	fallback := executorSurface("surface-fallback", "fallback metadata", contracts.TransportSubscriptionCLI, "balanced")
	decision, err := SelectExecutorSurface(surfaceRequest(t, target), []ExecutorSurface{preferred, fallback}, surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	if decision.SelectedSurfaceID != fallback.ID || !decision.Fallback || decision.ReasonCodes[0] != ReasonAllowedFallback {
		t.Fatalf("denied preferred surface outranked explicit fallback: %#v", decision)
	}
	if decision.Evaluations[1].SurfaceID != preferred.ID || decision.Evaluations[1].Eligible || !contains(decision.Evaluations[1].ReasonCodes, ReasonSecurityDenied) {
		t.Fatalf("security eligibility evidence was not recorded before affinity: %#v", decision.Evaluations)
	}

	unlisted := executorSurface("surface-unlisted", "unlisted metadata", contracts.TransportSubscriptionCLI, "economy")
	decision, err = SelectExecutorSurface(surfaceRequest(t, target), []ExecutorSurface{preferred, unlisted}, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceNoEligible || decision.SelectedSurfaceID != "" {
		t.Fatalf("routing used an unlisted fallback: %#v %v", decision, err)
	}
	prohibitedTarget := target
	prohibitedTarget.Target.AllowedFallbackProfiles = nil
	decision, err = SelectExecutorSurface(surfaceRequest(t, prohibitedTarget), []ExecutorSurface{preferred, unlisted}, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceFallbackProhibited {
		t.Fatalf("missing fallback permission did not produce stable failure: %#v %v", decision, err)
	}
}

func TestEveryHardEligibilityDimensionOutranksAffinity(t *testing.T) {
	target := surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles = []string{"high-depth"}
		target.AllowedFallbackProfiles = []string{"balanced"}
	})
	cases := []struct {
		name   string
		deny   func(*ExecutorSurface)
		reason string
	}{
		{name: "capability", deny: func(surface *ExecutorSurface) { surface.Capabilities = nil }, reason: ReasonCapabilityMissing},
		{name: "security", deny: func(surface *ExecutorSurface) { surface.SecurityEligible = false }, reason: ReasonSecurityDenied},
		{name: "authority", deny: func(surface *ExecutorSurface) { surface.AuthorityEligible = false }, reason: ReasonAuthorityDenied},
		{name: "policy", deny: func(surface *ExecutorSurface) { surface.PolicyEligible = false }, reason: ReasonPolicyDenied},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			preferred := executorSurface("surface-a", "preferred metadata", contracts.TransportSubscriptionCLI, "high-depth")
			test.deny(&preferred)
			fallback := executorSurface("surface-b", "fallback metadata", contracts.TransportSubscriptionCLI, "balanced")
			decision, err := SelectExecutorSurface(surfaceRequest(t, target), []ExecutorSurface{preferred, fallback}, surfaceDecisionTime)
			if err != nil || decision.SelectedSurfaceID != fallback.ID || !decision.Fallback || decision.Evaluations[0].Eligible || !contains(decision.Evaluations[0].ReasonCodes, test.reason) {
				t.Fatalf("%s eligibility did not precede affinity: %#v %v", test.name, decision, err)
			}
		})
	}
}

func TestRequiredProfileFailsClosedAndAPIPolicyForbidsMeteredFallback(t *testing.T) {
	requiredTarget := surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.RequiredProfiles = []string{"regulated"}
		target.AllowedFallbackProfiles = []string{"balanced"}
	})
	required := executorSurface("surface-required", "required metadata", contracts.TransportSubscriptionCLI, "regulated")
	required.AuthorityEligible = false
	fallback := executorSurface("surface-fallback", "fallback metadata", contracts.TransportSubscriptionCLI, "balanced")
	decision, err := SelectExecutorSurface(surfaceRequest(t, requiredTarget), []ExecutorSurface{required, fallback}, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceRequiredUnavailable || decision.SelectedSurfaceID != "" {
		t.Fatalf("required-unavailable route did not fail closed: %#v %v", decision, err)
	}

	apiTarget := surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.APIPolicy = contracts.APIPolicyForbid
		target.TransportPolicy = []contracts.TransportClass{contracts.TransportSubscriptionCLI}
		target.PreferredProfiles = []string{"high-depth"}
		target.AllowedFallbackProfiles = []string{"balanced"}
	})
	apiFallback := executorSurface("surface-api", "metered metadata", contracts.TransportMeteredAPI, "balanced")
	cliOther := executorSurface("surface-cli", "subscription metadata", contracts.TransportSubscriptionCLI, "economy")
	decision, err = SelectExecutorSurface(surfaceRequest(t, apiTarget), []ExecutorSurface{apiFallback, cliOther}, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceAPIUseProhibited || decision.SelectedSurfaceID != "" {
		t.Fatalf("API forbid allowed metered fallback: %#v %v", decision, err)
	}
	if !contains(decision.Evaluations[0].ReasonCodes, ReasonAPIForbidden) {
		t.Fatalf("metered exclusion reason missing: %#v", decision.Evaluations)
	}
}

func TestUnknownRequiredTelemetryIsIneligibleWithoutFabricatedValue(t *testing.T) {
	target := surfaceTarget(func(target *contracts.ExecutionTarget) {
		target.PreferredProfiles = []string{"high-depth"}
		target.AllowedFallbackProfiles = []string{"balanced"}
		target.TelemetryRequirements = []string{"quota_state"}
	})
	unknown := executorSurface("surface-a", "unknown telemetry", contracts.TransportSubscriptionCLI, "high-depth")
	ready := executorSurface("surface-b", "known telemetry", contracts.TransportSubscriptionCLI, "balanced")
	ready.Telemetry = map[string]TelemetryValue{"quota_state": {Known: true, Value: "available"}}
	decision, err := SelectExecutorSurface(surfaceRequest(t, target), []ExecutorSurface{unknown, ready}, surfaceDecisionTime)
	if err != nil {
		t.Fatal(err)
	}
	value := decision.Evaluations[0].Telemetry["quota_state"]
	if decision.SelectedSurfaceID != ready.ID || !decision.Fallback || value.Known || value.Value != "" || !contains(decision.Evaluations[0].ReasonCodes, ReasonTelemetryUnknown+":quota_state") {
		t.Fatalf("unknown telemetry was fabricated or treated as eligible: %#v", decision)
	}
	decision, err = SelectExecutorSurface(surfaceRequest(t, target), []ExecutorSurface{unknown}, surfaceDecisionTime)
	if err != nil || decision.Outcome != SurfaceTelemetryUnsatisfied {
		t.Fatalf("unknown required telemetry did not produce stable failure: %#v %v", decision, err)
	}
}

func TestSurfaceDecisionPersistsAcrossRestartWithStableReasons(t *testing.T) {
	ctx := context.Background()
	target := surfaceTarget(func(target *contracts.ExecutionTarget) { target.PreferredProfiles = []string{"interactive"} })
	request := surfaceRequest(t, target)
	decision, err := SelectExecutorSurface(request, []ExecutorSurface{executorSurface("surface-a", "Codex subscription CLI", contracts.TransportSubscriptionCLI, "interactive")}, surfaceDecisionTime)
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
	if err != nil || replayed == nil || replayed.ID != decision.ID || replayed.SelectedSurfaceID != decision.SelectedSurfaceID || !reflect.DeepEqual(replayed.ReasonCodes, decision.ReasonCodes) || replayed.Request.AgentGeneration != "generation:7" {
		t.Fatalf("surface selection or reasons changed after restart: %#v %v", replayed, err)
	}
	tampered := *replayed
	tampered.ReasonCodes = []string{"tampered"}
	if err := VerifySurfaceRoutingDecision(tampered); err == nil {
		t.Fatal("content-addressed routing evidence accepted tampering")
	}
}
