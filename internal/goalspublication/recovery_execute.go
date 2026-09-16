package goalspublication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/effect"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var recoverySteps = []string{"manifest", "archive", "signature", "verify-draft", "publish", "verify-published"}

type RecoveryExecution struct {
	Repository goalstore.Repository
	Adapter    Adapter
	Assets     Assets
	Now        func() time.Time
}

func (e RecoveryExecution) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// PrepareIntent is read-only with respect to the external repository. It binds
// the durable abandonment and checks the exact currently established GitHub
// state before a canonical successor request can be saved.
func (e RecoveryExecution) PrepareIntent(ctx context.Context, predecessorRequestID, identity string, expires time.Time) (contracts.ActionIntent, error) {
	if err := e.Assets.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := VerifySigning(ctx, e.Repository, e.Assets, e.now()); err != nil {
		return contracts.ActionIntent{}, err
	}
	key := executionKey(predecessorRequestID)
	var eventID string
	var body []byte
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?`, key).Scan(&eventID, &body); err != nil {
		return contracts.ActionIntent{}, err
	}
	var p abandonmentPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return contracts.ActionIntent{}, err
	}
	if p.RequestID != predecessorRequestID || hash(body) == "" {
		return contracts.ActionIntent{}, errors.New("predecessor abandonment mismatch")
	}
	remote, ok := e.Adapter.(RecoveryGitHub)
	if !ok {
		if value, yes := e.Adapter.(*RecoveryGitHub); yes && value != nil {
			remote = *value
		} else {
			return contracts.ActionIntent{}, errors.New("fixed read-only successor GitHub adapter unavailable")
		}
	}
	id, err := remote.Identity(ctx)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	oldIntentDigest := p.IntentDigest
	a, err := contracts.NewGoalsPublicationRecoveryIntent(contracts.GoalsRecoveryInput{CreatedAt: e.now(), ExpiresAt: expires, Identity: identity, AccountID: id.AccountID, Sizes: [3]int64{int64(len(e.Assets[0])), int64(len(e.Assets[1])), int64(len(e.Assets[2]))}, PredecessorRequestID: p.RequestID, PredecessorRequestDigest: p.RequestDigest, PredecessorIntentID: p.IntentID, PredecessorIntentDigest: oldIntentDigest, AbandonmentEventID: eventID, AbandonmentDigest: hash(body)})
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if _, err = e.predecessor(ctx, a); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err = remote.Check(ctx, a, "manifest", nil); err != nil {
		return contracts.ActionIntent{}, err
	}
	return a, nil
}

// PrepareChainedIntent constructs the one permitted successor after an
// abandoned recovery successor. The prior generation is bound explicitly;
// no prior authority or effect is reused.
func (e RecoveryExecution) PrepareChainedIntent(ctx context.Context, priorRecoveryRequestID, identity string, expires time.Time) (contracts.ActionIntent, error) {
	if err := e.Assets.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := VerifySigning(ctx, e.Repository, e.Assets, e.now()); err != nil {
		return contracts.ActionIntent{}, err
	}
	if !strings.HasPrefix(priorRecoveryRequestID, "goals-publication-recovery-request:") {
		return contracts.ActionIntent{}, errors.New("not a recovery request")
	}
	key := recoveryKey(priorRecoveryRequestID)
	var eventID string
	var body []byte
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication-recovery.abandoned' AND correlation_id=?`, key).Scan(&eventID, &body); err != nil {
		return contracts.ActionIntent{}, err
	}
	var abandoned recoveryAbandonmentPayload
	if err := json.Unmarshal(body, &abandoned); err != nil {
		return contracts.ActionIntent{}, err
	}
	if abandoned.RequestID != priorRecoveryRequestID || abandoned.ExecutionID != key || abandoned.ExecutionStatus != "abandoned" || abandoned.CompletionEstablished || len(abandoned.Effects) == 0 || abandoned.Effects[0].State != string(state.EffectUnknown) {
		return contracts.ActionIntent{}, errors.New("prior recovery abandonment is not exact and terminal")
	}
	manifestPayload, err := loadRecoveryManifestPayload(ctx, e.Repository.Store.DB(), key+":manifest")
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	var sp recoveryStepPayload
	if err := json.Unmarshal(manifestPayload, &sp); err != nil || sp.RequestID != priorRecoveryRequestID || sp.Step != "manifest" {
		return contracts.ActionIntent{}, errors.New("prior recovery manifest lineage mismatch")
	}
	priorIntentDigest, err := sp.Intent.Digest()
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if priorIntentDigest != abandoned.IntentDigest || sp.Intent.ID != abandoned.IntentID || sp.Authority.Generation.Digest != abandoned.AuthorityDigest {
		return contracts.ActionIntent{}, errors.New("prior recovery authority lineage mismatch")
	}
	if err := contracts.ValidateGoalsPublicationRecoveryIntent(sp.Intent); err != nil {
		return contracts.ActionIntent{}, err
	}
	if _, err := e.predecessor(ctx, sp.Intent); err != nil {
		return contracts.ActionIntent{}, err
	}
	remote, ok := e.Adapter.(RecoveryGitHub)
	if !ok {
		return contracts.ActionIntent{}, errors.New("fixed read-only successor GitHub adapter unavailable")
	}
	id, err := remote.Identity(ctx)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	a, err := contracts.NewGoalsPublicationChainedRecoveryIntent(contracts.GoalsChainedRecoveryInput{GoalsRecoveryInput: contracts.GoalsRecoveryInput{CreatedAt: e.now(), ExpiresAt: expires, Identity: identity, AccountID: id.AccountID, Sizes: [3]int64{int64(len(e.Assets[0])), int64(len(e.Assets[1])), int64(len(e.Assets[2]))}, PredecessorRequestID: sp.Intent.Parameters["predecessor_request_id"], PredecessorRequestDigest: sp.Intent.Parameters["predecessor_request_digest"], PredecessorIntentID: sp.Intent.Parameters["predecessor_intent_id"], PredecessorIntentDigest: sp.Intent.Parameters["predecessor_intent_digest"], AbandonmentEventID: sp.Intent.Parameters["abandonment_event_id"], AbandonmentDigest: sp.Intent.Parameters["abandonment_digest"]}, PriorRecoveryRequestID: priorRecoveryRequestID, PriorRecoveryRequestDigest: abandoned.RequestDigest, PriorRecoveryIntentID: abandoned.IntentID, PriorRecoveryIntentDigest: abandoned.IntentDigest, PriorRecoveryAuthorityDigest: abandoned.AuthorityDigest, PriorRecoveryExecutionID: abandoned.ExecutionID, PriorRecoveryAbandonmentEventID: eventID, PriorRecoveryAbandonmentDigest: hash(body), PriorRecoveryManifestEffectID: key + ":manifest", PriorRecoveryManifestState: abandoned.Effects[0].State, PriorRecoveryManifestAttempts: abandoned.Effects[0].Attempts, PriorRecoveryManifestRequestDigest: abandoned.Effects[0].Request, PriorRecoveryManifestResultDigest: abandoned.Effects[0].Result, PriorRecoveryManifestReconciliationDigest: abandoned.Effects[0].Reconciliation})
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := e.Repository.ValidateGoalsPublicationChainedRecoveryBinding(ctx, a); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := remote.Check(ctx, a, "manifest", nil); err != nil {
		return contracts.ActionIntent{}, err
	}
	return a, nil
}

// PrepareOrderedIntent reconstructs the complete finite abandoned recovery
// chain and creates a fresh ordered-chain successor. It never writes Praxis or
// performs a provider mutation.
func (e RecoveryExecution) PrepareOrderedIntent(ctx context.Context, latestRecoveryRequestID, identity string, expires time.Time) (contracts.ActionIntent, error) {
	if err := e.Assets.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := VerifySigning(ctx, e.Repository, e.Assets, e.now()); err != nil {
		return contracts.ActionIntent{}, err
	}
	if !strings.HasPrefix(latestRecoveryRequestID, "goals-publication-recovery-request:") {
		return contracts.ActionIntent{}, errors.New("not a recovery request")
	}
	type loaded struct {
		id, event string
		body      []byte
		p         recoveryAbandonmentPayload
		sp        struct {
			RequestID, Step string
			Intent          contracts.ActionIntent
		}
	}
	var rev []loaded
	next := latestRecoveryRequestID
	for {
		key := recoveryKey(next)
		var eventID string
		var body []byte
		if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication-recovery.abandoned' AND correlation_id=?`, key).Scan(&eventID, &body); err != nil {
			return contracts.ActionIntent{}, err
		}
		var ap recoveryAbandonmentPayload
		if err := json.Unmarshal(body, &ap); err != nil {
			return contracts.ActionIntent{}, err
		}
		manifest, err := loadRecoveryManifestPayload(ctx, e.Repository.Store.DB(), key+":manifest")
		if err != nil {
			return contracts.ActionIntent{}, err
		}
		var sp struct {
			RequestID, Step string
			Intent          contracts.ActionIntent
		}
		if err := json.Unmarshal(manifest, &sp); err != nil || sp.RequestID != next || sp.Step != "manifest" {
			return contracts.ActionIntent{}, errors.New("ordered recovery manifest lineage mismatch")
		}
		if err := contracts.ValidateGoalsPublicationRecoveryIntent(sp.Intent); err != nil {
			return contracts.ActionIntent{}, err
		}
		if ap.ExecutionID != key || ap.ExecutionStatus != "abandoned" || ap.CompletionEstablished || len(ap.Effects) == 0 || ap.Effects[0].State != "unknown" || ap.Effects[0].Attempts != 1 {
			return contracts.ActionIntent{}, errors.New("ordered recovery abandonment state invalid")
		}
		rev = append(rev, loaded{id: next, event: eventID, body: body, p: ap, sp: sp})
		if sp.Intent.Parameters["contract"] != contracts.GoalsChainedRecoveryContract {
			break
		}
		next = sp.Intent.Parameters["prior_recovery_request_id"]
		if next == "" {
			return contracts.ActionIntent{}, errors.New("ordered recovery chain predecessor missing")
		}
	}
	chain := make([]contracts.RecoveryGenerationBinding, len(rev))
	for i := range rev {
		x := rev[len(rev)-1-i]
		d, _ := x.sp.Intent.Digest()
		chain[i] = contracts.RecoveryGenerationBinding{RequestID: x.p.RequestID, RequestDigest: x.p.RequestDigest, IntentID: x.p.IntentID, IntentDigest: d, AuthorityDigest: x.p.AuthorityDigest, ExecutionID: x.p.ExecutionID, AbandonmentEventID: x.event, AbandonmentDigest: hash(x.body), ManifestEffectID: x.p.Effects[0].ID, ManifestState: x.p.Effects[0].State, ManifestAttempts: x.p.Effects[0].Attempts, ManifestRequestDigest: x.p.Effects[0].Request, ManifestResultDigest: x.p.Effects[0].Result, ManifestReconciliationDigest: x.p.Effects[0].Reconciliation}
	}
	for i := 1; i < len(chain); i++ {
		if rev[len(rev)-1-i].sp.Intent.Parameters["prior_recovery_request_id"] != chain[i-1].RequestID {
			return contracts.ActionIntent{}, errors.New("ordered recovery chain ordering mismatch")
		}
	}
	root := rev[len(rev)-1].sp.Intent
	if _, err := e.predecessor(ctx, root); err != nil {
		return contracts.ActionIntent{}, err
	}
	remote, ok := e.Adapter.(RecoveryGitHub)
	if !ok {
		return contracts.ActionIntent{}, errors.New("fixed read-only successor GitHub adapter unavailable")
	}
	id, err := remote.Identity(ctx)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	a, err := contracts.NewGoalsPublicationOrderedRecoveryIntent(contracts.GoalsOrderedRecoveryInput{GoalsRecoveryInput: contracts.GoalsRecoveryInput{CreatedAt: e.now(), ExpiresAt: expires, Identity: identity, AccountID: id.AccountID, Sizes: [3]int64{int64(len(e.Assets[0])), int64(len(e.Assets[1])), int64(len(e.Assets[2]))}, PredecessorRequestID: root.Parameters["predecessor_request_id"], PredecessorRequestDigest: root.Parameters["predecessor_request_digest"], PredecessorIntentID: root.Parameters["predecessor_intent_id"], PredecessorIntentDigest: root.Parameters["predecessor_intent_digest"], AbandonmentEventID: root.Parameters["abandonment_event_id"], AbandonmentDigest: root.Parameters["abandonment_digest"]}, Chain: chain})
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := e.Repository.ValidateGoalsPublicationOrderedRecoveryBinding(ctx, a); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := remote.Check(ctx, a, "manifest", nil); err != nil {
		return contracts.ActionIntent{}, err
	}
	return a, nil
}

// PrepareFailedVerificationIntent creates the bounded /4 successor after a
// terminal local draft-verification failure. It binds existing effects and
// performs only read-only provider checks; uploads are never scheduled.
func (e RecoveryExecution) PrepareFailedVerificationIntent(ctx context.Context, failedRequestID, identity string, expires time.Time) (contracts.ActionIntent, error) {
	if !strings.HasPrefix(failedRequestID, "goals-publication-recovery-request:") {
		return contracts.ActionIntent{}, errors.New("not a recovery request")
	}
	key := recoveryKey(failedRequestID)
	load := func(step string) (storedEffect, recoveryStepPayload, error) {
		v, err := e.load(ctx, key+":"+step)
		if err != nil {
			return v, recoveryStepPayload{}, err
		}
		var p recoveryStepPayload
		if err = json.Unmarshal(v.Payload, &p); err != nil {
			return v, p, err
		}
		return v, p, nil
	}
	manifest, sp, err := load("manifest")
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	archive, _, err := load("archive")
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	signature, _, err := load("signature")
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	verify, _, err := load("verify-draft")
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if manifest.State != string(state.EffectSucceeded) || archive.State != string(state.EffectSucceeded) || signature.State != string(state.EffectSucceeded) || verify.State != string(state.EffectFailed) {
		return contracts.ActionIntent{}, errors.New("failed verification predecessor state is not exact")
	}
	if sp.RequestID != failedRequestID || sp.Intent.Parameters["contract"] != contracts.GoalsOrderedRecoveryContract {
		return contracts.ActionIntent{}, errors.New("failed predecessor intent mismatch")
	}
	var requestDigest string
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT object_digest FROM secure_blobs WHERE namespace='authority_request' AND object_id=? AND object_version='1'`, failedRequestID).Scan(&requestDigest); err != nil {
		return contracts.ActionIntent{}, err
	}
	historical, err := e.Repository.LoadExpiredHistoricalAuthorityEvidence(ctx, failedRequestID, "1", requestDigest, contracts.GoalsPublicationRoot, key, []string{key + ":manifest", key + ":archive", key + ":signature", key + ":verify-draft"}, e.now())
	if err != nil {
		return contracts.ActionIntent{}, fmt.Errorf("load expired predecessor authority evidence: %w", err)
	}
	if historical.GenerationDigest != sp.Authority.Generation.Digest || historical.IntentID != sp.Intent.ID {
		return contracts.ActionIntent{}, errors.New("failed predecessor authority mismatch")
	}
	var verifyRec []byte
	if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT reconciliation_evidence FROM effects WHERE effect_id=?`, key+":verify-draft").Scan(&verifyRec); err != nil {
		return contracts.ActionIntent{}, err
	}
	chain, err := contracts.ParseRecoveryChain(sp.Intent.Parameters["recovery_chain"])
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	id, err := e.Adapter.(RecoveryGitHub).Identity(ctx)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	assetID := func(v storedEffect) string {
		var o Observation
		_ = json.Unmarshal(v.Result, &o)
		if o.Asset != nil {
			return fmt.Sprint(o.Asset.ID)
		}
		return ""
	}
	failedIntentDigest, err := sp.Intent.Digest()
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	in := contracts.GoalsFailedVerificationInput{GoalsOrderedRecoveryInput: contracts.GoalsOrderedRecoveryInput{GoalsRecoveryInput: contracts.GoalsRecoveryInput{CreatedAt: e.now(), ExpiresAt: expires, Identity: identity, AccountID: id.AccountID, Sizes: [3]int64{int64(len(e.Assets[0])), int64(len(e.Assets[1])), int64(len(e.Assets[2]))}, PredecessorRequestID: sp.Intent.Parameters["predecessor_request_id"], PredecessorRequestDigest: sp.Intent.Parameters["predecessor_request_digest"], PredecessorIntentID: sp.Intent.Parameters["predecessor_intent_id"], PredecessorIntentDigest: sp.Intent.Parameters["predecessor_intent_digest"], AbandonmentEventID: sp.Intent.Parameters["abandonment_event_id"], AbandonmentDigest: sp.Intent.Parameters["abandonment_digest"]}, Chain: chain}, FailedRequestID: failedRequestID, FailedRequestDigest: requestDigest, FailedIntentID: sp.Intent.ID, FailedIntentDigest: failedIntentDigest, FailedAuthorityDigest: sp.Authority.Generation.Digest, FailedHistoricalAuthorityDigest: historical.Digest, FailedExecutionID: key, FailedManifestEffectID: key + ":manifest", FailedArchiveEffectID: key + ":archive", FailedSignatureEffectID: key + ":signature", FailedVerifyEffectID: key + ":verify-draft", FailedManifestState: manifest.State, FailedArchiveState: archive.State, FailedSignatureState: signature.State, FailedVerifyState: verify.State, FailedVerifyAttempts: 1, FailedManifestRequestDigest: hash(manifest.Payload), FailedArchiveRequestDigest: hash(archive.Payload), FailedSignatureRequestDigest: hash(signature.Payload), FailedVerifyRequestDigest: hash(verify.Payload), FailedVerifyResultDigest: hash(verify.Result), FailedVerifyReconciliationDigest: hash(verifyRec), AssetIDs: [3]string{assetID(manifest), assetID(archive), assetID(signature)}}
	if in.AssetIDs[0] == "" || in.AssetIDs[1] == "" || in.AssetIDs[2] == "" {
		return contracts.ActionIntent{}, errors.New("successful asset evidence missing")
	}
	a, err := contracts.NewGoalsPublicationFailedVerificationIntent(in)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if err = e.Repository.ValidateGoalsPublicationFailedVerificationBinding(ctx, a); err != nil {
		return contracts.ActionIntent{}, err
	}
	remote, ok := e.Adapter.(RecoveryGitHub)
	if !ok {
		return contracts.ActionIntent{}, errors.New("recovery GitHub adapter unavailable")
	}
	if err = remote.Check(ctx, a, "verify-draft", nil); err != nil {
		return contracts.ActionIntent{}, err
	}
	return a, nil
}
func recoveryKey(id string) string { return "goals-publication-recovery:" + hash([]byte(id)) }

type recoveryStepPayload struct {
	Version, RequestID, Step string
	Intent                   contracts.ActionIntent
	Authority                contracts.PackagePublishAuthorization
	PredecessorAbandonment   string
}

func (e RecoveryExecution) load(ctx context.Context, id string) (storedEffect, error) {
	var v storedEffect
	var at string
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT e.state,e.request_payload,e.observed_result,e.created_at FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id AND v.command_id=c.command_id WHERE e.effect_id=? AND e.command_id=e.effect_id AND c.command_type='goals-publication-recovery.step' AND c.command_version='1' AND v.event_type='goals-publication-recovery.step-admitted' AND v.event_version='1' AND c.payload=e.request_payload AND v.payload=e.request_payload AND e.target_adapter='goals-recovery-github' AND ((e.state='pending' AND e.attempts=0) OR (e.state!='pending' AND e.attempts=1))`, id).Scan(&v.State, &v.Payload, &v.Result, &at)
	if err != nil {
		return v, err
	}
	v.Created, err = time.Parse(time.RFC3339Nano, at)
	return v, err
}
func (e RecoveryExecution) predecessor(ctx context.Context, a contracts.ActionIntent) (abandonmentPayload, error) {
	var p abandonmentPayload
	id := a.Parameters["abandonment_event_id"]
	var b []byte
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND event_type='goals-publication.abandoned'`, id).Scan(&b); err != nil {
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	d := hash(b)
	if d != a.Parameters["abandonment_digest"] || p.ExecutionID != executionKey(p.RequestID) || p.ExecutionStatus != "abandoned" || p.CompletionEstablished || p.RequestID != a.Parameters["predecessor_request_id"] || p.IntentID != a.Parameters["predecessor_intent_id"] || p.IntentDigest != a.Parameters["predecessor_intent_digest"] || len(p.Effects) != 3 || p.Effects[2].State != string(state.EffectUnknown) {
		return p, errors.New("successor predecessor abandonment lineage mismatch")
	}
	for i, step := range []string{"refs", "draft", "manifest"} {
		id := p.ExecutionID + ":" + step
		var st string
		var req, obs, rec []byte
		if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT state,request_payload,observed_result,reconciliation_evidence FROM effects WHERE effect_id=?`, id).Scan(&st, &req, &obs, &rec); err != nil {
			return p, err
		}
		x := p.Effects[i]
		if x.ID != id || x.State != st || x.Request != hash(req) || x.Result != hash(obs) || x.Reconciliation != hash(rec) {
			return p, errors.New("predecessor evidence changed after abandonment")
		}
	}
	var done int
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id=?`, p.ExecutionID+":completed").Scan(&done); err != nil || done != 0 {
		return p, errors.New("predecessor completion appeared after abandonment")
	}
	return p, nil
}
func (e RecoveryExecution) current(ctx context.Context, id string) (contracts.PackagePublishAuthorization, error) {
	now := e.now()
	m, err := e.Repository.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return contracts.PackagePublishAuthorization{}, err
	}
	if m.ActiveVersion != contracts.AuthorityModelGoalsRecoveryVersion || m.ActiveDigest != contracts.AuthorityModelGoalsRecoveryDigest() || m.State != "committed" {
		return contracts.PackagePublishAuthorization{}, errors.New("v5 recovery authority model not adopted")
	}
	return e.Repository.LoadGoalsPublicationRecoveryAuthorization(ctx, id, now, now)
}
func (e RecoveryExecution) Execute(ctx context.Context, requestID string) (string, error) {
	if e.Repository.Store == nil || e.Adapter == nil {
		return "", errors.New("successor execution dependencies missing")
	}
	if !strings.HasPrefix(requestID, "goals-publication-recovery-request:") {
		return "", errors.New("not a Goals recovery request")
	}
	if stopped, err := e.recoveryAbandoned(ctx, requestID); err != nil {
		return "", err
	} else if stopped {
		return "", errors.New("abandoned Goals recovery execution is permanently fenced")
	}
	now := e.now()
	auth, err := e.current(ctx, requestID)
	if err != nil {
		return "", err
	}
	if auth.Request.Intent == nil {
		return "", errors.New("successor request has no protected intent")
	}
	intent := *auth.Request.Intent
	if err = contracts.ValidateGoalsPublicationRecoveryIntent(intent); err != nil {
		return "", err
	}
	if err = e.Assets.MatchRecovery(intent); err != nil {
		return "", err
	}
	if err = VerifySigning(ctx, e.Repository, e.Assets, now); err != nil {
		return "", err
	}
	pred, err := e.predecessor(ctx, intent)
	if err != nil {
		return "", err
	}
	key := recoveryKey(requestID)
	steps := recoverySteps
	if intent.Parameters["contract"] == contracts.GoalsFailedVerificationContract {
		steps = []string{"verify-draft", "publish", "verify-published"}
	}
	previous := []Observation{}
	for i, step := range steps {
		if stopped, err := e.recoveryAbandoned(ctx, requestID); err != nil {
			return "", err
		} else if stopped {
			return "", errors.New("abandoned Goals recovery execution is permanently fenced")
		}
		id := key + ":" + step
		v, loadErr := e.load(ctx, id)
		if errors.Is(loadErr, sql.ErrNoRows) {
			auth, err = e.current(ctx, requestID)
			if err != nil {
				return "", err
			}
			intent = *auth.Request.Intent
			if _, err = e.predecessor(ctx, intent); err != nil {
				return "", err
			}
			payload := mustJSON(recoveryStepPayload{Version: "1", RequestID: requestID, Step: step, Intent: intent, Authority: auth, PredecessorAbandonment: intent.Parameters["abandonment_digest"]})
			dig, _ := intent.Digest()
			at := e.now()
			cmd := state.CommandRecord{ID: id, Type: "goals-publication-recovery.step", Version: "1", Actor: intent.Actor, Scope: intent.Scope, CorrelationID: key, Payload: payload, CreatedAt: at}
			ev := state.EventRecord{ID: id, AggregateID: key, AggregateType: "goals-publication-recovery", AggregateVersion: int64(i + 1), Type: "goals-publication-recovery.step-admitted", Version: "1", Actor: intent.Actor, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: payload, CreatedAt: at}
			ef := state.EffectRecord{ID: id, CommandID: id, ActionIntentDigest: dig, TargetAdapter: "goals-recovery-github", TargetPrincipal: intent.Actor.ID, PreconditionsJSON: mustJSON(intent.Preconditions), CryptoProfile: intent.CryptoProfile, State: string(state.EffectPending), RequestPayload: payload, CreatedAt: at, UpdatedAt: at}
			if err = e.Repository.Store.CommitTransition(ctx, cmd, int64(i), ev, "", "", &ef); err != nil {
				return "", err
			}
			v, loadErr = e.load(ctx, id)
		}
		if loadErr != nil {
			return "", loadErr
		}
		var sp recoveryStepPayload
		if err = json.Unmarshal(v.Payload, &sp); err != nil {
			return "", err
		}
		if sp.RequestID != requestID || sp.Step != step || sp.Version != "1" || sp.PredecessorAbandonment != intent.Parameters["abandonment_digest"] || sp.Intent.ID != intent.ID || !sameRecoveryAuthorization(sp.Authority, auth) {
			return "", errors.New("successor step payload/authority mismatch")
		}
		if v.State == string(state.EffectSucceeded) {
			var o Observation
			if err = json.Unmarshal(v.Result, &o); err != nil {
				return "", err
			}
			if err = validateRecoveryObservation(intent, step, o, previous); err != nil {
				return "", err
			}
			previous = append(previous, o)
			continue
		}
		if v.State != string(state.EffectPending) {
			if v.State == string(state.EffectUnknown) || v.State == string(state.EffectDispatched) || v.State == string(state.EffectReconciling) {
				return "", e.Reconcile(ctx, requestID)
			}
			return "", errors.New("successor effect is terminally failed; no retry")
		}
		boundary := recoveryBoundary{execution: e, requestID: requestID, intent: intent, step: step, previous: previous}
		dig, _ := intent.Digest()
		res, err := e.Repository.Store.DB().ExecContext(ctx, `UPDATE effects SET state='dispatched',attempts=attempts+1,updated_at=? WHERE effect_id=? AND state='pending'`, e.now().Format(time.RFC3339Nano), id)
		if err != nil {
			return "", err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return "", errors.New("successor effect already claimed")
		}
		result, err := (effect.Coordinator{Revalidator: boundary, Preconditions: boundary, Dispatcher: boundary}).Commit(ctx, intent, effect.AuthorizationSnapshot{IntentDigest: dig}, "")
		if err != nil {
			var evidence []byte
			var failure dispatchFailure
			outcome := state.EffectUnknown
			if errors.As(err, &failure) {
				evidence = failure.Evidence()
				if failure.outcome.Class == "local_pre_dispatch_failure" {
					outcome = state.EffectFailed
				}
			}
			saveErr := e.Repository.Store.MarkEffectOutcome(ctx, id, outcome, evidence, nil, e.now())
			return "", errors.Join(err, saveErr)
		}
		var o Observation
		if err = json.Unmarshal(result.Evidence, &o); err != nil {
			return "", err
		}
		if err = validateRecoveryObservation(intent, step, o, previous); err != nil {
			saveErr := e.Repository.Store.MarkEffectOutcome(ctx, id, state.EffectUnknown, result.Evidence, nil, e.now())
			return "", errors.Join(err, saveErr)
		}
		if err = e.Repository.Store.MarkEffectOutcome(ctx, id, state.EffectSucceeded, result.Evidence, nil, e.now()); err != nil {
			return "", err
		}
		previous = append(previous, o)
	}
	if _, err = e.predecessor(ctx, intent); err != nil {
		return "", err
	}
	if stopped, err := e.recoveryAbandoned(ctx, requestID); err != nil {
		return "", err
	} else if stopped {
		return "", errors.New("abandoned Goals recovery execution is permanently fenced")
	}
	effectHashes := make([]string, 0, len(steps))
	for _, step := range steps {
		v, err := e.load(ctx, key+":"+step)
		if err != nil {
			return "", err
		}
		if v.State != string(state.EffectSucceeded) {
			return "", errors.New("successor incomplete")
		}
		effectHashes = append(effectHashes, hash(v.Payload)+"/"+hash(v.Result))
	}
	auth, err = e.current(ctx, requestID)
	if err != nil {
		return "", err
	}
	completion := key + ":completed"
	payload := mustJSON(struct {
		Version, RequestID, IntentID, IntentDigest, AuthorityGeneration, SigningProvenance, PredecessorAbandonment, PredecessorManifestOutcome string
		Effects                                                                                                                                []string
		Final                                                                                                                                  Observation
	}{"1", requestID, intent.ID, mustDigestValue(intent), auth.Generation.Digest, contracts.GoalsPublicationSigningReceipt, intent.Parameters["abandonment_digest"], "unknown-unresolved", effectHashes, previous[len(previous)-1]})
	at := e.now()
	cmd := state.CommandRecord{ID: completion, Type: "goals-publication-recovery.complete", Version: "1", Actor: intent.Actor, Scope: intent.Scope, CorrelationID: key, Payload: payload, CreatedAt: at}
	ev := state.EventRecord{ID: completion, AggregateID: key, AggregateType: "goals-publication-recovery", AggregateVersion: int64(len(steps) + 1), Type: "goals-publication-recovery.completed", Version: "1", Actor: intent.Actor, CommandID: completion, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: payload, CreatedAt: at}
	var existing []byte
	if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, completion).Scan(&existing); err == nil {
		if string(existing) != string(payload) {
			return "", errors.New("successor completion evidence conflicts")
		}
		return completion, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err = e.Repository.Store.CommitTransition(ctx, cmd, int64(len(steps)), ev, "", "", nil); err != nil {
		return "", err
	}
	_ = pred
	return completion, nil
}
func mustDigestValue(a contracts.ActionIntent) string { d, _ := a.Digest(); return d }
func sameRecoveryAuthorization(a, b contracts.PackagePublishAuthorization) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

type recoveryBoundary struct {
	execution RecoveryExecution
	requestID string
	intent    contracts.ActionIntent
	step      string
	previous  []Observation
}

func (b recoveryBoundary) Revalidate(ctx context.Context, a contracts.ActionIntent, s effect.AuthorizationSnapshot) error {
	if err := VerifySigning(ctx, b.execution.Repository, b.execution.Assets, b.execution.now()); err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	if _, err := b.execution.predecessor(ctx, a); err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	auth, err := b.execution.current(ctx, b.requestID)
	if err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	dig, _ := auth.Request.Intent.Digest()
	got, _ := a.Digest()
	if dig != got || dig != s.IntentDigest {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: errors.New("fresh successor authority changed")}
	}
	return nil
}
func (b recoveryBoundary) Check(ctx context.Context, a contracts.ActionIntent) error {
	if err := b.execution.Adapter.Check(ctx, a, b.step, b.previous); err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	return nil
}
func (b recoveryBoundary) Dispatch(ctx context.Context, a contracts.ActionIntent, _ string) (effect.Result, error) {
	dig, _ := a.Digest()
	if err := (recoveryBoundary{execution: b.execution, requestID: b.requestID, intent: a, step: b.step, previous: b.previous}).Revalidate(ctx, a, effect.AuthorizationSnapshot{IntentDigest: dig}); err != nil {
		return effect.Result{}, err
	}
	o, err := b.execution.Adapter.Dispatch(ctx, a, b.step, b.previous, b.execution.Assets)
	if err != nil {
		return effect.Result{}, err
	}
	return effect.Result{ObservedState: "observed", Evidence: mustJSON(o)}, nil
}
func validateRecoveryObservation(a contracts.ActionIntent, step string, o Observation, previous []Observation) error {
	if fmt.Sprint(o.RepositoryID) != a.Parameters["repository_id"] || fmt.Sprint(o.OwnerID) != a.Parameters["owner_id"] || fmt.Sprint(o.AccountID) != a.Parameters["account_id"] || fmt.Sprint(o.Commit) != a.Parameters["commit"] || fmt.Sprint(o.Tree) != a.Parameters["tree"] {
		return errors.New("successor observation identity mismatch")
	}
	if step == "manifest" || step == "archive" || step == "signature" {
		i := map[string]int{"manifest": 0, "archive": 1, "signature": 2}[step]
		if o.Asset == nil || o.Asset.Name != assetNames[i] || fmt.Sprint(o.Asset.Size) != a.Parameters[[]string{"manifest_size", "archive_size", "signature_size"}[i]] || o.Asset.State != "uploaded" || o.Asset.ID <= 0 || fmt.Sprint(o.Asset.Uploader.ID) != a.Parameters["account_id"] {
			return errors.New("successor asset observation mismatch")
		}
		return nil
	}
	if o.Release == nil || fmt.Sprint(o.Release.ID) != a.Parameters["release_id"] || o.Release.Tag != a.Parameters["tag"] || o.Release.Target != a.Parameters["commit"] {
		return errors.New("successor release observation mismatch")
	}
	wantDraft := step == "verify-draft"
	if o.Release.Draft != wantDraft || o.Release.Prerelease {
		return errors.New("successor release state mismatch")
	}
	if step == "verify-draft" || step == "verify-published" {
		if len(o.AssetDigests) != 3 || o.AssetDigests[0] != contracts.GoalsPublicationManifest || o.AssetDigests[1] != contracts.GoalsPublicationArchive || o.AssetDigests[2] != contracts.GoalsPublicationSignature {
			return errors.New("successor asset read-back mismatch")
		}
	}
	_ = previous
	return nil
}

// Reconcile appends inspection evidence for the one uncertain successor
// effect; it never changes that effect state or dispatches another mutation.
func (e RecoveryExecution) Reconcile(ctx context.Context, requestID string) error {
	if stopped, err := e.recoveryAbandoned(ctx, requestID); err != nil {
		return err
	} else if stopped {
		return errors.New("abandoned Goals recovery execution is permanently fenced")
	}
	key := recoveryKey(requestID)
	previous := []Observation{}
	for _, step := range recoverySteps {
		v, err := e.load(ctx, key+":"+step)
		if err != nil {
			return err
		}
		if v.State == string(state.EffectUnknown) || v.State == string(state.EffectDispatched) {
			var p recoveryStepPayload
			if err = json.Unmarshal(v.Payload, &p); err != nil {
				return err
			}
			if _, err = e.predecessor(ctx, p.Intent); err != nil {
				return err
			}
			o, inspectErr := e.Adapter.Reconcile(ctx, p.Intent, step, previous)
			reason := "matching state does not attribute the dispatch"
			if inspectErr != nil {
				reason = inspectErr.Error()
			}
			id := key + ":" + step + ":reconciliation:" + e.now().Format("20060102T150405.000000000Z")
			body := mustJSON(struct {
				EffectID, Outcome, Reason string
				Observation               Observation
			}{key + ":" + step, "unresolved", reason, o})
			at := e.now()
			cmd := state.CommandRecord{ID: id, Type: "goals-publication-recovery.reconcile", Version: "1", Actor: p.Intent.Actor, Scope: p.Intent.Scope, CorrelationID: key, Payload: body, CreatedAt: at}
			ev := state.EventRecord{ID: id, AggregateID: id, AggregateType: "goals-publication-recovery-reconciliation", AggregateVersion: 1, Type: "goals-publication-recovery.reconciled", Version: "1", Actor: p.Intent.Actor, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: body, CreatedAt: at}
			if err = e.Repository.Store.CommitTransition(ctx, cmd, 0, ev, "", "", nil); err != nil {
				return err
			}
			return errors.New("successor effect remains unresolved")
		}
		if v.State == string(state.EffectSucceeded) {
			var p recoveryStepPayload
			var o Observation
			if err = json.Unmarshal(v.Payload, &p); err != nil {
				return err
			}
			if err = json.Unmarshal(v.Result, &o); err != nil {
				return err
			}
			if err = validateRecoveryObservation(p.Intent, step, o, previous); err != nil {
				return err
			}
			previous = append(previous, o)
		}
	}
	return errors.New("no uncertain successor effect")
}
