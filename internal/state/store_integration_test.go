package state

import (
	"context"
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
