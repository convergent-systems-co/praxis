package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSQLiteEventStoreAppendLoadAndConflict(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	store := NewSQLiteEventStore(db)
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	proposed := []eventstore.Event{
		{ID: "e1", AggregateType: "run", Type: "run.started", Version: "1", Actor: actor, CommandID: "cmd-1", CorrelationID: "corr-1", Trust: contracts.TrustObserved, Payload: []byte(`{"n":1}`), CreatedAt: now},
		{ID: "e2", AggregateType: "run", Type: "run.transitioned", Version: "1", Actor: actor, CommandID: "cmd-1", CorrelationID: "corr-1", Trust: contracts.TrustObserved, Payload: []byte(`{"n":2}`), CreatedAt: now},
	}
	appended, err := store.Append(ctx, "run-1", 0, proposed)
	if err != nil {
		t.Fatal(err)
	}
	if len(appended) != 2 || appended[0].AggregateVersion != 1 || appended[1].AggregateVersion != 2 || appended[0].Sequence <= 0 || appended[1].Sequence <= appended[0].Sequence {
		t.Fatalf("unexpected append metadata: %+v", appended)
	}
	var commandCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command_id='cmd-1'`).Scan(&commandCount); err != nil {
		t.Fatal(err)
	}
	if commandCount != 1 {
		t.Fatalf("expected one command provenance row, got %d", commandCount)
	}

	loaded, err := store.LoadAggregate(ctx, "run-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[1].ID != "e2" || loaded[1].Trust != contracts.TrustObserved {
		t.Fatalf("unexpected aggregate replay: %+v", loaded)
	}
	from, err := store.ReadFrom(ctx, appended[0].Sequence, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(from) != 1 || from[0].ID != "e2" {
		t.Fatalf("unexpected sequence read: %+v", from)
	}

	_, err = store.Append(ctx, "run-1", 0, []eventstore.Event{{ID: "stale", AggregateType: "run", Type: "run.started", Version: "1", Actor: actor, CommandID: "cmd-1", CorrelationID: "corr-1", Payload: []byte(`{}`)}})
	if !errors.Is(err, eventstore.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestSQLiteEventStoreRejectsCommandIDMetadataHijack(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteEventStore(db)
	original := eventstore.Event{ID: "e1", AggregateType: "run", Type: "run.started", Version: "1", Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, CommandID: "cmd-shared", CorrelationID: "corr-1", Payload: []byte(`{}`)}
	if _, err := store.Append(ctx, "run-a", 0, []eventstore.Event{original}); err != nil {
		t.Fatal(err)
	}
	hijack := eventstore.Event{ID: "e2", AggregateType: "run", Type: "run.started", Version: "1", Actor: contracts.PrincipalRef{ID: "agent-2", Kind: "agent"}, CommandID: "cmd-shared", CorrelationID: "corr-1", Payload: []byte(`{}`)}
	if _, err := store.Append(ctx, "run-b", 0, []eventstore.Event{hijack}); err == nil {
		t.Fatal("expected reused command metadata mismatch to fail")
	}
	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE aggregate_id='run-b'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("expected failed hijack transaction to roll back, got %d events", eventCount)
	}
}
