package plugin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type Transport interface {
	Call(ctx context.Context, instance InstanceIdentity, operation string, payload []byte) ([]byte, error)
}

type DispatchRequest struct {
	Principal         contracts.PrincipalRef
	Capability        string
	Operation         string
	Scope             string
	RequiredIsolation []IsolationProperty
	Binding           LeaseBinding
	Payload           []byte
	Now               time.Time
}

type Gateway struct {
	Registry  *Registry
	Transport Transport
}

// Dispatch is the deterministic boundary before a plugin transport call. It
// requires both provider eligibility and a lease bound to the exact instance
// and runtime session selected for the call.
func (g Gateway) Dispatch(ctx context.Context, req DispatchRequest) ([]byte, error) {
	if g.Registry == nil || g.Transport == nil {
		return nil, errors.New("plugin registry and transport are required")
	}
	if err := req.Principal.Validate(); err != nil {
		return nil, fmt.Errorf("dispatch principal: %w", err)
	}
	if req.Capability == "" || req.Operation == "" || req.Scope == "" {
		return nil, errors.New("dispatch capability, operation, and scope are required")
	}
	providers, err := g.Registry.Resolve(req.Capability, req.RequiredIsolation)
	if err != nil {
		return nil, err
	}
	var selected *Provider
	for i := range providers {
		if providers[i].Identity.InstanceID == req.Binding.InstanceID && providers[i].Identity.RuntimeSession == req.Binding.SessionID {
			candidate := providers[i]
			selected = &candidate
			break
		}
	}
	if selected == nil {
		return nil, errors.New("lease-bound plugin instance is not an eligible provider")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := req.Binding.Evaluate(selected.Identity, capability.Request{
		Principal: req.Principal, Capability: req.Capability, Operation: req.Operation, Scope: req.Scope,
	}, now); err != nil {
		return nil, fmt.Errorf("plugin capability denied: %w", err)
	}
	return g.Transport.Call(ctx, selected.Identity, req.Operation, append([]byte(nil), req.Payload...))
}
