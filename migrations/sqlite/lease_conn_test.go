package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func openLike(t *testing.T, path string) *sql.DB {
	t.Helper()
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("_foreign_keys", "1")
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "WAL")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

// Two independent connections (as two processes would hold) must serialize
// the lease claim: the second claimant observes the first holder.
func TestMigrationLeaseClaimIsExclusiveAcrossConnections(t *testing.T) {
	ctx := context.Background()
	db := v12Database(t)
	path := ""
	db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path)
	db.Close()
	if path == "" {
		t.Skip("cannot resolve database path")
	}
	a, b := openLike(t, filepath.Clean(path)), openLike(t, filepath.Clean(path))
	defer a.Close()
	defer b.Close()
	now := time.Unix(1700000000, 0).UTC()
	plan, err := NewPlan(ctx, a, "sha256:bootstrap", "installation-owner:bootstrap", "authority:root", "sha256:root", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimMigrationLease(ctx, a, plan, plan.OwnerID, "holder-a", 0, now); err != nil {
		t.Fatal(err)
	}
	if _, err := claimMigrationLease(ctx, b, plan, plan.OwnerID, "holder-b", 0, now.Add(time.Second)); !errors.Is(err, ErrMigrationInProgress) {
		t.Fatalf("second connection must observe the first holder: %v", err)
	}
	if _, err := claimMigrationLease(ctx, a, plan, plan.OwnerID, "holder-a", 0, now.Add(2*time.Second)); err != nil {
		t.Fatalf("holder may renew its own claim: %v", err)
	}
	// A second preview of the same pending migrations has a different plan
	// digest; the installation-wide lease still excludes it.
	other, err := NewPlan(ctx, b, "sha256:bootstrap", "installation-owner:bootstrap", "authority:root", "sha256:root", now.Add(time.Minute))
	if err != nil || other.PlanDigest == plan.PlanDigest {
		t.Fatalf("expected a distinct plan digest: %v", err)
	}
	if _, err := claimMigrationLease(ctx, b, other, other.OwnerID, "holder-b", 0, now.Add(3*time.Second)); !errors.Is(err, ErrMigrationInProgress) {
		t.Fatalf("installation-wide lease must exclude a different plan digest: %v", err)
	}
	lease, err := ReadMaintenanceLease(ctx, b, "sha256:bootstrap")
	if err != nil || lease.State != "held" || lease.Holder != "holder-a" || lease.PlanDigest != plan.PlanDigest {
		t.Fatalf("maintenance lease: %+v %v", lease, err)
	}
	// A stale pre-claim view (ledger advanced by another holder) is refused
	// inside the claim transaction even when the lease is free.
	if err := releaseMigrationLease(ctx, a, Journal{PlanDigest: plan.PlanDigest, State: "committed"}, plan, plan.OwnerID, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ExecContext(ctx, `INSERT INTO praxis_schema_migrations(name,applied_at) VALUES(?,?)`, plan.Migrations[0].Name, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := claimMigrationLease(ctx, b, other, other.OwnerID, "holder-b", 0, now.Add(5*time.Second)); !errors.Is(err, ErrMigrationInProgress) {
		t.Fatalf("stale ledger view must be refused at the claim boundary: %v", err)
	}
}
