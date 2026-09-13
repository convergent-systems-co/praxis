package state

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteReadOnlyCannotMutateAuthoritativeState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	writable, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writable.ExecContext(ctx, `INSERT INTO schema_meta(key,value) VALUES('readonly-proof','present')`); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}

	readonly, err := OpenSQLiteReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	var value string
	if err := readonly.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key='readonly-proof'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "present" {
		t.Fatalf("unexpected read value %q", value)
	}
	if _, err := readonly.ExecContext(ctx, `UPDATE schema_meta SET value='changed' WHERE key='readonly-proof'`); err == nil {
		t.Fatal("expected query-only connection to reject mutation")
	}
}
