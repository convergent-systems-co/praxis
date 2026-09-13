package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/projection"
)

func TestSQLiteCheckpointStorePersistsMonotonically(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewSQLiteCheckpointStore(db)
	checkpoint := projection.Checkpoint{
		Name: "runs", Version: "1", Consistency: projection.StrongCheckpointed,
		LastSequence: 5, UpdatedAt: time.Unix(5, 0).UTC(),
	}
	if err := store.SaveCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.LoadCheckpoint(ctx, "runs", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.LastSequence != 5 || loaded.Consistency != projection.StrongCheckpointed {
		t.Fatalf("unexpected checkpoint: %+v ok=%v", loaded, ok)
	}

	stale := checkpoint
	stale.LastSequence = 4
	if err := store.SaveCheckpoint(ctx, stale); err == nil {
		t.Fatal("stale checkpoint must fail closed")
	}
	changedClass := checkpoint
	changedClass.LastSequence = 6
	changedClass.Consistency = projection.Eventual
	if err := store.SaveCheckpoint(ctx, changedClass); err == nil {
		t.Fatal("consistency class change must fail closed")
	}
	advanced := checkpoint
	advanced.LastSequence = 6
	if err := store.SaveCheckpoint(ctx, advanced); err != nil {
		t.Fatal(err)
	}
}
