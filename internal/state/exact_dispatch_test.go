package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func exactDispatchIntent() contracts.ActionIntent {
	params := map[string]string{"goal_ref": "goal:weather", "graph_id": "praxis.package.develop.default", "graph_version": "0.2.0", "node_id": "implement", "agent_id": "agent:weather", "agent_generation": "7", "run_id": "run:weather", "request_id": "request:weather", "route_record_id": "route:weather", "surface_id": "surface:local", "executor_id": "executor:local", "provider_id": "provider:local"}
	return contracts.ActionIntent{Version: "v1", ID: "intent:weather", Actor: contracts.PrincipalRef{ID: "agent:weather", Kind: "agent"}, Operation: ExactDispatchCapability, Target: "executor:executor:local@provider:provider:local", Parameters: params, Scope: "issued-route:route:weather", Preconditions: map[string]string{"request_id": "request:weather", "route_record_id": "route:weather"}}
}

func TestExactDispatchAuthorityConsumesBothGrantsAndCannotReplay(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	intent := exactDispatchIntent()
	grant, err := store.IssueExactDispatchAuthority(ctx, intent, contracts.PrincipalRef{ID: "human:thomas", Kind: "human"}, now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := store.AuthorizeExactDispatch(ctx, intent, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if invocation.ApprovalID != grant.ApprovalID || invocation.LeaseID != grant.LeaseID || invocation.EffectID == "" {
		t.Fatalf("unexpected invocation %#v", invocation)
	}
	assertScalarInt(t, db, `SELECT remaining_uses FROM approvals WHERE approval_id='`+grant.ApprovalID+`'`, 0)
	assertScalarInt(t, db, `SELECT remaining_uses FROM capability_leases WHERE lease_id='`+grant.LeaseID+`'`, 0)
	assertScalarInt(t, db, `SELECT COUNT(*) FROM effects WHERE effect_id='`+invocation.EffectID+`' AND state='pending'`, 1)
	if _, err := store.AuthorizeExactDispatch(ctx, intent, now.Add(2*time.Minute)); err == nil {
		t.Fatal("consumed exact authority replayed")
	}
}

func TestExactDispatchAuthorityRejectsMutationRevocationAndExpiry(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		mutate func(*Store, contracts.ActionIntent, string, time.Time) contracts.ActionIntent
	}{
		{name: "identity", mutate: func(_ *Store, in contracts.ActionIntent, _ string, _ time.Time) contracts.ActionIntent {
			in.Parameters["node_id"] = "review"
			return in
		}},
		{name: "revoked", mutate: func(s *Store, in contracts.ActionIntent, digest string, now time.Time) contracts.ActionIntent {
			if err := s.RevokeExactDispatchAuthority(ctx, digest, now); err != nil {
				t.Fatal(err)
			}
			return in
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := New(db)
			now := time.Now().UTC().Truncate(time.Microsecond)
			intent := exactDispatchIntent()
			grant, err := store.IssueExactDispatchAuthority(ctx, intent, contracts.PrincipalRef{ID: "human:thomas", Kind: "human"}, now, now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			intent = tc.mutate(store, intent, grant.IntentDigest, now.Add(time.Minute))
			if _, err := store.AuthorizeExactDispatch(ctx, intent, now.Add(2*time.Minute)); err == nil {
				t.Fatal("invalid exact authority dispatched")
			}
		})
	}
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "expired.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	intent := exactDispatchIntent()
	if _, err := store.IssueExactDispatchAuthority(ctx, intent, contracts.PrincipalRef{ID: "human:thomas", Kind: "human"}, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthorizeExactDispatch(ctx, intent, now.Add(2*time.Minute)); !errors.Is(err, ErrExactDispatchAuthorityUnavailable) {
		t.Fatalf("expected expired authority denial, got %v", err)
	}
}
