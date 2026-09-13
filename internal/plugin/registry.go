package plugin

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var ErrNoEligibleProvider = errors.New("no eligible plugin provider")

type Provider struct {
	Manifest  Manifest
	Identity  InstanceIdentity
	State     State
	Isolation IsolationProfile
	Priority  int
}

func (p Provider) Validate() error {
	if err := p.Identity.ValidateAgainst(p.Manifest); err != nil {
		return fmt.Errorf("provider identity: %w", err)
	}
	if err := p.Isolation.Satisfies(p.Manifest.RequiredIsolation); err != nil {
		return fmt.Errorf("provider isolation: %w", err)
	}
	if p.State != StateReady && p.State != StateDegraded {
		return fmt.Errorf("provider state %q is not dispatchable", p.State)
	}
	return nil
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: map[string]Provider{}} }

func (r *Registry) Register(provider Provider) error {
	if r == nil {
		return errors.New("plugin registry is required")
	}
	if err := provider.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.providers[provider.Identity.InstanceID]; ok {
		if existing.Identity != provider.Identity {
			return errors.New("plugin instance id collision")
		}
	}
	r.providers[provider.Identity.InstanceID] = provider
	return nil
}

func (r *Registry) Remove(instanceID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.providers, instanceID)
	r.mu.Unlock()
}

// Resolve returns deterministic eligible providers that advertise capability.
// Advertisement is discovery metadata only. Callers must still validate a
// principal-bound CapabilityLease before dispatch.
func (r *Registry) Resolve(capability string, requiredIsolation []IsolationProperty) ([]Provider, error) {
	if r == nil || capability == "" {
		return nil, errors.New("plugin registry and capability are required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	eligible := make([]Provider, 0)
	for _, provider := range r.providers {
		if provider.State != StateReady {
			// Degraded instances remain valid lifecycle entries but are not selected
			// for new work unless a future policy explicitly allows it.
			continue
		}
		if !containsString(provider.Manifest.Capabilities, capability) {
			continue
		}
		required := append([]IsolationProperty(nil), provider.Manifest.RequiredIsolation...)
		required = append(required, requiredIsolation...)
		if err := provider.Isolation.Satisfies(uniqueIsolation(required)); err != nil {
			continue
		}
		eligible = append(eligible, provider)
	}
	if len(eligible) == 0 {
		return nil, ErrNoEligibleProvider
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Priority != eligible[j].Priority {
			return eligible[i].Priority > eligible[j].Priority
		}
		if eligible[i].Manifest.ID != eligible[j].Manifest.ID {
			return eligible[i].Manifest.ID < eligible[j].Manifest.ID
		}
		return eligible[i].Identity.InstanceID < eligible[j].Identity.InstanceID
	})
	return eligible, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uniqueIsolation(values []IsolationProperty) []IsolationProperty {
	seen := map[IsolationProperty]struct{}{}
	out := make([]IsolationProperty, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
