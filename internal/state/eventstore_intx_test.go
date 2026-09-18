package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func openIntxTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func intxEvent(id, commandID string, actor contracts.PrincipalRef) eventstore.Event {
	return eventstore.Event{ID: id, AggregateType: "lifecycle_transition", Type: "lifecycle.transition", Version: "v1", Actor: actor, CommandID: commandID, CorrelationID: "plan-1", Trust: contracts.TrustPolicy, Payload: []byte(`{"n":1}`), CreatedAt: time.Now().UTC()}
}

// TestAppendInTxNeverOpensOrCommitsItsOwnTransaction proves the tx-scoped
// primitive genuinely uses only the caller-supplied *sql.Tx: a rollback
// after AppendInTx returns leaves nothing durable, and a commit makes the
// event durable — exactly the atomicity PLAN-016 WU3 requires a governed
// Apply boundary to be built on.
func TestAppendInTxNeverOpensOrCommitsItsOwnTransaction(t *testing.T) {
	ctx := context.Background()
	db := openIntxTestDB(t)
	store := NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendInTx(ctx, tx, "agg-rollback", 0, []eventstore.Event{intxEvent("e1", "cmd-1", actor)}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadAggregate(ctx, "agg-rollback", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("event committed despite caller rolling back its own transaction: %+v", loaded)
	}

	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	appended, err := store.AppendInTx(ctx, tx2, "agg-commit", 0, []eventstore.Event{intxEvent("e2", "cmd-2", actor)})
	if err != nil {
		t.Fatal(err)
	}
	if len(appended) != 1 || appended[0].AggregateVersion != 1 {
		t.Fatalf("unexpected append metadata: %+v", appended)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	loaded2, err := store.LoadAggregate(ctx, "agg-commit", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded2) != 1 || loaded2[0].ID != "e2" {
		t.Fatalf("event not durable after caller committed: %+v", loaded2)
	}
}

func TestAppendInTxPreservesCommandProvenance(t *testing.T) {
	ctx := context.Background()
	db := openIntxTestDB(t)
	store := NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendInTx(ctx, tx, "agg-1", 0, []eventstore.Event{intxEvent("e1", "cmd-provenance", actor)}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command_id='cmd-provenance'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one command provenance row from AppendInTx, got %d", count)
	}
}

func TestAppendInTxEnforcesExpectedVersionSameAsAppend(t *testing.T) {
	ctx := context.Background()
	db := openIntxTestDB(t)
	store := NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := store.AppendInTx(ctx, tx, "agg-1", 0, []eventstore.Event{intxEvent("e1", "cmd-1", actor)}); err != nil {
		t.Fatal(err)
	}
	// Stale expected version within the same open transaction must be
	// rejected exactly as Append rejects it outside one.
	if _, err := store.AppendInTx(ctx, tx, "agg-1", 0, []eventstore.Event{intxEvent("e2", "cmd-2", actor)}); !errors.Is(err, eventstore.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestAppendInTxRequiresAnActiveTransaction(t *testing.T) {
	ctx := context.Background()
	db := openIntxTestDB(t)
	store := NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	if _, err := store.AppendInTx(ctx, nil, "agg-1", 0, []eventstore.Event{intxEvent("e1", "cmd-1", actor)}); err == nil {
		t.Fatal("expected error for nil transaction")
	}
	if _, err := store.LoadAggregateInTx(ctx, nil, "agg-1", 0); err == nil {
		t.Fatal("expected error for nil transaction")
	}
}

// TestLoadAggregateInTxMatchesLoadAggregateAndAvoidsPoolDeadlock proves
// LoadAggregateInTx reads through the same connection the caller's open
// transaction already holds (never a second pooled connection, which this
// store's single-connection pool would deadlock on) by successfully
// performing both a read and a write through one open transaction before
// ever committing, then verifies the result matches an ordinary
// LoadAggregate read after commit.
func TestLoadAggregateInTxMatchesLoadAggregateAndAvoidsPoolDeadlock(t *testing.T) {
	ctx := context.Background()
	db := openIntxTestDB(t)
	store := NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.LoadAggregateInTx(ctx, tx, "agg-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("expected empty history before first append: %+v", before)
	}
	if _, err := store.AppendInTx(ctx, tx, "agg-1", int64(len(before)), []eventstore.Event{intxEvent("e1", "cmd-1", actor)}); err != nil {
		t.Fatal(err)
	}
	during, err := store.LoadAggregateInTx(ctx, tx, "agg-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(during) != 1 || during[0].ID != "e1" {
		t.Fatalf("tx-scoped read did not observe the tx-scoped write before commit: %+v", during)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	after, err := store.LoadAggregate(ctx, "agg-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != during[0].ID || after[0].AggregateVersion != during[0].AggregateVersion {
		t.Fatalf("LoadAggregate after commit does not match LoadAggregateInTx before commit: %+v vs %+v", after, during)
	}
}

func TestAppendInTxRejectsIllegalEventsIdenticallyToAppend(t *testing.T) {
	ctx := context.Background()
	db := openIntxTestDB(t)
	store := NewSQLiteEventStore(db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := store.AppendInTx(ctx, tx, "agg-1", 0, []eventstore.Event{{ID: "", AggregateType: "lifecycle_transition", Type: "lifecycle.transition", Version: "v1"}}); err == nil {
		t.Fatal("expected rejection of event missing identity/metadata")
	}
}
