package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestInvocationRuntimeBindingIsRequiredAndGenerationBound(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	store := New(db)
	manifest := activateFixturePackage(t, ctx, db, store, fixturePackage("runtime/pkg", "1", "", "runtime"), now)
	resolved, err := store.ResolveInvocationAlias(ctx, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RuntimeBinding.PackageDigest != manifest.ContentDigest || resolved.RuntimeBinding.ContractDigest != resolved.ContractDigest {
		t.Fatalf("binding did not preserve exact generation and contract: %+v", resolved.RuntimeBinding)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM invocation_runtime_bindings WHERE package_id=?`, manifest.PackageID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveInvocationAlias(ctx, "runtime"); err == nil {
		t.Fatal("missing runtime binding was executable")
	}
}

func TestInvocationRuntimeBindingRejectsSubstitutionAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := New(db)
	manifest := activateFixturePackage(t, ctx, db, store, fixturePackage("runtime/substitute", "1", "", "substitute"), now)
	if _, err := db.ExecContext(ctx, `UPDATE invocation_runtime_bindings SET runtime_digest=? WHERE package_id=?`, "sha256:"+repeatHex(64), manifest.PackageID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	reopened, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := New(reopened).ResolveInvocationAlias(ctx, "substitute"); err == nil {
		t.Fatal("substituted runtime binding survived restart")
	}
}

func repeatHex(n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += "f"
	}
	return result
}
