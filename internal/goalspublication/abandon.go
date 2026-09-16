package goalspublication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

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

// Abandon appends an owner-authorized terminal fence for the one accepted
// predecessor execution. The exact historical EffectRecords are only read.
func (e Execution) Abandon(ctx context.Context, requestID, osUser, reason, confirmation string) (string, error) {
	if e.Repository.Store == nil || !strings.HasPrefix(requestID, "goals-publication-request:") || osUser == "" || strings.TrimSpace(reason) == "" {
		return "", errors.New("invalid abandonment request")
	}
	key := executionKey(requestID)
	var prior []byte
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id=?`, key).Scan(&prior)
	if err == nil {
		var p abandonmentPayload
		if json.Unmarshal(prior, &p) == nil && p.Reason == reason && p.OSUser == osUser {
			d := hash(prior)
			if confirmation == "ABANDON "+d {
				return "goals-publication-abandoned:" + d, nil
			}
		}
		return "", errors.New("conflicting abandonment already exists")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var effects []abandonmentEffect
	rows, err := e.Repository.Store.DB().QueryContext(ctx, `SELECT e.effect_id,e.state,e.attempts,e.request_payload,COALESCE(e.observed_result,''),COALESCE(e.reconciliation_evidence,'') FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id WHERE c.command_type='goals-publication.step' AND c.correlation_id=? ORDER BY v.aggregate_version`, key)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var x abandonmentEffect
		if err = rows.Scan(&x.ID, &x.State, &x.Attempts, &x.Request, &x.Result, &x.Reconciliation); err != nil {
			rows.Close()
			return "", err
		}
		x.Request = hash([]byte(x.Request))
		x.Result = hash([]byte(x.Result))
		x.Reconciliation = hash([]byte(x.Reconciliation))
		effects = append(effects, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()
	if len(effects) != 3 {
		return "", errors.New("predecessor effect lineage is not the exact refs/draft/manifest prefix")
	}
	var first []byte
	err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, effects[0].ID).Scan(&first)
	if err != nil {
		return "", err
	}
	var sp stepPayload
	if err = json.Unmarshal(first, &sp); err != nil {
		return "", err
	}
	if sp.RequestID != requestID || sp.Step != "refs" || sp.Intent.ID == "" || contracts.ValidateGoalsPublicationIntent(sp.Intent) != nil {
		return "", errors.New("predecessor intent lineage mismatch")
	}
	intentDigest, err := sp.Intent.Digest()
	if err != nil {
		return "", err
	}
	reqDigest, err := sp.Authority.Request.DigestAt(sp.Authority.Decision.IssuedAt)
	if err != nil {
		return "", err
	}
	if sp.Authority.Request.ID != requestID || sp.Authority.Request.IntentDigest != intentDigest || sp.Authority.Generation.Digest == "" {
		return "", errors.New("predecessor authority lineage mismatch")
	}
	issued := sp.Authority.Decision.IssuedAt
	resolved, err := e.Repository.LoadGoalsPublicationAuthorization(ctx, requestID, issued, issued)
	if err != nil {
		return "", fmt.Errorf("predecessor authorization lineage invalid: %w", err)
	}
	if !reflect.DeepEqual(resolved, sp.Authority) {
		return "", errors.New("predecessor effect authority differs from canonical request/decision/generation")
	}
	for i, step := range []string{"refs", "draft", "manifest"} {
		var body []byte
		if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, effects[i].ID).Scan(&body); err != nil {
			return "", err
		}
		var observed stepPayload
		if err = json.Unmarshal(body, &observed); err != nil {
			return "", err
		}
		if effects[i].ID != key+":"+step || observed.RequestID != requestID || observed.Step != step || observed.Version != "1" || observed.Intent.ID != sp.Intent.ID || !reflect.DeepEqual(observed.Authority, sp.Authority) {
			return "", errors.New("predecessor effect lineage contains a substitution")
		}
		var bound int
		if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM effects e JOIN commands c ON c.command_id=e.command_id JOIN events v ON v.event_id=e.effect_id WHERE e.effect_id=? AND e.command_id=e.effect_id AND c.command_type='goals-publication.step' AND c.correlation_id=? AND c.payload=e.request_payload AND v.payload=e.request_payload AND v.event_type='goals-publication.step-admitted' AND e.action_intent_digest=? AND e.target_principal=? AND e.target_adapter='goals-initial-github'`, effects[i].ID, key, intentDigest, sp.Intent.Actor.ID).Scan(&bound); err != nil || bound != 1 {
			return "", errors.New("predecessor command/event/effect binding mismatch")
		}
	}
	root, err := contracts.InstallationOwnerPrincipal(contracts.GoalsPublicationBootstrap)
	if err != nil {
		return "", err
	}
	gens, err := e.Repository.ListAuthorityGenerations(ctx, e.now())
	if err != nil {
		return "", err
	}
	ownerOK := false
	for _, g := range gens {
		if g.ParentRef == "" && g.Principal == root && strings.HasSuffix(g.ProvenanceRef, ":os-user:"+osUser) {
			ownerOK = true
			break
		}
	}
	if !ownerOK {
		return "", errors.New("current OS user is not the authenticated installation owner")
	}
	if effects[0].State != string(state.EffectSucceeded) || effects[1].State != string(state.EffectSucceeded) || effects[2].State != string(state.EffectUnknown) || effects[0].Attempts != 1 || effects[1].Attempts != 1 || effects[2].Attempts != 1 {
		return "", errors.New("predecessor is not in the exact abandonable effect state")
	}
	var completed int
	if err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id=?`, key+":completed").Scan(&completed); err != nil {
		return "", err
	}
	if completed != 0 {
		return "", errors.New("predecessor completion already exists")
	}
	var recs []reconciliationSnapshot
	rows, err = e.Repository.Store.DB().QueryContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication.reconciled' AND correlation_id=? ORDER BY event_id`, key)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var id string
		var body []byte
		if err = rows.Scan(&id, &body); err != nil {
			rows.Close()
			return "", err
		}
		recs = append(recs, reconciliationSnapshot{ID: id, PayloadDigest: hash(body)})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	rows.Close()
	p := abandonmentPayload{Version: "1", RequestID: requestID, RequestDigest: reqDigest, IntentID: sp.Intent.ID, IntentDigest: intentDigest, AuthorityRef: sp.Authority.Generation.Ref, AuthorityVersion: sp.Authority.Generation.Version, AuthorityDigest: sp.Authority.Generation.Digest, ExecutionID: key, Effects: effects, ReconciliationEvents: recs, Owner: root, OSUser: osUser, Reason: reason, ExecutionStatus: "abandoned", CompletionEstablished: false, CreatedAt: e.now().Format(time.RFC3339Nano)}
	b, _ := json.Marshal(p)
	d := hash(b)
	id := "goals-publication-abandoned:" + d
	if confirmation != "ABANDON "+d {
		return "", fmt.Errorf("exact owner confirmation required: ABANDON %s", d)
	}
	cmd := state.CommandRecord{ID: id, Type: "goals-publication.abandon", Version: "1", Actor: root, Scope: sp.Intent.Scope, CorrelationID: key, Payload: b, CreatedAt: e.now()}
	ev := state.EventRecord{ID: id, AggregateID: id, AggregateType: "goals-publication-abandonment", AggregateVersion: 1, Type: "goals-publication.abandoned", Version: "1", Actor: root, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: b, CreatedAt: e.now()}
	if err = e.Repository.Store.CommitGoalsPublicationAbandonment(ctx, cmd, ev, key); err != nil {
		return "", err
	}
	return id, nil
}
