package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type staticRunControlLeases []contracts.CapabilityLease

func (s staticRunControlLeases) LeasesForPrincipal(_ context.Context, _ contracts.PrincipalRef, _ string) ([]contracts.CapabilityLease, error) {
	return append([]contracts.CapabilityLease(nil), s...), nil
}

func TestLeaseRunControlAuthorizerRequiresExactOperationAndScope(t *testing.T) {
	now := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	lease := contracts.CapabilityLease{
		ID: "lease-1", Principal: actor, Capability: RunControlCapability,
		Operations: []string{string(RunControlCancel)}, Scope: "run:run-1", IssuedAt: now.Add(-time.Minute),
	}
	authorizer := LeaseRunControlAuthorizer{Leases: staticRunControlLeases{lease}, Now: func() time.Time { return now }}
	if err := authorizer.AuthorizeRunControl(context.Background(), actor, RunExecution{RunID: "run-1"}, RunControlCancel); err != nil {
		t.Fatal(err)
	}
	if err := authorizer.AuthorizeRunControl(context.Background(), actor, RunExecution{RunID: "run-1"}, RunControlResume); err == nil {
		t.Fatal("expected ungranted resume operation to be denied")
	}
	if err := authorizer.AuthorizeRunControl(context.Background(), actor, RunExecution{RunID: "run-2"}, RunControlCancel); err == nil {
		t.Fatal("expected different run scope to be denied")
	}
}

func TestLeaseRunControlAuthorizerSupportsExplicitHierarchicalScope(t *testing.T) {
	now := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	lease := contracts.CapabilityLease{
		ID: "lease-all-runs", Principal: actor, Capability: RunControlCapability,
		Operations: []string{string(RunControlCancel), string(RunControlResume)}, Scope: "run:*", IssuedAt: now.Add(-time.Minute),
	}
	authorizer := LeaseRunControlAuthorizer{Leases: staticRunControlLeases{lease}, Now: func() time.Time { return now }}
	if err := authorizer.AuthorizeRunControl(context.Background(), actor, RunExecution{RunID: "run-99"}, RunControlResume); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseRunControlAuthorizerRejectsFiniteUseLeaseWithoutAtomicConsumption(t *testing.T) {
	now := time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	uses := uint64(1)
	lease := contracts.CapabilityLease{
		ID: "lease-once", Principal: actor, Capability: RunControlCapability,
		Operations: []string{string(RunControlCancel)}, Scope: "run:run-1", IssuedAt: now.Add(-time.Minute), RemainingUses: &uses,
	}
	authorizer := LeaseRunControlAuthorizer{Leases: staticRunControlLeases{lease}, Now: func() time.Time { return now }}
	if err := authorizer.AuthorizeRunControl(context.Background(), actor, RunExecution{RunID: "run-1"}, RunControlCancel); err == nil {
		t.Fatal("expected finite-use lease to fail closed without atomic consumption")
	}
}
