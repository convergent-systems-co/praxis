package goalspublication

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// persistedObservationResolutionFixture uses the same secure-blob, request,
// decision, delegated-generation, effect, and event writers as production.
// It is intentionally built in a temporary database and never touches dogfood.
type persistedObservationResolutionFixture struct {
	repo     goalstore.Repository
	request  contracts.AuthorityRequest
	effectID string
	original Observation
	now      time.Time
	adapter  *resolutionSpy
}

func persistedResolutionFixture(t *testing.T) persistedObservationResolutionFixture {
	t.Helper()
	ctx := context.Background()
	assets := Assets{[]byte("a"), []byte("bb"), []byte("ccc")}
	r, f, at, predecessor, _, _ := predecessorExecutionFixture(t, assets)
	root := rootOf(f)
	owner := strings.Split(root.ProvenanceRef, ":os-user:")[1]
	old := Execution{Repository: r, Adapter: &fakeAdapter{}, Now: func() time.Time { return at }}
	frozen, confirmation, err := old.PrepareAbandonment(ctx, predecessor.ID, owner, "fixture abandonment")
	must(t, err)
	abandonID, err := old.ConfirmAbandonment(ctx, frozen, owner, "ABANDON "+confirmation)
	must(t, err)
	adoptRecovery(t, r, f, at)
	oldDigest, _ := predecessor.Intent.Digest()
	chainIntent, err := contracts.NewGoalsPublicationOrderedRecoveryIntent(contracts.GoalsOrderedRecoveryInput{GoalsRecoveryInput: contracts.GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(30 * time.Minute), Identity: strings.Repeat("3", 64), AccountID: 8497216, Sizes: [3]int64{2018, 8877585, 401}, PredecessorRequestID: predecessor.ID, PredecessorRequestDigest: confirmation, PredecessorIntentID: predecessor.Intent.ID, PredecessorIntentDigest: oldDigest, AbandonmentEventID: abandonID, AbandonmentDigest: confirmation}})
	must(t, err)
	failedReqDigest, _ := chainIntent.Digest()
	// Build a valid /4 intent with immutable predecessor bindings. The actual
	// current effect is the /4 verification frontier.
	failedID := "goals-publication-recovery-request:" + failedReqDigest
	a4, err := contracts.NewGoalsPublicationFailedVerificationIntent(contracts.GoalsFailedVerificationInput{GoalsOrderedRecoveryInput: contracts.GoalsOrderedRecoveryInput{GoalsRecoveryInput: contracts.GoalsRecoveryInput{CreatedAt: at.Add(2 * time.Hour), ExpiresAt: at.Add(3 * time.Hour), Identity: strings.Repeat("4", 64), AccountID: 8497216, Sizes: [3]int64{2018, 8877585, 401}, PredecessorRequestID: predecessor.ID, PredecessorRequestDigest: confirmation, PredecessorIntentID: predecessor.Intent.ID, PredecessorIntentDigest: oldDigest, AbandonmentEventID: abandonID, AbandonmentDigest: confirmation}}, FailedRequestID: failedID, FailedRequestDigest: "sha256:" + strings.Repeat("1", 64), FailedIntentID: chainIntent.ID, FailedIntentDigest: failedReqDigest, FailedAuthorityDigest: "sha256:" + strings.Repeat("2", 64), FailedHistoricalAuthorityDigest: "sha256:" + strings.Repeat("3", 64), FailedExecutionID: "goals-publication-recovery:" + strings.Repeat("e", 64), FailedManifestEffectID: "failed:manifest", FailedArchiveEffectID: "failed:archive", FailedSignatureEffectID: "failed:signature", FailedVerifyEffectID: "failed:verify-draft", FailedManifestState: "succeeded", FailedArchiveState: "succeeded", FailedSignatureState: "succeeded", FailedVerifyState: "failed", FailedVerifyAttempts: 1, FailedManifestRequestDigest: "sha256:" + strings.Repeat("4", 64), FailedArchiveRequestDigest: "sha256:" + strings.Repeat("5", 64), FailedSignatureRequestDigest: "sha256:" + strings.Repeat("6", 64), FailedVerifyRequestDigest: "sha256:" + strings.Repeat("7", 64), FailedVerifyResultDigest: "sha256:" + strings.Repeat("8", 64), FailedVerifyReconciliationDigest: "sha256:" + strings.Repeat("9", 64), AssetIDs: [3]string{"568522192", "568522548", "568523193"}})
	must(t, err)
	// Persist the request and its normal owner decision/child through the
	// repository's canonical authority writers.
	exp := at.Add(3 * time.Hour)
	intentDigest, _ := a4.Digest()
	d := contracts.DelegationRequest{Profile: contracts.GoalsPublicationRecoveryProfile, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: a4.Actor, TargetKind: "action-intent", TargetIdentity: a4.ID, TargetVersion: a4.Version, TargetDigest: intentDigest, TargetConstraints: []string{a4.Target, contracts.GoalsRecoveryRepositoryID}, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: contracts.GoalsRecoveryOperation, RequestedScope: a4.Scope, ProposalVersion: a4.Version, ProposalDigest: intentDigest, ReviewVersion: a4.Version, ReviewDigest: intentDigest, ExpiresAt: exp, Reason: "publish the exact signed Goals assets to the established draft release", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsRecoveryVersion, PolicyDigest: contracts.AuthorityModelGoalsRecoveryDigest(), SubjectKind: "publisher", SubjectID: contracts.FirstPartyPublisherPrincipal, SubjectVersion: "2", SubjectDigest: contracts.GoalsPublicationPublisher, SubjectKeyDigest: contracts.GoalsRecoveryPublisherKey}
	q := contracts.AuthorityRequest{ID: "goals-publication-recovery-request:" + intentDigest, Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: a4.Scope, Reason: d.Reason, Status: contracts.AuthorityRequestPending, Delegation: &d, Intent: &a4, IntentDigest: intentDigest, InstallationDigest: contracts.GoalsPublicationRoot}
	_, err = r.SaveAuthorityRequest(ctx, q, at.Add(2*time.Hour), &exp)
	must(t, err)
	decision := contracts.AuthorityDecision{RequestID: q.ID, RequestVersion: "1", RequestDigest: qDigestForTest(t, q, at.Add(2*time.Hour)), DecisionRef: "authority-decision:" + q.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: q.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelGoalsRecoveryDigest(), IssuedAt: at.Add(2 * time.Hour), ExpiresAt: &exp, Delegation: &d}
	_, err = r.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx, q.ID, q.Version, decision, goalstore.GoalsPublicationRecoveryPolicy{Repository: r, Request: q}, at.Add(2*time.Hour))
	must(t, err)
	auth, err := r.LoadGoalsPublicationRecoveryAuthorization(ctx, q.ID, at.Add(2*time.Hour), at.Add(2*time.Hour))
	must(t, err)
	obs := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	key := recoveryKey(q.ID)
	effectID := key + ":verify-draft"
	payload := mustJSON(recoveryStepPayload{Version: "1", RequestID: q.ID, Step: "verify-draft", Intent: a4, Authority: auth, PredecessorAbandonment: a4.Parameters["abandonment_digest"]})
	atEffect := at.Add(2*time.Hour + time.Minute)
	cmd := state.CommandRecord{ID: effectID, Type: "goals-publication-recovery.step", Version: "1", Actor: a4.Actor, Scope: a4.Scope, CorrelationID: key, Payload: payload, CreatedAt: atEffect}
	ev := state.EventRecord{ID: effectID, AggregateID: key, AggregateType: "goals-publication-recovery", AggregateVersion: 1, Type: "goals-publication-recovery.step-admitted", Version: "1", Actor: a4.Actor, CommandID: effectID, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: payload, CreatedAt: atEffect}
	ef := state.EffectRecord{ID: effectID, CommandID: effectID, ActionIntentDigest: intentDigest, TargetAdapter: "goals-recovery-github", TargetPrincipal: a4.Actor.ID, PreconditionsJSON: mustJSON(a4.Preconditions), CryptoProfile: a4.CryptoProfile, State: string(state.EffectUnknown), RequestPayload: payload, CreatedAt: atEffect, UpdatedAt: atEffect}
	must(t, r.Store.CommitTransition(ctx, cmd, 0, ev, "", "", &ef))
	obsBytes := mustJSON(obs)
	_, err = r.Store.DB().ExecContext(ctx, `UPDATE effects SET observed_result=?, attempts=1 WHERE effect_id=?`, obsBytes, effectID)
	must(t, err)
	recBody := mustJSON(struct {
		EffectID, Outcome, Reason string
		Observation               Observation
	}{effectID, "unresolved", "matching state does not attribute dispatch", obs})
	recID := effectID + ":reconciliation:fixture"
	rc := state.CommandRecord{ID: recID, Type: "goals-publication-recovery.reconcile", Version: "1", Actor: a4.Actor, Scope: a4.Scope, CorrelationID: key, Payload: recBody, CreatedAt: atEffect.Add(time.Minute)}
	re := state.EventRecord{ID: recID, AggregateID: recID, AggregateType: "goals-publication-recovery-reconciliation", AggregateVersion: 1, Type: "goals-publication-recovery.reconciled", Version: "1", Actor: a4.Actor, CommandID: recID, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: recBody, CreatedAt: atEffect.Add(time.Minute)}
	must(t, r.Store.CommitTransition(ctx, rc, 0, re, "", "", nil))
	return persistedObservationResolutionFixture{repo: r, request: q, effectID: effectID, original: obs, now: at.Add(2*time.Hour + 2*time.Minute), adapter: &resolutionSpy{}}
}
func qDigestForTest(t *testing.T, q contracts.AuthorityRequest, at time.Time) string {
	d, e := q.DigestAt(at)
	must(t, e)
	return d
}

type resolutionSpy struct{ dispatch, reconcile int }

func (s *resolutionSpy) Check(context.Context, contracts.ActionIntent, string, []Observation) error {
	return nil
}
func (s *resolutionSpy) Dispatch(context.Context, contracts.ActionIntent, string, []Observation, Assets) (Observation, error) {
	s.dispatch++
	return Observation{}, nil
}
func (s *resolutionSpy) Reconcile(context.Context, contracts.ActionIntent, string, []Observation) (Observation, error) {
	s.reconcile++
	return Observation{}, nil
}

func TestObservationResolutionSemanticEqualityIgnoresProviderOrder(t *testing.T) {
	a := recoveryReadBackObservation([]int{0, 1, 2}, assetDigests)
	b := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	if err := observationSemanticallyEqual(a, b); err != nil {
		t.Fatalf("provider ordering changed semantic observation: %v", err)
	}

	b.AssetDigests[0], b.AssetDigests[1] = b.AssetDigests[1], b.AssetDigests[0]
	if err := observationSemanticallyEqual(a, b); err == nil {
		t.Fatal("swapped identity-bound content was accepted")
	}
}

func TestObservationResolutionDigestIsStableAcrossInvocations(t *testing.T) {
	a := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	b := recoveryReadBackObservation([]int{2, 1, 0}, assetDigests)
	x := observationResolutionSnapshot{RequestID: "request", EffectID: "effect", IntentDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111", AuthorityDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222", ExecutionID: "execution", Step: "verify-draft", Adapter: "goals-recovery-github", Contract: "goals-established-state-publication/4", Operation: "publish-goals-from-established-state", Attempts: 1, Original: a, Reconciliation: b, ReconciliationEventID: "event", ReconciliationPayloadDigest: "sha256:3333333333333333333333333333333333333333333333333333333333333333"}
	y := x
	y.Original = recoveryReadBackObservation([]int{1, 0, 2}, assetDigests)
	y.Reconciliation = recoveryReadBackObservation([]int{2, 0, 1}, assetDigests)
	d1, err := resolutionDigest(x)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := resolutionDigest(y)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("resolution digest changed with provider ordering: %s != %s", d1, d2)
	}
}

func TestObservationResolutionPersistedChallengeAndResolution(t *testing.T) {
	f := persistedResolutionFixture(t)
	run := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now }}
	digest, err := run.ObservationResolutionChallenge(context.Background(), f.request.ID, f.effectID)
	if err != nil {
		t.Fatalf("challenge failed: %v", err)
	}
	if f.adapter.dispatch != 0 || f.adapter.reconcile != 0 {
		t.Fatalf("challenge invoked adapter: %+v", f.adapter)
	}
	var before int
	must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_type='goals-publication-recovery.observation-resolved'`).Scan(&before))
	if _, err := run.ResolveObservation(context.Background(), f.request.ID, f.effectID, "RESOLVE "+digest); err != nil {
		t.Fatalf("resolution failed: %v", err)
	}
	if f.adapter.dispatch != 0 || f.adapter.reconcile != 0 {
		t.Fatalf("resolution invoked adapter: %+v", f.adapter)
	}
	var st string
	var attempts int
	var result []byte
	must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT state,attempts,observed_result FROM effects WHERE effect_id=?`, f.effectID).Scan(&st, &attempts, &result))
	if st != string(state.EffectSucceeded) || attempts != 1 || string(result) != string(mustJSON(f.original)) {
		t.Fatalf("resolution state incorrect: %s %d", st, attempts)
	}
	var after int
	must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_type='goals-publication-recovery.observation-resolved'`).Scan(&after))
	if after != before+1 {
		t.Fatalf("resolution event count got %d want %d", after, before+1)
	}
	id, err := run.ResolveObservation(context.Background(), f.request.ID, f.effectID, "RESOLVE "+digest)
	if err != nil || id == "" {
		t.Fatalf("exact replay failed: %v", err)
	}
	if _, err := run.ResolveObservation(context.Background(), f.request.ID, f.effectID, "RESOLVE sha256:"+strings.Repeat("f", 64)); err == nil {
		t.Fatal("conflicting confirmation accepted")
	}
	var final int
	must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_type='goals-publication-recovery.observation-resolved'`).Scan(&final))
	if final != after {
		t.Fatalf("replay appended duplicate: %d", final)
	}
}

func TestObservationResolutionChallengeStableAcrossResolverInstances(t *testing.T) {
	f := persistedResolutionFixture(t)
	first := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now }}
	d1, err := first.ObservationResolutionChallenge(context.Background(), f.request.ID, f.effectID)
	if err != nil {
		t.Fatal(err)
	}
	second := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now.Add(45 * time.Second) }}
	d2, err := second.ObservationResolutionChallenge(context.Background(), f.request.ID, f.effectID)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 || "RESOLVE "+d1 != "RESOLVE "+d2 {
		t.Fatalf("challenge changed across instances: %s != %s", d1, d2)
	}
	var commands, events int
	must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM commands WHERE command_type=?`, observationResolutionCommandType).Scan(&commands))
	must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_type=?`, observationResolutionEventType).Scan(&events))
	if commands != 0 || events != 0 || f.adapter.dispatch != 0 || f.adapter.reconcile != 0 {
		t.Fatalf("challenge mutated or invoked adapter: commands=%d events=%d spy=%+v", commands, events, f.adapter)
	}
}

func TestObservationResolutionPersistedAdversarialEligibility(t *testing.T) {
	cases := map[string]func(*persistedObservationResolutionFixture){
		"attempt-zero": func(f *persistedObservationResolutionFixture) {
			_, _ = f.repo.Store.DB().Exec(`UPDATE effects SET attempts=0 WHERE effect_id=?`, f.effectID)
		},
		"attempt-two": func(f *persistedObservationResolutionFixture) {
			_, _ = f.repo.Store.DB().Exec(`UPDATE effects SET attempts=2 WHERE effect_id=?`, f.effectID)
		},
		"missing-original": func(f *persistedObservationResolutionFixture) {
			_, _ = f.repo.Store.DB().Exec(`UPDATE effects SET observed_result=NULL WHERE effect_id=?`, f.effectID)
		},
		"wrong-adapter": func(f *persistedObservationResolutionFixture) {
			_, _ = f.repo.Store.DB().Exec(`UPDATE effects SET target_adapter='other' WHERE effect_id=?`, f.effectID)
		},
		"substituted-original": func(f *persistedObservationResolutionFixture) {
			_, _ = f.repo.Store.DB().Exec(`UPDATE effects SET observed_result='{}' WHERE effect_id=?`, f.effectID)
		},
		"wrong-request-payload": func(f *persistedObservationResolutionFixture) {
			_, _ = f.repo.Store.DB().Exec(`UPDATE effects SET request_payload='{}' WHERE effect_id=?`, f.effectID)
		},
		"reconciliation-outcome": func(f *persistedObservationResolutionFixture) {
			key := recoveryKey(f.request.ID)
			_, _ = f.repo.Store.DB().Exec(`UPDATE commands SET payload=replace(CAST(payload AS TEXT),'unresolved','succeeded') WHERE correlation_id=?`, key)
			_, _ = f.repo.Store.DB().Exec(`UPDATE events SET payload=replace(CAST(payload AS TEXT),'unresolved','succeeded') WHERE correlation_id=?`, key)
		},
		"reconciliation-substituted": func(f *persistedObservationResolutionFixture) {
			key := recoveryKey(f.request.ID)
			_, _ = f.repo.Store.DB().Exec(`UPDATE events SET payload='{}' WHERE correlation_id=? AND event_type='goals-publication-recovery.reconciled'`, key)
		},
		"wrong-effect": func(f *persistedObservationResolutionFixture) { f.effectID = recoveryKey(f.request.ID) + ":publish" },
		"wrong-request": func(f *persistedObservationResolutionFixture) {
			f.request.ID = "goals-publication-recovery-request:substituted"
		},
		"later-publish-effect": func(f *persistedObservationResolutionFixture) {
			ctx := context.Background()
			key := recoveryKey(f.request.ID)
			var payload []byte
			must(t, f.repo.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, f.effectID).Scan(&payload))
			id := key + ":publish"
			at := f.now
			_, err := f.repo.Store.DB().ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, "goals-publication-recovery.step", "1", "publisher:praxis-first-party", "publisher", key, key, payload, "committed", at.Format(time.RFC3339Nano))
			must(t, err)
			_, err = f.repo.Store.DB().ExecContext(ctx, `INSERT INTO effects(effect_id,command_id,action_intent_digest,target_adapter,state,attempts,request_payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, id, "sha256:"+strings.Repeat("a", 64), "goals-recovery-github", "pending", 0, payload, at.Format(time.RFC3339Nano), at.Format(time.RFC3339Nano))
			must(t, err)
			_, err = f.repo.Store.DB().ExecContext(ctx, `INSERT INTO events(event_id,aggregate_id,aggregate_type,aggregate_version,event_type,event_version,actor_id,actor_kind,command_id,correlation_id,trust_class,payload,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, key, "goals-publication-recovery", 2, "goals-publication-recovery.step-admitted", "1", "publisher:praxis-first-party", "publisher", id, key, "policy", payload, at.Format(time.RFC3339Nano))
			must(t, err)
		},
		"abandoned": func(f *persistedObservationResolutionFixture) {
			key := recoveryKey(f.request.ID)
			at := f.now
			id := key + ":abandon"
			_, err := f.repo.Store.DB().ExecContext(context.Background(), `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, "goals-publication-recovery.abandon", "1", "installation-owner", "human", key, key, []byte(`{}`), "committed", at.Format(time.RFC3339Nano))
			must(t, err)
			_, err = f.repo.Store.DB().ExecContext(context.Background(), `INSERT INTO events(event_id,aggregate_id,aggregate_type,aggregate_version,event_type,event_version,actor_id,actor_kind,command_id,correlation_id,payload,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, key, "goals-publication-recovery", 3, "goals-publication-recovery.abandoned", "1", "installation-owner", "human", id, key, []byte(`{}`), at.Format(time.RFC3339Nano))
			must(t, err)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := persistedResolutionFixture(t)
			mutate(&f)
			run := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now }}
			if _, err := run.ObservationResolutionChallenge(context.Background(), f.request.ID, f.effectID); err == nil {
				t.Fatal("malformed persisted resolution was accepted")
			}
			var n int
			must(t, f.repo.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_type=?`, observationResolutionEventType).Scan(&n))
			if n != 0 {
				t.Fatalf("rejected challenge wrote resolution event: %d", n)
			}
		})
	}
}

func TestObservationResolutionAlreadySucceededWithoutResolutionFailsClosed(t *testing.T) {
	f := persistedResolutionFixture(t)
	_, err := f.repo.Store.DB().Exec(`UPDATE effects SET state='succeeded' WHERE effect_id=?`, f.effectID)
	must(t, err)
	run := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now }}
	if _, err := run.ResolveObservation(context.Background(), f.request.ID, f.effectID, "RESOLVE sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("succeeded effect without resolution identity accepted")
	}
}

func TestObservationResolutionIntentDigestIgnoresMapInsertionOrder(t *testing.T) {
	a := contracts.ActionIntent{Version: "1", ID: "intent:map", Actor: contracts.PrincipalRef{ID: "publisher", Kind: "publisher"}, Operation: "op", Target: "target", Scope: "scope", Parameters: map[string]string{}, Preconditions: map[string]string{}}
	b := a
	a.Parameters["a"] = "1"
	a.Parameters["b"] = "2"
	b.Parameters = map[string]string{"b": "2", "a": "1"}
	a.Preconditions["x"] = "1"
	a.Preconditions["y"] = "2"
	b.Preconditions = map[string]string{"y": "2", "x": "1"}
	d1, err := a.Digest()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := b.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("map insertion order changed intent digest: %s != %s", d1, d2)
	}
}
