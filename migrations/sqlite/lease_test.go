package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrationMaintenanceLeaseIsExclusiveBoundedAndReleased(t *testing.T) {
	ctx := context.Background()
	db := v12Database(t)
	defer db.Close()
	now := time.Unix(1700000000, 0).UTC()
	plan, err := NewPlan(ctx, db, "sha256:bootstrap", "installation-owner:bootstrap", "authority:root", "sha256:root", now)
	if err != nil {
		t.Fatal(err)
	}
	// Another live process holds the lease: this process must fail closed
	// before touching the snapshot or the ledger.
	held, err := claimMigrationLease(ctx, db, plan, plan.OwnerID, "other-host:1:abcd", 0, now)
	if err != nil || held.Holder != "other-host:1:abcd" || held.State != "prepared" {
		t.Fatalf("claim: %+v %v", held, err)
	}
	snapshot := filepath.Join(t.TempDir(), "migration.snapshot")
	if _, err := ApplyPlan(ctx, db, plan, snapshot, plan.OwnerID, now.Add(time.Second)); !errors.Is(err, ErrMigrationInProgress) {
		t.Fatalf("expected ErrMigrationInProgress, got %v", err)
	}
	if status, err := StatusOf(ctx, db); err != nil || status.CurrentSchema != 12 {
		t.Fatalf("refused migrator must not change state: %+v %v", status, err)
	}
	// The lease is bounded: after expiry a new holder takes over from the
	// durable ledger, completes, and releases the lease on commit.
	journal, err := ApplyPlan(ctx, db, plan, snapshot, plan.OwnerID, now.Add(MigrationLeaseDuration+time.Second))
	if err != nil || journal.State != "committed" || journal.Holder != "" || journal.LeaseExpiresAt != nil {
		t.Fatalf("takeover after expiry: %+v %v", journal, err)
	}
	if status, err := StatusOf(ctx, db); err != nil || status.CurrentSchema != 19 {
		t.Fatalf("status after takeover: %+v %v", status, err)
	}
	// Exact replay after commit returns the committed journal without a lease.
	again, err := ApplyPlan(ctx, db, plan, snapshot, plan.OwnerID, now.Add(MigrationLeaseDuration+2*time.Second))
	if err != nil || again.State != "committed" || again.Holder != "" {
		t.Fatalf("replay: %+v %v", again, err)
	}
}
