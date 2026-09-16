package goalspublication

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type recoveryAbandonmentPayload struct {
	Version, RequestID, RequestDigest, IntentID, IntentDigest string
	AuthorityDigest, ExecutionID, ExecutionStatus             string
	Effects                                                   []abandonmentEffect
	ReconciliationEvents                                      []reconciliationSnapshot
	Owner                                                     contracts.PrincipalRef
	OSUser, Reason, CreatedAt                                 string
	CompletionEstablished                                     bool
}

func (e RecoveryExecution) recoveryAbandoned(ctx context.Context, requestID string) (bool, error) {
	var n int
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type='goals-publication-recovery.abandoned' AND correlation_id=?`, recoveryKey(requestID)).Scan(&n)
	return n != 0, err
}

func (e RecoveryExecution) buildRecoveryAbandonment(ctx context.Context, requestID, osUser, reason string, created time.Time) ([]byte, error) {
	if e.Repository.Store == nil || !strings.HasPrefix(requestID, "goals-publication-recovery-request:") || osUser == "" || strings.TrimSpace(reason) == "" {
		return nil, errors.New("invalid recovery abandonment request")
	}
	key := recoveryKey(requestID)
	payload, err := loadRecoveryManifestPayload(ctx, e.Repository.Store.DB(), key+":manifest")
	if err != nil {
		return nil, err
	}
	var sp recoveryStepPayload
	if err := json.Unmarshal(payload, &sp); err != nil || sp.RequestID != requestID || sp.Step != "manifest" || sp.Intent.ID == "" {
		return nil, errors.New("successor intent lineage mismatch")
	}
	intentDigest, err := sp.Intent.Digest()
	if err != nil {
		return nil, err
	}
	if err = contracts.ValidateGoalsPublicationRecoveryIntent(sp.Intent); err != nil {
		return nil, err
	}
	if _, err = e.predecessor(ctx, sp.Intent); err != nil {
		return nil, err
	}
	authDigest := sp.Authority.Generation.Digest
	requestDigest, err := sp.Authority.Request.DigestAt(sp.Authority.Decision.IssuedAt)
	if err != nil {
		return nil, err
	}
	var effects []abandonmentEffect
	for i, step := range recoverySteps {
		id := key + ":" + step
		var x abandonmentEffect
		err = e.Repository.Store.DB().QueryRowContext(ctx, `SELECT e.effect_id,e.state,e.attempts,e.request_payload,COALESCE(e.observed_result,''),COALESCE(e.reconciliation_evidence,'') FROM effects e WHERE e.effect_id=?`, id).Scan(&x.ID, &x.State, &x.Attempts, &payload, &x.Result, &x.Reconciliation)
		if errors.Is(err, sql.ErrNoRows) {
			if i == 0 {
				return nil, errors.New("successor manifest effect is missing")
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		x.Request = hash(payload)
		x.Result = hash([]byte(x.Result))
		x.Reconciliation = hash([]byte(x.Reconciliation))
		effects = append(effects, x)
	}
	if len(effects) == 0 || effects[0].State != string(state.EffectUnknown) || effects[0].Attempts != 1 {
		return nil, errors.New("successor is not in the exact abandonable state")
	}
	for _, x := range effects {
		if x.State == string(state.EffectDispatched) || x.State == string(state.EffectReconciling) {
			return nil, errors.New("successor dispatch is in flight")
		}
	}
	var recs []reconciliationSnapshot
	rows, err := e.Repository.Store.DB().QueryContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication-recovery.reconciled' AND correlation_id=? ORDER BY event_id`, key)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var b []byte
		if err = rows.Scan(&id, &b); err != nil {
			rows.Close()
			return nil, err
		}
		recs = append(recs, reconciliationSnapshot{ID: id, PayloadDigest: hash(b)})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	root, err := contracts.InstallationOwnerPrincipal(contracts.GoalsPublicationBootstrap)
	if err != nil {
		return nil, err
	}
	gens, err := e.Repository.ListAuthorityGenerations(ctx, e.now())
	if err != nil {
		return nil, err
	}
	ownerOK := false
	for _, g := range gens {
		if g.ParentRef == "" && g.Principal == root && strings.HasSuffix(g.ProvenanceRef, ":os-user:"+osUser) {
			ownerOK = true
			break
		}
	}
	if !ownerOK {
		return nil, errors.New("current OS user is not installation owner")
	}
	p := recoveryAbandonmentPayload{Version: "1", RequestID: requestID, RequestDigest: requestDigest, IntentID: sp.Intent.ID, IntentDigest: intentDigest, AuthorityDigest: authDigest, ExecutionID: key, ExecutionStatus: "abandoned", Effects: effects, ReconciliationEvents: recs, Owner: root, OSUser: osUser, Reason: reason, CreatedAt: created.UTC().Format(time.RFC3339Nano), CompletionEstablished: false}
	b, _ := json.Marshal(p)
	return b, nil
}

func loadRecoveryManifestPayload(ctx context.Context, db *sql.DB, effectID string) ([]byte, error) {
	var payload []byte
	if err := db.QueryRowContext(ctx, `SELECT request_payload FROM effects WHERE effect_id=?`, effectID).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("successor manifest effect is missing")
		}
		return nil, fmt.Errorf("lookup successor manifest effect: %w", err)
	}
	return payload, nil
}

func (e RecoveryExecution) PrepareAbandonment(ctx context.Context, requestID, osUser, reason string) ([]byte, string, error) {
	var n int
	if err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_type='goals-publication-recovery.abandoned' AND correlation_id=?`, recoveryKey(requestID)).Scan(&n); err != nil {
		return nil, "", err
	}
	if n != 0 {
		return nil, "", errors.New("conflicting recovery abandonment already exists")
	}
	b, err := e.buildRecoveryAbandonment(ctx, requestID, osUser, reason, e.now())
	if err != nil {
		return nil, "", err
	}
	return b, hash(b), nil
}

func (e RecoveryExecution) ConfirmAbandonment(ctx context.Context, frozen []byte, osUser, confirmation string) (string, error) {
	var p recoveryAbandonmentPayload
	if err := json.Unmarshal(frozen, &p); err != nil {
		return "", errors.New("invalid frozen recovery abandonment payload")
	}
	d := hash(frozen)
	if confirmation != "ABANDON "+d {
		return "", fmt.Errorf("exact owner confirmation required: ABANDON %s", d)
	}
	if p.OSUser != osUser || p.Version != "1" || p.RequestID == "" {
		return "", errors.New("frozen recovery abandonment owner/request mismatch")
	}
	key := recoveryKey(p.RequestID)
	var prior []byte
	err := e.Repository.Store.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE event_type='goals-publication-recovery.abandoned' AND correlation_id=?`, key).Scan(&prior)
	if err == nil {
		if bytes.Equal(prior, frozen) {
			return "goals-publication-recovery-abandoned:" + d, nil
		}
		return "", errors.New("conflicting recovery abandonment already exists")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	created, err := time.Parse(time.RFC3339Nano, p.CreatedAt)
	if err != nil {
		return "", errors.New("invalid frozen recovery abandonment timestamp")
	}
	current, err := e.buildRecoveryAbandonment(ctx, p.RequestID, osUser, p.Reason, created)
	if err != nil || !bytes.Equal(current, frozen) {
		return "", errors.New("successor state changed after abandonment confirmation")
	}
	id := "goals-publication-recovery-abandoned:" + d
	now := e.now()
	cmd := state.CommandRecord{ID: id, Type: "goals-publication-recovery.abandon", Version: "1", Actor: p.Owner, Scope: p.IntentID, CorrelationID: key, Payload: frozen, CreatedAt: now}
	ev := state.EventRecord{ID: id, AggregateID: id, AggregateType: "goals-publication-recovery-abandonment", AggregateVersion: 1, Type: "goals-publication-recovery.abandoned", Version: "1", Actor: p.Owner, CommandID: id, CorrelationID: key, TrustClass: contracts.TrustPolicy, Payload: frozen, CreatedAt: now}
	if err = e.Repository.Store.CommitGoalsPublicationRecoveryAbandonment(ctx, cmd, ev, key); err != nil {
		return "", err
	}
	return id, nil
}
