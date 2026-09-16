package goalspublication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pc "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/publisher"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// These fixtures contain public historical evidence only. All writes and
// synthetic grants in these tests go exclusively to a new temporary database.
type publicFixture struct {
	Generations []contracts.AuthorityGeneration       `json:"generations"`
	Publish     contracts.PackagePublishAuthorization `json:"publish"`
	Deploy      contracts.AuthorityGeneration         `json:"deploy"`
	Preview     contracts.SigningPreview              `json:"receipt_preview"`
	Publisher   contracts.PublisherGeneration         `json:"publisher"`
	Envelope    json.RawMessage                       `json:"envelope"`
	Provenance  publisher.Provenance                  `json:"provenance"`
}
type testWrapper struct{}

func (testWrapper) Capabilities(context.Context, string) (pc.Capabilities, error) {
	return pc.Capabilities{Classical: true}, nil
}
func (testWrapper) Wrap(_ context.Context, k string, p contracts.CryptoProfile, b []byte) (pc.WrappedKey, error) {
	return pc.WrappedKey{Ciphertext: append([]byte(nil), b...), SuiteID: "test", KeyRef: k, KeyVersion: "1", SelectedProfile: p}, nil
}
func (testWrapper) Unwrap(_ context.Context, w pc.WrappedKey) ([]byte, error) {
	return append([]byte(nil), w.Ciphertext...), nil
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func put(t *testing.T, r goalstore.Repository, ns, id string, v any, at time.Time, expiry *time.Time) {
	t.Helper()
	b, e := json.Marshal(v)
	must(t, e)
	d := hash(b)
	env, e := r.Crypto.Seal(context.Background(), r.KeyRef, r.Profile, b, state.SecureBlobAAD(ns, id, "1", d))
	must(t, e)
	must(t, r.Store.PutSecureBlob(context.Background(), state.SecureBlobRecord{Namespace: ns, ObjectID: id, ObjectVersion: "1", ObjectDigest: d, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: env, CreatedAt: at, ExpiresAt: expiry}))
}
func fixture(t *testing.T) (goalstore.Repository, publicFixture, time.Time) {
	t.Helper()
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	b, e := os.ReadFile("testdata/public-signing-lineage.json")
	must(t, e)
	var f publicFixture
	must(t, json.Unmarshal(b, &f))
	db, e := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "test.db"))
	must(t, e)
	t.Cleanup(func() { db.Close() })
	r := goalstore.Repository{Store: state.New(db), Crypto: pc.EnvelopeService{Wrapper: testWrapper{}}, KeyRef: "test-only", Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential}
	for _, g := range f.Generations {
		put(t, r, "authority_generation", g.Ref, g, g.EffectiveAt, g.ExpiresAt)
	}
	req, dec := f.Publish.Request, f.Publish.Decision
	put(t, r, "authority_request", req.ID, req, dec.IssuedAt, dec.ExpiresAt)
	put(t, r, "authority_decision", req.ID, struct {
		Request  contracts.AuthorityRequest
		Decision contracts.AuthorityDecision
	}{req, dec}, dec.IssuedAt, dec.ExpiresAt)
	_, e = r.SaveSigningPreview(ctx, f.Preview, f.Preview.CreatedAt)
	must(t, e)
	// Fixed publisher bytes are enrolled as isolated fixture data, never regenerated.
	_, e = r.Store.CommitCanonicalPublisherEnrollment(ctx, f.Publisher, dec.DecidedBy, "test-enrollment", "test-approval", "test-preview", at)
	must(t, e)
	must(t, r.Store.PersistPublisherSigningReceipt(ctx, contracts.GoalsPublicationSigningReceipt, contracts.GoalsPublicationPublisher, "praxis.package.goals", "0.1.0", f.Envelope, f.Provenance, f.Provenance.SignedAt))
	put(t, r, "publisher_governance", "active-authority-model", contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: "v3", ActiveDigest: contracts.AuthorityModelDeploymentDigest(), State: "committed"}, at, nil)
	return r, f, at
}
func adopt(t *testing.T, r goalstore.Repository, f publicFixture, at time.Time) {
	t.Helper()
	root := rootOf(f)
	a := contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v3-to-v4", Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: "v3", FromDigest: contracts.AuthorityModelDeploymentDigest(), ToModel: contracts.AuthorityModelID, ToVersion: "v4", ToDigest: contracts.AuthorityModelGoalsPublicationDigest(), RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest, Reason: "isolated qualification", CreatedAt: at}
	d, e := a.Digest()
	must(t, e)
	user := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	_, e = r.AdoptAuthorityModel(context.Background(), a, contracts.GoalsPublicationBootstrap, user, "ADOPT "+d, at)
	must(t, e)
}
func adoptRecovery(t *testing.T, r goalstore.Repository, f publicFixture, at time.Time) {
	t.Helper()
	root := rootOf(f)
	a := contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v4-to-v5", Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelGoalsPublicationVersion, FromDigest: contracts.AuthorityModelGoalsPublicationDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelGoalsRecoveryVersion, ToDigest: contracts.AuthorityModelGoalsRecoveryDigest(), RootRef: root.Ref, RootVersion: root.Version, RootDigest: root.Digest, Reason: "isolated recovery qualification", CreatedAt: at}
	d, e := a.Digest()
	must(t, e)
	user := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	_, e = r.AdoptAuthorityModel(context.Background(), a, contracts.GoalsPublicationBootstrap, user, "ADOPT "+d, at)
	must(t, e)
}
func rootOf(f publicFixture) contracts.AuthorityGeneration {
	for _, g := range f.Generations {
		if g.Digest == contracts.GoalsPublicationRoot {
			return g
		}
	}
	panic("root fixture missing")
}
func intent(t *testing.T, at time.Time, a Assets) contracts.ActionIntent {
	t.Helper()
	sizes := []int64{1, 2, 3}
	if a[0] != nil {
		for i := range a {
			sizes[i] = int64(len(a[i]))
		}
	}
	v, e := contracts.NewGoalsPublicationIntent(contracts.GoalsPublicationInput{RepositoryID: 123, OwnerID: 456, AccountID: 789, CreatedAt: at, ExpiresAt: at.Add(time.Hour), Nonce: strings.Repeat("ab", 32), ManifestSize: sizes[0], ArchiveSize: sizes[1], SignatureSize: sizes[2]})
	must(t, e)
	return v
}
func authorize(t *testing.T, r goalstore.Repository, f publicFixture, at time.Time, a contracts.ActionIntent) contracts.AuthorityRequest {
	t.Helper()
	ctx := context.Background()
	q, d, e := r.SaveGoalsPublicationRequest(ctx, a, at)
	must(t, e)
	root := rootOf(f)
	dec := contracts.AuthorityDecision{RequestID: q.ID, RequestVersion: q.Version, RequestDigest: d, DecisionRef: "authority-decision:" + q.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: q.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelGoalsPublicationDigest(), IssuedAt: at, ExpiresAt: &q.Delegation.ExpiresAt, Delegation: q.Delegation}
	_, e = r.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx, q.ID, q.Version, dec, goalstore.GoalsPublicationPolicy{Repository: r, Request: q}, at)
	must(t, e)
	return q
}
func exactAssets(t *testing.T) Assets {
	t.Helper()
	dir := os.Getenv("PRAXIS_GOALS_QUALIFICATION_ASSETS")
	if dir == "" {
		t.Skip("set PRAXIS_GOALS_QUALIFICATION_ASSETS to the unchanged signed Goals artifacts for exact-byte integration qualification")
	}
	a, e := ReadAssets(dir)
	must(t, e)
	return a
}

func TestAuthorityClosedEdgeAndHistoricalEvidence(t *testing.T) {
	r, f, at := fixture(t)
	ctx := context.Background()
	a := intent(t, at, Assets{})
	if _, _, e := r.SaveGoalsPublicationRequest(ctx, a, at); e == nil {
		t.Fatal("v3 authorized external mutation")
	}
	adopt(t, r, f, at)
	for _, id := range []string{"absent", f.Publish.Request.ID, f.Deploy.DelegationRef} {
		if _, e := r.LoadGoalsPublicationAuthorization(ctx, id, at, at); e == nil {
			t.Fatal("legacy/unrelated authority accepted")
		}
	}
	q := authorize(t, r, f, at, a)
	auth, e := r.LoadGoalsPublicationAuthorization(ctx, q.ID, at, at)
	must(t, e)
	if auth.Generation.DelegationProfile != contracts.GoalsPublicationProfile {
		t.Fatal("wrong child")
	}
	later := at.Add(2 * time.Hour)
	if _, e = r.LoadGoalsPublicationAuthorization(ctx, q.ID, later, later); e == nil {
		t.Fatal("expired grant executed")
	}
	_, e = r.LoadGoalsPublicationAuthorization(ctx, q.ID, at, later)
	must(t, e)
	// The exact original signing request retains its digest after expiry; current
	// permission still fails, while historical verification uses signing time.
	d, e := f.Publish.Request.DigestAt(f.Provenance.SignedAt)
	must(t, e)
	if d != f.Publish.Decision.RequestDigest {
		t.Fatal("historical digest changed")
	}
	if _, e = f.Publish.Request.DigestAt(f.Publish.Decision.ExpiresAt.Add(time.Second)); e == nil {
		t.Fatal("expired request current-valid")
	}
	_, e = r.ResolvePackagePublishAuthority(ctx, contracts.GoalsPublicationPublisher, "praxis.package.goals", f.Provenance.SignedAt)
	must(t, e)

}

func TestV5SuccessorRequestRequiresFreshExactDecisionAndGeneration(t *testing.T) {
	r, f, at, predecessor, _, _ := predecessorExecutionFixture(t)
	old := Execution{Repository: r, Now: func() time.Time { return at }}
	root := rootOf(f)
	user := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	_, err := old.Abandon(context.Background(), predecessor.ID, user, "fixture abandonment", "wrong")
	if err == nil {
		t.Fatal("expected frozen owner confirmation")
	}
	confirmation := strings.TrimPrefix(err.Error(), "exact owner confirmation required: ABANDON ")
	abandonmentID, err := old.Abandon(context.Background(), predecessor.ID, user, "fixture abandonment", "ABANDON "+confirmation)
	must(t, err)
	var abandonmentBytes []byte
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT payload FROM events WHERE event_id=?`, abandonmentID).Scan(&abandonmentBytes))
	var abandoned abandonmentPayload
	must(t, json.Unmarshal(abandonmentBytes, &abandoned))
	adoptRecovery(t, r, f, at)
	oldDigest, _ := predecessor.Intent.Digest()
	a, err := contracts.NewGoalsPublicationRecoveryIntent(contracts.GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(time.Hour), Identity: strings.Repeat("c", 64), AccountID: 789, Sizes: [3]int64{1, 2, 3}, PredecessorRequestID: predecessor.ID, PredecessorRequestDigest: abandoned.RequestDigest, PredecessorIntentID: predecessor.Intent.ID, PredecessorIntentDigest: oldDigest, AbandonmentEventID: abandonmentID, AbandonmentDigest: confirmation})
	must(t, err)
	q, d, err := r.SaveGoalsPublicationRecoveryRequest(context.Background(), a, at)
	must(t, err)
	for name, mutate := range map[string]func(*contracts.DelegationRequest){"sign-only": func(x *contracts.DelegationRequest) { x.RequestedOperation = "sign" }, "deploy": func(x *contracts.DelegationRequest) { x.RequestedAuthority = contracts.GovernedPackageDeploy }, "general-github": func(x *contracts.DelegationRequest) { x.RequestedCapabilities = []string{"github.write"} }, "wrong-target": func(x *contracts.DelegationRequest) { x.TargetConstraints = []string{"*"} }, "wrong-key": func(x *contracts.DelegationRequest) { x.SubjectKeyDigest = "sha256:" + strings.Repeat("f", 64) }, "wrong-policy": func(x *contracts.DelegationRequest) { x.PolicyVersion = "v4" }, "altered-review": func(x *contracts.DelegationRequest) { x.ReviewVersion = "2" }} {
		bad := *q.Delegation
		mutate(&bad)
		if contracts.ValidateGoalsPublicationRecoveryDelegation(rootOf(f), bad, a, at) == nil {
			t.Fatalf("v5 authority expansion accepted: %s", name)
		}
	}
	if _, err = r.LoadGoalsPublicationRecoveryAuthorization(context.Background(), q.ID, at, at); err == nil {
		t.Fatal("prepared request implicitly authorized")
	}
	decision := contracts.AuthorityDecision{RequestID: q.ID, RequestVersion: q.Version, RequestDigest: d, DecisionRef: "authority-decision:" + q.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: q.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelGoalsRecoveryDigest(), IssuedAt: at, ExpiresAt: &q.Delegation.ExpiresAt, Delegation: q.Delegation}
	_, err = r.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(context.Background(), q.ID, q.Version, decision, goalstore.GoalsPublicationRecoveryPolicy{Repository: r, Request: q}, at)
	must(t, err)
	auth, err := r.LoadGoalsPublicationRecoveryAuthorization(context.Background(), q.ID, at, at)
	must(t, err)
	if auth.Generation.DelegationProfile != contracts.GoalsPublicationRecoveryProfile || auth.Generation.Digest == f.Publish.Generation.Digest {
		t.Fatal("successor inherited predecessor authority")
	}
}

func predecessorExecutionFixture(t *testing.T, signed ...Assets) (goalstore.Repository, publicFixture, time.Time, contracts.AuthorityRequest, contracts.PackagePublishAuthorization, string) {
	t.Helper()
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	assets := Assets{}
	if len(signed) > 0 {
		assets = signed[0]
	}
	a := intent(t, at, assets)
	q := authorize(t, r, f, at, a)
	auth, err := r.LoadGoalsPublicationAuthorization(context.Background(), q.ID, at, at)
	must(t, err)
	key := executionKey(q.ID)
	for i, step := range []string{"refs", "draft", "manifest"} {
		payload := mustJSON(stepPayload{Version: "1", RequestID: q.ID, Intent: a, Step: step, Authority: auth})
		id := key + ":" + step
		intentDigest, _ := a.Digest()
		cmd := state.CommandRecord{ID: id, Type: "goals-publication.step", Version: "1", Actor: a.Actor, Scope: a.Scope, CorrelationID: key, Payload: payload, CreatedAt: at}
		ev := state.EventRecord{ID: id, AggregateID: key, AggregateType: "goals-initial-publication", AggregateVersion: int64(i + 1), Type: "goals-publication.step-admitted", Version: "1", Actor: a.Actor, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: payload, CreatedAt: at}
		ef := state.EffectRecord{ID: id, CommandID: id, ActionIntentDigest: intentDigest, TargetAdapter: "goals-initial-github", TargetPrincipal: a.Actor.ID, PreconditionsJSON: mustJSON(a.Preconditions), CryptoProfile: a.CryptoProfile, State: string(state.EffectPending), RequestPayload: payload, CreatedAt: at, UpdatedAt: at}
		must(t, r.Store.CommitTransition(context.Background(), cmd, int64(i), ev, "", "", &ef))
		_, err = r.Store.DB().ExecContext(context.Background(), `UPDATE effects SET state='dispatched',attempts=1 WHERE effect_id=?`, id)
		must(t, err)
		outcome := state.EffectSucceeded
		observed := []byte(`{"historical":"success"}`)
		reconciliation := []byte(nil)
		if step == "manifest" {
			outcome = state.EffectUnknown
			observed = []byte(`{"dispatch":"ambiguous"}`)
			reconciliation = []byte(`{"unresolved":true}`)
		}
		must(t, r.Store.MarkEffectOutcome(context.Background(), id, outcome, observed, reconciliation, at))
	}
	recID := key + ":manifest:reconciliation:fixture"
	rb := []byte(`{"outcome":"unresolved"}`)
	rc := state.CommandRecord{ID: recID, Type: "goals-publication.reconcile", Version: "1", Actor: a.Actor, Scope: a.Scope, CorrelationID: key, Payload: rb, CreatedAt: at}
	re := state.EventRecord{ID: recID, AggregateID: recID, AggregateType: "goals-publication-reconciliation", AggregateVersion: 1, Type: "goals-publication.reconciled", Version: "1", Actor: a.Actor, CommandID: recID, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: rb, CreatedAt: at}
	must(t, r.Store.CommitTransition(context.Background(), rc, 0, re, "", "", nil))
	return r, f, at, q, auth, key
}

func TestAbandonmentPreservesLineageAndPermanentlyFencesPredecessor(t *testing.T) {
	r, f, at, q, auth, key := predecessorExecutionFixture(t)
	ctx := context.Background()
	e := Execution{Repository: r, Adapter: &fakeAdapter{}, Now: func() time.Time { return at }}
	if _, err := e.Abandon(ctx, "goals-publication-request:substituted", "test", "reason", "ABANDON anything"); err == nil {
		t.Fatal("wrong request accepted")
	}
	root := rootOf(f)
	user := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	if _, err := e.Abandon(ctx, q.ID, "other-user", "reason", "ABANDON anything"); err == nil {
		t.Fatal("non-owner accepted")
	}
	before := make(map[string]string)
	for _, step := range []string{"refs", "draft", "manifest"} {
		v, err := e.load(ctx, key+":"+step)
		must(t, err)
		before[step] = hash(v.Payload) + "/" + hash(v.Result) + "/" + v.State
		var rec []byte
		must(t, r.Store.DB().QueryRowContext(ctx, `SELECT reconciliation_evidence FROM effects WHERE effect_id=?`, key+":"+step).Scan(&rec))
		before[step] += "/" + hash(rec)
	}
	_, err := e.Abandon(ctx, q.ID, user, "fixed reason", "wrong")
	if err == nil {
		t.Fatal("missing exact owner confirmation accepted")
	} else if !strings.Contains(err.Error(), "exact owner confirmation required: ABANDON ") {
		t.Fatal(err)
	}
	digest := strings.TrimPrefix(err.Error(), "exact owner confirmation required: ABANDON ")
	id, err := e.Abandon(ctx, q.ID, user, "fixed reason", "ABANDON "+digest)
	must(t, err)
	if again, err := e.Abandon(ctx, q.ID, user, "fixed reason", "ABANDON "+digest); err != nil || again != id {
		t.Fatalf("byte-identical replay failed: %s %v", again, err)
	}
	if _, err = e.Abandon(ctx, q.ID, user, "altered reason", "ABANDON "+digest); err == nil {
		t.Fatal("altered replay accepted")
	}
	for _, step := range []string{"refs", "draft", "manifest"} {
		v, err := e.load(ctx, key+":"+step)
		must(t, err)
		var rec []byte
		must(t, r.Store.DB().QueryRowContext(ctx, `SELECT reconciliation_evidence FROM effects WHERE effect_id=?`, key+":"+step).Scan(&rec))
		after := hash(v.Payload) + "/" + hash(v.Result) + "/" + v.State + "/" + hash(rec)
		if after != before[step] {
			t.Fatalf("abandonment changed %s effect/evidence", step)
		}
	}
	if _, err = e.Execute(ctx, q.ID); err == nil || !strings.Contains(err.Error(), "terminally abandoned") {
		t.Fatalf("old execution not fenced: %v", err)
	}
	if err = e.Reconcile(ctx, q.ID); err == nil || !strings.Contains(err.Error(), "permanently fenced") {
		t.Fatalf("old reconciliation not fenced: %v", err)
	}
	var n int
	must(t, r.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id=?`, key+":completed").Scan(&n))
	if n != 0 {
		t.Fatal("abandoned predecessor has completion")
	}
	completion := key + ":completed"
	cmd := state.CommandRecord{ID: completion, Type: "goals-publication.complete", Version: "1", Actor: auth.Request.Intent.Actor, Scope: auth.Request.Intent.Scope, CorrelationID: key, Payload: []byte(`{"forged":true}`), CreatedAt: at}
	ev := state.EventRecord{ID: completion, AggregateID: key, AggregateType: "goals-initial-publication", AggregateVersion: 4, Type: "goals-publication.completed", Version: "1", Actor: cmd.Actor, CommandID: completion, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: cmd.Payload, CreatedAt: at}
	if err := r.Store.CommitGoalsPublicationCompletion(ctx, cmd, 3, ev, key); err == nil || !strings.Contains(err.Error(), "cannot complete") {
		t.Fatalf("completion path passed abandonment fence: %v", err)
	}
	loaded, err := r.LoadGoalsPublicationAuthorization(ctx, q.ID, at, at)
	must(t, err)
	if loaded.Generation.Digest != auth.Generation.Digest {
		t.Fatal("abandonment revoked or changed historical authority")
	}
	var p abandonmentPayload
	var payload []byte
	must(t, r.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, id).Scan(&payload))
	must(t, json.Unmarshal(payload, &p))
	if p.CompletionEstablished || p.ExecutionStatus != "abandoned" || len(p.Effects) != 3 || p.Effects[2].State != "unknown" || len(p.ReconciliationEvents) != 1 || p.ReconciliationEvents[0].PayloadDigest == "" {
		t.Fatalf("abandonment evidence incomplete: %+v", p)
	}
}

func TestAbandonmentRejectsRequestIntentGenerationAndEffectSubstitution(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, goalstore.Repository, string){
		"intent": func(t *testing.T, r goalstore.Repository, key string) {
			var b []byte
			must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT request_payload FROM effects WHERE effect_id=?`, key+":manifest").Scan(&b))
			var p stepPayload
			must(t, json.Unmarshal(b, &p))
			p.Intent.Parameters["package"] = "other.package@9"
			b = mustJSON(p)
			_, err := r.Store.DB().ExecContext(context.Background(), `UPDATE effects SET request_payload=? WHERE effect_id=?`, b, key+":manifest")
			must(t, err)
		},
		"authority-generation": func(t *testing.T, r goalstore.Repository, key string) {
			var b []byte
			must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT request_payload FROM effects WHERE effect_id=?`, key+":manifest").Scan(&b))
			var p stepPayload
			must(t, json.Unmarshal(b, &p))
			p.Authority.Generation.Digest = "sha256:" + strings.Repeat("f", 64)
			b = mustJSON(p)
			_, err := r.Store.DB().ExecContext(context.Background(), `UPDATE effects SET request_payload=? WHERE effect_id=?`, b, key+":manifest")
			must(t, err)
		},
		"effect-principal": func(t *testing.T, r goalstore.Repository, key string) {
			_, err := r.Store.DB().ExecContext(context.Background(), `UPDATE effects SET target_principal='substituted' WHERE effect_id=?`, key+":manifest")
			must(t, err)
		},
		"effect-state": func(t *testing.T, r goalstore.Repository, key string) {
			_, err := r.Store.DB().ExecContext(context.Background(), `UPDATE effects SET state='failed' WHERE effect_id=?`, key+":manifest")
			must(t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, f, at, q, _, key := predecessorExecutionFixture(t)
			mutate(t, r, key)
			root := rootOf(f)
			user := strings.Split(root.ProvenanceRef, ":os-user:")[1]
			e := Execution{Repository: r, Now: func() time.Time { return at }}
			if _, err := e.Abandon(context.Background(), q.ID, user, "reason", "ABANDON forged"); err == nil {
				t.Fatal("substituted predecessor lineage accepted")
			}
		})
	}
}

func TestAuthorizedSuccessorExecutionAndAcquisitionLineage(t *testing.T) {
	assets := exactAssets(t)
	r, at, q, assets := authorizedRecoveryFixture(t, assets)
	confirmation := q.Intent.Parameters["abandonment_digest"]
	adapter := &recoveryFakeAdapter{}
	run := RecoveryExecution{Repository: r, Adapter: adapter, Assets: assets, Now: func() time.Time { return at }}
	completion, err := run.Execute(context.Background(), q.ID)
	must(t, err)
	if !strings.HasSuffix(completion, ":completed") || strings.Join(adapter.calls, ",") != strings.Join(recoverySteps, ",") {
		t.Fatalf("successor executed wrong effects: %v", adapter.calls)
	}
	for _, step := range []string{"manifest", "archive", "signature", "publish"} {
		v, err := run.load(context.Background(), recoveryKey(q.ID)+":"+step)
		must(t, err)
		if v.State != string(state.EffectSucceeded) {
			t.Fatalf("acknowledged %s effect was not durable: %s", step, v.State)
		}
		var observation Observation
		must(t, json.Unmarshal(v.Result, &observation))
		if observation.DispatchOutcome == nil || observation.DispatchOutcome.Class != "acknowledged_success" {
			t.Fatalf("%s acknowledgment missing from durable effect evidence: %+v", step, observation.DispatchOutcome)
		}
	}
	var payload []byte
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT payload FROM events WHERE event_id=? AND event_type='goals-publication-recovery.completed'`, completion).Scan(&payload))
	var completed struct {
		PredecessorManifestOutcome string
		PredecessorAbandonment     string
	}
	must(t, json.Unmarshal(payload, &completed))
	if completed.PredecessorManifestOutcome != "unknown-unresolved" || completed.PredecessorAbandonment != confirmation {
		t.Fatalf("completion rewrote predecessor context: %+v", completed)
	}
	acquirer := Execution{Repository: r, Adapter: adapter, Now: func() time.Time { return at }}
	release := releaseOf(assets)
	got, err := acquirer.CheckAcquisition(context.Background(), release, assets[1])
	must(t, err)
	if got != completion {
		t.Fatal("acquisition did not resolve exact successor completion")
	}
	must(t, acquirer.RecordAcquisitionCheck(context.Background(), completion))
}

func TestRecoveryDispatchClassificationsPersistAndNeverBlindlyRetry(t *testing.T) {
	assets := exactAssets(t)
	cases := []struct {
		name      string
		outcome   DispatchOutcome
		apply     bool
		wantState state.EffectState
	}{{"launch-failure", DispatchOutcome{Version: "1", Process: "launch_failed", Class: "local_pre_dispatch_failure"}, false, state.EffectFailed}, {"provider-rejection", DispatchOutcome{Version: "1", Process: "started", Class: "provider_response", HTTPStatus: 422, RequestID: "ABCDEF12", Stderr: "provider returned HTTP status 422"}, false, state.EffectUnknown}, {"lost-response", DispatchOutcome{Version: "1", Process: "started", Class: "ambiguous", Stderr: "process started but no provider response was acknowledged"}, true, state.EffectUnknown}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, at, q, assets := authorizedRecoveryFixture(t, assets)
			adapter := &recoveryFakeAdapter{failStep: "manifest", failure: tc.outcome, applyBeforeFailure: tc.apply}
			run := RecoveryExecution{Repository: r, Adapter: adapter, Assets: assets, Now: func() time.Time { return at }}
			if _, err := run.Execute(context.Background(), q.ID); err == nil {
				t.Fatal("scripted dispatch failure reported success")
			}
			id := recoveryKey(q.ID) + ":manifest"
			v, err := run.load(context.Background(), id)
			must(t, err)
			if v.State != string(tc.wantState) {
				t.Fatalf("classification state got %s want %s", v.State, tc.wantState)
			}
			var got DispatchOutcome
			must(t, json.Unmarshal(v.Result, &got))
			if got.Class != tc.outcome.Class || got.Process != tc.outcome.Process || got.HTTPStatus != tc.outcome.HTTPStatus || got.RequestID != tc.outcome.RequestID {
				t.Fatalf("persisted diagnostics mismatch: %+v", got)
			}
			if _, err = run.Execute(context.Background(), q.ID); err == nil {
				t.Fatal("unresolved/failed effect retried as success")
			}
			if len(adapter.calls) != 1 {
				t.Fatalf("effect dispatched more than once: %v", adapter.calls)
			}
			old := executionKey(q.Intent.Parameters["predecessor_request_id"]) + ":manifest"
			var historical string
			must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT state FROM effects WHERE effect_id=?`, old).Scan(&historical))
			if historical != string(state.EffectUnknown) {
				t.Fatal("transport evidence changed historical predecessor UNKNOWN")
			}
		})
	}
}

func authorizedRecoveryFixture(t *testing.T, assets Assets) (goalstore.Repository, time.Time, contracts.AuthorityRequest, Assets) {
	t.Helper()
	r, f, at, pred, _, _ := predecessorExecutionFixture(t, assets)
	old := Execution{Repository: r, Adapter: &fakeAdapter{}, Now: func() time.Time { return at }}
	root := rootOf(f)
	owner := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	_, err := old.Abandon(context.Background(), pred.ID, owner, "fixture abandonment", "wrong")
	if err == nil {
		t.Fatal("expected owner confirmation")
	}
	confirmation := strings.TrimPrefix(err.Error(), "exact owner confirmation required: ABANDON ")
	abandonID, err := old.Abandon(context.Background(), pred.ID, owner, "fixture abandonment", "ABANDON "+confirmation)
	must(t, err)
	var raw []byte
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT payload FROM events WHERE event_id=?`, abandonID).Scan(&raw))
	var abandoned abandonmentPayload
	must(t, json.Unmarshal(raw, &abandoned))
	adoptRecovery(t, r, f, at)
	oldDigest, _ := pred.Intent.Digest()
	intent, err := contracts.NewGoalsPublicationRecoveryIntent(contracts.GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(time.Hour), Identity: strings.Repeat("d", 64), AccountID: 789, Sizes: [3]int64{int64(len(assets[0])), int64(len(assets[1])), int64(len(assets[2]))}, PredecessorRequestID: pred.ID, PredecessorRequestDigest: abandoned.RequestDigest, PredecessorIntentID: pred.Intent.ID, PredecessorIntentDigest: oldDigest, AbandonmentEventID: abandonID, AbandonmentDigest: confirmation})
	must(t, err)
	q, d, err := r.SaveGoalsPublicationRecoveryRequest(context.Background(), intent, at)
	must(t, err)
	decision := contracts.AuthorityDecision{RequestID: q.ID, RequestVersion: q.Version, RequestDigest: d, DecisionRef: "authority-decision:" + q.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: q.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelGoalsRecoveryDigest(), IssuedAt: at, ExpiresAt: &q.Delegation.ExpiresAt, Delegation: q.Delegation}
	_, err = r.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(context.Background(), q.ID, q.Version, decision, goalstore.GoalsPublicationRecoveryPolicy{Repository: r, Request: q}, at)
	must(t, err)
	return r, at, q, assets
}

func TestIntentAndDelegationSubstitutions(t *testing.T) {
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	a := intent(t, at, Assets{})
	q := authorize(t, r, f, at, a)
	for k := range a.Parameters {
		t.Run("parameter/"+k, func(t *testing.T) {
			var b contracts.ActionIntent
			must(t, json.Unmarshal(mustJSON(a), &b))
			b.Parameters[k] += "changed"
			if contracts.ValidateGoalsPublicationIntent(b) == nil {
				t.Fatal("substitution accepted")
			}
		})
	}
	for k := range a.Preconditions {
		t.Run("precondition/"+k, func(t *testing.T) {
			var b contracts.ActionIntent
			must(t, json.Unmarshal(mustJSON(a), &b))
			b.Preconditions[k] = "ignored"
			if contracts.ValidateGoalsPublicationIntent(b) == nil {
				t.Fatal("substitution accepted")
			}
		})
	}
	cases := map[string]func(*contracts.DelegationRequest){"sign-only": func(d *contracts.DelegationRequest) { d.Profile = contracts.DelegationProfilePackagePublish }, "deploy-only": func(d *contracts.DelegationRequest) { d.RequestedAuthority = contracts.GovernedPackageDeploy }, "wrong-model": func(d *contracts.DelegationRequest) { d.PolicyVersion = "v3" }, "publisher": func(d *contracts.DelegationRequest) { d.SubjectDigest = contracts.GoalsPublicationRoot }, "key": func(d *contracts.DelegationRequest) { d.SubjectKeyDigest = contracts.GoalsPublicationRoot }, "runtime-capability": func(d *contracts.DelegationRequest) { d.RequestedCapabilities = []string{"github.write"} }, "scope": func(d *contracts.DelegationRequest) { d.RequestedScope = "*" }, "intent": func(d *contracts.DelegationRequest) { d.TargetDigest = contracts.GoalsPublicationRoot }, "expiry": func(d *contracts.DelegationRequest) { d.ExpiresAt = at }, "parent": func(d *contracts.DelegationRequest) { d.ParentDigest = contracts.GoalsPublicationPublisher }}
	for n, mut := range cases {
		t.Run(n, func(t *testing.T) {
			d := *q.Delegation
			mut(&d)
			if contracts.ValidateGoalsPublicationDelegation(rootOf(f), d, a, at) == nil {
				t.Fatal("bad delegation accepted")
			}
		})
	}
}

type fakeAdapter struct {
	mu       sync.Mutex
	calls    []string
	fail     string
	checkErr error
	hook     func(string)
}

func (f *fakeAdapter) Check(context.Context, contracts.ActionIntent, string, []Observation) error {
	return f.checkErr
}
func observation(a contracts.ActionIntent, s string, p []Observation) Observation {
	o := Observation{RepositoryID: 123, OwnerID: 456, AccountID: 789, Commit: a.Parameters["commit"], Tree: a.Parameters["tree"]}
	if s == "refs" {
		o.DispatchEvidence = "*\tnew-main\n*\tnew-tag\n"
		return o
	}
	if idx, ok := map[string]int{"manifest": 0, "archive": 1, "signature": 2}[s]; ok {
		size := int64(0)
		json.Unmarshal([]byte(a.Parameters[s+"_size"]), &size)
		o.Asset = &Asset{ID: int64(100 + idx), Name: assetNames[idx], Size: size, State: "uploaded", Uploader: githubUser{789}}
		return o
	}
	o.Release = &Release{ID: 10, Tag: contracts.GoalsPublicationTag, Target: a.Parameters["commit"], Name: a.Parameters["release_name"], Body: a.Parameters["release_body"], Draft: s == "draft" || s == "verify", Author: githubUser{789}}
	if s != "draft" {
		for i := 2; i < 5; i++ {
			o.Release.Assets = append(o.Release.Assets, *p[i].Asset)
		}
	}
	if s == "verify" || s == "verify-published" {
		o.AssetDigests = append([]string{}, assetDigests...)
	}
	return o
}
func (f *fakeAdapter) Dispatch(_ context.Context, a contracts.ActionIntent, s string, p []Observation, _ Assets) (Observation, error) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
	if f.hook != nil {
		f.hook(s)
	}
	if s == f.fail {
		return Observation{}, errors.New("lost response after mutation")
	}
	return observation(a, s, p), nil
}
func (f *fakeAdapter) Reconcile(context.Context, contracts.ActionIntent, string, []Observation) (Observation, error) {
	return Observation{}, errors.New("ambiguous creator; no retry")
}

type recoveryFakeAdapter struct {
	calls              []string
	uploaded           int
	failStep           string
	failure            DispatchOutcome
	applyBeforeFailure bool
}

func (f *recoveryFakeAdapter) Check(_ context.Context, _ contracts.ActionIntent, step string, _ []Observation) error {
	want := map[string]int{"manifest": 0, "archive": 1, "signature": 2, "verify-draft": 3, "publish": 3, "verify-published": 3}[step]
	if f.uploaded != want {
		return fmt.Errorf("fixture inventory before %s: got %d want %d", step, f.uploaded, want)
	}
	return nil
}
func (f *recoveryFakeAdapter) Dispatch(_ context.Context, a contracts.ActionIntent, step string, previous []Observation, _ Assets) (Observation, error) {
	f.calls = append(f.calls, step)
	if step == f.failStep && f.applyBeforeFailure {
		f.uploaded++
	}
	if step == f.failStep {
		return Observation{}, dispatchFailure{outcome: f.failure, err: errors.New("scripted provider outcome")}
	}
	o := Observation{RepositoryID: 1372388187, OwnerID: 263966243, AccountID: 789, Commit: contracts.GoalsRecoveryCommit, Tree: contracts.GoalsRecoveryTree}
	switch step {
	case "manifest", "archive", "signature":
		i := map[string]int{"manifest": 0, "archive": 1, "signature": 2}[step]
		var size int64
		json.Unmarshal([]byte(a.Parameters[[]string{"manifest_size", "archive_size", "signature_size"}[i]]), &size)
		o.Asset = &Asset{ID: int64(600 + i), Name: assetNames[i], Size: size, State: "uploaded", Uploader: githubUser{789}}
		o.DispatchOutcome = &DispatchOutcome{Version: "1", Process: "started", Class: "acknowledged_success"}
		f.uploaded++
	case "verify-draft", "publish", "verify-published":
		draft := step == "verify-draft"
		r := &Release{ID: 389997269, Tag: contracts.GoalsPublicationTag, Target: contracts.GoalsRecoveryCommit, Name: contracts.GoalsPublicationPackage, Body: "Exact signed Goals initial publication; intent " + contracts.GoalsRecoveryNonce, Draft: draft, Author: githubUser{789}}
		for i := 0; i < 3; i++ {
			r.Assets = append(r.Assets, Asset{ID: int64(600 + i), Name: assetNames[i], Size: parseSize(a.Parameters[[]string{"manifest_size", "archive_size", "signature_size"}[i]]), State: "uploaded", Uploader: githubUser{789}})
		}
		o.Release = r
		if step != "publish" {
			o.AssetDigests = append([]string{}, assetDigests...)
		}
		if step == "publish" {
			o.DispatchOutcome = &DispatchOutcome{Version: "1", Process: "started", Class: "acknowledged_success"}
		}
	}
	return o, nil
}
func (f *recoveryFakeAdapter) Reconcile(context.Context, contracts.ActionIntent, string, []Observation) (Observation, error) {
	return Observation{}, errors.New("fixture deliberately cannot attribute external state")
}
func parseSize(v string) int64 { n, _ := strconv.ParseInt(v, 10, 64); return n }
func releaseOf(a Assets) distribution.Release {
	r := distribution.Release{Ref: distribution.PackageRef{Source: "github-releases", Owner: "convergent-systems-co", Repo: "praxis-packages"}, Tag: contracts.GoalsPublicationTag, ManifestBytes: a[0], SignatureBytes: a[2], ManifestDigest: contracts.GoalsPublicationManifest}
	json.Unmarshal(a[0], &r.Manifest)
	json.Unmarshal(a[2], &r.Signature)
	return r
}
func TestExactSignedPackageExecutionReplayAndAcquisition(t *testing.T) {
	a := exactAssets(t)
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q := authorize(t, r, f, at, intent(t, at, a))
	adapter := &fakeAdapter{}
	e := Execution{Repository: r, Adapter: adapter, Assets: a, Now: func() time.Time { return at }}
	ctx := context.Background()
	must(t, VerifySigning(ctx, r, a, at))
	id, err := e.Execute(ctx, q.ID)
	must(t, err)
	again, err := e.Execute(ctx, q.ID)
	must(t, err)
	if id != again || len(adapter.calls) != len(steps) {
		t.Fatal("replay dispatched again")
	}
	got, err := e.CheckAcquisition(ctx, releaseOf(a), a[1])
	must(t, err)
	if got != id {
		t.Fatal("wrong completion")
	}
	e.Now = func() time.Time { return at.Add(2 * time.Hour) }
	_, err = e.CheckAcquisition(ctx, releaseOf(a), a[1])
	must(t, err)
	adapter.checkErr = errors.New("moved tag")
	if _, err = e.CheckAcquisition(ctx, releaseOf(a), a[1]); err == nil {
		t.Fatal("remote substitution accepted")
	}
	adapter.checkErr = nil
	_, err = r.Store.DB().Exec(`UPDATE effects SET observed_result='{}' WHERE effect_id=?`, executionKey(q.ID)+":publish")
	must(t, err)
	if _, err = e.CheckAcquisition(ctx, releaseOf(a), a[1]); err == nil {
		t.Fatal("forged completion lineage accepted")
	}
}
func TestInterruptionsNeverRedispatchAmbiguousMutation(t *testing.T) {
	a := exactAssets(t)
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			r, f, at := fixture(t)
			adopt(t, r, f, at)
			q := authorize(t, r, f, at, intent(t, at, a))
			ad := &fakeAdapter{fail: step}
			e := Execution{Repository: r, Adapter: ad, Assets: a, Now: func() time.Time { return at }}
			if _, err := e.Execute(context.Background(), q.ID); err == nil {
				t.Fatal("ambiguous effect completed")
			}
			n := len(ad.calls)
			ad.fail = ""
			if _, err := e.Execute(context.Background(), q.ID); err == nil {
				t.Fatal("uncertainty silently resolved")
			}
			if len(ad.calls) != n {
				t.Fatal("mutation repeated")
			}
			var count int
			must(t, r.Store.DB().QueryRow(`SELECT count(*) FROM events WHERE event_type='goals-publication.completed'`).Scan(&count))
			if count != 0 {
				t.Fatal("false completion")
			}
		})
	}
}
func TestConcurrentConsumersAndAuthorityLoss(t *testing.T) {
	a := exactAssets(t)
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q := authorize(t, r, f, at, intent(t, at, a))
	ad := &fakeAdapter{}
	e := Execution{Repository: r, Adapter: ad, Assets: a, Now: func() time.Time { return at }}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.Execute(context.Background(), q.ID) }()
	}
	wg.Wait()
	// A contender may halt on a concurrently dispatched record. The winner or
	// subsequent original execution completes without a second external mutation.
	_, err := e.Execute(context.Background(), q.ID)
	must(t, err)
	if len(ad.calls) != len(steps) {
		t.Fatalf("duplicate dispatch: %v", ad.calls)
	}
	t.Run("expiry-before-next-effect", func(t *testing.T) {
		r, f, at := fixture(t)
		adopt(t, r, f, at)
		q := authorize(t, r, f, at, intent(t, at, a))
		now := at
		ad := &fakeAdapter{hook: func(s string) {
			if s == "refs" {
				now = at.Add(2 * time.Hour)
			}
		}}
		e := Execution{Repository: r, Adapter: ad, Assets: a, Now: func() time.Time { return now }}
		if _, err := e.Execute(context.Background(), q.ID); err == nil {
			t.Fatal("authority loss ignored")
		}
		if len(ad.calls) != 1 {
			t.Fatal("mutated after expiry")
		}
	})
}

func TestRevocationAndSigningTamperFenceExecution(t *testing.T) {
	a := exactAssets(t)
	for _, tc := range []string{"root-revoked", "grant-revoked", "publisher-revoked", "provenance-forged", "wrong-assets", "no-completion"} {
		t.Run(tc, func(t *testing.T) {
			r, f, at := fixture(t)
			adopt(t, r, f, at)
			q := authorize(t, r, f, at, intent(t, at, a))
			ad := &fakeAdapter{}
			e := Execution{Repository: r, Adapter: ad, Assets: a, Now: func() time.Time { return at }}
			ctx := context.Background()
			switch tc {
			case "root-revoked", "grant-revoked":
				ref := rootOf(f).Ref
				if tc == "grant-revoked" {
					ref = "authority-delegation:" + q.ID
				}
				put(t, r, "authority_generation_invalidation", ref, map[string]string{"test": "revoked"}, at, nil)
			case "publisher-revoked":
				_, err := r.Store.DB().Exec(`UPDATE publisher_generations SET state='revoked'`)
				must(t, err)
			case "provenance-forged":
				_, err := r.Store.DB().Exec(`UPDATE publisher_signing_receipts SET provenance_json='{}'`)
				must(t, err)
			case "wrong-assets":
				e.Assets[0] = []byte("changed")
			case "no-completion":
				if _, err := e.CheckAcquisition(ctx, releaseOf(a), a[1]); err == nil {
					t.Fatal("accepted missing completion")
				}
				return
			}
			if _, err := e.Execute(ctx, q.ID); err == nil {
				t.Fatal("trust fence bypassed")
			}
			if len(ad.calls) != 0 {
				t.Fatal("external effect escaped fence")
			}
		})
	}
}
func TestGenerationSubstitutionFailsEvenWithValidDigest(t *testing.T) {
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q := authorize(t, r, f, at, intent(t, at, Assets{}))
	ctx := context.Background()
	auth, err := r.LoadGoalsPublicationAuthorization(ctx, q.ID, at, at)
	must(t, err)
	auth.Generation.Capabilities = []string{"unexpected"}
	auth.Generation.Digest, err = auth.Generation.ComputeDigest()
	must(t, err)
	_, err = r.Store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace='authority_generation' AND object_id=?`, auth.Generation.Ref)
	must(t, err)
	put(t, r, "authority_generation", auth.Generation.Ref, auth.Generation, at, auth.Generation.ExpiresAt)
	if _, err = r.LoadGoalsPublicationAuthorization(ctx, q.ID, at, at); err == nil {
		t.Fatal("broadened generation accepted")
	}
}
func TestBuiltinPoliciesCannotAuthorizeExternalPublication(t *testing.T) {
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q := authorize(t, r, f, at, intent(t, at, Assets{}))
	if contracts.ValidateBuiltinDelegation(rootOf(f), *q.Delegation, at) == nil {
		t.Fatal("v1 policy broadened")
	}
	if contracts.ValidateBuiltinPackagePublishDelegation(rootOf(f), *q.Delegation, at) == nil {
		t.Fatal("v2 policy broadened")
	}
	if contracts.ValidateBuiltinPackageDeployDelegation(rootOf(f), *q.Delegation, at) == nil {
		t.Fatal("v3 policy broadened")
	}
}
func TestReconciliationAfterAuthorityLossIsReadOnlyAndDurable(t *testing.T) {
	a := exactAssets(t)
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q := authorize(t, r, f, at, intent(t, at, a))
	ad := &fakeAdapter{fail: "draft"}
	e := Execution{Repository: r, Adapter: ad, Assets: a, Now: func() time.Time { return at }}
	if _, err := e.Execute(context.Background(), q.ID); err == nil {
		t.Fatal("lost response completed")
	}
	put(t, r, "authority_generation_invalidation", "authority-delegation:"+q.ID, map[string]string{"test": "revoked"}, at, nil)
	e.Now = func() time.Time { return at.Add(2 * time.Hour) }
	if err := e.Reconcile(context.Background(), q.ID); err == nil {
		t.Fatal("invented reconciliation success")
	}
	if len(ad.calls) != 2 {
		t.Fatal("reconciliation mutated")
	}
	var n int
	must(t, r.Store.DB().QueryRow(`SELECT count(*) FROM events WHERE event_type='goals-publication.reconciled'`).Scan(&n))
	if n != 1 {
		t.Fatal("lost reconciliation evidence")
	}
}

func TestPersistenceInterruptionsAtEveryStep(t *testing.T) {
	a := exactAssets(t)
	for _, step := range steps {
		for _, boundary := range []string{"before-admission", "before-outcome"} {
			t.Run(step+"/"+boundary, func(t *testing.T) {
				r, f, at := fixture(t)
				adopt(t, r, f, at)
				q := authorize(t, r, f, at, intent(t, at, a))
				ad := &fakeAdapter{}
				e := Execution{Repository: r, Adapter: ad, Assets: a, Now: func() time.Time { return at }}
				id := executionKey(q.ID) + ":" + step
				var sql string
				if boundary == "before-admission" {
					sql = `CREATE TRIGGER interrupt BEFORE INSERT ON commands WHEN NEW.command_id='` + id + `' BEGIN SELECT RAISE(ABORT,'test interruption'); END`
				} else {
					sql = `CREATE TRIGGER interrupt BEFORE UPDATE OF state ON effects WHEN NEW.effect_id='` + id + `' AND NEW.state='succeeded' BEGIN SELECT RAISE(ABORT,'test interruption'); END`
				}
				_, err := r.Store.DB().Exec(sql)
				must(t, err)
				if _, err = e.Execute(context.Background(), q.ID); err == nil {
					t.Fatal("injected interruption ignored")
				}
				n := len(ad.calls)
				_, err = r.Store.DB().Exec(`DROP TRIGGER interrupt`)
				must(t, err)
				_, err = e.Execute(context.Background(), q.ID)
				if boundary == "before-admission" {
					must(t, err)
					if len(ad.calls) != len(steps) {
						t.Fatal("redispatched succeeded steps")
					}
				} else {
					if err == nil {
						t.Fatal("lost outcome fabricated success")
					}
					if len(ad.calls) != n {
						t.Fatal("ambiguous dispatch repeated")
					}
				}
			})
		}
	}
}
func TestAcquisitionRejectsChangedRawSignatureBytes(t *testing.T) {
	a := exactAssets(t)
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q := authorize(t, r, f, at, intent(t, at, a))
	e := Execution{Repository: r, Adapter: &fakeAdapter{}, Assets: a, Now: func() time.Time { return at }}
	_, err := e.Execute(context.Background(), q.ID)
	must(t, err)
	release := releaseOf(a)
	release.SignatureBytes = append(append([]byte{}, a[2]...), '\n')
	if _, err = e.CheckAcquisition(context.Background(), release, a[1]); err == nil {
		t.Fatal("remarshalling hid changed signature-envelope bytes")
	}
}

func TestV4PreservesExistingDeploymentGenerationWithoutRenewal(t *testing.T) {
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	g, err := r.ResolvePackageManagerAuthority(context.Background(), contracts.GoalsPublicationRoot, f.Deploy.EffectiveAt.Add(time.Second))
	must(t, err)
	if g.Digest != f.Deploy.Digest || !g.ExpiresAt.Equal(*f.Deploy.ExpiresAt) {
		t.Fatal("deployment authority replaced or renewed")
	}
	if _, err = r.ResolvePackageManagerAuthority(context.Background(), contracts.GoalsPublicationRoot, f.Deploy.ExpiresAt.Add(time.Second)); err == nil {
		t.Fatal("v4 renewed expired deployment permission")
	}
}
func TestPreparedRequestAndCredentialsDoNotAuthorizeDispatch(t *testing.T) {
	a := exactAssets(t)
	r, f, at := fixture(t)
	adopt(t, r, f, at)
	q, _, err := r.SaveGoalsPublicationRequest(context.Background(), intent(t, at, a), at)
	must(t, err)
	adapter := &fakeAdapter{}
	e := Execution{Repository: r, Adapter: adapter, Assets: a, Now: func() time.Time { return at }}
	if _, err = e.Execute(context.Background(), q.ID); err == nil {
		t.Fatal("unsigned approval inferred from request or credentials")
	}
	if len(adapter.calls) != 0 {
		t.Fatal("credential-only dispatch")
	}
}
