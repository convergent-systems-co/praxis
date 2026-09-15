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
	if version != "13" {
		t.Fatalf("expected schema version 13, got %s", version)
	}
	for _, migration := range []string{"0001_praxis2_core.sql", "0004_packages_and_invocations.sql", "0005_package_contents.sql", "0006_governed_package_activation.sql", "0007_governed_package_transitions.sql", "0008_verified_package_artifacts.sql", "0009_package_rollback_receipts.sql", "0010_plugin_supervisor_snapshots.sql", "0011_scheduler_resource_leases.sql", "0012_invocation_runtime_bindings.sql", "0013_executable_invocation_bindings.sql"} {
		assertScalarInt(t, db, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name='`+migration+`'`, 1)
	}

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
	assertScalarInt(t, second, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name='0006_governed_package_activation.sql'`, 1)
	assertScalarInt(t, second, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name='0007_governed_package_transitions.sql'`, 1)
	assertScalarInt(t, second, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name='0008_verified_package_artifacts.sql'`, 1)
	assertScalarInt(t, second, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name='0009_package_rollback_receipts.sql'`, 1)
	assertScalarInt(t, second, `SELECT COUNT(*) FROM pragma_table_info('package_contents') WHERE name='artifact_bytes'`, 1)
	assertScalarInt(t, second, `SELECT COUNT(*) FROM pragma_table_info('package_activation_receipts') WHERE name='artifact_bytes'`, 1)
	var boundInstanceColumns int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('capability_leases') WHERE name='bound_instance_id'`).Scan(&boundInstanceColumns); err != nil {
		t.Fatal(err)
	}
	if boundInstanceColumns != 1 {
		t.Fatalf("expected one bound_instance_id column, got %d", boundInstanceColumns)
	}
	var secureBlobTables int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='secure_blobs'`).Scan(&secureBlobTables); err != nil {
		t.Fatal(err)
	}
	if secureBlobTables != 1 {
		t.Fatalf("expected secure_blobs table exactly once, got %d", secureBlobTables)
	}
	var packageTables int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='installed_packages'`).Scan(&packageTables); err != nil {
		t.Fatal(err)
	}
	if packageTables != 1 {
		t.Fatalf("expected installed_packages table exactly once, got %d", packageTables)
	}
	var contentTables int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='package_contents'`).Scan(&contentTables); err != nil {
		t.Fatal(err)
	}
	if contentTables != 1 {
		t.Fatalf("expected package_contents table exactly once, got %d", contentTables)
	}
	var activationReceiptTables int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='package_activation_receipts'`).Scan(&activationReceiptTables); err != nil {
		t.Fatal(err)
	}
	if activationReceiptTables != 1 {
		t.Fatalf("expected governed package activation receipt table, got %d", activationReceiptTables)
	}
	var transitionReceiptTables int
	if err := second.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='package_transition_receipts'`).Scan(&transitionReceiptTables); err != nil {
		t.Fatal(err)
	}
	if transitionReceiptTables != 1 {
		t.Fatalf("expected governed package transition receipt table, got %d", transitionReceiptTables)
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
