package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/scheduler"
)

func TestSchedulerResourceLeasesAreAtomicAndRecoverAfterRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	if err := store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "cpu", Capacity: 2}); err != nil {
		t.Fatal(err)
	}
	if err := store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "gpu", Capacity: 1, Exclusive: true}); err != nil {
		t.Fatal(err)
	}
	expiry := now.Add(time.Minute)
	leases, err := store.AcquireSchedulerResourceLeases(ctx, "slice-1", "attempt-1", []scheduler.ResourceRequirement{{Key: "gpu", Capacity: 1, Exclusive: true}, {Key: "cpu", Capacity: 1}}, now, &expiry)
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 2 || leases[0].ResourceKey != "cpu" || leases[1].ResourceKey != "gpu" {
		t.Fatalf("requirements were not deterministically acquired: %+v", leases)
	}
	if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice-2", "attempt-1", []scheduler.ResourceRequirement{{Key: "gpu", Capacity: 1, Exclusive: true}, {Key: "cpu", Capacity: 1}}, now, nil); err != ErrResourceUnavailable {
		t.Fatalf("expected atomic denial without partial lease, got %v", err)
	}
	recovered, err := store.RecoverSchedulerResourceLeases(ctx, now.Add(2*time.Minute))
	if err != nil || len(recovered) != 2 {
		t.Fatalf("expected both expired leases to be recovered, got %d %v", len(recovered), err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = New(db)
	if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice-3", "attempt-1", []scheduler.ResourceRequirement{{Key: "gpu", Capacity: 1, Exclusive: true}}, now.Add(2*time.Minute), nil); err != nil {
		t.Fatalf("recovered lease did not become available after restart: %v", err)
	}
}
