package goalstore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type historicalAdversarialFixture struct {
	repo                      Repository
	store                     *state.Store
	root                      contracts.AuthorityGeneration
	child                     contracts.AuthorityGeneration
	request                   contracts.AuthorityRequest
	decision                  contracts.AuthorityDecision
	requestDigest             string
	requestObjectDigest       string
	effectIDs                 []string
	effectGenerationDigest    string
	effectAt                  time.Time
	created, now, childExpiry time.Time
}

func newHistoricalAdversarialFixture(t *testing.T, mutate func(*historicalAdversarialFixture)) historicalAdversarialFixture {
	t.Helper()
	ctx := context.Background()
	repo, store := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	created := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	now := created.Add(3 * time.Hour)
	bootstrap := contracts.GoalsPublicationBootstrap
	root := contracts.AuthorityGeneration{Ref: "installation-governance:" + bootstrap, Version: "1", Principal: contracts.PrincipalRef{ID: "installation-owner:" + bootstrap, Kind: "human"}, Scope: "installation-governance:" + bootstrap, Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:test", ProvenanceDigest: bootstrap, State: contracts.AuthorityGenerationActive, EffectiveAt: created}
	root.Digest, _ = root.ComputeDigest()
	childExpiry := created.Add(time.Hour)
	child := contracts.AuthorityGeneration{Ref: "authority-delegation:test-request", Version: "1", Principal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"}, Scope: "goals-established-state-publication:test", Authorities: []string{contracts.GovernedPackagePublish}, ProvenanceRef: "authority-decision:test", ProvenanceDigest: "sha256:" + strings.Repeat("d", 64), State: contracts.AuthorityGenerationActive, EffectiveAt: created.Add(10 * time.Minute), ExpiresAt: &childExpiry, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedBy: root.Principal, DelegationRef: "test-request/1", DelegationDigest: "sha256:" + strings.Repeat("e", 64), PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsRecoveryVersion, PolicyDigest: contracts.AuthorityModelGoalsRecoveryDigest(), AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelGoalsRecoveryVersion, AuthorityModelDigest: contracts.AuthorityModelGoalsRecoveryDigest()}
	child.Digest, _ = child.ComputeDigest()
	f := historicalAdversarialFixture{repo: repo, store: store, root: root, child: child, created: created, now: now, childExpiry: childExpiry, effectIDs: []string{"execution:test:manifest", "execution:test:archive", "execution:test:signature", "execution:test:verify-draft"}, effectAt: created.Add(20 * time.Minute)}
	intent := contracts.ActionIntent{Version: "1", ID: "intent:test", Actor: child.Principal, Operation: contracts.GoalsRecoveryOperation, Target: "github-release:389997269", Scope: child.Scope, Parameters: map[string]string{"created_at": created.Format(time.RFC3339Nano), "expires_at": childExpiry.Format(time.RFC3339Nano)}}
	intentDigest, _ := intent.Digest()
	delegation := contracts.DelegationRequest{Profile: contracts.GoalsPublicationRecoveryProfile, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: child.Principal, TargetKind: "action-intent", TargetIdentity: intent.ID, TargetVersion: intent.Version, TargetDigest: intentDigest, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: contracts.GoalsRecoveryOperation, RequestedScope: child.Scope, ProposalVersion: intent.Version, ProposalDigest: intentDigest, ReviewVersion: intent.Version, ReviewDigest: intentDigest, ExpiresAt: childExpiry, Reason: "test", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsRecoveryVersion, PolicyDigest: contracts.AuthorityModelGoalsRecoveryDigest()}
	request := contracts.AuthorityRequest{ID: "test-request", Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: child.Scope, Reason: "test", Status: contracts.AuthorityRequestPending, Delegation: &delegation, Intent: &intent, IntentDigest: intentDigest, InstallationDigest: contracts.GoalsPublicationRoot}
	requestDigest, _ := request.DigestAt(created)
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision:test", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: child.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelGoalsRecoveryDigest(), IssuedAt: created.Add(5 * time.Minute), ExpiresAt: &childExpiry, Delegation: &delegation}
	f.request, f.decision, f.requestDigest = request, decision, requestDigest
	if mutate != nil {
		mutate(&f)
	}
	// Recompute child after mutations that affect its signed semantic fields.
	f.child.Digest, _ = f.child.ComputeDigest()
	if f.effectGenerationDigest == "" {
		f.effectGenerationDigest = f.child.Digest
	}
	persist := func(namespace, id, version string, value any, expires *time.Time) string {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		digest := payloadDigest(payload)
		envelope, err := repo.Crypto.Seal(ctx, repo.KeyRef, repo.Profile, payload, state.SecureBlobAAD(namespace, id, version, digest))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutSecureBlob(ctx, state.SecureBlobRecord{Namespace: namespace, ObjectID: id, ObjectVersion: version, ObjectDigest: digest, Sensitivity: repo.Sensitivity, CryptoProfile: repo.Profile, Envelope: envelope, CreatedAt: created, ExpiresAt: expires}); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	persist(authorityGenerationNamespace, f.root.Ref, f.root.Version, f.root, nil)
	persist(authorityGenerationNamespace, f.child.Ref, f.child.Version, f.child, &f.childExpiry)
	f.requestDigest, _ = f.request.DigestAt(created)
	f.requestObjectDigest = persist(authorityRequestNamespace, f.request.ID, f.request.Version, f.request, &f.childExpiry)
	decisionPayload := authorityDecisionRecord{Request: f.request, Decision: f.decision}
	persist(authorityDecisionNamespace, f.request.ID, f.request.Version, decisionPayload, &f.childExpiry)
	for _, id := range f.effectIDs {
		auth := contracts.PackagePublishAuthorization{Generation: f.child, Request: f.request, Decision: f.decision}
		if f.effectGenerationDigest != f.child.Digest {
			auth.Generation.Digest = f.effectGenerationDigest
		}
		payload, _ := json.Marshal(struct {
			Version                string
			RequestID              string
			Step                   string
			Intent                 contracts.ActionIntent
			Authority              contracts.PackagePublishAuthorization
			PredecessorAbandonment string
		}{"1", f.request.ID, strings.TrimPrefix(id, "execution:test:"), *f.request.Intent, auth, ""})
		at := f.effectAt.Format(time.RFC3339Nano)
		if _, err := store.DB().ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, "test", "1", f.child.Principal.ID, f.child.Principal.Kind, f.child.Scope, "execution:test", payload, "completed", at); err != nil {
			t.Fatal(err)
		}
		if _, err := store.DB().ExecContext(ctx, `INSERT INTO effects(effect_id,command_id,action_intent_digest,target_adapter,state,attempts,request_payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, id, f.request.IntentDigest, "test", "succeeded", 1, payload, at, at); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestExpiredHistoricalAuthorityDelegatedChildAdversarialLineage(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*historicalAdversarialFixture)
		want   string
	}{
		{"parent supplied where child required", func(f *historicalAdversarialFixture) { f.effectGenerationDigest = f.root.Digest }, "historical generation digest mismatch"},
		{"substituted child object identity", func(f *historicalAdversarialFixture) { f.child.Ref = "authority-delegation:substituted" }, "secure blob not found"},
		{"substituted child semantic digest", func(f *historicalAdversarialFixture) { f.effectGenerationDigest = "sha256:" + strings.Repeat("b", 64) }, "historical generation digest mismatch"},
		{"child principal mismatch", func(f *historicalAdversarialFixture) {
			f.child.Principal = contracts.PrincipalRef{ID: "publisher:substituted", Kind: "publisher"}
		}, "historical delegated authority lineage mismatch"},
		{"child scope mismatch", func(f *historicalAdversarialFixture) { f.child.Scope = "goals-established-state-publication:other" }, "historical delegated authority lineage mismatch"},
		{"child expiration mismatch", func(f *historicalAdversarialFixture) { x := f.childExpiry.Add(time.Minute); f.child.ExpiresAt = &x }, "historical delegated authority lineage mismatch"},
		{"child delegated-by mismatch", func(f *historicalAdversarialFixture) {
			f.child.DelegatedBy = contracts.PrincipalRef{ID: "installation-owner:other", Kind: "human"}
		}, "historical delegated authority lineage mismatch"},
		{"child parent digest mismatch", func(f *historicalAdversarialFixture) { f.child.ParentDigest = "sha256:" + strings.Repeat("c", 64) }, "historical delegated authority lineage mismatch"},
		{"effect authority digest mismatch", func(f *historicalAdversarialFixture) { f.effectGenerationDigest = "sha256:" + strings.Repeat("b", 64) }, "historical generation digest mismatch"},
		{"effect before child effective", func(f *historicalAdversarialFixture) { f.effectAt = f.child.EffectiveAt.Add(-time.Minute) }, "historical authority was not valid when effect occurred"},
		{"effect after child expiry", func(f *historicalAdversarialFixture) { f.effectAt = f.childExpiry.Add(time.Minute) }, "historical authority was not valid when effect occurred"},
		{"decision not bound to parent", func(f *historicalAdversarialFixture) {
			f.decision.AuthorityGenerationDigest = "sha256:" + strings.Repeat("b", 64)
		}, "historical generation"},
		{"delegation not bound to request and decision", func(f *historicalAdversarialFixture) {
			f.request.Delegation.TargetDigest = "sha256:" + strings.Repeat("b", 64)
		}, "validate historical decision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHistoricalAdversarialFixture(t, tc.mutate)
			evidence, err := f.repo.LoadExpiredHistoricalAuthorityEvidence(context.Background(), f.request.ID, f.request.Version, f.requestDigest, contracts.GoalsPublicationRoot, "execution:test", f.effectIDs, f.now)
			if err == nil || evidence.GenerationDigest != "" || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected fail-closed %q, evidence=%+v err=%v", tc.want, evidence, err)
			}
		})
	}
}

func TestExpiredHistoricalAuthorityProjectionUsesDelegatedChild(t *testing.T) {
	f := newHistoricalAdversarialFixture(t, nil)
	evidence, err := f.repo.LoadExpiredHistoricalAuthorityEvidence(context.Background(), f.request.ID, f.request.Version, f.requestDigest, contracts.GoalsPublicationRoot, "execution:test", f.effectIDs, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.GenerationRef != f.child.Ref || evidence.GenerationDigest != f.child.Digest || evidence.ParentRef != f.root.Ref || evidence.ParentDigest != f.root.Digest {
		t.Fatalf("projection must expose child operational generation and distinct parent: %+v", evidence)
	}
}

func TestHistoricalRecoveryEffectProductionWireContractRequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing request id", func(m map[string]any) { delete(m, "RequestID") }},
		{"empty request id", func(m map[string]any) { m["RequestID"] = "" }},
		{"wrong request id", func(m map[string]any) { m["RequestID"] = "request:substituted" }},
		{"missing version", func(m map[string]any) { delete(m, "Version") }},
		{"unsupported version", func(m map[string]any) { m["Version"] = "2" }},
		{"missing step", func(m map[string]any) { delete(m, "Step") }},
		{"wrong step", func(m map[string]any) { m["Step"] = "verify" }},
		{"missing authority", func(m map[string]any) { delete(m, "Authority") }},
		{"lowercase synthetic shape", func(m map[string]any) {
			m["request_id"] = m["RequestID"]
			delete(m, "RequestID")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHistoricalAdversarialFixture(t, nil)
			ctx := context.Background()
			var body []byte
			if err := f.store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, f.effectIDs[0]).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatal(err)
			}
			tc.mutate(m)
			body, _ = json.Marshal(m)
			if _, err := f.store.DB().ExecContext(ctx, `UPDATE effects SET request_payload=? WHERE effect_id=?`, body, f.effectIDs[0]); err != nil {
				t.Fatal(err)
			}
			evidence, err := f.repo.LoadExpiredHistoricalAuthorityEvidence(ctx, f.request.ID, f.request.Version, f.requestDigest, contracts.GoalsPublicationRoot, "execution:test", f.effectIDs, f.now)
			if err == nil || evidence.GenerationDigest != "" || !strings.Contains(err.Error(), "historical effect request lineage mismatch") {
				t.Fatalf("expected canonical wire validation failure, evidence=%+v err=%v", evidence, err)
			}
		})
	}
}
