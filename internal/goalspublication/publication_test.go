package goalspublication

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
