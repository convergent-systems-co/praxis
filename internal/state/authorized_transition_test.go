package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestCommitTransitionAuthorizedLeaseConsumesOneShotAuthorityAtomically(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	store := New(db)
	now := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"lease-once", actor.ID, actor.Kind, "run.control", []byte(`["cancel"]`), "run:run-1", now.Add(-time.Minute).Format(time.RFC3339Nano), 1); err != nil { t.Fatal(err) }

	cmd := CommandRecord{ID: "cmd-1", Type: "run.control", Version: "1", Actor: actor, Scope: "run:run-1", CorrelationID: "corr-1", Payload: []byte(`{}`), CreatedAt: now}
	event := EventRecord{ID: "run-1:000001", AggregateID: "run-1", AggregateType: "run", AggregateVersion: 1, Type: "run.terminal", Version: "1", Actor: actor, CommandID: cmd.ID, CorrelationID: cmd.CorrelationID, TrustClass: contracts.TrustObserved, Payload: []byte(`{"kind":"terminal"}`), CreatedAt: now}
	leaseID, err := store.CommitTransitionAuthorizedLease(ctx, cmd, 0, event, capability.Request{Principal: actor, Capability: "run.control", Operation: "cancel", Scope: "run:run-1"})
	if err != nil { t.Fatal(err) }
	if leaseID != "lease-once" { t.Fatalf("expected lease-once, got %q", leaseID) }
	var remaining, version int64
	if err := db.QueryRowContext(ctx, `SELECT remaining_uses,version FROM capability_leases WHERE lease_id='lease-once'`).Scan(&remaining, &version); err != nil { t.Fatal(err) }
	if remaining != 0 || version != 2 { t.Fatalf("expected consumed lease remaining=0 version=2, got %d/%d", remaining, version) }
	var events int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE aggregate_id='run-1'`).Scan(&events); err != nil { t.Fatal(err) }
	if events != 1 { t.Fatalf("expected one committed event, got %d", events) }
}

func TestCommitTransitionAuthorizedLeaseDeniedRequestRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	store := New(db)
	now := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"lease-resume", actor.ID, actor.Kind, "run.control", []byte(`["resume"]`), "run:run-1", now.Add(-time.Minute).Format(time.RFC3339Nano), 1); err != nil { t.Fatal(err) }
	cmd := CommandRecord{ID: "cmd-denied", Type: "run.control", Version: "1", Actor: actor, Scope: "run:run-1", CorrelationID: "corr-1", Payload: []byte(`{}`), CreatedAt: now}
	event := EventRecord{ID: "run-1:000001", AggregateID: "run-1", AggregateType: "run", AggregateVersion: 1, Type: "run.terminal", Version: "1", Actor: actor, CommandID: cmd.ID, CorrelationID: cmd.CorrelationID, TrustClass: contracts.TrustObserved, Payload: []byte(`{}`), CreatedAt: now}
	_, err = store.CommitTransitionAuthorizedLease(ctx, cmd, 0, event, capability.Request{Principal: actor, Capability: "run.control", Operation: "cancel", Scope: "run:run-1"})
	if !errors.Is(err, ErrLeaseUnavailable) { t.Fatalf("expected lease unavailable, got %v", err) }
	for _, query := range []string{`SELECT COUNT(*) FROM commands WHERE command_id='cmd-denied'`, `SELECT COUNT(*) FROM events WHERE aggregate_id='run-1'`, `SELECT COUNT(*) FROM aggregate_versions WHERE aggregate_id='run-1'`} {
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil { t.Fatal(err) }
		if count != 0 { t.Fatalf("denied transition left durable state for query %q: %d", query, count) }
	}
	var remaining int64
	if err := db.QueryRowContext(ctx, `SELECT remaining_uses FROM capability_leases WHERE lease_id='lease-resume'`).Scan(&remaining); err != nil { t.Fatal(err) }
	if remaining != 1 { t.Fatalf("denied transition consumed lease: %d", remaining) }
}

func TestCommitTransitionAuthorizedLeaseRejectsRevokedLease(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	store := New(db)
	now := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,revoked_at,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"lease-revoked", actor.ID, actor.Kind, "run.control", []byte(`["cancel"]`), "run:run-1", now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil { t.Fatal(err) }
	cmd := CommandRecord{ID: "cmd-revoked", Type: "run.control", Version: "1", Actor: actor, Scope: "run:run-1", CorrelationID: "corr-1", Payload: []byte(`{}`), CreatedAt: now}
	event := EventRecord{ID: "run-1:000001", AggregateID: "run-1", AggregateType: "run", AggregateVersion: 1, Type: "run.terminal", Version: "1", Actor: actor, CommandID: cmd.ID, CorrelationID: cmd.CorrelationID, Payload: []byte(`{}`), CreatedAt: now}
	if _, err := store.CommitTransitionAuthorizedLease(ctx, cmd, 0, event, capability.Request{Principal: actor, Capability: "run.control", Operation: "cancel", Scope: "run:run-1"}); !errors.Is(err, ErrLeaseUnavailable) {
		t.Fatalf("expected revoked lease rejection, got %v", err)
	}
}
