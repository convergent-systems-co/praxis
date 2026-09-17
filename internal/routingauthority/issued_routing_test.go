package routingauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fakeIssuanceStore struct {
	values  map[string]contracts.RoutingIssuance
	revoked map[string]bool
}

func (s *fakeIssuanceStore) LoadRoutingIssuance(_ context.Context, ref contracts.RoutingIssuanceRef, now time.Time) (contracts.RoutingIssuance, error) {
	v, ok := s.values[ref.ID]
	if !ok || v.Version != ref.Version {
		return v, errors.New("not issued")
	}
	if s.revoked[ref.ID] || v.ExpiresAt == nil || !now.Before(*v.ExpiresAt) {
		return v, errors.New("inactive issuance")
	}
	return v, nil
}

func testDigest(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func freezeIssued(t *testing.T, kind contracts.RoutingIssuanceKind, authority, scope string, payload any, at time.Time) contracts.RoutingIssuance {
	t.Helper()
	raw, _ := json.Marshal(payload)
	expiry := at.Add(time.Hour)
	principal := contracts.PrincipalRef{ID: "controller:routing", Kind: "controller"}
	value, err := contracts.FreezeRoutingIssuance(contracts.RoutingIssuance{Version: "1", Kind: kind, Authority: authority, Scope: scope, RequestID: "authority-request", RequestVersion: "1", DecisionRef: "authority-decision", DecisionVersion: "1", GenerationRef: "generation:routing", GenerationVersion: "1", GenerationDigest: testDigest("generation"), IssuedBy: principal, Payload: raw, PayloadDigest: testDigest(json.RawMessage(raw)), EffectiveAt: at, ExpiresAt: &expiry})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestIssuedRoutingRejectsSwapsCrossScopeExpiryRevocationAndReplays(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	target := contracts.ExecutionTarget{Version: "1", RequiredCapabilities: []string{"reasoning"}, ReasoningTier: "D1", TransportPolicy: []contracts.TransportClass{contracts.TransportLocal}, APIPolicy: contracts.APIPolicyAllow, SourceAuthority: "node", Scope: "node:deliver"}
	targetScope, _ := contracts.RoutingTargetContributionScope(target.Scope, target.Version, testDigest(target))
	targetIssuance := freezeIssued(t, contracts.RoutingTargetContribution, contracts.AuthorityRoutingTargetContributionIssue, targetScope, IssuedTargetContribution{Authority: contracts.AuthorityNode, Target: target}, now)
	store := &fakeIssuanceStore{values: map[string]contracts.RoutingIssuance{targetIssuance.ID: targetIssuance}, revoked: map[string]bool{}}
	authority := &IssuedRoutingAuthority{repository: store}
	targetRef := contracts.RoutingIssuanceRef{ID: targetIssuance.ID, Version: "1"}
	effective, err := authority.MergeExecutionTargets(ctx, []contracts.RoutingIssuanceRef{targetRef})
	if err != nil {
		t.Fatal(err)
	}
	route, _ := inference.FreezeRouteRequest(inference.RouteRequest{SubjectAgentID: "agent", RunID: "run", GoalClass: "delivery", Domain: "software", BehaviorKey: "implement", Context: "node", Tier: inference.D1})
	request, _ := inference.FreezeSurfaceRouteRequest(inference.SurfaceRouteRequest{RouteRequest: route, AgentGeneration: "7", Target: effective})
	surface := inference.ExecutorSurface{ID: "surface", ExecutorID: "executor", ProviderID: "provider", Capabilities: []string{"reasoning"}, Profiles: []string{"local"}, Transport: contracts.TransportLocal}
	surfaceDigest := testDigest(surface)
	evidence, err := inference.FreezeEligibility(inference.EligibilityEvidence{RequestID: route.ID, ExecutorID: surface.ExecutorID, ProviderID: surface.ProviderID, Tier: route.Tier, AuthorityID: "controller:routing", CapabilityEvidenceRef: "cap", CapabilityEvidenceDigest: testDigest("cap"), PolicyEvidenceRef: "policy", PolicyEvidenceDigest: testDigest("policy"), Authorized: true, CapabilityGranted: true, PolicyAllowed: true, Available: true, EvaluatedAt: now, SurfaceRequestID: request.ID, SurfaceID: surface.ID, SurfaceDigest: surfaceDigest, AuthorityGenerationRef: "generation:routing", AuthorityGenerationVersion: "1", AuthorityGenerationDigest: testDigest("generation"), AuthorityScope: request.ID, SecurityAllowed: true, SecurityEvidenceRef: "security", SecurityEvidenceDigest: testDigest("security"), AvailabilityEvidenceRef: "availability", AvailabilityEvidenceDigest: testDigest("availability"), BudgetAllowed: true, BudgetEvidenceRef: "budget", BudgetEvidenceDigest: testDigest("budget"), QuotaAllowed: true, QuotaEvidenceRef: "quota", QuotaEvidenceDigest: testDigest("quota"), ValidUntil: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	eligibilityScope, _ := contracts.RoutingSurfaceEligibilityScope(request.ID, "1", request.ID, surfaceDigest)
	eligibilityIssuance := freezeIssued(t, contracts.RoutingSurfaceEligibility, contracts.AuthorityRoutingSurfaceEligibilityIssue, eligibilityScope, IssuedSurfaceEligibility{RequestID: request.ID, SurfaceID: surface.ID, SurfaceDigest: surfaceDigest, Evidence: evidence}, now)
	store.values[eligibilityIssuance.ID] = eligibilityIssuance
	eligibilityRef := contracts.RoutingIssuanceRef{ID: eligibilityIssuance.ID, Version: "1"}
	decision, err := SelectExecutorSurface(ctx, request, []inference.ExecutorSurface{surface}, authority, []contracts.RoutingIssuanceRef{targetRef}, []contracts.RoutingIssuanceRef{eligibilityRef})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectExecutorSurface(ctx, request, []inference.ExecutorSurface{surface}, authority, []contracts.RoutingIssuanceRef{eligibilityRef}, []contracts.RoutingIssuanceRef{targetRef}); err == nil {
		t.Fatal("swapped issuance refs accepted")
	}
	cross := eligibilityIssuance
	var crossPayload IssuedSurfaceEligibility
	_ = json.Unmarshal(cross.Payload, &crossPayload)
	crossPayload.RequestID = "other"
	cross = freezeIssued(t, contracts.RoutingSurfaceEligibility, contracts.AuthorityRoutingSurfaceEligibilityIssue, eligibilityScope, crossPayload, now)
	store.values[cross.ID] = cross
	if _, err := SelectExecutorSurface(ctx, request, []inference.ExecutorSurface{surface}, authority, []contracts.RoutingIssuanceRef{targetRef}, []contracts.RoutingIssuanceRef{{ID: cross.ID, Version: "1"}}); err == nil {
		t.Fatal("cross-request eligibility accepted")
	}
	record, _ := inference.FreezeUnifiedRouteRecord(inference.UnifiedRouteRecord{Decision: decision, TargetIssuances: []contracts.RoutingIssuanceRef{targetRef}, EligibilityIssuances: []contracts.RoutingIssuanceRef{eligibilityRef}})
	events := eventstore.NewMemoryStore()
	ledger, _ := NewIssuedRouteLedger(events, authority)
	if err := ledger.RecordUnifiedRoute(ctx, record); err != nil {
		t.Fatal(err)
	}
	restarted, _ := NewIssuedRouteLedger(events, &IssuedRoutingAuthority{repository: store})
	if routes, err := restarted.UnifiedRoutes(ctx, "agent"); err != nil || len(routes) != 1 {
		t.Fatalf("restart replay: %v %#v", err, routes)
	}
	store.revoked[eligibilityIssuance.ID] = true
	if _, err := restarted.UnifiedRoutes(ctx, "agent"); err == nil {
		t.Fatal("revoked issuance replayed")
	}
	store.revoked[eligibilityIssuance.ID] = false
	expired := eligibilityIssuance
	past := time.Now().Add(-time.Second)
	expired.ExpiresAt = &past
	store.values[eligibilityIssuance.ID] = expired
	if _, err := restarted.UnifiedRoutes(ctx, "agent"); err == nil {
		t.Fatal("expired issuance replayed")
	}
}
