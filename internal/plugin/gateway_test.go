package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type recordingTransport struct {
	calls    int
	instance InstanceIdentity
}

func (r *recordingTransport) Call(_ context.Context, instance InstanceIdentity, _ string, payload []byte) ([]byte, error) {
	r.calls++
	r.instance = instance
	return append([]byte("ok:"), payload...), nil
}

func TestGatewayDispatchRequiresExactLeaseBoundInstance(t *testing.T) {
	registry := NewRegistry()
	provider := fixtureProvider("provider", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	uses := uint64(1)
	principal := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	binding := LeaseBinding{
		Lease: contracts.CapabilityLease{
			ID: "lease-1", Principal: principal, Capability: "workspace.search.text",
			Operations: []string{"search"}, Scope: "workspace:repo-a", IssuedAt: now,
			ExpiresAt: &expires, RemainingUses: &uses,
		},
		InstanceID: provider.Identity.InstanceID,
		SessionID:  provider.Identity.RuntimeSession,
	}
	transport := &recordingTransport{}
	gateway := Gateway{Registry: registry, Transport: transport}
	response, err := gateway.Dispatch(context.Background(), DispatchRequest{
		Principal: principal, Capability: "workspace.search.text", Operation: "search", Scope: "workspace:repo-a",
		Binding: binding, Payload: []byte("needle"), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "ok:needle" || transport.calls != 1 || transport.instance.InstanceID != provider.Identity.InstanceID {
		t.Fatalf("unexpected dispatch response=%q calls=%d instance=%+v", response, transport.calls, transport.instance)
	}
}

func TestGatewayRejectsRestartedSessionUsingStaleLease(t *testing.T) {
	registry := NewRegistry()
	provider := fixtureProvider("provider", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	principal := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	binding := LeaseBinding{
		Lease: contracts.CapabilityLease{ID: "lease-1", Principal: principal, Capability: "workspace.search.text", Operations: []string{"search"}, Scope: "workspace:repo-a", IssuedAt: now},
		InstanceID: provider.Identity.InstanceID,
		SessionID:  "old-session",
	}
	transport := &recordingTransport{}
	gateway := Gateway{Registry: registry, Transport: transport}
	_, err := gateway.Dispatch(context.Background(), DispatchRequest{Principal: principal, Capability: "workspace.search.text", Operation: "search", Scope: "workspace:repo-a", Binding: binding, Now: now})
	if err == nil {
		t.Fatal("stale runtime session must not dispatch")
	}
	if transport.calls != 0 {
		t.Fatal("transport must not be reached on stale lease binding")
	}
}

func TestGatewayRejectsUnauthorizedOperationBeforeTransport(t *testing.T) {
	registry := NewRegistry()
	provider := fixtureProvider("provider", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	principal := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	binding := LeaseBinding{
		Lease: contracts.CapabilityLease{ID: "lease-1", Principal: principal, Capability: "workspace.search.text", Operations: []string{"search"}, Scope: "workspace:repo-a", IssuedAt: now},
		InstanceID: provider.Identity.InstanceID, SessionID: provider.Identity.RuntimeSession,
	}
	transport := &recordingTransport{}
	gateway := Gateway{Registry: registry, Transport: transport}
	_, err := gateway.Dispatch(context.Background(), DispatchRequest{Principal: principal, Capability: "workspace.search.text", Operation: "delete", Scope: "workspace:repo-a", Binding: binding, Now: now})
	if err == nil {
		t.Fatal("unauthorized operation must fail")
	}
	if transport.calls != 0 {
		t.Fatal("transport must not be reached on denied operation")
	}
}

func TestGatewayFailsIfBoundProviderNotEligible(t *testing.T) {
	registry := NewRegistry()
	provider := fixtureProvider("provider", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	principal := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	binding := LeaseBinding{
		Lease: contracts.CapabilityLease{ID: "lease-1", Principal: principal, Capability: "workspace.search.text", Operations: []string{"search"}, Scope: "workspace:repo-a", IssuedAt: now},
		InstanceID: provider.Identity.InstanceID, SessionID: provider.Identity.RuntimeSession,
	}
	transport := &recordingTransport{}
	gateway := Gateway{Registry: registry, Transport: transport}
	_, err := gateway.Dispatch(context.Background(), DispatchRequest{
		Principal: principal, Capability: "workspace.search.text", Operation: "search", Scope: "workspace:repo-a",
		RequiredIsolation: []IsolationProperty{IsolationNetwork}, Binding: binding, Now: now,
	})
	if !errors.Is(err, ErrNoEligibleProvider) {
		t.Fatalf("expected no eligible provider, got %v", err)
	}
	if transport.calls != 0 {
		t.Fatal("transport must not be reached without required isolation")
	}
}
