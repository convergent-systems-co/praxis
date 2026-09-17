package contracts

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func reRequestFixture(t *testing.T) (AuthorityRequest, AuthorityDecision, AuthorityGeneration, AuthorityReRequestEligibility) {
	t.Helper()
	now := time.Unix(1_800_000_000, 0).UTC()
	rootDigest := "sha256:" + strings.Repeat("1", 64)
	root := AuthorityGeneration{Ref: "root", Version: "1", Principal: PrincipalRef{ID: "owner", Kind: "human"}, Scope: "installation:" + rootDigest, Capabilities: []string{AuthorityDelegateCapability}, ProvenanceRef: "bootstrap", ProvenanceDigest: rootDigest, State: AuthorityGenerationActive, EffectiveAt: now.Add(-4 * time.Hour)}
	var err error
	root.Digest, err = root.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	intent := ActionIntent{Version: "1", ID: "intent:stable", Actor: PrincipalRef{ID: "controller:test", Kind: "controller"}, Operation: "test.publish", Target: "target:test", Scope: "scope:test", Parameters: map[string]string{"stable": "true"}}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	delegated := PrincipalRef{ID: "controller:test", Kind: "controller"}
	oldExpiry := now.Add(-time.Hour)
	delegation := &DelegationRequest{Profile: "test", ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: delegated, TargetKind: "action-intent", TargetIdentity: intent.ID, TargetVersion: intent.Version, TargetDigest: intentDigest, RequestedAuthority: GovernedPackagePublish, RequestedOperation: intent.Operation, RequestedScope: intent.Scope, ProposalVersion: "1", ProposalDigest: intentDigest, ReviewVersion: "1", ReviewDigest: intentDigest, ExpiresAt: oldExpiry, Reason: "test", PolicyRef: AuthorityModelID, PolicyVersion: AuthorityModelVersion, PolicyDigest: AuthorityModelDigest()}
	prior := AuthorityRequest{ID: "request:r1", Version: "1", RequestedAuthority: GovernedPackagePublish, RequestedScope: intent.Scope, Reason: "test", Status: AuthorityRequestPending, IntentDigest: intentDigest, Intent: &intent, Delegation: delegation}
	priorDigest, err := prior.DigestAt(now.Add(-2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	decision := AuthorityDecision{RequestID: prior.ID, RequestVersion: prior.Version, RequestDigest: priorDigest, DecisionRef: "decision:r1", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: intent.Scope, Outcome: AuthorityApprove, AuthorityDigest: "sha256:" + strings.Repeat("2", 64), IssuedAt: now.Add(-2 * time.Hour), Delegation: delegation}
	child := AuthorityGeneration{Ref: "authority-delegation:" + prior.ID, Version: "1", Principal: delegated, Scope: intent.Scope, ProvenanceRef: "authority-decision:" + decision.DecisionRef + ":" + decision.DecisionVersion, ProvenanceDigest: decision.AuthorityDigest, State: AuthorityGenerationActive, EffectiveAt: now.Add(-2 * time.Hour), ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedBy: root.Principal, DelegationRef: prior.ID + "/1", DelegationDigest: priorDigest, PolicyRef: delegation.PolicyRef, PolicyVersion: delegation.PolicyVersion, PolicyDigest: delegation.PolicyDigest, AuthorityModel: AuthorityModelID, AuthorityModelVersion: delegation.PolicyVersion, AuthorityModelDigest: delegation.PolicyDigest, Authorities: []string{GovernedPackagePublish}, DelegationProfile: delegation.Profile, SubjectKind: "action-intent", SubjectID: intent.ID, SubjectVersion: intent.Version, SubjectDigest: intentDigest, ExpiresAt: &oldExpiry}
	child.Digest, err = child.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	return prior, decision, child, AuthorityReRequestEligibility{Now: now, FreshExpiresAt: now.Add(time.Hour), EffectState: AuthorityReRequestNoEffect, IntentCurrent: true}
}

func TestBuildAuthorityReRequestPreservesIntentAndHistoricalLineage(t *testing.T) {
	prior, decision, generation, eligibility := reRequestFixture(t)
	fresh, err := BuildAuthorityReRequest(prior, decision, generation, eligibility)
	if err != nil {
		t.Fatal(err)
	}
	oldIntentDigest, _ := prior.Intent.Digest()
	newIntentDigest, _ := fresh.Intent.Digest()
	if oldIntentDigest != newIntentDigest || fresh.ReRequestOf == nil || fresh.ReRequestOf.RequestID != prior.ID || fresh.ID == prior.ID {
		t.Fatalf("fresh request changed intent or lineage: old=%s new=%s fresh=%+v", oldIntentDigest, newIntentDigest, fresh)
	}
	oldDigest, _ := prior.DigestAt(generation.EffectiveAt)
	if fresh.ReRequestOf.RequestDigest != oldDigest || fresh.ReRequestOf.DecisionDigest == "" || fresh.ReRequestOf.GenerationDigest != generation.Digest {
		t.Fatalf("incomplete predecessor lineage: %+v", fresh.ReRequestOf)
	}
	if after, _ := prior.DigestAt(generation.EffectiveAt); after != oldDigest {
		t.Fatal("historical request digest changed during re-request preparation")
	}
	if err := decision.Validate(fresh, eligibility.Now); err == nil {
		t.Fatal("historical decision authorized the fresh request")
	}
	if err := fresh.ValidateAt(eligibility.Now); err != nil {
		t.Fatal(err)
	}
	freshDecision := decision
	freshRequestDigest, _ := fresh.DigestAt(eligibility.Now)
	freshDecision.RequestID = fresh.ID
	freshDecision.RequestVersion = fresh.Version
	freshDecision.RequestDigest = freshRequestDigest
	freshDecision.DecisionRef = "decision:r2"
	freshDecision.IssuedAt = eligibility.Now
	freshDecision.Delegation = fresh.Delegation
	if err := freshDecision.Validate(fresh, eligibility.Now); err != nil {
		t.Fatalf("fresh decision did not bind exact fresh request: %v", err)
	}
	freshGeneration := generation
	freshGeneration.Ref = "authority-delegation:" + fresh.ID
	freshGeneration.EffectiveAt = eligibility.Now
	freshGeneration.ExpiresAt = &eligibility.FreshExpiresAt
	freshGeneration.DelegationRef = fresh.ID + "/" + fresh.Version
	freshGeneration.DelegationDigest = freshRequestDigest
	freshGeneration.ProvenanceRef = "authority-decision:" + freshDecision.DecisionRef + ":" + freshDecision.DecisionVersion
	freshGeneration.ProvenanceDigest = freshDecision.AuthorityDigest
	freshGeneration.Digest, _ = freshGeneration.ComputeDigest()
	if err := freshGeneration.VerifyDigest(); err != nil || freshGeneration.DelegationRef != fresh.ID+"/"+fresh.Version || freshGeneration.DelegationDigest != freshRequestDigest {
		t.Fatalf("fresh generation did not bind exact fresh request/decision: %v", err)
	}
}

func TestBuildAuthorityReRequestRejectsUnsafeLifecycleStates(t *testing.T) {
	states := []struct {
		name string
		edit func(*AuthorityReRequestEligibility)
	}{
		{"unknown", func(e *AuthorityReRequestEligibility) { e.EffectState = AuthorityReRequestUnknown }},
		{"partial", func(e *AuthorityReRequestEligibility) { e.EffectState = AuthorityReRequestPartial }},
		{"post-dispatch-failed", func(e *AuthorityReRequestEligibility) { e.EffectState = AuthorityReRequestFailed; e.EffectAttempts = 1 }},
		{"completed", func(e *AuthorityReRequestEligibility) { e.Completed = true }},
		{"abandoned", func(e *AuthorityReRequestEligibility) { e.Abandoned = true }},
		{"superseded", func(e *AuthorityReRequestEligibility) { e.Superseded = true }},
		{"revoked", func(e *AuthorityReRequestEligibility) { e.AuthorityRevoked = true }},
		{"conflict", func(e *AuthorityReRequestEligibility) { e.ConflictingChain = true }},
		{"attempted", func(e *AuthorityReRequestEligibility) { e.EffectAttempts = 1 }},
	}
	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			prior, decision, generation, eligibility := reRequestFixture(t)
			tc.edit(&eligibility)
			if _, err := BuildAuthorityReRequest(prior, decision, generation, eligibility); err == nil {
				t.Fatal("unsafe lifecycle state was accepted")
			}
		})
	}
}

func TestBuildAuthorityReRequestRejectsSubstitutionAndExpiryReuse(t *testing.T) {
	prior, decision, generation, eligibility := reRequestFixture(t)
	prior.Intent.Parameters["stable"] = "substituted"
	if _, err := BuildAuthorityReRequest(prior, decision, generation, eligibility); err == nil {
		t.Fatal("substituted ActionIntent was accepted")
	}
	prior, decision, generation, eligibility = reRequestFixture(t)
	eligibility.Now = generation.ExpiresAt.Add(-time.Second)
	eligibility.FreshExpiresAt = eligibility.Now.Add(time.Hour)
	if _, err := BuildAuthorityReRequest(prior, decision, generation, eligibility); err == nil {
		t.Fatal("unexpired authority was treated as eligible")
	}
	prior, decision, generation, eligibility = reRequestFixture(t)
	eligibility.FreshExpiresAt = eligibility.Now.Add(-time.Second)
	if _, err := BuildAuthorityReRequest(prior, decision, generation, eligibility); err == nil {
		t.Fatal("invalid fresh expiry was accepted")
	}
}

func TestBuildAuthorityReRequestRejectsPrincipalCapabilityAndScopeChanges(t *testing.T) {
	mutations := []struct {
		name string
		edit func(*AuthorityRequest)
	}{
		{"principal", func(r *AuthorityRequest) {
			r.Delegation.DelegatedPrincipal = PrincipalRef{ID: "controller:other", Kind: "controller"}
		}},
		{"capability", func(r *AuthorityRequest) { r.RequestedAuthority = "different.authority" }},
		{"scope", func(r *AuthorityRequest) {
			r.RequestedScope = "scope:broader"
			r.Delegation.RequestedScope = "scope:broader"
		}},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			prior, decision, generation, eligibility := reRequestFixture(t)
			tc.edit(&prior)
			if _, err := BuildAuthorityReRequest(prior, decision, generation, eligibility); err == nil {
				t.Fatal("changed authority boundary was accepted")
			}
		})
	}
}

func TestBuildAuthorityReRequestIsDeterministicUnderConcurrency(t *testing.T) {
	prior, decision, generation, eligibility := reRequestFixture(t)
	const workers = 32
	results := make([][]byte, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fresh, err := BuildAuthorityReRequest(prior, decision, generation, eligibility)
			errs[i] = err
			if err == nil {
				results[i], _ = json.Marshal(fresh)
			}
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if string(results[i]) != string(results[0]) {
			t.Fatal("concurrent re-request invocations diverged")
		}
	}
}
