package goalspublication

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type abandonmentEffect struct {
	ID, State                       string
	Attempts                        int
	Request, Result, Reconciliation string
}
type abandonmentPayload struct {
	Version, RequestID, RequestDigest, IntentID, IntentDigest string
	AuthorityRef, AuthorityVersion, AuthorityDigest           string
	ExecutionID                                               string
	Effects                                                   []abandonmentEffect
	ReconciliationEvents                                      []reconciliationSnapshot
	Owner                                                     contracts.PrincipalRef
	OSUser                                                    string
	Reason                                                    string
	ExecutionStatus                                           string
	CompletionEstablished                                     bool
	CreatedAt                                                 string
}
type reconciliationSnapshot struct{ ID, PayloadDigest string }

// PrepareAbandonment freezes the exact owner-confirmable payload. The caller
// must retain these bytes and pass them unchanged to ConfirmAbandonment.
func (e Execution) PrepareAbandonment(ctx context.Context, requestID, osUser, reason string) ([]byte, string, error) {
	if e.Repository.Store == nil || !strings.HasPrefix(requestID, "goals-publication-request:") || osUser == "" || strings.TrimSpace(reason) == "" {
		return nil, "", errors.New("invalid abandonment request")
	}
	key := executionKey(requestID)
	var existing int
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?`, key).Scan(&existing); err != nil {
		return nil, "", err
	}
	if existing != 0 {
		return nil, "", errors.New("conflicting abandonment already exists")
	}
	b, err := e.buildAbandonmentPayload(ctx, requestID, osUser, reason, e.now())
	if err != nil {
		return nil, "", err
	}
	return b, hash(b), nil
}

// ConfirmAbandonment re-resolves every mutable predecessor precondition and
// appends the same frozen payload bytes that the owner confirmed.
func (e Execution) ConfirmAbandonment(ctx context.Context, frozen []byte, osUser, confirmation string) (string, error) {
	if e.Repository.Store == nil || len(frozen) == 0 || osUser == "" {
		return "", errors.New("invalid abandonment confirmation")
	}
	var p abandonmentPayload
	if err := json.Unmarshal(frozen, &p); err != nil {
		return "", errors.New("invalid frozen abandonment payload")
	}
	if p.Version != "1" || p.OSUser != osUser || p.RequestID == "" || p.Reason == "" {
		return "", errors.New("frozen abandonment payload does not bind the current owner and request")
	}
	d := hash(frozen)
	if confirmation != "ABANDON "+d {
		return "", fmt.Errorf("exact owner confirmation required: ABANDON %s", d)
	}
	key := executionKey(p.RequestID)
	id := "goals-publication-abandoned:" + d
	var prior []byte
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?`, key).Scan(&prior)
	if err == nil {
		if bytes.Equal(prior, frozen) {
			return id, nil
		}
		return "", errors.New("conflicting abandonment already exists")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	created, err := time.Parse(time.RFC3339Nano, p.CreatedAt)
	if err != nil {
		return "", errors.New("frozen abandonment timestamp is invalid")
	}
	current, err := e.buildAbandonmentPayload(ctx, p.RequestID, osUser, p.Reason, created)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(current, frozen) {
		return "", errors.New("predecessor state changed after abandonment payload confirmation")
	}
	var first []byte
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, key+":refs").Scan(&first); err != nil {
		return "", err
	}
	var sp stepPayload
	if err := json.Unmarshal(first, &sp); err != nil {
		return "", err
	}
	cmd := state.CommandRecord{ID: id, Type: "goals-publication.abandon", Version: "1", Actor: p.Owner, Scope: sp.Intent.Scope, CorrelationID: key, Payload: frozen, CreatedAt: e.now()}
	ev := state.EventRecord{ID: id, AggregateID: id, AggregateType: "goals-publication-abandonment", AggregateVersion: 1, Type: "goals-publication.abandoned", Version: "1", Actor: p.Owner, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: frozen, CreatedAt: e.now()}
	if err = e.Repository.Store.CommitGoalsPublicationAbandonment(ctx, cmd, ev, key); err != nil {
		return "", err
	}
	return id, nil
}

func (e Execution) buildAbandonmentPayload(ctx context.Context, requestID, osUser, reason string, createdAt time.Time) ([]byte, error) {
	if e.Repository.Store == nil || !strings.HasPrefix(requestID, "goals-publication-request:") || osUser == "" || strings.TrimSpace(reason) == "" {
		return nil, errors.New("invalid abandonment request")
	}
	key := executionKey(requestID)
	var effects []abandonmentEffect
	rows, err := e.Repository.Store.DB().QueryContext(ctx, `SELECT e.effect_id,e.state,e.attempts,e.request_payload,COALESCE(e.observed_result,''),COALESCE(e.reconciliation_evidence,'') FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id WHERE c.command_type='goals-publication.step' AND c.correlation_id=? ORDER BY v.aggregate_version`, key)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var x abandonmentEffect
		if err = rows.Scan(&x.ID, &x.State, &x.Attempts, &x.Request, &x.Result, &x.Reconciliation); err != nil {
			rows.Close()
			return nil, err
		}
		x.Request = hash([]byte(x.Request))
		x.Result = hash([]byte(x.Result))
		x.Reconciliation = hash([]byte(x.Reconciliation))
		effects = append(effects, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(effects) != 3 {
		return nil, errors.New("predecessor effect lineage is not the exact refs/draft/manifest prefix")
	}
	var first []byte
	err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, effects[0].ID).Scan(&first)
	if err != nil {
		return nil, err
	}
	var sp stepPayload
	if err = json.Unmarshal(first, &sp); err != nil {
		return nil, err
	}
	if sp.RequestID != requestID || sp.Step != "refs" || sp.Intent.ID == "" || contracts.ValidateGoalsPublicationIntent(sp.Intent) != nil {
		return nil, errors.New("predecessor intent lineage mismatch")
	}
	intentDigest, err := sp.Intent.Digest()
	if err != nil {
		return nil, err
	}
	reqDigest, err := sp.Authority.Request.DigestAt(sp.Authority.Decision.IssuedAt)
	if err != nil {
		return nil, err
	}
	if sp.Authority.Request.ID != requestID || sp.Authority.Request.IntentDigest != intentDigest || sp.Authority.Generation.Digest == "" {
		return nil, errors.New("predecessor authority lineage mismatch")
	}
	issued := sp.Authority.Decision.IssuedAt
	resolved, err := e.Repository.LoadGoalsPublicationAuthorization(ctx, requestID, issued, issued)
	if err != nil {
		return nil, fmt.Errorf("predecessor authorization lineage invalid: %w", err)
	}
	if !reflect.DeepEqual(resolved, sp.Authority) {
		return nil, errors.New("predecessor effect authority differs from canonical request/decision/generation")
	}
	for i, step := range []string{"refs", "draft", "manifest"} {
		var body []byte
		if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, effects[i].ID).Scan(&body); err != nil {
			return nil, err
		}
		var observed stepPayload
		if err = json.Unmarshal(body, &observed); err != nil {
			return nil, err
		}
		if effects[i].ID != key+":"+step || observed.RequestID != requestID || observed.Step != step || observed.Version != "1" || observed.Intent.ID != sp.Intent.ID || !reflect.DeepEqual(observed.Authority, sp.Authority) {
			return nil, errors.New("predecessor effect lineage contains a substitution")
		}
		var bound int
		if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id WHERE e.effect_id=? AND e.command_id=e.effect_id AND c.command_type='goals-publication.step' AND c.correlation_id=? AND c.payload=e.request_payload AND v.payload=e.request_payload AND v.event_type='goals-publication.step-admitted' AND e.action_intent_digest=? AND e.target_principal=? AND e.target_adapter='goals-initial-github'`, effects[i].ID, key, intentDigest, sp.Intent.Actor.ID).Scan(&bound); err != nil || bound != 1 {
			return nil, errors.New("predecessor command/event/effect binding mismatch")
		}
	}
	root, err := contracts.InstallationOwnerPrincipal(contracts.GoalsPublicationBootstrap)
	if err != nil {
		return nil, err
	}
	ownerOK, err := requireCurrentOwnerOSUser(ctx, e.Repository, e.now(), osUser)
	if err != nil {
		return nil, err
	}
	if !ownerOK {
		return nil, errors.New("current OS user is not the authenticated installation owner")
	}
	if effects[0].State != string(state.EffectSucceeded) || effects[1].State != string(state.EffectSucceeded) || effects[2].State != string(state.EffectUnknown) || effects[0].Attempts != 1 || effects[1].Attempts != 1 || effects[2].Attempts != 1 {
		return nil, errors.New("predecessor is not in the exact abandonable effect state")
	}
	var completed int
	if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id=?`, key+":completed").Scan(&completed); err != nil {
		return nil, err
	}
	if completed != 0 {
		return nil, errors.New("predecessor completion already exists")
	}
	var recs []reconciliationSnapshot
	rows, err = e.Repository.Store.DB().QueryContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication.reconciled' AND correlation_id=? ORDER BY event_id`, key)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var body []byte
		if err = rows.Scan(&id, &body); err != nil {
			rows.Close()
			return nil, err
		}
		recs = append(recs, reconciliationSnapshot{ID: id, PayloadDigest: hash(body)})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	p := abandonmentPayload{Version: "1", RequestID: requestID, RequestDigest: reqDigest, IntentID: sp.Intent.ID, IntentDigest: intentDigest, AuthorityRef: sp.Authority.Generation.Ref, AuthorityVersion: sp.Authority.Generation.Version, AuthorityDigest: sp.Authority.Generation.Digest, ExecutionID: key, Effects: effects, ReconciliationEvents: recs, Owner: root, OSUser: osUser, Reason: reason, ExecutionStatus: "abandoned", CompletionEstablished: false, CreatedAt: createdAt.UTC().Format(time.RFC3339Nano)}
	b, _ := json.Marshal(p)
	return b, nil
}

// requireCurrentOwnerOSUser authenticates the OS user against the installation
// owner's CURRENT root-shaped generation only (N17 equivalent path): a retired or
// superseded root-shaped record must not authenticate anyone. Both abandonment
// paths use it.
func requireCurrentOwnerOSUser(ctx context.Context, repository goalstore.Repository, now time.Time, osUser string) (bool, error) {
	owner, err := contracts.InstallationOwnerPrincipal(contracts.GoalsPublicationBootstrap)
	if err != nil {
		return false, err
	}
	gens, err := repository.ListCurrentAuthorityGenerations(ctx, now)
	if err != nil {
		return false, err
	}
	for _, g := range gens {
		if g.ParentRef == "" && g.Principal == owner && strings.HasSuffix(g.ProvenanceRef, ":os-user:"+osUser) {
			return true, nil
		}
	}
	return false, nil
}
