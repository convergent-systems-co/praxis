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

// LeaseConsumer performs authoritative persisted revalidation and atomic use
// consumption immediately before the transport boundary.
type LeaseConsumer interface {
	ConsumePluginLease(ctx context.Context, binding LeaseBinding, instance InstanceIdentity, req capability.Request, now time.Time) error
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
	Registry      *Registry
	Transport     Transport
	LeaseConsumer LeaseConsumer
}

// Dispatch is the deterministic boundary before a plugin transport call. It
// requires provider eligibility plus authoritative atomic consumption of a
// capability lease bound to the exact instance/runtime session selected.
func (g Gateway) Dispatch(ctx context.Context, req DispatchRequest) ([]byte, error) {
	if g.Registry == nil || g.Transport == nil || g.LeaseConsumer == nil {
		return nil, errors.New("plugin registry, transport, and authoritative lease consumer are required")
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
	capReq := capability.Request{Principal:req.Principal, Capability:req.Capability, Operation:req.Operation, Scope:req.Scope, Now:now}
	if err := g.LeaseConsumer.ConsumePluginLease(ctx, req.Binding, selected.Identity, capReq, now); err != nil {
		return nil, fmt.Errorf("plugin capability denied at authoritative dispatch boundary: %w", err)
	}
	return g.Transport.Call(ctx, selected.Identity, req.Operation, append([]byte(nil), req.Payload...))
}
