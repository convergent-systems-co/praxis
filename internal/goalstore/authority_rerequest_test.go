package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSaveAuthorityReRequestUsesImmutableHistoricalEvidenceAndConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	root := authorityGenerationFixture(now.Add(-4 * time.Hour))
	if err := repo.SaveAuthorityGeneration(ctx, root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	intent := contracts.ActionIntent{Version: "1", ID: "intent:rerequest", Actor: contracts.PrincipalRef{ID: "controller:rerequest", Kind: "controller"}, Operation: "test.publish", Target: "target:rerequest", Scope: "scope:rerequest"}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	oldExpiry := now.Add(-time.Hour)
	delegation := &contracts.DelegationRequest{Profile: "test", ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: intent.Actor, TargetKind: "action-intent", TargetIdentity: intent.ID, TargetVersion: intent.Version, TargetDigest: intentDigest, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: intent.Operation, RequestedScope: intent.Scope, ProposalVersion: "1", ProposalDigest: intentDigest, ReviewVersion: "1", ReviewDigest: intentDigest, ExpiresAt: oldExpiry, Reason: "test", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelVersion, PolicyDigest: contracts.AuthorityModelDigest()}
	prior := contracts.AuthorityRequest{ID: "authority-request:rerequest-1", Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: intent.Scope, Reason: "test", Status: contracts.AuthorityRequestPending, IntentDigest: intentDigest, Intent: &intent, Delegation: delegation}
	priorDigest, err := prior.DigestAt(now.Add(-2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: prior.ID, RequestVersion: prior.Version, RequestDigest: priorDigest, DecisionRef: "authority-decision:rerequest-1", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: intent.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "sha256:" + strings.Repeat("d", 64), IssuedAt: now.Add(-2 * time.Hour), Delegation: delegation}
	child := contracts.AuthorityGeneration{Ref: "authority-delegation:" + prior.ID, Version: "1", Principal: intent.Actor, Scope: intent.Scope, ProvenanceRef: "authority-decision:" + decision.DecisionRef + ":1", ProvenanceDigest: decision.AuthorityDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-2 * time.Hour), ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedBy: root.Principal, DelegationRef: prior.ID + "/1", DelegationDigest: priorDigest, PolicyRef: delegation.PolicyRef, PolicyVersion: delegation.PolicyVersion, PolicyDigest: delegation.PolicyDigest, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: delegation.PolicyVersion, AuthorityModelDigest: delegation.PolicyDigest, Authorities: []string{contracts.GovernedPackagePublish}, DelegationProfile: delegation.Profile, SubjectKind: "action-intent", SubjectID: intent.ID, SubjectVersion: intent.Version, SubjectDigest: intentDigest, ExpiresAt: &oldExpiry}
	child.Digest, err = child.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	priorCreated := now.Add(-2 * time.Hour)
	for _, item := range []struct {
		namespace, id, version string
		value                  any
		created                time.Time
		expires                *time.Time
	}{
		{authorityRequestNamespace, prior.ID, prior.Version, prior, priorCreated, &oldExpiry},
		{authorityDecisionNamespace, prior.ID, prior.Version, authorityDecisionRecord{Request: prior, Decision: decision}, decision.IssuedAt, &oldExpiry},
		{authorityGenerationNamespace, child.Ref, child.Version, child, child.EffectiveAt, &oldExpiry},
	} {
		payload, marshalErr := json.Marshal(item.value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if item.namespace == authorityGenerationNamespace {
			generation, ok := item.value.(contracts.AuthorityGeneration)
			if !ok {
				t.Fatalf("authority generation fixture has type %T", item.value)
			}
			if err := repo.Store.PutAuthorityGeneration(ctx, state.AuthorityGenerationWrite{Generation: generation, Crypto: repo.Crypto, KeyRef: repo.KeyRef, Profile: repo.Profile, Sensitivity: repo.Sensitivity, CreatedAt: item.created, ExpiresAt: item.expires}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := repo.putWorkPlanBlob(ctx, item.namespace, item.id, item.version, payload, item.created, item.expires); err != nil {
			t.Fatal(err)
		}
	}
	eligibility := contracts.AuthorityReRequestEligibility{Now: now, FreshExpiresAt: now.Add(time.Hour), EffectState: contracts.AuthorityReRequestNoEffect, IntentCurrent: true}
	fresh, digest, err := repo.SaveAuthorityReRequest(ctx, prior, decision, child, eligibility)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	restarted, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	if _, err := restarted.LoadAuthorityGeneration(ctx, child.Ref, child.Version, now); !errors.Is(err, state.ErrSecureBlobExpired) {
		t.Fatalf("expired predecessor generation remained executable: %v", err)
	}
	repeated, repeatedDigest, err := restarted.SaveAuthorityReRequest(ctx, prior, decision, child, eligibility)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID != repeated.ID || digest != repeatedDigest {
		t.Fatalf("duplicate re-request diverged: %s/%s vs %s/%s", fresh.ID, digest, repeated.ID, repeatedDigest)
	}
	forged := prior
	forged.Intent = &contracts.ActionIntent{Version: intent.Version, ID: intent.ID, Actor: intent.Actor, Operation: intent.Operation, Target: "target:substituted", Scope: intent.Scope}
	if _, _, err := restarted.SaveAuthorityReRequest(ctx, forged, decision, child, eligibility); err == nil {
		t.Fatal("caller-substituted predecessor was accepted")
	}
}
