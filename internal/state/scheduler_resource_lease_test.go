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

func TestSchedulerResourceLeaseAdmissionIsIdempotentForRetriedAttempt(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	if err := store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "cpu", Capacity: 2}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 17, 0, 0, 0, time.UTC)
	requirements := []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}}
	first, err := store.AcquireSchedulerResourceLeases(ctx, "slice-retry", "attempt-1", requirements, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AcquireSchedulerResourceLeases(ctx, "slice-retry", "attempt-1", requirements, now.Add(time.Second), nil)
	if err != nil {
		t.Fatalf("retry of committed admission must return the existing lease: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID {
		t.Fatalf("retry created or returned the wrong lease set: first=%+v second=%+v", first, second)
	}
	var active int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_resource_leases WHERE released_at IS NULL`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("idempotent retry must not allocate a second lease, got %d active leases", active)
	}
}

func TestSchedulerResourceLeaseRetryRejectsMismatchedRequirements(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	if err := store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "cpu", Capacity: 2}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 17, 0, 0, 0, time.UTC)
	if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice-mismatch", "attempt-1", []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}}, now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice-mismatch", "attempt-1", []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 2}}, now.Add(time.Second), nil); err == nil {
		t.Fatal("retry with different requirements must fail closed")
	}
}

func TestSchedulerResourceLeaseCancellationReleasesOnlyExactAttempt(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	if err := store.DefineSchedulerResource(ctx, scheduler.ResourceState{Key: "cpu", Capacity: 2}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice-cancel", "attempt-a", []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}}, now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireSchedulerResourceLeases(ctx, "slice-cancel", "attempt-b", []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}}, now, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseSchedulerResourceLeasesForAttempt(ctx, "slice-cancel", "attempt-a", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var activeA, activeB int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_resource_leases WHERE slice_id=? AND attempt_id=? AND released_at IS NULL`, "slice-cancel", "attempt-a").Scan(&activeA); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scheduler_resource_leases WHERE slice_id=? AND attempt_id=? AND released_at IS NULL`, "slice-cancel", "attempt-b").Scan(&activeB); err != nil {
		t.Fatal(err)
	}
	if activeA != 0 || activeB != 1 {
		t.Fatalf("cancellation must release only the exact attempt: activeA=%d activeB=%d", activeA, activeB)
	}
	if err := store.ReleaseSchedulerResourceLeasesForAttempt(ctx, "slice-cancel", "attempt-a", now.Add(2*time.Second)); err != nil {
		t.Fatalf("repeated cancellation must be idempotent: %v", err)
	}
}
