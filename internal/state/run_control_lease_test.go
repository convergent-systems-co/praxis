package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSQLiteRunControlLeaseSourceLoadsAndAuthorizesPersistedLease(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,required_enforcement_json,issued_at,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"lease-run-control", "operator-1", "user", kernel.RunControlCapability, []byte(`["cancel","resume"]`), "run:*", []byte(`["policy"]`), now.Add(-time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	store := New(db)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	leases, err := store.LeasesForPrincipal(ctx, actor, kernel.RunControlCapability)
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 {
		t.Fatalf("expected one lease, got %d", len(leases))
	}
	if len(leases[0].RequiredEnforcement) != 1 || leases[0].RequiredEnforcement[0] != "policy" {
		t.Fatalf("unexpected enforcement metadata: %#v", leases[0].RequiredEnforcement)
	}
	authorizer := kernel.LeaseRunControlAuthorizer{Leases: store, Now: func() time.Time { return now }}
	if err := authorizer.AuthorizeRunControl(ctx, actor, kernel.RunExecution{RunID: "run-9"}, kernel.RunControlCancel); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteRunControlLeaseSourcePreservesFiniteUseSemantics(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"lease-once", "operator-1", "user", kernel.RunControlCapability, []byte(`["cancel"]`), "run:run-1", now.Add(-time.Minute).Format(time.RFC3339Nano), 1); err != nil {
		t.Fatal(err)
	}
	store := New(db)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	leases, err := store.LeasesForPrincipal(ctx, actor, kernel.RunControlCapability)
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 || leases[0].RemainingUses == nil || *leases[0].RemainingUses != 1 {
		t.Fatalf("expected finite-use lease to be preserved, got %#v", leases)
	}
	authorizer := kernel.LeaseRunControlAuthorizer{Leases: store, Now: func() time.Time { return now }}
	if err := authorizer.AuthorizeRunControl(ctx, actor, kernel.RunExecution{RunID: "run-1"}, kernel.RunControlCancel); err == nil {
		t.Fatal("expected decision-only authorizer to reject finite-use lease")
	}
}
