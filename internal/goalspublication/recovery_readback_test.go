package goalspublication

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func recoveryReadBackIntent() contracts.ActionIntent {
	return contracts.ActionIntent{Parameters: map[string]string{
		"repository_id": "1372388187", "owner_id": "263966243", "commit": "fbdc98828d49cf5ddd515edf91d457b606e89a97", "tree": "06476050446988f592cd2064823ff73c5a7a09f0", "release_id": "389997269", "tag": "goals/v0.1.0",
		"account_id": "8497216", "asset_manifest_id": "568522192", "asset_archive_id": "568522548", "asset_signature_id": "568523193",
		"manifest_size": "2018", "archive_size": "8877585", "signature_size": "401",
	}}
}

func recoveryReadBackObservation(order []int, digests []string) Observation {
	names := []string{"praxis-package.json", "praxis-package.tar.gz", "praxis-package.sig.json"}
	sizes := []int64{2018, 8877585, 401}
	ids := []int64{568522192, 568522548, 568523193}
	assets := make([]Asset, 0, 3)
	orderedDigests := make([]string, 0, 3)
	for _, i := range order {
		assets = append(assets, Asset{ID: ids[i], Name: names[i], Size: sizes[i], State: "uploaded", Uploader: githubUser{ID: 8497216}})
		orderedDigests = append(orderedDigests, digests[i])
	}
	return Observation{RepositoryID: 1372388187, OwnerID: 263966243, AccountID: 8497216, Commit: "fbdc98828d49cf5ddd515edf91d457b606e89a97", Tree: "06476050446988f592cd2064823ff73c5a7a09f0", Release: &Release{ID: 389997269, Tag: "goals/v0.1.0", Target: "fbdc98828d49cf5ddd515edf91d457b606e89a97", Draft: true, Assets: assets}, AssetDigests: orderedDigests}
}

func TestValidateRecoveryAssetReadBackIsIdentityBoundAndOrderIndependent(t *testing.T) {
	a := recoveryReadBackIntent()
	permutations := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, order := range permutations {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			if err := validateRecoveryObservation(a, "verify-draft", recoveryReadBackObservation(order, assetDigests), nil); err != nil {
				t.Fatalf("valid permutation rejected: %v", err)
			}
		})
	}
	published := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	published.Release.Draft = false
	if err := validateRecoveryObservation(a, "verify-published", published, nil); err != nil {
		t.Fatalf("valid published permutation rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Observation){
		"unknown-name":    func(o *Observation) { o.Release.Assets[0].Name = "other" },
		"duplicate-name":  func(o *Observation) { o.Release.Assets[1].Name = o.Release.Assets[0].Name },
		"duplicate-id":    func(o *Observation) { o.Release.Assets[1].ID = o.Release.Assets[0].ID },
		"wrong-id":        func(o *Observation) { o.Release.Assets[0].ID++ },
		"wrong-size":      func(o *Observation) { o.Release.Assets[0].Size++ },
		"swapped-content": func(o *Observation) { o.AssetDigests[0], o.AssetDigests[1] = o.AssetDigests[1], o.AssetDigests[0] },
		"missing-digest":  func(o *Observation) { o.AssetDigests = o.AssetDigests[:2] },
	} {
		t.Run(name, func(t *testing.T) {
			o := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
			mutate(&o)
			if err := validateRecoveryObservation(a, "verify-draft", o, nil); err == nil {
				t.Fatal("malformed read-back accepted")
			}
		})
	}
}

type reconcileProbe struct{ steps []string }

func (p *reconcileProbe) Check(context.Context, contracts.ActionIntent, string, []Observation) error {
	return nil
}
func (p *reconcileProbe) Dispatch(context.Context, contracts.ActionIntent, string, []Observation, Assets) (Observation, error) {
	return Observation{}, errors.New("dispatch not expected")
}
func (p *reconcileProbe) Reconcile(_ context.Context, _ contracts.ActionIntent, step string, _ []Observation) (Observation, error) {
	p.steps = append(p.steps, step)
	return Observation{}, errors.New("probe reconciliation remains unresolved")
}

func persistRecoveryTestEffect(t *testing.T, r goalstore.Repository, at time.Time, q contracts.AuthorityRequest, a contracts.ActionIntent, auth contracts.PackagePublishAuthorization, step string, aggregate int64) {
	t.Helper()
	key := recoveryKey(q.ID)
	id := key + ":" + step
	payload := mustJSON(recoveryStepPayload{Version: "1", RequestID: q.ID, Step: step, Intent: a, Authority: auth, PredecessorAbandonment: a.Parameters["abandonment_digest"]})
	digest, err := a.Digest()
	must(t, err)
	cmd := state.CommandRecord{ID: id, Type: "goals-publication-recovery.step", Version: "1", Actor: a.Actor, Scope: a.Scope, CorrelationID: key, Payload: payload, CreatedAt: at}
	ev := state.EventRecord{ID: id, AggregateID: key, AggregateType: "goals-publication-recovery", AggregateVersion: aggregate, Type: "goals-publication-recovery.step-admitted", Version: "1", Actor: a.Actor, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: payload, CreatedAt: at}
	effect := state.EffectRecord{ID: id, CommandID: id, ActionIntentDigest: digest, TargetAdapter: "goals-recovery-github", TargetPrincipal: a.Actor.ID, PreconditionsJSON: mustJSON(a.Preconditions), CryptoProfile: a.CryptoProfile, State: string(state.EffectPending), RequestPayload: payload, CreatedAt: at, UpdatedAt: at}
	must(t, r.Store.CommitTransition(context.Background(), cmd, aggregate-1, ev, "", "", &effect))
	_, err = r.Store.DB().ExecContext(context.Background(), `UPDATE effects SET state='dispatched', attempts=1 WHERE effect_id=?`, id)
	must(t, err)
}

func TestRecoveryReconcileUsesPreparedFailedVerificationFrontier(t *testing.T) {
	r, at, q, _ := authorizedRecoveryFixture(t, Assets{[]byte("a"), []byte("bb"), []byte("ccc")})
	a := *q.Intent
	a.Parameters["contract"] = contracts.GoalsFailedVerificationContract
	a.Parameters["permitted_effects"] = "verify-draft-assets,publish-existing-release,verify-published-release"
	auth, err := r.LoadGoalsPublicationRecoveryAuthorization(context.Background(), q.ID, at, at)
	must(t, err)
	key := recoveryKey(q.ID)
	id := key + ":verify-draft"
	persistRecoveryTestEffect(t, r, at, q, a, auth, "verify-draft", 1)
	must(t, r.Store.MarkEffectOutcome(context.Background(), id, state.EffectUnknown, []byte(`{"observation":"unknown"}`), nil, at))
	probe := &reconcileProbe{}
	run := RecoveryExecution{Repository: r, Adapter: probe, Now: func() time.Time { return at }}
	if err := run.Reconcile(context.Background(), q.ID); err == nil || err.Error() != "successor effect remains unresolved" {
		t.Fatalf("unexpected reconciliation result: %v", err)
	}
	if fmt.Sprint(probe.steps) != "[verify-draft]" {
		t.Fatalf("wrong reconciliation frontier: %v", probe.steps)
	}
	var stateValue string
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT state FROM effects WHERE effect_id=?`, id).Scan(&stateValue))
	if stateValue != string(state.EffectUnknown) {
		t.Fatalf("reconciliation changed UNKNOWN state: %s", stateValue)
	}
}

func TestRecoveryReconcileRejectsGraphGapsAndOutOfGraphEffects(t *testing.T) {
	for name, step := range map[string]string{"gap": "publish", "out-of-graph": "manifest"} {
		t.Run(name, func(t *testing.T) {
			r, at, q, _ := authorizedRecoveryFixture(t, Assets{[]byte("a"), []byte("bb"), []byte("ccc")})
			a := *q.Intent
			a.Parameters["contract"] = contracts.GoalsFailedVerificationContract
			a.Parameters["permitted_effects"] = "verify-draft-assets,publish-existing-release,verify-published-release"
			auth, err := r.LoadGoalsPublicationRecoveryAuthorization(context.Background(), q.ID, at, at)
			must(t, err)
			persistRecoveryTestEffect(t, r, at, q, a, auth, step, 1)
			run := RecoveryExecution{Repository: r, Adapter: &reconcileProbe{}, Now: func() time.Time { return at }}
			if err := run.Reconcile(context.Background(), q.ID); err == nil {
				t.Fatal("invalid recovery graph accepted")
			}
		})
	}
}

func TestRecoveryGraphSelectionIsClosed(t *testing.T) {
	for _, contract := range []string{contracts.GoalsRecoveryContract, contracts.GoalsChainedRecoveryContract, contracts.GoalsOrderedRecoveryContract} {
		graph, err := recoveryGraph(contracts.ActionIntent{Operation: "publish-goals-from-established-state", Parameters: map[string]string{"contract": contract}})
		if err != nil || fmt.Sprint(graph) != fmt.Sprint(recoverySteps) {
			t.Fatalf("legacy graph mismatch for %s: %v %v", contract, graph, err)
		}
	}
	graph, err := recoveryGraph(contracts.ActionIntent{Operation: "publish-goals-from-established-state", Parameters: map[string]string{"contract": contracts.GoalsFailedVerificationContract}})
	if err != nil || fmt.Sprint(graph) != "[verify-draft publish verify-published]" {
		t.Fatalf("failed-verification graph mismatch: %v %v", graph, err)
	}
	for _, a := range []contracts.ActionIntent{
		{Operation: "other", Parameters: map[string]string{"contract": contracts.GoalsFailedVerificationContract}},
		{Operation: "publish-goals-from-established-state", Parameters: map[string]string{"contract": "unknown"}},
	} {
		if _, err := recoveryGraph(a); err == nil {
			t.Fatal("unsupported recovery graph accepted")
		}
	}
}
