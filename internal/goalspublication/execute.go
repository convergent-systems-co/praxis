package goalspublication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/effect"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var steps = []string{"refs", "draft", "manifest", "archive", "signature", "verify", "publish", "verify-published"}

type Adapter interface {
	Check(context.Context, contracts.ActionIntent, string, []Observation) error
	Dispatch(context.Context, contracts.ActionIntent, string, []Observation, Assets) (Observation, error)
	Reconcile(context.Context, contracts.ActionIntent, string, []Observation) (Observation, error)
}

// Execution is a fixed-operation handler, not a new workflow engine.
type Execution struct {
	Repository goalstore.Repository
	Adapter    Adapter
	Assets     Assets
	Now        func() time.Time
}

func (e Execution) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

type stepPayload struct {
	Version   string
	RequestID string
	Intent    contracts.ActionIntent
	Step      string
	Authority contracts.PackagePublishAuthorization
}
type storedEffect struct {
	State   string
	Payload []byte
	Result  []byte
	Created time.Time
}

func (e Execution) load(ctx context.Context, id string) (storedEffect, error) {
	var v storedEffect
	var at string
	var result []byte
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT e.state,e.request_payload,e.observed_result,e.created_at
FROM effects e JOIN commands c ON c.command_id=e.command_id
JOIN events v ON v.event_id=e.effect_id AND v.command_id=c.command_id
WHERE e.effect_id=? AND e.command_id=e.effect_id
AND c.command_type='goals-publication.step' AND c.command_version='1'
AND v.event_type='goals-publication.step-admitted' AND v.event_version='1'
AND c.payload=e.request_payload AND v.payload=e.request_payload
AND e.target_adapter='goals-initial-github'
AND ((e.state='pending' AND e.attempts=0) OR (e.state!='pending' AND e.attempts=1))`, id).Scan(&v.State, &v.Payload, &result, &at)
	if err != nil {
		return v, err
	}
	v.Result = result
	v.Created, err = time.Parse(time.RFC3339Nano, at)
	return v, err
}
func executionKey(req string) string { return "goals-initial-publication:" + hash([]byte(req)) }
func (e Execution) abandoned(ctx context.Context, requestID string) (bool, error) {
	var n int
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?`, executionKey(requestID)).Scan(&n)
	return n != 0, err
}
func (e Execution) current(ctx context.Context, requestID string) (contracts.PackagePublishAuthorization, error) {
	now := e.now()
	model, err := e.Repository.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return contracts.PackagePublishAuthorization{}, err
	}
	if model.ActiveVersion != contracts.AuthorityModelGoalsPublicationVersion || model.ActiveDigest != contracts.AuthorityModelGoalsPublicationDigest() || model.State != "committed" {
		return contracts.PackagePublishAuthorization{}, errors.New("publication model not adopted")
	}
	return e.Repository.LoadGoalsPublicationAuthorization(ctx, requestID, now, now)
}
func (e Execution) validateStored(ctx context.Context, v storedEffect, requestID, step string) (contracts.ActionIntent, error) {
	var p stepPayload
	if err := json.Unmarshal(v.Payload, &p); err != nil {
		return contracts.ActionIntent{}, err
	}
	auth, err := e.Repository.LoadGoalsPublicationAuthorization(ctx, requestID, v.Created, e.now())
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	if auth.Request.Intent == nil {
		return contracts.ActionIntent{}, errors.New("missing original publication intent")
	}
	want := stepPayload{Version: "1", RequestID: requestID, Intent: *auth.Request.Intent, Step: step, Authority: auth}
	b, _ := json.Marshal(want)
	intentDigest, _ := auth.Request.Intent.Digest()
	var bound int
	err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM effects WHERE effect_id=? AND action_intent_digest=? AND target_principal=?`, executionKey(requestID)+":"+step, intentDigest, auth.Request.Intent.Actor.ID).Scan(&bound)
	if err != nil || bound != 1 {
		return contracts.ActionIntent{}, errors.New("effect lost its admitted intent/principal binding")
	}
	if string(b) != string(v.Payload) {
		return contracts.ActionIntent{}, errors.New("persisted publication step authority mismatch")
	}
	return p.Intent, nil
}

// Execute never repeats an ambiguous external mutation. The deterministic
// command/aggregate identities atomically admit only one execution per exact
// request; the bound generation cannot authorize a different execution ID.
func (e Execution) Execute(ctx context.Context, requestID string) (string, error) {
	if e.Adapter == nil || e.Repository.Store == nil {
		return "", errors.New("publication execution dependencies missing")
	}
	if !strings.HasPrefix(requestID, "goals-publication-request:") {
		return "", errors.New("not a Goals publication request")
	}
	if stopped, err := e.abandoned(ctx, requestID); err != nil {
		return "", err
	} else if stopped {
		return "", errors.New("publication execution is terminally abandoned")
	}
	if err := VerifySigning(ctx, e.Repository, e.Assets, e.now()); err != nil {
		return "", err
	}
	key := executionKey(requestID)
	previous := []Observation{}
	var intent contracts.ActionIntent
	for i, step := range steps {
		id := key + ":" + step
		v, err := e.load(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			auth, er := e.current(ctx, requestID)
			if er != nil {
				return "", er
			}
			intent = *auth.Request.Intent
			if er = e.Assets.Match(intent); er != nil {
				return "", er
			}
			now := e.now()
			payload, _ := json.Marshal(stepPayload{Version: "1", RequestID: requestID, Intent: intent, Step: step, Authority: auth})
			digest, _ := intent.Digest()
			event := state.EventRecord{ID: id, AggregateID: key, AggregateType: "goals-initial-publication", AggregateVersion: int64(i + 1), Type: "goals-publication.step-admitted", Version: "1", Actor: intent.Actor, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: payload, CreatedAt: now}
			cmd := state.CommandRecord{ID: id, Type: "goals-publication.step", Version: "1", Actor: intent.Actor, Scope: intent.Scope, CorrelationID: key, Payload: payload, CreatedAt: now}
			ef := state.EffectRecord{ID: id, CommandID: id, ActionIntentDigest: digest, TargetAdapter: "goals-initial-github", TargetPrincipal: intent.Actor.ID, PreconditionsJSON: mustJSON(intent.Preconditions), CryptoProfile: intent.CryptoProfile, State: string(state.EffectPending), RequestPayload: payload, CreatedAt: now, UpdatedAt: now}
			if er = e.Repository.Store.CommitTransition(ctx, cmd, int64(i), event, "", "", &ef); er != nil {
				return "", er
			}
			v, err = e.load(ctx, id)
		}
		if err != nil {
			return "", err
		}
		intent, err = e.validateStored(ctx, v, requestID, step)
		if err != nil {
			return "", err
		}
		if err = e.Assets.Match(intent); err != nil {
			return "", err
		}
		if stopped, fenceErr := e.abandoned(ctx, requestID); fenceErr != nil {
			return "", fenceErr
		} else if stopped {
			return "", errors.New("publication execution is terminally abandoned")
		}
		if v.State == string(state.EffectSucceeded) {
			var o Observation
			if err = json.Unmarshal(v.Result, &o); err != nil {
				return "", err
			}
			if err = validateObservation(intent, step, o, previous); err != nil {
				return "", err
			}
			previous = append(previous, o)
			continue
		}
		if v.State != string(state.EffectPending) {
			if v.State == string(state.EffectFailed) {
				return "", errors.New("publication step failed; no automatic retry")
			}
			return "", e.Reconcile(ctx, requestID)
		}
		boundary := stepBoundary{execution: e, requestID: requestID, intent: intent, step: step, previous: previous}
		d, _ := intent.Digest()
		// CAS claim before dispatch prevents concurrent execution of a pending step.
		res, claimErr := e.Repository.Store.DB().ExecContext(ctx, `UPDATE effects SET state='dispatched',attempts=attempts+1,updated_at=? WHERE effect_id=? AND state='pending' AND NOT EXISTS (SELECT 1 FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?)`, e.now().Format(time.RFC3339Nano), id, key)
		if claimErr != nil {
			return "", claimErr
		}
		claimed, claimErr := res.RowsAffected()
		if claimErr != nil || claimed != 1 {
			return "", errors.New("publication effect already claimed; reconcile without redispatch")
		}
		result, err := (effect.Coordinator{Revalidator: boundary, Preconditions: boundary, Dispatcher: boundary}).Commit(ctx, intent, effect.AuthorizationSnapshot{IntentDigest: d}, "")
		if err != nil {
			var evidence []byte
			var dispatched dispatchFailure
			outcome := state.EffectUnknown
			if errors.As(err, &dispatched) {
				evidence = dispatched.Evidence()
				if dispatched.outcome.Class == "local_pre_dispatch_failure" {
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
		if err = validateObservation(intent, step, o, previous); err != nil {
			saveErr := e.Repository.Store.MarkEffectOutcome(ctx, id, state.EffectUnknown, result.Evidence, nil, e.now())
			return "", errors.Join(err, saveErr)
		}
		if err = e.Repository.Store.MarkEffectOutcome(ctx, id, state.EffectSucceeded, result.Evidence, nil, e.now()); err != nil {
			return "", err
		}
		previous = append(previous, o)
	}
	// Completion is an existing event; no separate receipt protocol or table.
	completion := key + ":completed"
	if stopped, fenceErr := e.abandoned(ctx, requestID); fenceErr != nil {
		return "", fenceErr
	} else if stopped {
		return "", errors.New("publication execution is terminally abandoned")
	}
	evidence := make([]string, 0, len(steps))
	for _, step := range steps {
		v, err := e.load(ctx, key+":"+step)
		if err != nil {
			return "", err
		}
		if v.State != string(state.EffectSucceeded) {
			return "", errors.New("publication incomplete")
		}
		evidence = append(evidence, hash(v.Payload)+"/"+hash(v.Result))
	}
	payload := mustJSON(struct {
		Version, RequestID, SigningProvenance string
		Effects                               []string
	}{"1", requestID, contracts.GoalsPublicationSigningReceipt, evidence})
	var existing []byte
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, completion).Scan(&existing)
	if err == nil {
		if string(existing) != string(payload) {
			return "", errors.New("publication completion evidence mismatch")
		}
		return completion, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	now := e.now()
	cmd := state.CommandRecord{ID: completion, Type: "goals-publication.complete", Version: "1", Actor: intent.Actor, Scope: intent.Scope, CorrelationID: key, Payload: payload, CreatedAt: now}
	event := state.EventRecord{ID: completion, AggregateID: key, AggregateType: "goals-initial-publication", AggregateVersion: int64(len(steps) + 1), Type: "goals-publication.completed", Version: "1", Actor: intent.Actor, CommandID: completion, CorrelationID: key, TrustClass: contracts.TrustObserved, Payload: payload, CreatedAt: now}
	if err = e.Repository.Store.CommitGoalsPublicationCompletion(ctx, cmd, int64(len(steps)), event, key); err != nil {
		return "", err
	}
	return completion, nil
}
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

type stepBoundary struct {
	execution Execution
	requestID string
	intent    contracts.ActionIntent
	step      string
	previous  []Observation
}

func (b stepBoundary) Revalidate(ctx context.Context, a contracts.ActionIntent, s effect.AuthorizationSnapshot) error {
	if err := VerifySigning(ctx, b.execution.Repository, b.execution.Assets, b.execution.now()); err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	auth, err := b.execution.current(ctx, b.requestID)
	if err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	d, _ := auth.Request.Intent.Digest()
	got, _ := a.Digest()
	if d != got || d != s.IntentDigest {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: errors.New("publication authority changed")}
	}
	return nil
}
func (b stepBoundary) Check(ctx context.Context, a contracts.ActionIntent) error {
	if err := b.execution.Adapter.Check(ctx, a, b.step, b.previous); err != nil {
		return dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "not_started", Class: "local_pre_dispatch_failure"}, err: err}
	}
	return nil
}
func (b stepBoundary) Dispatch(ctx context.Context, a contracts.ActionIntent, _ string) (effect.Result, error) {
	// Preconditions may involve slow downloads. Fence authority again immediately
	// before the adapter can mutate the remote.
	d, _ := a.Digest()
	if err := b.Revalidate(ctx, a, effect.AuthorizationSnapshot{IntentDigest: d}); err != nil {
		return effect.Result{}, err
	}
	o, err := b.execution.Adapter.Dispatch(ctx, a, b.step, b.previous, b.execution.Assets)
	if err != nil {
		return effect.Result{}, err
	}
	return effect.Result{ObservedState: "observed", Evidence: mustJSON(o)}, nil
}
func validateObservation(a contracts.ActionIntent, step string, o Observation, prior []Observation) error {
	if fmt.Sprint(o.RepositoryID) != a.Parameters["repository_id"] || fmt.Sprint(o.OwnerID) != a.Parameters["owner_id"] || fmt.Sprint(o.AccountID) != a.Parameters["account_id"] || o.Commit != a.Parameters["commit"] || o.Tree != a.Parameters["tree"] {
		return errors.New("publication observation identity mismatch")
	}
	switch step {
	case "refs":
		if o.DispatchEvidence == "" {
			return errors.New("missing attributable ref creation result")
		}
	case "draft", "verify", "publish", "verify-published":
		if o.Release == nil {
			return errors.New("missing release observation")
		}
		if err := releaseMatches(*o.Release, a, step == "draft" || step == "verify"); err != nil {
			return err
		}
		if step != "draft" && (len(prior) < 2 || prior[1].Release == nil || prior[1].Release.ID != o.Release.ID) {
			return errors.New("release observation changed identity")
		}
		if err := validateInventory(a, *o.Release, prior, step == "draft"); err != nil {
			return err
		}
		if step == "verify" || step == "verify-published" {
			if string(mustJSON(o.AssetDigests)) != string(mustJSON(assetDigests)) {
				return errors.New("missing exact asset verification")
			}
		}
	case "manifest", "archive", "signature":
		idx := map[string]int{"manifest": 0, "archive": 1, "signature": 2}[step]
		if o.Asset == nil {
			return errors.New("missing asset observation")
		}
		size, _ := strconv.ParseInt(a.Parameters[step+"_size"], 10, 64)
		if o.Asset.ID <= 0 || o.Asset.Name != assetNames[idx] || o.Asset.Size != size || o.Asset.State != "uploaded" || fmt.Sprint(o.Asset.Uploader.ID) != a.Parameters["account_id"] {
			return errors.New("asset observation mismatch")
		}
	default:
		return errors.New("unknown publication step")
	}
	return nil
}
