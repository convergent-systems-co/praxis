package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func v12Database(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	descriptors, bodies, err := migrationDescriptors()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range descriptors {
		if migration.ToSchema > 12 {
			break
		}
		if _, err := db.Exec(string(bodies[migration.Name])); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS praxis_schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
			db.Close()
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO praxis_schema_migrations(name,applied_at) VALUES(?,?)`, migration.Name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	return db
}

func TestPlanDiscoversExactV12ToV18AndAppliesOnlyBoundPlan(t *testing.T) {
	ctx := context.Background()
	db := v12Database(t)
	defer db.Close()
	status, err := StatusOf(ctx, db)
	if err != nil || status.CurrentSchema != 12 || len(status.Pending) != 7 {
		t.Fatalf("unexpected status: %+v %v", status, err)
	}
	plan, err := NewPlan(ctx, db, "sha256:bootstrap", "installation-owner:bootstrap", "authority:root", "sha256:root", time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Verify(); err != nil || plan.TargetSchema != 19 || len(plan.Migrations) != 7 {
		t.Fatalf("invalid exact plan: %+v %v", plan, err)
	}
	snapshot := filepath.Join(t.TempDir(), "migration.snapshot")
	journal, err := ApplyPlan(ctx, db, plan, snapshot, plan.OwnerID, time.Unix(1700000001, 0).UTC())
	if err != nil || journal.State != "committed" {
		t.Fatalf("migration failed: %+v %v", journal, err)
	}
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatal(err)
	}
	status, err = StatusOf(ctx, db)
	if err != nil || status.CurrentSchema != 19 || len(status.Pending) != 0 {
		t.Fatalf("migration did not reach exact target: %+v %v", status, err)
	}
	replayed, err := ApplyPlan(ctx, db, plan, snapshot, plan.OwnerID, time.Unix(1700000002, 0).UTC())
	if err != nil || replayed.State != "committed" || replayed.PlanDigest != journal.PlanDigest {
		t.Fatalf("exact migration replay was not idempotent: %+v %v", replayed, err)
	}
}

func TestPlanRejectsMigrationOrderOrContentSubstitution(t *testing.T) {
	ctx := context.Background()
	db := v12Database(t)
	defer db.Close()
	plan, err := NewPlan(ctx, db, "sha256:bootstrap", "installation-owner:bootstrap", "authority:root", "sha256:root", time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	plan.Migrations[0].Digest = "sha256:" + string(make([]byte, 64))
	plan.PlanDigest, _ = plan.Digest()
	if _, err := ApplyPlan(ctx, db, plan, filepath.Join(t.TempDir(), "snapshot"), plan.OwnerID, time.Now().UTC()); err == nil {
		t.Fatal("substituted migration content must fail closed")
	}
}
