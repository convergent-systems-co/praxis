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
	if _, err := db.ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES('cmd-1','run','1','agent-1','agent','run:1','corr-1',X'7B7D','processing',?)`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

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
