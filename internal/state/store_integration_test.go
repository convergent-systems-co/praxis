package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestCommitTransitionIsAtomic(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	future := now.Add(time.Hour).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id, approver_id, approver_kind, intent_digest, issued_at, expires_at, remaining_uses) VALUES('approval-1','human-1','human','sha256:abc',?,?,1)`, now.Format(time.RFC3339Nano), future); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id, principal_id, principal_kind, capability, operations_json, scope, issued_at, expires_at, remaining_uses) VALUES('lease-1','agent-1','agent','workspace.write','["write"]','workspace:1',?,?,1)`, now.Format(time.RFC3339Nano), future); err != nil {
		t.Fatal(err)
	}

	store := New(db)
	cmd := CommandRecord{
		ID: "cmd-1", Type: "test", Version: "1",
		Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		Scope: "workspace:1", CorrelationID: "corr-1", Payload: []byte(`{}`), CreatedAt: now,
	}
	event := EventRecord{
		ID: "event-1", AggregateID: "aggregate-1", AggregateType: "fixture", AggregateVersion: 1,
		Type: "fixture.changed", Version: "1", Actor: cmd.Actor, CommandID: cmd.ID,
		CorrelationID: cmd.CorrelationID, TrustClass: contracts.TrustObserved, Payload: []byte(`{"ok":true}`), CreatedAt: now,
	}
	effect := &EffectRecord{
		ID: "effect-1", CommandID: cmd.ID, ActionIntentDigest: "sha256:abc", TargetAdapter: "fixture",
		CapabilityLeaseID: "lease-1", ApprovalID: "approval-1", State: "pending",
		RequestPayload: []byte(`{"do":"thing"}`), CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CommitTransition(ctx, cmd, 0, event, "approval-1", "lease-1", effect); err != nil {
		t.Fatal(err)
	}

	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval-1'`, 0)
	assertScalarInt(t, db, `SELECT remaining_uses FROM capability_leases WHERE lease_id='lease-1'`, 0)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM events WHERE event_id='event-1'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM effects WHERE effect_id='effect-1' AND state='pending'`, 1)
	assertScalarInt(t, db, `SELECT version FROM aggregate_versions WHERE aggregate_id='aggregate-1'`, 1)
}

func TestGoalsPublicationAbandonmentSerializesAgainstDispatch(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "fence.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Now().UTC()
	cmd := CommandRecord{ID: "step", Type: "goals-publication.step", Version: "1", Actor: contracts.PrincipalRef{ID: "publisher", Kind: "publisher"}, Scope: "fixed", CorrelationID: "execution", Payload: []byte(`{}`), CreatedAt: now}
	ev := EventRecord{ID: "step", AggregateID: "execution", AggregateType: "goals", AggregateVersion: 1, Type: "goals-publication.step-admitted", Version: "1", Actor: cmd.Actor, CommandID: "step", CorrelationID: "execution", Payload: []byte(`{}`), CreatedAt: now}
	effect := &EffectRecord{ID: "step", CommandID: "step", ActionIntentDigest: "sha256:abc", TargetAdapter: "goals-initial-github", TargetPrincipal: "publisher", State: "dispatched", RequestPayload: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
	if err = store.CommitTransition(ctx, cmd, 0, ev, "", "", effect); err != nil {
		t.Fatal(err)
	}
	abandon := CommandRecord{ID: "abandon", Type: "goals-publication.abandon", Version: "1", Actor: contracts.PrincipalRef{ID: "owner", Kind: "human"}, Scope: "fixed", CorrelationID: "execution", Payload: []byte(`{}`), CreatedAt: now}
	ae := EventRecord{ID: "abandon", AggregateID: "abandon", AggregateType: "goals-publication-abandonment", AggregateVersion: 1, Type: "goals-publication.abandoned", Version: "1", Actor: abandon.Actor, CommandID: "abandon", CorrelationID: "execution", Payload: []byte(`{}`), CreatedAt: now}
	if err = store.CommitGoalsPublicationAbandonment(ctx, abandon, ae, "execution"); err == nil {
		t.Fatal("in-flight mutation must prevent abandonment")
	}
	if _, err = db.ExecContext(ctx, `UPDATE effects SET state='unknown' WHERE effect_id='step'`); err != nil {
		t.Fatal(err)
	}
	if err = store.CommitGoalsPublicationAbandonment(ctx, abandon, ae, "execution"); err != nil {
		t.Fatal(err)
	}
	res, err := db.ExecContext(ctx, `UPDATE effects SET state='dispatched',attempts=attempts+1 WHERE effect_id='step' AND state='pending' AND NOT EXISTS (SELECT 1 FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id='execution')`)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := res.RowsAffected()
	if n != 0 {
		t.Fatal("abandoned execution passed dispatch fence")
	}
	assertScalarInt(t, db, `SELECT count(*) FROM events WHERE event_type='goals-publication.abandoned' AND correlation_id='execution'`, 1)
}

func TestCommitTransitionRollsBackAuthorityOnVersionConflict(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	future := now.Add(time.Hour).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id, approver_id, approver_kind, intent_digest, issued_at, expires_at, remaining_uses) VALUES('approval-1','human-1','human','sha256:abc',?,?,1)`, now.Format(time.RFC3339Nano), future); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO aggregate_versions(aggregate_id, aggregate_type, version) VALUES('aggregate-1','fixture',2)`); err != nil {
		t.Fatal(err)
	}

	store := New(db)
	cmd := CommandRecord{
		ID: "cmd-conflict", Type: "test", Version: "1",
		Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, Scope: "workspace:1",
		CorrelationID: "corr-conflict", Payload: []byte(`{}`), CreatedAt: now,
	}
	event := EventRecord{
		ID: "event-conflict", AggregateID: "aggregate-1", AggregateType: "fixture", AggregateVersion: 1,
		Type: "fixture.changed", Version: "1", Actor: cmd.Actor, CommandID: cmd.ID,
		CorrelationID: cmd.CorrelationID, Payload: []byte(`{}`), CreatedAt: now,
	}
	err = store.CommitTransition(ctx, cmd, 0, event, "approval-1", "", nil)
	if !errors.Is(err, ErrAggregateVersion) {
		t.Fatalf("expected aggregate version conflict, got %v", err)
	}

	// The failed optimistic transition must not consume approval or leave the
	// idempotency command behind.
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='approval-1'`, 1)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM commands WHERE command_id='cmd-conflict'`, 0)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM events WHERE event_id='event-conflict'`, 0)
	assertScalarInt(t, db, `SELECT version FROM aggregate_versions WHERE aggregate_id='aggregate-1'`, 2)
}

func assertScalarInt(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query %q: expected %d, got %d", query, want, got)
	}
}
