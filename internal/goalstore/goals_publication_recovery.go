package goalstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// SaveGoalsPublicationRecoveryRequest persists one fresh successor intent in
// the existing protected authority-request store.
func (r Repository) SaveGoalsPublicationRecoveryRequest(ctx context.Context, a contracts.ActionIntent, now time.Time) (contracts.AuthorityRequest, string, error) {
	fail := func(err error) (contracts.AuthorityRequest, string, error) {
		return contracts.AuthorityRequest{}, "", err
	}
	if err := contracts.ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		return fail(err)
	}
	var bindErr error
	if a.Parameters["contract"] == contracts.GoalsChainedRecoveryContract {
		bindErr = r.ValidateGoalsPublicationChainedRecoveryBinding(ctx, a)
		if bindErr == nil {
			bindErr = r.ValidateGoalsPublicationAbandonmentBinding(ctx, a)
		}
	} else if a.Parameters["contract"] == contracts.GoalsOrderedRecoveryContract {
		bindErr = r.ValidateGoalsPublicationOrderedRecoveryBinding(ctx, a)
		if bindErr == nil {
			bindErr = r.ValidateGoalsPublicationAbandonmentBinding(ctx, a)
		}
	} else if a.Parameters["contract"] == contracts.GoalsFailedVerificationContract {
		bindErr = r.ValidateGoalsPublicationFailedVerificationBinding(ctx, a)
		if bindErr == nil {
			bindErr = r.ValidateGoalsPublicationAbandonmentBinding(ctx, a)
		}
	} else if a.Parameters["contract"] == contracts.GoalsFailedPublicationContract {
		bindErr = r.ValidateGoalsPublicationFailedPublicationBinding(ctx, a)
	} else {
		bindErr = r.ValidateGoalsPublicationAbandonmentBinding(ctx, a)
	}
	if bindErr != nil {
		return fail(bindErr)
	}
	m, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return fail(err)
	}
	if m.ActiveVersion != contracts.AuthorityModelGoalsRecoveryVersion || m.ActiveDigest != contracts.AuthorityModelGoalsRecoveryDigest() || m.State != "committed" {
		return fail(errors.New("v5 Goals recovery model must be explicitly adopted"))
	}
	root, err := r.LoadAuthorityGeneration(ctx, contracts.InstallationGovernanceScopePrefix+contracts.GoalsPublicationBootstrap, "1", now)
	if err != nil {
		return fail(err)
	}
	if err = r.CheckGoalsPublicationInvalidation(ctx, root.Ref, root.Version, "", "", now); err != nil {
		return fail(err)
	}
	pub, err := r.Store.PublisherGeneration(ctx, contracts.GoalsPublicationPublisher)
	if err != nil {
		return fail(err)
	}
	if pub.State != "active" || pub.Generation.Principal != a.Actor || pub.Generation.Generation != "2" {
		return fail(errors.New("exact enrolled publisher unavailable"))
	}
	if err := pub.Generation.Validate(); err != nil {
		return fail(err)
	}
	pd, err := pub.Generation.Digest()
	if err != nil || pd != contracts.GoalsPublicationPublisher {
		return fail(errors.New("publisher generation digest mismatch"))
	}
	id, err := a.Digest()
	if err != nil {
		return fail(err)
	}
	expiry, _ := time.Parse(time.RFC3339Nano, a.Parameters["expires_at"])
	operation := contracts.GoalsRecoveryOperation
	if a.Parameters["contract"] == contracts.GoalsFailedPublicationContract {
		operation = contracts.GoalsFailedPublicationOperation
	}
	d := contracts.DelegationRequest{Profile: contracts.GoalsPublicationRecoveryProfile, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: a.Actor, TargetKind: "action-intent", TargetIdentity: a.ID, TargetVersion: a.Version, TargetDigest: id, TargetConstraints: []string{a.Target, a.Parameters["repository_id"]}, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: operation, RequestedScope: a.Scope, ProposalVersion: a.Version, ProposalDigest: id, ReviewVersion: a.Version, ReviewDigest: id, ExpiresAt: expiry, Reason: "publish the exact signed Goals assets to the established draft release", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelGoalsRecoveryVersion, PolicyDigest: contracts.AuthorityModelGoalsRecoveryDigest(), SubjectKind: "publisher", SubjectID: a.Actor.ID, SubjectVersion: "2", SubjectDigest: pd, SubjectKeyDigest: pub.Generation.PublicKeyDigest}
	if err = contracts.ValidateGoalsPublicationRecoveryDelegation(root, d, a, now); err != nil {
		return fail(err)
	}
	req := contracts.AuthorityRequest{ID: "goals-publication-recovery-request:" + id, Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: a.Scope, Reason: d.Reason, Status: contracts.AuthorityRequestPending, Delegation: &d, Intent: &a, IntentDigest: id, InstallationDigest: contracts.GoalsPublicationRoot}
	rd, err := r.SaveAuthorityRequest(ctx, req, now, &expiry)
	if err != nil {
		return fail(err)
	}
	return req, rd, nil
}

// ValidateGoalsPublicationFailedVerificationBinding binds the exact failed
// ordered-recovery execution and its three established uploads. It does not
// alter any historical effect and authorizes no upload operation.
func (r Repository) ValidateGoalsPublicationFailedVerificationBinding(ctx context.Context, a contracts.ActionIntent) error {
	if err := contracts.ValidateGoalsPublicationFailedVerificationIntent(a); err != nil {
		return err
	}
	p := a.Parameters
	if p["failed_predecessor_execution_id"] != "goals-publication-recovery:"+recoveryHash([]byte(p["failed_predecessor_request_id"])) {
		return errors.New("failed predecessor execution identity mismatch")
	}
	for _, k := range []string{"failed_manifest_effect_id", "failed_archive_effect_id", "failed_signature_effect_id", "failed_verify_effect_id"} {
		var state string
		var attempts int
		var payload []byte
		if err := r.Store.DB().QueryRowContext(ctx, `SELECT state,attempts,request_payload FROM effects WHERE effect_id=?`, p[k]).Scan(&state, &attempts, &payload); err != nil {
			return err
		}
		want := p["failed_"+strings.TrimSuffix(strings.TrimPrefix(k, "failed_"), "_effect_id")+"_state"]
		if state != want {
			return errors.New("failed verification effect state mismatch")
		}
		if k == "failed_verify_effect_id" && attempts != 1 {
			return errors.New("failed verification attempts mismatch")
		}
		var step struct {
			RequestID, Step string
			Intent          contracts.ActionIntent
			Authority       contracts.PackagePublishAuthorization
		}
		wantStep, ok := failedVerificationStep(k)
		if !ok {
			return errors.New("failed verification effect mapping is unknown")
		}
		if err := json.Unmarshal(payload, &step); err != nil || step.RequestID != p["failed_predecessor_request_id"] || step.Step != wantStep || step.Intent.ID != p["failed_predecessor_intent_id"] || step.Authority.Generation.Digest != p["failed_predecessor_authority_digest"] {
			return errors.New("failed verification lineage payload mismatch")
		}
	}
	for _, k := range []string{"failed_predecessor_request_id", "failed_predecessor_intent_id", "failed_predecessor_authority_digest", "failed_predecessor_execution_id"} {
		if p[k] == "" {
			return errors.New("failed predecessor identity missing")
		}
	}
	return nil
}

func (r Repository) ValidateGoalsPublicationFailedPublicationBinding(ctx context.Context, a contracts.ActionIntent) error {
	if err := contracts.ValidateGoalsPublicationFailedPublicationIntent(a); err != nil {
		return err
	}
	p := a.Parameters
	var st, adapter, actionDigest string
	var attempts int
	var observed, reconciliation, payload []byte
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT state,attempts,target_adapter,action_intent_digest,COALESCE(observed_result,''),COALESCE(reconciliation_evidence,''),request_payload FROM effects WHERE effect_id=?`, p["failed_publication_effect_id"]).Scan(&st, &attempts, &adapter, &actionDigest, &observed, &reconciliation, &payload); err != nil {
		return err
	}
	if st != "failed" || attempts != 1 || len(observed) != 0 || len(reconciliation) != 0 {
		return errors.New("failed-publication effect is not pre-dispatch terminal")
	}
	if adapter != "goals-recovery-github" || actionDigest == "" {
		return errors.New("failed-publication effect authority binding mismatch")
	}
	if recoveryHash(payload) != p["failed_publication_payload_digest"] {
		return errors.New("failed-publication payload digest mismatch")
	}
	var stepPayload struct {
		Version, RequestID, Step string
		Intent                   contracts.ActionIntent
	}
	if err := json.Unmarshal(payload, &stepPayload); err != nil || stepPayload.Version != "1" || stepPayload.RequestID != p["failed_publication_predecessor_request_id"] || stepPayload.Step != "publish" {
		return errors.New("failed-publication payload lineage mismatch")
	}
	id, err := stepPayload.Intent.Digest()
	if err != nil || stepPayload.Intent.ID != p["failed_publication_predecessor_intent_id"] || id != p["failed_publication_predecessor_intent_digest"] || actionDigest != id {
		return errors.New("failed-publication payload intent mismatch")
	}
	var ct, cv, et, ev string
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT c.command_type,c.command_version,v.event_type,v.event_version FROM commands c JOIN events v ON v.event_id=c.command_id WHERE c.command_id=? AND c.payload=?`, p["failed_publication_command_id"], payload).Scan(&ct, &cv, &et, &ev); err != nil {
		return err
	}
	if ct != "goals-publication-recovery.step" || cv != "1" || et != "goals-publication-recovery.step-admitted" || ev != "1" {
		return errors.New("failed-publication command/event mismatch")
	}
	return nil
}

func failedVerificationStep(effectField string) (string, bool) {
	step, ok := map[string]string{
		"failed_manifest_effect_id":  "manifest",
		"failed_archive_effect_id":   "archive",
		"failed_signature_effect_id": "signature",
		"failed_verify_effect_id":    "verify-draft",
	}[effectField]
	return step, ok
}

func (r Repository) ValidateGoalsPublicationOrderedRecoveryBinding(ctx context.Context, a contracts.ActionIntent) error {
	if err := contracts.ValidateGoalsPublicationOrderedRecoveryIntent(a); err != nil {
		return err
	}
	chain, _ := contracts.ParseRecoveryChain(a.Parameters["recovery_chain"])
	for i, x := range chain {
		var body []byte
		if err := r.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type='goals-publication-recovery.abandoned' AND event_version='1'`, x.AbandonmentEventID).Scan(&body); err != nil {
			return err
		}
		if recoveryHash(body) != x.AbandonmentDigest {
			return errors.New("ordered recovery abandonment digest mismatch")
		}
		var p struct {
			Version, RequestID, RequestDigest, IntentID, IntentDigest, AuthorityDigest, ExecutionID, ExecutionStatus string
			Effects                                                                                                  []struct {
				ID, State                       string
				Attempts                        int
				Request, Result, Reconciliation string
			}
			CompletionEstablished bool
		}
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		if p.Version != "1" || p.ExecutionStatus != "abandoned" || p.CompletionEstablished || p.RequestID != x.RequestID || p.RequestDigest != x.RequestDigest || p.IntentID != x.IntentID || p.IntentDigest != x.IntentDigest || p.AuthorityDigest != x.AuthorityDigest || p.ExecutionID != x.ExecutionID || len(p.Effects) == 0 || p.Effects[0].ID != x.ManifestEffectID || p.Effects[0].State != "unknown" || p.Effects[0].Attempts != 1 || p.Effects[0].Request != x.ManifestRequestDigest || p.Effects[0].Result != x.ManifestResultDigest || p.Effects[0].Reconciliation != x.ManifestReconciliationDigest {
			return errors.New("ordered recovery lineage mismatch")
		}
		var payload []byte
		if err := r.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, x.ManifestEffectID).Scan(&payload); err != nil {
			return err
		}
		var sp struct {
			RequestID, Step string
			Intent          contracts.ActionIntent
		}
		if err := json.Unmarshal(payload, &sp); err != nil || sp.RequestID != x.RequestID || sp.Step != "manifest" {
			return errors.New("ordered recovery manifest payload mismatch")
		}
		d, err := sp.Intent.Digest()
		if err != nil || d != x.IntentDigest {
			return errors.New("ordered recovery intent digest mismatch")
		}
		if i > 0 && sp.Intent.Parameters["prior_recovery_request_id"] != chain[i-1].RequestID {
			return errors.New("ordered recovery chain ordering mismatch")
		}
		if i == 0 && sp.Intent.Parameters["prior_recovery_request_id"] != "" {
			return errors.New("ordered recovery chain root mismatch")
		}
	}
	return nil
}

// ValidateGoalsPublicationChainedRecoveryBinding verifies the fixed two-
// generation recovery chain. It intentionally accepts no arbitrary history.
func (r Repository) ValidateGoalsPublicationChainedRecoveryBinding(ctx context.Context, a contracts.ActionIntent) error {
	if err := contracts.ValidateGoalsPublicationChainedRecoveryIntent(a); err != nil {
		return err
	}
	p := a.Parameters
	var body []byte
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type='goals-publication-recovery.abandoned' AND event_version='1'`, p["prior_recovery_abandonment_event_id"]).Scan(&body); err != nil {
		return err
	}
	if recoveryHash(body) != p["prior_recovery_abandonment_digest"] {
		return errors.New("prior recovery abandonment digest mismatch")
	}
	var x struct {
		Version, RequestID, RequestDigest, IntentID, IntentDigest, AuthorityDigest, ExecutionID, ExecutionStatus string
		Effects                                                                                                  []struct {
			ID, State                       string
			Attempts                        int
			Request, Result, Reconciliation string
		}
		CompletionEstablished bool
	}
	if err := json.Unmarshal(body, &x); err != nil {
		return err
	}
	if x.Version != "1" || x.ExecutionStatus != "abandoned" || x.CompletionEstablished || x.RequestID != p["prior_recovery_request_id"] || x.RequestDigest != p["prior_recovery_request_digest"] || x.IntentID != p["prior_recovery_intent_id"] || x.IntentDigest != p["prior_recovery_intent_digest"] || x.AuthorityDigest != p["prior_recovery_authority_digest"] || x.ExecutionID != p["prior_recovery_execution_id"] || len(x.Effects) == 0 || x.Effects[0].State != "unknown" || x.Effects[0].Attempts != 1 {
		return errors.New("prior recovery abandonment lineage mismatch")
	}
	if x.Effects[0].ID != p["prior_recovery_manifest_effect_id"] || x.Effects[0].Request != p["prior_recovery_manifest_request_digest"] || x.Effects[0].Result != p["prior_recovery_manifest_result_digest"] || x.Effects[0].Reconciliation != p["prior_recovery_manifest_reconciliation_digest"] {
		return errors.New("prior recovery manifest evidence mismatch")
	}
	return nil
}

func recoveryHash(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

type abandonmentLineage struct {
	Version, RequestID, RequestDigest, IntentID, IntentDigest, AuthorityRef, AuthorityVersion, AuthorityDigest, ExecutionID, ExecutionStatus string
	Effects                                                                                                                                  []struct {
		ID, State                       string
		Attempts                        int
		Request, Result, Reconciliation string
	}
	Owner                 contracts.PrincipalRef
	Reason                string
	CompletionEstablished bool
}

func (r Repository) ValidateGoalsPublicationAbandonmentBinding(ctx context.Context, a contracts.ActionIntent) error {
	p := a.Parameters
	var body []byte
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type='goals-publication.abandoned' AND event_version='1'`, p["abandonment_event_id"]).Scan(&body); err != nil {
		return err
	}
	if recoveryHash(body) != p["abandonment_digest"] {
		return errors.New("abandonment event digest mismatch")
	}
	var x abandonmentLineage
	if err := json.Unmarshal(body, &x); err != nil {
		return err
	}
	if x.Version != "1" || x.ExecutionStatus != "abandoned" || x.CompletionEstablished || x.RequestID != p["predecessor_request_id"] || x.RequestDigest != p["predecessor_request_digest"] || x.IntentID != p["predecessor_intent_id"] || x.IntentDigest != p["predecessor_intent_digest"] || len(x.Effects) != 3 || x.Effects[0].State != "succeeded" || x.Effects[1].State != "succeeded" || x.Effects[2].State != "unknown" {
		return errors.New("abandonment lineage does not bind exact unresolved predecessor")
	}
	key := "goals-initial-publication:" + recoveryHash([]byte(x.RequestID))
	var done int
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id=?`, key+":completed").Scan(&done); err != nil || done != 0 {
		return errors.New("predecessor completion is present")
	}
	rows, err := r.Store.DB().QueryContext(ctx, `SELECT e.effect_id,e.state,e.attempts,e.request_payload,COALESCE(e.observed_result,''),COALESCE(e.reconciliation_evidence,'') FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id WHERE c.command_type='goals-publication.step' AND c.correlation_id=? ORDER BY v.aggregate_version`, key)
	if err != nil {
		return err
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var id, st string
		var attempts int
		var req, obs, rec []byte
		if err = rows.Scan(&id, &st, &attempts, &req, &obs, &rec); err != nil {
			return err
		}
		if i >= len(x.Effects) {
			return errors.New("unexpected predecessor effect")
		}
		q := x.Effects[i]
		if q.ID != id || q.State != st || q.Attempts != attempts || q.Request != recoveryHash(req) || q.Result != recoveryHash(obs) || q.Reconciliation != recoveryHash(rec) {
			return errors.New("predecessor effect changed since abandonment")
		}
		i++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if i != 3 {
		return errors.New("predecessor effect lineage incomplete")
	}
	return nil
}

type GoalsPublicationRecoveryPolicy struct {
	Repository Repository
	Request    contracts.AuthorityRequest
}

func (p GoalsPublicationRecoveryPolicy) ContainDelegation(parent contracts.AuthorityGeneration, d contracts.DelegationRequest, now time.Time) error {
	ctx := context.Background()
	m, err := p.Repository.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return err
	}
	if m.ActiveVersion != contracts.AuthorityModelGoalsRecoveryVersion || m.ActiveDigest != contracts.AuthorityModelGoalsRecoveryDigest() || m.State != "committed" {
		return errors.New("v5 successor model is not active")
	}
	req, err := p.Repository.LoadAuthorityRequest(ctx, p.Request.ID, p.Request.Version, now)
	if err != nil {
		return err
	}
	if req.Intent == nil || req.Delegation == nil {
		return errors.New("protected successor intent missing")
	}
	id, err := req.Intent.Digest()
	if err != nil {
		return err
	}
	if req.ID != "goals-publication-recovery-request:"+id || req.IntentDigest != id || req.InstallationDigest != contracts.GoalsPublicationRoot || req.RequestedAuthority != contracts.GovernedPackagePublish || req.RequestedScope != req.Intent.Scope || req.Status != contracts.AuthorityRequestPending {
		return errors.New("successor request identity mismatch")
	}
	a, _ := json.Marshal(d)
	b, _ := json.Marshal(req.Delegation)
	if string(a) != string(b) {
		return errors.New("successor delegation substitution")
	}
	if err = p.Repository.CheckGoalsPublicationInvalidation(ctx, parent.Ref, parent.Version, "", "", now); err != nil {
		return err
	}
	return contracts.ValidateGoalsPublicationRecoveryDelegation(parent, d, *req.Intent, now)
}

// LoadGoalsPublicationRecoveryAuthorization resolves the exact v5 request,
// owner decision and child generation. It never falls back to the v4 grant.
func (r Repository) LoadGoalsPublicationRecoveryAuthorization(ctx context.Context, id string, at, current time.Time) (contracts.PackagePublishAuthorization, error) {
	fail := func(e error) (contracts.PackagePublishAuthorization, error) {
		return contracts.PackagePublishAuthorization{}, e
	}
	payload, _, err := r.loadWorkPlanBlob(ctx, authorityRequestNamespace, id, "1", at)
	if err != nil {
		return fail(err)
	}
	var req contracts.AuthorityRequest
	if err = json.Unmarshal(payload, &req); err != nil {
		return fail(err)
	}
	if req.ID != id || req.Version != "1" || req.Delegation == nil || req.Intent == nil || req.Delegation.Profile != contracts.GoalsPublicationRecoveryProfile {
		return fail(errors.New("not a protected Goals successor request"))
	}
	d := req.Delegation
	root, err := r.LoadAuthorityGeneration(ctx, d.ParentRef, d.ParentVersion, at)
	if err != nil {
		return fail(err)
	}
	if err = contracts.ValidateGoalsPublicationRecoveryDelegation(root, *d, *req.Intent, at); err != nil {
		return fail(err)
	}
	idigest, err := req.Intent.Digest()
	if err != nil || req.ID != "goals-publication-recovery-request:"+idigest || req.IntentDigest != idigest || req.InstallationDigest != contracts.GoalsPublicationRoot || req.RequestedAuthority != contracts.GovernedPackagePublish || req.RequestedScope != req.Intent.Scope {
		return fail(errors.New("successor request binding mismatch"))
	}
	body, _, err := r.loadWorkPlanBlob(ctx, authorityDecisionNamespace, req.ID, req.Version, at)
	if err != nil {
		return fail(err)
	}
	var stored authorityDecisionRecord
	if err = json.Unmarshal(body, &stored); err != nil {
		return fail(err)
	}
	a, _ := json.Marshal(req)
	b, _ := json.Marshal(stored.Request)
	if string(a) != string(b) {
		return fail(errors.New("successor decision request mismatch"))
	}
	dec := stored.Decision
	if err = dec.Validate(req, at); err != nil {
		return fail(err)
	}
	rd, err := req.DigestAt(at)
	if err != nil {
		return fail(err)
	}
	if dec.RequestID != req.ID || dec.RequestVersion != req.Version || dec.RequestDigest != rd || dec.Outcome != contracts.AuthorityApprove || dec.DecidedBy != root.Principal || dec.AuthorityRef != root.Ref || dec.AuthorityVersion != root.Version || dec.AuthorityGenerationDigest != root.Digest || dec.GrantedScope != req.RequestedScope || dec.AuthorityDigest != contracts.AuthorityModelGoalsRecoveryDigest() || dec.ExpiresAt == nil || !at.Before(*dec.ExpiresAt) || dec.Delegation == nil {
		return fail(errors.New("successor owner decision is invalid"))
	}
	a, _ = json.Marshal(dec.Delegation)
	b, _ = json.Marshal(d)
	if string(a) != string(b) {
		return fail(errors.New("successor decision delegation mismatch"))
	}
	gen, err := r.LoadAuthorityGeneration(ctx, "authority-delegation:"+req.ID, "1", at)
	if err != nil {
		return fail(err)
	}
	want := contracts.AuthorityGeneration{Ref: "authority-delegation:" + req.ID, Version: "1", Principal: d.DelegatedPrincipal, Scope: d.RequestedScope, Authorities: []string{d.RequestedAuthority}, DelegationProfile: d.Profile, SubjectKind: d.SubjectKind, SubjectID: d.SubjectID, SubjectVersion: d.SubjectVersion, SubjectDigest: d.SubjectDigest, SubjectKeyDigest: d.SubjectKeyDigest, EffectiveAt: dec.IssuedAt.UTC(), ExpiresAt: &d.ExpiresAt, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedBy: dec.DecidedBy, DelegationRef: req.ID + "/" + req.Version, DelegationDigest: rd, PolicyRef: d.PolicyRef, PolicyVersion: d.PolicyVersion, PolicyDigest: d.PolicyDigest, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: d.PolicyVersion, AuthorityModelDigest: d.PolicyDigest, State: contracts.AuthorityGenerationActive, ProvenanceRef: "authority-decision:" + dec.DecisionRef + ":" + dec.DecisionVersion, ProvenanceDigest: dec.AuthorityDigest}
	want.Digest, err = want.ComputeDigest()
	if err != nil || !reflect.DeepEqual(gen, want) || gen.ExpiresAt == nil || !at.Before(*gen.ExpiresAt) || !dec.ExpiresAt.Equal(d.ExpiresAt) {
		return fail(fmt.Errorf("successor operational generation mismatch"))
	}
	for _, g := range []contracts.AuthorityGeneration{root, gen} {
		if err = r.CheckGoalsPublicationInvalidation(ctx, g.Ref, g.Version, req.ID, req.Version, current); err != nil {
			return fail(err)
		}
	}
	return contracts.PackagePublishAuthorization{Generation: gen, Request: req, Decision: dec}, nil
}

// RevalidateGoalsPublicationRecoveryAuthorizationTx re-confirms, using the
// caller's own open *sql.Tx (never a second pooled connection: this store's
// pool is capped at one connection, so any nested query against
// r.Store.DB() while the caller's transaction is open would deadlock), that
// a recovery authorization already fully resolved and validated by an
// earlier LoadGoalsPublicationRecoveryAuthorization call has not since
// expired or been invalidated (revoked, or its authority generation
// superseded/invalidated). It intentionally does not re-derive or
// re-decrypt the full authorization from scratch — that already happened —
// it only re-checks the exact expiry and invalidation gates that
// LoadGoalsPublicationRecoveryAuthorization itself enforces, using the same
// namespaces CheckGoalsPublicationInvalidation reads. Existence of a
// revocation record for the request is treated as disqualifying without
// decoding its effective time: for gating NEW durable execution admission
// (as opposed to reading historical evidence), any revocation record
// targeting this exact request is fail-closed regardless of its recorded
// effective instant.
//
// Callers must invoke this from inside the same transaction that will admit
// the durable command/event/effect, strictly before that insert, so that no
// concurrent write (revocation, invalidation, or anything else) can land
// between this check and the durable admission it guards.
func (r Repository) RevalidateGoalsPublicationRecoveryAuthorizationTx(ctx context.Context, tx *sql.Tx, auth contracts.PackagePublishAuthorization, now time.Time) error {
	if tx == nil {
		return errors.New("recovery authority re-check requires an open transaction")
	}
	if auth.Request.ID == "" || auth.Request.Version == "" || auth.Generation.Ref == "" || auth.Generation.Version == "" {
		return errors.New("recovery authority re-check requires a resolved authorization")
	}
	if auth.Decision.ExpiresAt == nil || !now.Before(*auth.Decision.ExpiresAt) {
		return errors.New("recovery authority decision has expired before durable admission")
	}
	if auth.Generation.ExpiresAt == nil || !now.Before(*auth.Generation.ExpiresAt) {
		return errors.New("recovery authority delegated generation has expired before durable admission")
	}
	exists := func(namespace, objectID, objectVersion string) (bool, error) {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, objectID, objectVersion).Scan(&n); err != nil {
			return false, fmt.Errorf("re-check %s record: %w", namespace, err)
		}
		return n != 0, nil
	}
	if revoked, err := exists(authorityRevocationNamespace, auth.Request.ID, auth.Request.Version); err != nil {
		return err
	} else if revoked {
		return errors.New("recovery authority decision was revoked before durable admission")
	}
	if auth.Generation.ParentRef != "" {
		if invalidated, err := exists(authorityGenerationInvalidationNamespace, auth.Generation.ParentRef, auth.Generation.ParentVersion); err != nil {
			return err
		} else if invalidated {
			return errors.New("recovery root authority generation was invalidated before durable admission")
		}
	}
	if invalidated, err := exists(authorityGenerationInvalidationNamespace, auth.Generation.Ref, auth.Generation.Version); err != nil {
		return err
	} else if invalidated {
		return errors.New("recovery delegated authority generation was invalidated before durable admission")
	}
	return nil
}
