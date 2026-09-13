package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteAppliesMigrationsAndPragmas(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "2" {
		t.Fatalf("expected schema version 2, got %s", version)
	}
	assertScalarInt(t, db, `SELECT COUNT(*) FROM praxis_schema_migrations`, 2)

	assertPragmaInt(t, db, "foreign_keys", 1)
	assertPragmaInt(t, db, "busy_timeout", 5000)
	assertPragmaInt(t, db, "secure_delete", 1)

	var journal string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("expected WAL journal mode, got %s", journal)
	}
}

func TestOpenSQLiteDoesNotReapplyLedgeredMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	first, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	assertScalarInt(t, second, `SELECT COUNT(*) FROM praxis_schema_migrations`, 2)
	var boundInstanceColumns int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('capability_leases') WHERE name='bound_instance_id'`).Scan(&boundInstanceColumns); err != nil {
		t.Fatal(err)
	}
	if boundInstanceColumns != 1 {
		t.Fatalf("expected one bound_instance_id column, got %d", boundInstanceColumns)
	}
}

func assertPragmaInt(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, name string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("pragma %s: expected %d, got %d", name, want, got)
	}
}
