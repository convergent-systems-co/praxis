package capability

import (
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func baseLease(now time.Time) contracts.CapabilityLease {
	return contracts.CapabilityLease{
		ID: "l1",
		Principal: contracts.PrincipalRef{ID: "plugin-1", Kind: "plugin"},
		Capability: "workspace.search.text",
		Operations: []string{"query"},
		Scope: "workspace:1:*",
		IssuedAt: now,
	}
}

func TestEvaluateAllowsBoundedScope(t *testing.T) {
	now := time.Now().UTC()
	lease := baseLease(now)
	err := Evaluate(lease, Request{Principal: lease.Principal, Capability: lease.Capability, Operation: "query", Scope: "workspace:1:src", Now: now})
	if err != nil { t.Fatalf("expected allowed request: %v", err) }
}

func TestEvaluateRejectsPrincipalSubstitution(t *testing.T) {
	now := time.Now().UTC()
	lease := baseLease(now)
	err := Evaluate(lease, Request{Principal: contracts.PrincipalRef{ID: "plugin-2", Kind: "plugin"}, Capability: lease.Capability, Operation: "query", Scope: "workspace:1:src", Now: now})
	if !errors.Is(err, ErrPrincipalMismatch) { t.Fatalf("expected principal mismatch, got %v", err) }
}

func TestEvaluateRejectsScopeEscape(t *testing.T) {
	now := time.Now().UTC()
	lease := baseLease(now)
	err := Evaluate(lease, Request{Principal: lease.Principal, Capability: lease.Capability, Operation: "query", Scope: "workspace:2:src", Now: now})
	if !errors.Is(err, ErrScopeDenied) { t.Fatalf("expected scope denial, got %v", err) }
}
